package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"bridge-core/internal/model"
)

type topologyInboundCandidate struct {
	ref   model.TopologyInboundRef
	route model.XUIRouteTrace
}

type topologyOutboundCandidate struct {
	ref model.TopologyOutboundRef
}

func terminalAgentExitGeo(agentView model.DashboardAgentView, overview *model.XUIOverview, resolver *topologyResolver) (string, *model.IPGeoView) {
	if resolver == nil {
		return "", nil
	}
	summary := agentView.Summary
	if overview != nil {
		if summary.PublicIPv4 == "" {
			summary.PublicIPv4 = overview.Summary.PublicIPv4
		}
		if summary.PublicIPv6 == "" {
			summary.PublicIPv6 = overview.Summary.PublicIPv6
		}
		if summary.Hostname == "" {
			summary.Hostname = overview.Summary.Hostname
		}
	}
	for _, address := range []string{summary.ObservedIP, summary.PublicIPv4, summary.PublicIPv6} {
		if address == "" {
			continue
		}
		if ip, geo := resolver.lookupGeo(address, nil); geo != nil {
			return ip, geo
		}
	}
	if summary.Hostname != "" {
		resolved := resolver.lookupHost(summary.Hostname)
		if ip, geo := resolver.lookupGeo(summary.Hostname, resolved); geo != nil {
			return ip, geo
		}
	}
	if agentView.Geo != nil {
		return agentView.Geo.IP, cloneGeo(*agentView.Geo)
	}
	return "", nil
}

func buildInboundCandidates(agentViews map[string]model.DashboardAgentView, overviewByAgent map[string]*model.XUIOverview, resolver *topologyResolver) map[string]topologyInboundCandidate {
	result := make(map[string]topologyInboundCandidate)
	for agentID, overview := range overviewByAgent {
		agentView, ok := agentViews[agentID]
		if !ok {
			continue
		}
		certDomains := collectCertificateDomains(overview.Certificates)
		agentDomains := collectAgentDomains(overview)
		agentIPs := collectAgentIPs(overview)
		entryDomains := collectEntryDomains(agentView.Entry)
		entryIPs := collectEntryIPs(agentView.Entry)
		entryResolvedIPs := resolver.lookupAll(entryDomains)
		publicIPs := collectInboundIPs("", overview.Summary.PublicIPv4, overview.Summary.PublicIPv6, overview.Summary.ObservedIP)
		for _, node := range overview.Nodes {
			entryMappings := entryMappingsForInbound(agentView.Entry, node, resolver)
			mappingDomains := collectTopologyEntryMappingDomains(entryMappings)
			mappingIPs := collectTopologyEntryMappingIPs(entryMappings)
			mappingResolvedIPs := collectTopologyEntryMappingResolvedIPs(entryMappings)
			domains := mergeStringSets(collectInboundDomains(node, certDomains), agentDomains, entryDomains, mappingDomains)
			resolvedIPs := resolver.lookupAll(domains)
			ips := mergeStringSets(
				collectInboundIPs(node.Listen, overview.Summary.PublicIPv4, overview.Summary.PublicIPv6, overview.Summary.ObservedIP),
				agentIPs,
				entryIPs,
				entryResolvedIPs,
				mappingIPs,
				mappingResolvedIPs,
			)
			ref := model.TopologyInboundRef{
				AgentID:        agentID,
				AgentName:      agentView.AgentName,
				AgentTags:      cloneStrings(agentView.Tags),
				InboundID:      node.ID,
				InboundTag:     node.Tag,
				InboundName:    firstNonEmpty(node.Remark, node.Tag, fmt.Sprintf("Node #%d", node.ID)),
				Protocol:       node.Protocol,
				Port:           node.Port,
				Network:        node.Network,
				Security:       node.Security,
				WSPath:         node.WSPath,
				WSHost:         node.WSHost,
				Domains:        mergeStringSets(domains),
				IPs:            mergeStringSets(publicIPs, ips, resolvedIPs),
				ResolvedIPs:    resolvedIPs,
				EntryAddresses: mergeStringSets(agentView.Entry.Addresses, mappingAddresses(entryMappings)),
				EntryIPs:       mergeStringSets(entryIPs, entryResolvedIPs, mappingIPs, mappingResolvedIPs),
				EntryMappings:  entryMappings,
				AuthKeys:       cloneStrings(node.AuthKeys),
			}
			result[inboundTopologyKey(agentID, node.ID, node.Tag)] = topologyInboundCandidate{
				ref:   ref,
				route: node.Route,
			}
		}
	}
	for agentID, agentView := range agentViews {
		domains := mergeStringSets(
			collectEntryDomains(agentView.Entry),
			uniqueNormalizedDomains([]string{agentView.Summary.Hostname}),
		)
		resolvedIPs := resolver.lookupAll(domains)
		ips := mergeStringSets(
			collectEntryIPs(agentView.Entry),
			uniqueNormalizedIPs([]string{
				agentView.Summary.ObservedIP,
				agentView.Summary.PublicIPv4,
				agentView.Summary.PublicIPv6,
			}),
			resolvedIPs,
		)
		for _, rule := range activeRealmForwardRulesForTopology(agentView.Entry.PortForwarding.Rules) {
			tag := realmTopologyTag(rule)
			ref := model.TopologyInboundRef{
				AgentID:        agentID,
				AgentName:      agentView.AgentName,
				AgentTags:      cloneStrings(agentView.Tags),
				InboundID:      rule.ListenPort,
				InboundTag:     tag,
				InboundName:    firstNonEmpty(rule.Name, fmt.Sprintf("Realm :%d", rule.ListenPort)),
				Protocol:       "realm",
				Port:           rule.ListenPort,
				Network:        normalizeRealmForwardNetworkForTopology(rule.Network),
				Domains:        domains,
				IPs:            ips,
				ResolvedIPs:    resolvedIPs,
				EntryAddresses: mergeStringSets(agentView.Entry.Addresses, []string{agentView.Entry.ImportDomain}),
				EntryIPs:       collectEntryIPs(agentView.Entry),
			}
			result[inboundTopologyKey(agentID, ref.InboundID, ref.InboundTag)] = topologyInboundCandidate{
				ref: ref,
				route: model.XUIRouteTrace{
					MatchScope:  "realm",
					OutboundTag: tag,
					Note:        fmt.Sprintf("Realm :%d -> %s:%d", rule.ListenPort, rule.TargetAddress, rule.TargetPort),
				},
			}
		}
		for _, rule := range activeHAProxyRulesForTopology(agentView.Entry.HAProxy.Rules) {
			tag := haProxyTopologyTag(rule)
			ref := model.TopologyInboundRef{
				AgentID:        agentID,
				AgentName:      agentView.AgentName,
				AgentTags:      cloneStrings(agentView.Tags),
				InboundID:      rule.ListenPort,
				InboundTag:     tag,
				InboundName:    firstNonEmpty(rule.Name, fmt.Sprintf("HAProxy :%d", rule.ListenPort)),
				Protocol:       "haproxy",
				Port:           rule.ListenPort,
				Network:        "tcp",
				Domains:        domains,
				IPs:            ips,
				ResolvedIPs:    resolvedIPs,
				EntryAddresses: mergeStringSets(agentView.Entry.Addresses, []string{agentView.Entry.ImportDomain}),
				EntryIPs:       collectEntryIPs(agentView.Entry),
			}
			result[inboundTopologyKey(agentID, ref.InboundID, ref.InboundTag)] = topologyInboundCandidate{
				ref: ref,
				route: model.XUIRouteTrace{
					MatchScope:  "haproxy",
					OutboundTag: tag,
					Note:        fmt.Sprintf("HAProxy :%d -> 主用 %s:%d", rule.ListenPort, rule.Primary.Address, rule.Primary.Port),
				},
			}
		}
	}
	return result
}

func buildOutboundCandidates(agentViews map[string]model.DashboardAgentView, overviewByAgent map[string]*model.XUIOverview, resolver *topologyResolver) map[string]topologyOutboundCandidate {
	result := make(map[string]topologyOutboundCandidate)
	for agentID, overview := range overviewByAgent {
		agentView, ok := agentViews[agentID]
		if !ok {
			continue
		}
		for _, outbound := range overview.Outbounds {
			resolvedIPs := resolver.lookupAll(collectOutboundDomains(outbound))
			targetIP, targetGeo := resolver.lookupGeo(outbound.Address, resolvedIPs)
			if isTerminalOutbound(outbound.Protocol) && targetIP == "" {
				targetIP, targetGeo = terminalAgentExitGeo(agentView, overview, resolver)
			}
			target := outbound.Target
			if isTerminalOutbound(outbound.Protocol) && target == "" {
				target = "当前 Client 出口"
			}
			ref := model.TopologyOutboundRef{
				AgentID:       agentID,
				AgentName:     agentView.AgentName,
				AgentTags:     cloneStrings(agentView.Tags),
				OutboundTag:   outbound.Tag,
				Protocol:      outbound.Protocol,
				Target:        target,
				Address:       outbound.Address,
				Port:          outbound.Port,
				Network:       outbound.Network,
				Security:      outbound.Security,
				TLSServerName: outbound.TLSServerName,
				WSPath:        outbound.WSPath,
				WSHost:        outbound.WSHost,
				ResolvedIPs:   resolvedIPs,
				TargetIP:      targetIP,
				TargetGeo:     targetGeo,
				AuthKeys:      cloneStrings(outbound.AuthKeys),
			}
			result[outboundTopologyKey(agentID, outbound.Tag)] = topologyOutboundCandidate{ref: ref}
		}
	}
	for agentID, agentView := range agentViews {
		for _, rule := range activeRealmForwardRulesForTopology(agentView.Entry.PortForwarding.Rules) {
			targetAddress := strings.TrimSpace(rule.TargetAddress)
			if targetAddress == "" {
				targetAddress = realmTargetAgentAddress(rule.TargetAgentID, agentViews)
			}
			resolvedIPs := resolver.lookupHost(targetAddress)
			tag := realmTopologyTag(rule)
			ref := model.TopologyOutboundRef{
				AgentID:     agentID,
				AgentName:   agentView.AgentName,
				AgentTags:   cloneStrings(agentView.Tags),
				OutboundTag: tag,
				Protocol:    "realm",
				Target:      fmt.Sprintf("%s:%d", targetAddress, rule.TargetPort),
				Address:     targetAddress,
				Port:        rule.TargetPort,
				ListenPort:  rule.ListenPort,
				Network:     normalizeRealmForwardNetworkForTopology(rule.Network),
				ResolvedIPs: resolvedIPs,
			}
			result[outboundTopologyKey(agentID, tag)] = topologyOutboundCandidate{ref: ref}
		}
		for _, rule := range activeHAProxyRulesForTopology(agentView.Entry.HAProxy.Rules) {
			targetAddress := strings.TrimSpace(rule.Primary.Address)
			if targetAddress == "" {
				targetAddress = realmTargetAgentAddress(rule.Primary.AgentID, agentViews)
			}
			tag := haProxyTopologyTag(rule)
			ref := model.TopologyOutboundRef{
				AgentID:     agentID,
				AgentName:   agentView.AgentName,
				AgentTags:   cloneStrings(agentView.Tags),
				OutboundTag: tag,
				Protocol:    "haproxy",
				Target:      fmt.Sprintf("%s:%d", targetAddress, rule.Primary.Port),
				Address:     targetAddress,
				Port:        rule.Primary.Port,
				ListenPort:  rule.ListenPort,
				Network:     "tcp",
				ResolvedIPs: resolver.lookupHost(targetAddress),
			}
			result[outboundTopologyKey(agentID, tag)] = topologyOutboundCandidate{ref: ref}
		}
	}
	return result
}

func realmTopologyTag(rule model.RealmForwardRule) string {
	if name := strings.TrimSpace(rule.Name); name != "" {
		return fmt.Sprintf("%s (%d)", name, rule.ListenPort)
	}
	if id := strings.TrimSpace(rule.ID); id != "" {
		return "realm:" + id
	}
	return fmt.Sprintf("realm:%d", rule.ListenPort)
}

func haProxyTopologyTag(rule model.HAProxyRule) string {
	if name := strings.TrimSpace(rule.Name); name != "" {
		return fmt.Sprintf("%s (%d)", name, rule.ListenPort)
	}
	if id := strings.TrimSpace(rule.ID); id != "" {
		return "haproxy:" + id
	}
	return fmt.Sprintf("haproxy:%d", rule.ListenPort)
}

func realmTargetAgentAddress(agentID string, agentViews map[string]model.DashboardAgentView) string {
	agent, ok := agentViews[strings.TrimSpace(agentID)]
	if !ok {
		return ""
	}
	if value := strings.TrimSpace(agent.Entry.ImportDomain); value != "" {
		return value
	}
	for _, value := range agent.Entry.Addresses {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return firstNonEmpty(agent.Summary.ObservedIP, agent.Summary.PublicIPv4, agent.Summary.PublicIPv6, agent.Summary.Hostname)
}

func activeRealmForwardRulesForTopology(items []model.RealmForwardRule) []model.RealmForwardRule {
	rules := make([]model.RealmForwardRule, 0, len(items))
	for _, rule := range items {
		rule.TargetAddress = strings.TrimSpace(rule.TargetAddress)
		rule.TargetAgentID = strings.TrimSpace(rule.TargetAgentID)
		if !rule.Enabled || (rule.TargetAddress == "" && rule.TargetAgentID == "") || rule.ListenPort <= 0 || rule.TargetPort <= 0 {
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

func activeHAProxyRulesForTopology(items []model.HAProxyRule) []model.HAProxyRule {
	rules := make([]model.HAProxyRule, 0, len(items))
	for _, rule := range items {
		rule.Primary.AgentID = strings.TrimSpace(rule.Primary.AgentID)
		rule.Primary.Address = strings.TrimSpace(rule.Primary.Address)
		if !rule.Enabled || rule.ListenPort <= 0 || rule.Primary.Port <= 0 || (rule.Primary.AgentID == "" && rule.Primary.Address == "") {
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

func normalizeRealmForwardNetworkForTopology(network string) string {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "udp":
		return "udp"
	case "both", "tcp+udp", "all":
		return "tcp+udp"
	default:
		return "tcp"
	}
}

func buildBalancerCandidates(overviewByAgent map[string]*model.XUIOverview) map[string]map[string]model.XUIBalancerView {
	result := make(map[string]map[string]model.XUIBalancerView, len(overviewByAgent))
	for agentID, overview := range overviewByAgent {
		if len(overview.Balancers) == 0 {
			continue
		}
		byTag := make(map[string]model.XUIBalancerView, len(overview.Balancers))
		for _, balancer := range overview.Balancers {
			if balancer.Tag == "" {
				continue
			}
			byTag[balancer.Tag] = balancer
		}
		if len(byTag) > 0 {
			result[agentID] = byTag
		}
	}
	return result
}

func buildClientChains(
	agentViews map[string]model.DashboardAgentView,
	overviewByAgent map[string]*model.XUIOverview,
	inbounds map[string]topologyInboundCandidate,
	outbounds map[string]topologyOutboundCandidate,
	balancersByAgent map[string]map[string]model.XUIBalancerView,
	linkByOutbound map[string]model.TopologyLinkView,
) []model.ClientChainView {
	chains := make([]model.ClientChainView, 0)
	for agentID, overview := range overviewByAgent {
		agentView, ok := agentViews[agentID]
		if !ok {
			continue
		}
		for _, client := range overview.Clients {
			chain := model.ClientChainView{
				Key:                clientChainKey(agentID, client.InboundID, client.Email),
				RootAgentID:        agentID,
				RootAgentName:      agentView.AgentName,
				RootAgentTags:      cloneStrings(agentView.Tags),
				RootClientEmail:    client.Email,
				RootClientRemark:   firstNonEmpty(client.Comment, client.SubID),
				RootClientEnabled:  client.Enabled,
				RootInboundID:      client.InboundID,
				RootInboundTag:     client.InboundTag,
				RootInboundEnabled: clientInboundEnabled(overview.Nodes, client),
			}
			chain.Steps = append(chain.Steps, model.ClientChainStep{
				StepType:  "client",
				AgentID:   agentID,
				AgentName: agentView.AgentName,
				AgentTags: cloneStrings(agentView.Tags),
				Label:     firstNonEmpty(client.Email, "anonymous-client"),
				Detail:    firstNonEmpty(client.Comment, client.SubID),
				Protocol:  client.Protocol,
			})

			currentAgentID := agentID
			currentInboundKey := inboundTopologyKey(agentID, client.InboundID, client.InboundTag)
			currentRoute := client.Route
			visitedInbounds := map[string]struct{}{currentInboundKey: {}}
			visitedOutbounds := make(map[string]struct{})
			renderedInbounds := make(map[string]struct{})

			for depth := 0; depth < 8; depth++ {
				inbound, ok := inbounds[currentInboundKey]
				if !ok {
					chain.UnresolvedReason = "inbound metadata not found in latest snapshot"
					break
				}
				if _, rendered := renderedInbounds[currentInboundKey]; !rendered {
					chain.Steps = append(chain.Steps, model.ClientChainStep{
						StepType:  "inbound",
						AgentID:   inbound.ref.AgentID,
						AgentName: inbound.ref.AgentName,
						AgentTags: cloneStrings(inbound.ref.AgentTags),
						Label:     inbound.ref.InboundName,
						Detail:    formatInboundDetail(inbound.ref),
						Protocol:  inbound.ref.Protocol,
						Port:      inbound.ref.Port,
					})
					renderedInbounds[currentInboundKey] = struct{}{}
				}

				if currentRoute.BalancerTag != "" && currentRoute.OutboundTag == "" {
					balancers := balancersByAgent[currentAgentID]
					balancer, ok := balancers[currentRoute.BalancerTag]
					if !ok {
						chain.UnresolvedReason = fmt.Sprintf("balancer %q was referenced by route but is missing from the current config", currentRoute.BalancerTag)
						break
					}
					currentAgentView := agentViews[currentAgentID]
					selectedTag, balancerNote := selectBalancedOutbound(currentAgentID, balancer, outbounds, linkByOutbound)
					chain.Steps = append(chain.Steps, model.ClientChainStep{
						StepType:   "balancer",
						AgentID:    currentAgentID,
						AgentName:  currentAgentView.AgentName,
						AgentTags:  cloneStrings(currentAgentView.Tags),
						Label:      balancer.Tag,
						Detail:     formatBalancerDetail(balancer, selectedTag),
						RouteScope: currentRoute.MatchScope,
						RuleIndex:  currentRoute.RuleIndex,
					})
					if selectedTag == "" {
						chain.UnresolvedReason = firstNonEmpty(balancerNote, "balancer did not resolve to a usable outbound")
						break
					}
					currentRoute.OutboundTag = selectedTag
					currentRoute.MatchScope = "balancer"
					currentRoute.Note = balancerNote
				}
				if currentRoute.OutboundTag == "" {
					chain.UnresolvedReason = firstNonEmpty(currentRoute.Note, "no outbound was inferred for the current inbound/client")
					break
				}

				outboundKey := outboundTopologyKey(currentAgentID, currentRoute.OutboundTag)
				if _, seen := visitedOutbounds[outboundKey]; seen {
					chain.LoopDetected = true
					chain.UnresolvedReason = "detected a routing loop while following outbound targets"
					break
				}
				visitedOutbounds[outboundKey] = struct{}{}

				outbound, ok := outbounds[outboundKey]
				if !ok {
					chain.UnresolvedReason = fmt.Sprintf("outbound %q was referenced by route but is missing from the current config", currentRoute.OutboundTag)
					break
				}
				chain.Steps = append(chain.Steps, model.ClientChainStep{
					StepType:    "outbound",
					AgentID:     outbound.ref.AgentID,
					AgentName:   outbound.ref.AgentName,
					AgentTags:   cloneStrings(outbound.ref.AgentTags),
					Label:       outbound.ref.OutboundTag,
					Detail:      formatOutboundDetail(outbound.ref),
					Protocol:    outbound.ref.Protocol,
					Port:        outbound.ref.Port,
					RouteScope:  currentRoute.MatchScope,
					RuleIndex:   currentRoute.RuleIndex,
					OutboundTag: outbound.ref.OutboundTag,
					Target:      outbound.ref.Target,
					TargetIP:    outbound.ref.TargetIP,
					TargetGeo:   outbound.ref.TargetGeo,
				})
				if isTerminalOutbound(outbound.ref.Protocol) {
					break
				}

				link, ok := linkByOutbound[outboundKey]
				if !ok {
					chain.UnresolvedReason = "the outbound target did not match any registered client inbound"
					break
				}
				chain.MatchedLinkCount++

				nextInboundKey := inboundTopologyKey(link.Target.AgentID, link.Target.InboundID, link.Target.InboundTag)
				if _, seen := visitedInbounds[nextInboundKey]; seen {
					chain.LoopDetected = true
					chain.UnresolvedReason = "detected an inbound loop while following matched topology links"
					break
				}
				visitedInbounds[nextInboundKey] = struct{}{}

				nextInbound, ok := inbounds[nextInboundKey]
				if !ok {
					chain.UnresolvedReason = "matched topology link points to an inbound that is no longer present"
					break
				}

				chain.Steps = append(chain.Steps, model.ClientChainStep{
					StepType:    "match",
					AgentID:     nextInbound.ref.AgentID,
					AgentName:   nextInbound.ref.AgentName,
					AgentTags:   cloneStrings(nextInbound.ref.AgentTags),
					Label:       nextInbound.ref.InboundName,
					Detail:      formatInboundDetail(nextInbound.ref),
					Protocol:    nextInbound.ref.Protocol,
					Port:        nextInbound.ref.Port,
					MatchReason: link.MatchReason,
				})
				renderedInbounds[nextInboundKey] = struct{}{}

				currentAgentID = nextInbound.ref.AgentID
				currentInboundKey = nextInboundKey
				currentRoute = nextInbound.route
			}

			chains = append(chains, chain)
		}
	}

	sort.Slice(chains, func(i, j int) bool {
		if chains[i].RootAgentName != chains[j].RootAgentName {
			return chains[i].RootAgentName < chains[j].RootAgentName
		}
		if chains[i].RootInboundTag != chains[j].RootInboundTag {
			return chains[i].RootInboundTag < chains[j].RootInboundTag
		}
		return chains[i].RootClientEmail < chains[j].RootClientEmail
	})
	return chains
}

func collectCertificateDomains(certificates []model.XUILocalCertificate) []string {
	result := make([]string, 0)
	for _, certificate := range certificates {
		for _, name := range certificate.DNSNames {
			if strings.Contains(name, "*") {
				continue
			}
			result = append(result, name)
		}
	}
	return uniqueNormalizedDomains(result)
}

func collectAgentDomains(overview *model.XUIOverview) []string {
	if overview == nil {
		return nil
	}
	values := []string{overview.BaseURL, overview.Summary.Hostname}
	return uniqueNormalizedDomains(values)
}

func collectAgentIPs(overview *model.XUIOverview) []string {
	if overview == nil {
		return nil
	}
	values := []string{extractEndpointHost(overview.BaseURL), overview.Summary.Hostname, overview.Summary.ObservedIP}
	return uniqueNormalizedIPs(values)
}

func collectEntryDomains(entry model.AgentEntryConfig) []string {
	result := make([]string, 0, len(entry.Addresses)+len(entry.Mappings)+1)
	if normalizeIP(extractEndpointHost(entry.ImportDomain)) == "" {
		result = append(result, entry.ImportDomain)
	}
	for _, address := range entry.Addresses {
		if normalizeIP(extractEndpointHost(address)) == "" {
			result = append(result, address)
		}
	}
	for _, mapping := range entry.Mappings {
		if normalizeIP(extractEndpointHost(mapping.Address)) == "" {
			result = append(result, mapping.Address)
		}
	}
	return uniqueNormalizedDomains(result)
}

func collectEntryIPs(entry model.AgentEntryConfig) []string {
	result := make([]string, 0, len(entry.Addresses)+len(entry.Mappings)+1)
	if ip := normalizeIP(extractEndpointHost(entry.ImportDomain)); ip != "" {
		result = append(result, ip)
	}
	for _, address := range entry.Addresses {
		if ip := normalizeIP(extractEndpointHost(address)); ip != "" {
			result = append(result, ip)
		}
	}
	for _, mapping := range entry.Mappings {
		if ip := normalizeIP(extractEndpointHost(mapping.Address)); ip != "" {
			result = append(result, ip)
		}
	}
	return uniqueNormalizedIPs(result)
}

func entryMappingsForInbound(entry model.AgentEntryConfig, node model.XUINodeView, resolver *topologyResolver) []model.TopologyEntryMapping {
	if len(entry.Mappings) == 0 {
		return nil
	}
	result := make([]model.TopologyEntryMapping, 0, len(entry.Mappings))
	nodeProtocol := normalizedTopologyProtocol(node.Protocol)
	for _, mapping := range entry.Mappings {
		if mapping.Address == "" || mapping.ExternalPort == 0 {
			continue
		}
		protocol := normalizedTopologyProtocol(mapping.Protocol)
		if protocol == "" || protocol != nodeProtocol {
			continue
		}
		internalPort := mapping.InternalPort
		if internalPort == 0 {
			internalPort = mapping.ExternalPort
		}
		if node.Port != 0 && internalPort != node.Port {
			continue
		}
		result = append(result, model.TopologyEntryMapping{
			Address:      mapping.Address,
			ExternalPort: mapping.ExternalPort,
			InternalPort: internalPort,
			Protocol:     protocol,
			Note:         mapping.Note,
			ResolvedIPs:  resolver.lookupHost(mapping.Address),
		})
	}
	return result
}

func collectTopologyEntryMappingDomains(mappings []model.TopologyEntryMapping) []string {
	values := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if normalizeIP(extractEndpointHost(mapping.Address)) == "" {
			values = append(values, mapping.Address)
		}
	}
	return uniqueNormalizedDomains(values)
}

func collectTopologyEntryMappingIPs(mappings []model.TopologyEntryMapping) []string {
	values := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if ip := normalizeIP(extractEndpointHost(mapping.Address)); ip != "" {
			values = append(values, ip)
		}
	}
	return uniqueNormalizedIPs(values)
}

func collectTopologyEntryMappingResolvedIPs(mappings []model.TopologyEntryMapping) []string {
	values := make([]string, 0)
	for _, mapping := range mappings {
		values = append(values, mapping.ResolvedIPs...)
	}
	return uniqueNormalizedIPs(values)
}

func mappingAddresses(mappings []model.TopologyEntryMapping) []string {
	values := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		values = append(values, mapping.Address)
	}
	return values
}

func collectInboundDomains(node model.XUINodeView, certificateDomains []string) []string {
	result := make([]string, 0, len(certificateDomains)+3)
	result = append(result, node.TLSServerName, node.WSHost)
	if node.Listen != "" && normalizeIP(node.Listen) == "" {
		result = append(result, node.Listen)
	}
	result = append(result, certificateDomains...)
	return uniqueNormalizedDomains(result)
}

func collectInboundIPs(listen string, publicIPv4 string, publicIPv6 string, observedIP ...string) []string {
	result := make([]string, 0, 3+len(observedIP))
	if ip := normalizeIP(listen); ip != "" {
		result = append(result, ip)
	}
	if ip := normalizeIP(publicIPv4); ip != "" {
		result = append(result, ip)
	}
	if ip := normalizeIP(publicIPv6); ip != "" {
		result = append(result, ip)
	}
	for _, value := range observedIP {
		if ip := normalizeIP(value); ip != "" {
			result = append(result, ip)
		}
	}
	return uniqueNormalizedIPs(result)
}

func collectOutboundDomains(outbound model.XUIOutboundView) []string {
	result := make([]string, 0, 3)
	for _, value := range []string{outbound.Address, outbound.TLSServerName, outbound.WSHost} {
		if host := normalizeHost(value); host != "" {
			result = append(result, host)
		}
	}
	return uniqueNormalizedDomains(result)
}

func selectBalancedOutbound(
	agentID string,
	balancer model.XUIBalancerView,
	outbounds map[string]topologyOutboundCandidate,
	linkByOutbound map[string]model.TopologyLinkView,
) (string, string) {
	bestTag := ""
	bestScore := -1
	for _, tag := range balancer.OutboundTags {
		key := outboundTopologyKey(agentID, tag)
		if _, ok := outbounds[key]; !ok {
			continue
		}
		score := 100
		if link, ok := linkByOutbound[key]; ok {
			score = 1000 + link.MatchScore
		}
		if score > bestScore || (score == bestScore && (bestTag == "" || tag < bestTag)) {
			bestScore = score
			bestTag = tag
		}
	}
	if bestTag == "" {
		return "", fmt.Sprintf("balancer %q did not produce a usable outbound candidate", balancer.Tag)
	}
	if link, ok := linkByOutbound[outboundTopologyKey(agentID, bestTag)]; ok {
		return bestTag, fmt.Sprintf("balancer %s selected outbound %s matched to %s", balancer.Tag, bestTag, link.Target.InboundName)
	}
	return bestTag, fmt.Sprintf("balancer %s selected outbound %s", balancer.Tag, bestTag)
}

func formatBalancerDetail(balancer model.XUIBalancerView, selectedTag string) string {
	parts := []string{
		fmt.Sprintf("候选 %s", strings.Join(balancer.OutboundTags, ", ")),
	}
	if len(balancer.Selectors) > 0 {
		parts = append(parts, "selectors="+strings.Join(balancer.Selectors, ","))
	}
	if balancer.FallbackTag != "" {
		parts = append(parts, "fallback="+balancer.FallbackTag)
	}
	if selectedTag != "" {
		parts = append(parts, "selected="+selectedTag)
	}
	return strings.Join(filterEmpty(parts), " · ")
}

func formatInboundDetail(inbound model.TopologyInboundRef) string {
	parts := []string{fmt.Sprintf("%s:%d", firstNonEmpty(inbound.Domains...), inbound.Port)}
	if len(inbound.Domains) == 0 {
		parts[0] = fmt.Sprintf("%s:%d", firstNonEmpty(inbound.IPs...), inbound.Port)
	}
	if inbound.Protocol != "" {
		parts = append(parts, inbound.Protocol)
	}
	if inbound.Network != "" {
		parts = append(parts, inbound.Network)
	}
	return strings.Join(filterEmpty(parts), " · ")
}

func formatOutboundDetail(outbound model.TopologyOutboundRef) string {
	target := outbound.Target
	if target == "" {
		target = firstNonEmpty(outbound.Address, outbound.TLSServerName, outbound.WSHost)
		if outbound.Port != 0 && target != "" {
			target = fmt.Sprintf("%s:%d", target, outbound.Port)
		}
	}
	parts := []string{target, outbound.Protocol}
	if outbound.Network != "" {
		parts = append(parts, outbound.Network)
	}
	return strings.Join(filterEmpty(parts), " · ")
}

func isTerminalOutbound(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "freedom", "blackhole", "dns":
		return true
	default:
		return false
	}
}
