package server

import (
	"sort"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

func sanitizeTopologyLinksForAreaManager(links []model.TopologyLinkView, allowed map[string]struct{}, tagMap map[string][]string, agentNames map[string]string) []model.TopologyLinkView {
	filtered := make([]model.TopologyLinkView, 0, len(links))
	for _, link := range links {
		if _, ok := allowed[link.Source.AgentID]; !ok {
			continue
		}
		if _, ok := allowed[link.Target.AgentID]; !ok {
			continue
		}
		link.Source.AgentName = areaManagerDisplayName("", agentNames[link.Source.AgentID], link.Source.AgentID)
		link.Source.AgentTags = cloneStringSlice(tagMap[link.Source.AgentID])
		link.Source.Address = redactEndpointIP(link.Source.Address)
		link.Source.Target = redactEndpointIP(link.Source.Target)
		link.Source.TargetIP = ""
		link.Source.TargetGeo = sanitizeGeoForAreaManager(link.Source.TargetGeo)
		link.Source.ResolvedIPs = nil
		link.Target = sanitizeTopologyInboundForAreaManager(link.Target, tagMap, agentNames)
		if link.FinalTarget != nil {
			finalTarget := sanitizeTopologyInboundForAreaManager(*link.FinalTarget, tagMap, agentNames)
			link.FinalTarget = &finalTarget
		}
		for index := range link.RealmHops {
			link.RealmHops[index] = sanitizeTopologyInboundForAreaManager(link.RealmHops[index], tagMap, agentNames)
		}
		filtered = append(filtered, link)
	}
	return filtered
}

func sanitizeTopologyInboundForAreaManager(ref model.TopologyInboundRef, tagMap map[string][]string, agentNames map[string]string) model.TopologyInboundRef {
	ref.AgentName = areaManagerDisplayName("", agentNames[ref.AgentID], ref.AgentID)
	ref.AgentTags = cloneStringSlice(tagMap[ref.AgentID])
	ref.IPs = nil
	ref.ResolvedIPs = nil
	ref.EntryIPs = nil
	ref.EntryAddresses = filterDomainLikeValues(ref.EntryAddresses)
	ref.EntryMappings = sanitizeEntryMappingsForAreaManager(ref.EntryMappings)
	return ref
}

func sanitizeClientChainsForAreaManager(chains []model.ClientChainView, allowed map[string]struct{}, tagMap map[string][]string, agentNames map[string]string, clientScope areaManagerClientScope) []model.ClientChainView {
	filtered := make([]model.ClientChainView, 0, len(chains))
	for _, chain := range chains {
		if _, ok := allowed[chain.RootAgentID]; !ok {
			continue
		}
		if !clientScope.allowsClient(chain.RootAgentID, clientChainInboundID(chain), chain.RootInboundTag, chain.RootClientEmail) {
			continue
		}
		visible := true
		for _, step := range chain.Steps {
			if step.AgentID == "" {
				continue
			}
			if _, ok := allowed[step.AgentID]; !ok {
				visible = false
				break
			}
		}
		if !visible {
			continue
		}
		chain.RootAgentName = areaManagerDisplayName("", agentNames[chain.RootAgentID], chain.RootAgentID)
		chain.RootAgentTags = cloneStringSlice(tagMap[chain.RootAgentID])
		for index := range chain.Steps {
			step := &chain.Steps[index]
			step.AgentName = areaManagerDisplayName("", agentNames[step.AgentID], step.AgentID)
			step.AgentTags = cloneStringSlice(tagMap[step.AgentID])
			step.Target = redactEndpointIP(step.Target)
			step.TargetIP = ""
			step.TargetGeo = sanitizeGeoForAreaManager(step.TargetGeo)
		}
		filtered = append(filtered, chain)
	}
	return filtered
}

func sanitizeGeoForAreaManager(geo *model.IPGeoView) *model.IPGeoView {
	if geo == nil {
		return nil
	}
	return &model.IPGeoView{
		CountryCode: geo.CountryCode,
		CountryName: geo.CountryName,
	}
}

func filterTopologyLinksVisibleToAreaManager(links []model.TopologyLinkView, chains []model.ClientChainView, clientScope areaManagerClientScope) []model.TopologyLinkView {
	used := make(map[string]struct{})
	visibleForwardingPaths := areaManagerForwardingPathLinkKeys(links, clientScope)
	for _, chain := range chains {
		for _, step := range chain.Steps {
			if step.StepType != "outbound" || step.AgentID == "" || step.OutboundTag == "" {
				continue
			}
			used[outboundLinkKey(step.AgentID, step.OutboundTag)] = struct{}{}
		}
	}
	filtered := make([]model.TopologyLinkView, 0, len(links))
	for _, link := range links {
		if _, ok := used[outboundLinkKey(link.Source.AgentID, link.Source.OutboundTag)]; ok {
			filtered = append(filtered, link)
			continue
		}
		if _, ok := visibleForwardingPaths[outboundLinkKey(link.Source.AgentID, link.Source.OutboundTag)]; ok {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func areaManagerCanViewForwardingLink(link model.TopologyLinkView, clientScope areaManagerClientScope) bool {
	protocol := strings.ToLower(strings.TrimSpace(link.Source.Protocol))
	if protocol != "realm" && protocol != "haproxy" {
		return false
	}
	listenPort := link.Source.ListenPort
	if listenPort <= 0 {
		listenPort = forwardingListenPortFromOutboundTag(link.Source.OutboundTag)
	}
	finalTarget, ok := topologyLinkFinalTarget(link)
	if listenPort <= 0 || !ok {
		return false
	}
	return clientScope.allowsForwardingPort(link.Source.AgentID, listenPort, protocol) &&
		(clientScope.allowsInbound(finalTarget.AgentID, finalTarget.InboundID, finalTarget.InboundTag) ||
			clientScope.hasExactClientOnInbound(finalTarget.AgentID, finalTarget.InboundID, finalTarget.InboundTag))
}

func topologyLinkFinalTarget(link model.TopologyLinkView) (model.TopologyInboundRef, bool) {
	if link.FinalTarget != nil && link.FinalTarget.AgentID != "" {
		return *link.FinalTarget, true
	}
	if link.Target.AgentID != "" && !isForwardingProtocol(link.Target.Protocol) {
		return link.Target, true
	}
	return model.TopologyInboundRef{}, false
}

func areaManagerForwardingPathLinkKeys(links []model.TopologyLinkView, clientScope areaManagerClientScope) map[string]struct{} {
	result := make(map[string]struct{})
	byOutbound := make(map[string]model.TopologyLinkView, len(links))
	for _, link := range links {
		byOutbound[outboundLinkKey(link.Source.AgentID, link.Source.OutboundTag)] = link
	}
	for _, link := range links {
		if !areaManagerCanViewForwardingLink(link, clientScope) {
			continue
		}
		visited := make(map[string]struct{})
		current := link
		for {
			key := outboundLinkKey(current.Source.AgentID, current.Source.OutboundTag)
			if _, seen := visited[key]; seen {
				break
			}
			visited[key] = struct{}{}
			result[key] = struct{}{}
			if !isForwardingProtocol(current.Target.Protocol) {
				break
			}
			nextKey := outboundLinkKey(current.Target.AgentID, current.Target.InboundTag)
			next, ok := byOutbound[nextKey]
			if !ok {
				break
			}
			current = next
		}
	}
	return result
}

func expandAreaManagerForwardingPathAgents(allowed map[string]struct{}, links []model.TopologyLinkView, clientScope areaManagerClientScope) {
	visible := areaManagerForwardingPathLinkKeys(links, clientScope)
	for _, link := range links {
		if _, ok := visible[outboundLinkKey(link.Source.AgentID, link.Source.OutboundTag)]; !ok {
			continue
		}
		allowed[link.Source.AgentID] = struct{}{}
		allowed[link.Target.AgentID] = struct{}{}
		if link.FinalTarget != nil {
			allowed[link.FinalTarget.AgentID] = struct{}{}
		}
		for _, hop := range link.RealmHops {
			allowed[hop.AgentID] = struct{}{}
		}
	}
}

func cloneAgentSet(values map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for value := range values {
		result[value] = struct{}{}
	}
	return result
}

func forwardingListenPortFromOutboundTag(tag string) int {
	for _, part := range strings.FieldsFunc(tag, func(r rune) bool { return r < '0' || r > '9' }) {
		if value, err := strconv.Atoi(part); err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func isForwardingProtocol(protocol string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	return protocol == "realm" || protocol == "haproxy"
}

func applyAreaManagerClientCounts(view *model.GlobalDashboardView, clientScope areaManagerClientScope) {
	if view == nil {
		return
	}
	counts := make(map[string]map[string]struct{})
	nodeCounts := make(map[string]map[string]struct{})
	for _, chain := range view.ClientChains {
		inboundID := clientChainInboundID(chain)
		if !clientScope.allowsClient(chain.RootAgentID, inboundID, chain.RootInboundTag, chain.RootClientEmail) {
			continue
		}
		agentCounts := counts[chain.RootAgentID]
		if agentCounts == nil {
			agentCounts = make(map[string]struct{})
			counts[chain.RootAgentID] = agentCounts
		}
		agentCounts[areaClientExactKey(chain.RootAgentID, inboundID, chain.RootInboundTag, chain.RootClientEmail)] = struct{}{}
		agentNodes := nodeCounts[chain.RootAgentID]
		if agentNodes == nil {
			agentNodes = make(map[string]struct{})
			nodeCounts[chain.RootAgentID] = agentNodes
		}
		agentNodes[areaClientInboundKey(chain.RootAgentID, inboundID, chain.RootInboundTag)] = struct{}{}
	}
	for key := range clientScope.nodeOnlyInbounds {
		parts := strings.Split(key, "\x00")
		if len(parts) != 3 {
			continue
		}
		if nodeCounts[parts[0]] == nil {
			nodeCounts[parts[0]] = make(map[string]struct{})
		}
		exists := false
		for existing := range nodeCounts[parts[0]] {
			other := strings.Split(existing, "\x00")
			if len(other) == 3 && other[1] == parts[1] {
				exists = true
				break
			}
		}
		if !exists {
			nodeCounts[parts[0]][key] = struct{}{}
		}
	}
	for index := range view.Agents {
		view.Agents[index].ClientCount = len(counts[view.Agents[index].AgentID])
		view.Agents[index].NodeCount = len(nodeCounts[view.Agents[index].AgentID])
		if view.Agents[index].OnlineClientCount > view.Agents[index].ClientCount {
			view.Agents[index].OnlineClientCount = view.Agents[index].ClientCount
		}
	}
}

func clientChainInboundID(chain model.ClientChainView) int {
	if chain.RootInboundID > 0 {
		return chain.RootInboundID
	}
	parts := strings.Split(chain.Key, "::")
	if len(parts) < 3 {
		return 0
	}
	value, _ := strconv.Atoi(parts[1])
	return value
}

func rebuildAreaManagerDashboardStats(view *model.GlobalDashboardView) {
	totals := model.DashboardTotals{
		AgentCount: len(view.Agents),
		LinkCount:  len(view.Links),
		ChainCount: len(view.ClientChains),
	}
	tagStats := make(map[string]*model.DashboardTagView)
	for _, agent := range view.Agents {
		totals.NodeCount += agent.NodeCount
		totals.ClientCount += agent.ClientCount
		totals.OnlineClientCount += agent.OnlineClientCount
		totals.OutboundCount += agent.OutboundCount
		totals.RoutingRuleCount += agent.RoutingRuleCount
		if len(agent.Tags) > 0 {
			totals.TaggedAgentCount++
		}
		for _, tag := range agent.Tags {
			entry := tagStats[tag]
			if entry == nil {
				entry = &model.DashboardTagView{Tag: tag}
				tagStats[tag] = entry
			}
			entry.AgentCount++
			entry.NodeCount += agent.NodeCount
			entry.ClientCount += agent.ClientCount
			entry.OnlineClientCount += agent.OnlineClientCount
		}
	}
	tags := make([]model.DashboardTagView, 0, len(tagStats))
	for _, entry := range tagStats {
		tags = append(tags, *entry)
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].ClientCount != tags[j].ClientCount {
			return tags[i].ClientCount > tags[j].ClientCount
		}
		return tags[i].Tag < tags[j].Tag
	})
	view.Totals = totals
	view.Tags = tags
}

func sanitizeEntryMappingsForAreaManager(mappings []model.TopologyEntryMapping) []model.TopologyEntryMapping {
	result := make([]model.TopologyEntryMapping, 0, len(mappings))
	for _, mapping := range mappings {
		mapping.Address = redactEndpointIP(mapping.Address)
		mapping.ResolvedIPs = nil
		if mapping.Address != "" {
			result = append(result, mapping)
		}
	}
	return result
}
