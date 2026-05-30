package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/realmconfig"
)

type topologyInboundCandidate struct {
	ref   model.TopologyInboundRef
	route model.XUIRouteTrace
}

type topologyOutboundCandidate struct {
	ref model.TopologyOutboundRef
}

type topologyResolver struct {
	cache        map[string][]string
	geoCache     map[string]model.IPGeoView
	allowNetwork bool
}

var topologyLookupHostIPs = defaultTopologyLookupHostIPs
var topologyLookupIPGeo = defaultTopologyLookupIPGeo

type TopologyResolverData struct {
	Hosts map[string][]string
	Geos  map[string]model.IPGeoView
}

type GlobalDashboardOptions struct {
	IncludeTopology    bool
	IncludeGeo         bool
	AllowNetworkLookup bool
	ResolverData       TopologyResolverData
}

func BuildGlobalDashboard(agents []model.AgentRecord, snapshots []model.AgentSnapshot) model.GlobalDashboardView {
	return BuildGlobalDashboardWithOptions(agents, snapshots, GlobalDashboardOptions{
		IncludeTopology:    true,
		IncludeGeo:         true,
		AllowNetworkLookup: true,
	})
}

func BuildGlobalDashboardWithOptions(agents []model.AgentRecord, snapshots []model.AgentSnapshot, options GlobalDashboardOptions) model.GlobalDashboardView {
	resolver := newTopologyResolverWithData(options.ResolverData, options.AllowNetworkLookup)
	entryByAgent := make(map[string]model.AgentEntryConfig, len(agents))
	for _, agent := range agents {
		entryByAgent[agent.AgentID] = agent.Config.Entry
	}
	overviewByAgent := make(map[string]*model.XUIOverview, len(snapshots))
	realmByAgent := make(map[string]*model.RealmSnapshot, len(snapshots))
	networkPolicyByAgent := make(map[string]*model.NetworkPolicySnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if overview := BuildXUIOverviewWithOptions(snapshot, XUIOverviewOptions{Entry: entryByAgent[snapshot.AgentID]}); overview != nil {
			overviewByAgent[snapshot.AgentID] = overview
		}
		if snapshot.Realm != nil {
			realmByAgent[snapshot.AgentID] = snapshot.Realm
		}
		if snapshot.NetworkPolicy != nil {
			networkPolicyByAgent[snapshot.AgentID] = snapshot.NetworkPolicy
		}
	}

	agentViews := make([]model.DashboardAgentView, 0, len(agents))
	agentViewByID := make(map[string]model.DashboardAgentView, len(agents))
	tagStats := make(map[string]*model.DashboardTagView)
	totals := model.DashboardTotals{AgentCount: len(agents)}

	for _, agent := range agents {
		overview := overviewByAgent[agent.AgentID]
		summary := agent.Summary
		if summary.Hostname == "" {
			summary.Hostname = agent.Hostname
		}
		if summary.PublicIPv4 == "" {
			summary.PublicIPv4 = agent.PublicIPv4
		}
		if summary.PublicIPv6 == "" {
			summary.PublicIPv6 = agent.PublicIPv6
		}
		entry := MergeRealmSnapshotIntoEntry(agent.Config.Entry, realmByAgent[agent.AgentID])
		view := model.DashboardAgentView{
			AgentID:             agent.AgentID,
			AgentName:           agent.AgentName,
			CustomerDisplayName: agent.CustomerDisplayName,
			ClientVersion:       agent.Version,
			ClientOS:            agent.OS,
			ClientArch:          agent.Arch,
			SystemVersion:       agent.SystemVersion,
			SortOrder:           agent.SortOrder,
			Tags:                cloneStrings(agent.Tags),
			Renewal:             agent.Config.Renewal,
			Entry:               entry,
			ReportedAt:          agent.ReportedAt,
			RegisteredAt:        &agent.RegisteredAt,
			UpdatedAt:           &agent.UpdatedAt,
			LastSeenAt:          agent.LastSeenAt,
			HasConfig:           agent.HasConfig,
			Summary:             summary,
			NetworkPolicy:       networkPolicyByAgent[agent.AgentID],
		}
		if options.IncludeGeo {
			view.Geo = lookupAgentGeo(summary, resolver)
			if view.Geo == nil && overview != nil {
				view.Geo = lookupOverviewGeo(overview, resolver)
			}
		}
		if overview != nil {
			view.NodeCount = overview.NodeCount
			view.ClientCount = overview.ClientCount
			view.OnlineClientCount = overview.OnlineClientCount
			view.OutboundCount = len(overview.Outbounds)
			view.RoutingRuleCount = len(overview.RoutingRules)
		} else {
			view.NodeCount = agent.Summary.InboundCount
			view.OutboundCount = agent.Summary.OutboundCount
			view.RoutingRuleCount = agent.Summary.RoutingRuleCount
		}

		agentViews = append(agentViews, view)
		agentViewByID[view.AgentID] = view

		totals.NodeCount += view.NodeCount
		totals.ClientCount += view.ClientCount
		totals.OnlineClientCount += view.OnlineClientCount
		totals.OutboundCount += view.OutboundCount
		totals.RoutingRuleCount += view.RoutingRuleCount
		if len(view.Tags) > 0 {
			totals.TaggedAgentCount++
		}
		for _, tag := range view.Tags {
			entry := tagStats[tag]
			if entry == nil {
				entry = &model.DashboardTagView{Tag: tag}
				tagStats[tag] = entry
			}
			entry.AgentCount++
			entry.NodeCount += view.NodeCount
			entry.ClientCount += view.ClientCount
			entry.OnlineClientCount += view.OnlineClientCount
		}
	}

	sort.Slice(agentViews, func(i, j int) bool {
		left := time.Time{}
		right := time.Time{}
		if agentViews[i].ReportedAt != nil {
			left = *agentViews[i].ReportedAt
		}
		if agentViews[j].ReportedAt != nil {
			right = *agentViews[j].ReportedAt
		}
		if !left.Equal(right) {
			return left.After(right)
		}
		return agentViews[i].AgentID < agentViews[j].AgentID
	})

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

	var links []model.TopologyLinkView
	var chains []model.ClientChainView
	if options.IncludeTopology {
		inboundCandidates := buildInboundCandidates(agentViewByID, overviewByAgent, resolver)
		outboundCandidates := buildOutboundCandidates(agentViewByID, overviewByAgent, resolver)
		balancerCandidates := buildBalancerCandidates(overviewByAgent)
		var linkByOutboundKey map[string]model.TopologyLinkView
		links, linkByOutboundKey = matchTopologyLinks(inboundCandidates, outboundCandidates)
		chains = buildClientChains(agentViewByID, overviewByAgent, inboundCandidates, outboundCandidates, balancerCandidates, linkByOutboundKey)
		totals.LinkCount = len(links)
		totals.ChainCount = len(chains)
	}

	return model.GlobalDashboardView{
		GeneratedAt:  time.Now().UTC(),
		Totals:       totals,
		Tags:         tags,
		Agents:       agentViews,
		Links:        links,
		ClientChains: chains,
	}
}

func lookupAgentGeo(summary model.VPSSummary, resolver *topologyResolver) *model.IPGeoView {
	if resolver == nil {
		return nil
	}
	for _, address := range []string{summary.ObservedIP, summary.PublicIPv4, summary.PublicIPv6} {
		if address == "" {
			continue
		}
		if _, geo := resolver.lookupGeo(address, nil); geo != nil {
			return geo
		}
	}
	if summary.Hostname == "" {
		return nil
	}
	resolved := resolver.lookupHost(summary.Hostname)
	_, geo := resolver.lookupGeo(summary.Hostname, resolved)
	return geo
}

func lookupOverviewGeo(overview *model.XUIOverview, resolver *topologyResolver) *model.IPGeoView {
	if overview == nil || resolver == nil {
		return nil
	}
	candidates := []string{overview.Summary.ObservedIP, overview.Summary.PublicIPv4, overview.Summary.PublicIPv6, overview.BaseURL}
	for _, value := range candidates {
		if value == "" {
			continue
		}
		resolved := resolver.lookupHost(value)
		if _, geo := resolver.lookupGeo(value, resolved); geo != nil {
			return geo
		}
	}

	certDomains := collectCertificateDomains(overview.Certificates)
	domains := mergeStringSets(collectAgentDomains(overview), certDomains)
	ips := collectAgentIPs(overview)
	for _, node := range overview.Nodes {
		domains = mergeStringSets(domains, collectInboundDomains(node, certDomains))
		ips = mergeStringSets(ips, collectInboundIPs(node.Listen, overview.Summary.PublicIPv4, overview.Summary.PublicIPv6, overview.Summary.ObservedIP))
	}
	for _, ip := range ips {
		if _, geo := resolver.lookupGeo(ip, nil); geo != nil {
			return geo
		}
	}
	for _, domain := range domains {
		resolved := resolver.lookupHost(domain)
		if _, geo := resolver.lookupGeo(domain, resolved); geo != nil {
			return geo
		}
	}
	return nil
}

func MergeRealmSnapshotIntoEntry(entry model.AgentEntryConfig, snapshot *model.RealmSnapshot) model.AgentEntryConfig {
	return realmconfig.MergeSnapshotIntoEntry(entry, snapshot)
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
			ips := mergeStringSets(collectInboundIPs(node.Listen, overview.Summary.PublicIPv4, overview.Summary.PublicIPv6, overview.Summary.ObservedIP), agentIPs, entryIPs, entryResolvedIPs, mappingIPs, mappingResolvedIPs)
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
			resolvedIPs := resolver.lookupHost(rule.TargetAddress)
			ref := model.TopologyOutboundRef{
				AgentID:     agentID,
				AgentName:   agentView.AgentName,
				AgentTags:   cloneStrings(agentView.Tags),
				OutboundTag: firstNonEmpty(rule.Name, rule.ID, fmt.Sprintf("realm:%d", rule.ListenPort)),
				Protocol:    "realm",
				Target:      fmt.Sprintf("%s:%d", rule.TargetAddress, rule.TargetPort),
				Address:     rule.TargetAddress,
				Port:        rule.TargetPort,
				Network:     normalizeRealmForwardNetworkForTopology(rule.Network),
				ResolvedIPs: resolvedIPs,
			}
			result[outboundTopologyKey(agentID, "realm:"+firstNonEmpty(rule.ID, ref.OutboundTag))] = topologyOutboundCandidate{ref: ref}
		}
	}
	return result
}

func activeRealmForwardRulesForTopology(items []model.RealmForwardRule) []model.RealmForwardRule {
	rules := make([]model.RealmForwardRule, 0, len(items))
	for _, rule := range items {
		rule.TargetAddress = strings.TrimSpace(rule.TargetAddress)
		if !rule.Enabled || rule.TargetAddress == "" || rule.ListenPort <= 0 || rule.TargetPort <= 0 {
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

func matchTopologyLinks(inbounds map[string]topologyInboundCandidate, outbounds map[string]topologyOutboundCandidate) ([]model.TopologyLinkView, map[string]model.TopologyLinkView) {
	inboundList := make([]topologyInboundCandidate, 0, len(inbounds))
	for _, inbound := range inbounds {
		inboundList = append(inboundList, inbound)
	}

	links := make([]model.TopologyLinkView, 0)
	linkByOutbound := make(map[string]model.TopologyLinkView)
	for outboundKey, outbound := range outbounds {
		best := topologyMatchResult{}
		var bestInbound topologyInboundCandidate
		for _, inbound := range inboundList {
			match := scoreTopologyMatch(outbound.ref, inbound.ref)
			if match.Score > best.Score {
				best = match
				bestInbound = inbound
			}
		}
		if best.Score == 0 {
			continue
		}
		link := model.TopologyLinkView{
			Key:              outboundKey,
			Source:           outbound.ref,
			Target:           bestInbound.ref,
			MatchScore:       best.Score,
			MatchConfidence:  matchConfidence(best.Score, best.Fields),
			MatchReason:      best.Reason,
			MatchExplanation: best.Explanation,
			MatchFields:      cloneStrings(best.Fields),
		}
		links = append(links, link)
		linkByOutbound[outboundKey] = link
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].Source.AgentName != links[j].Source.AgentName {
			return links[i].Source.AgentName < links[j].Source.AgentName
		}
		return links[i].Source.OutboundTag < links[j].Source.OutboundTag
	})
	return links, linkByOutbound
}

type topologyMatchResult struct {
	Score       int
	Fields      []string
	Reason      string
	Explanation string
}

func scoreTopologyMatch(outbound model.TopologyOutboundRef, inbound model.TopologyInboundRef) topologyMatchResult {
	if !isSupportedNodeProtocol(outbound.Protocol) || !isSupportedNodeProtocol(inbound.Protocol) {
		return topologyMatchResult{}
	}

	if mapped := scoreTopologyEntryMappingMatch(outbound, inbound); mapped.Score > 0 {
		return mapped
	}

	fields := make([]string, 0, 6)
	score := 0
	hostMatched := false

	addressHost := extractEndpointHost(outbound.Address)
	address := normalizeHost(outbound.Address)
	serverName := normalizeHost(outbound.TLSServerName)
	wsHost := normalizeHost(outbound.WSHost)
	authMatched := false

	inboundDomains := uniqueNormalizedDomains(inbound.Domains)
	inboundIPs := make(map[string]struct{}, len(inbound.IPs))
	for _, ip := range inbound.IPs {
		if normalized := normalizeIP(ip); normalized != "" {
			inboundIPs[normalized] = struct{}{}
		}
	}

	if normalized := normalizeIP(addressHost); normalized != "" {
		if _, ok := inboundIPs[normalized]; ok {
			score += 120
			fields = append(fields, "address_ip")
			hostMatched = true
		}
	} else if address != "" {
		if matchesDomainPatterns(address, inboundDomains) {
			score += 120
			fields = append(fields, "address_domain")
			hostMatched = true
		}
	}
	for _, resolvedIP := range outbound.ResolvedIPs {
		if _, ok := inboundIPs[resolvedIP]; ok {
			score += 95
			fields = appendUnique(fields, "resolved_ip")
			hostMatched = true
			break
		}
	}
	if serverName != "" {
		if matchesDomainPatterns(serverName, inboundDomains) {
			score += 80
			fields = append(fields, "tls_server_name")
			hostMatched = true
		}
	}
	if wsHost != "" {
		if matchesDomainPatterns(wsHost, inboundDomains) {
			score += 60
			fields = append(fields, "ws_host")
			hostMatched = true
		}
	}
	if !hostMatched {
		return topologyMatchResult{}
	}
	if authScore := scoreAuthKeyMatch(outbound.AuthKeys, inbound.AuthKeys); authScore > 0 {
		score += authScore
		fields = append(fields, "credential")
		authMatched = true
	}
	if outbound.Port != 0 && inbound.Port != 0 {
		if outbound.Port != inbound.Port {
			if !authMatched {
				return topologyMatchResult{}
			}
			fields = append(fields, "port_mapped")
		} else {
			score += 20
			fields = append(fields, "port")
		}
	}
	if normalizedTopologyProtocol(outbound.Protocol) == normalizedTopologyProtocol(inbound.Protocol) && outbound.Protocol != "" {
		score += 15
		fields = append(fields, "protocol")
	}
	if strings.EqualFold(outbound.Network, inbound.Network) && outbound.Network != "" {
		score += 10
		fields = append(fields, "network")
	}
	if strings.EqualFold(outbound.Security, inbound.Security) && outbound.Security != "" {
		score += 10
		fields = append(fields, "security")
	}
	if outbound.WSPath != "" && outbound.WSPath == inbound.WSPath {
		score += 5
		fields = append(fields, "ws_path")
	}
	return topologyMatchResult{
		Score:       score,
		Fields:      fields,
		Reason:      formatTopologyMatchReason(fields),
		Explanation: formatTopologyMatchExplanation(outbound, inbound, fields),
	}
}

func scoreTopologyEntryMappingMatch(outbound model.TopologyOutboundRef, inbound model.TopologyInboundRef) topologyMatchResult {
	if len(inbound.EntryMappings) == 0 {
		return topologyMatchResult{}
	}
	outboundProtocol := normalizedTopologyProtocol(outbound.Protocol)
	inboundProtocol := normalizedTopologyProtocol(inbound.Protocol)
	for _, mapping := range inbound.EntryMappings {
		mappingProtocol := normalizedTopologyProtocol(mapping.Protocol)
		if mappingProtocol == "" || mappingProtocol != outboundProtocol || mappingProtocol != inboundProtocol {
			continue
		}
		if mapping.InternalPort != 0 && inbound.Port != 0 && mapping.InternalPort != inbound.Port {
			continue
		}
		if mapping.ExternalPort != 0 && outbound.Port != 0 && mapping.ExternalPort != outbound.Port {
			continue
		}
		if mapping.ExternalPort != 0 && outbound.Port == 0 {
			continue
		}
		if !outboundMatchesEntryAddress(outbound, mapping.Address, mapping.ResolvedIPs) {
			continue
		}
		fields := []string{"entry_mapping", "entry_address", "external_port", "internal_port", "protocol"}
		score := 320
		if authScore := scoreAuthKeyMatch(outbound.AuthKeys, inbound.AuthKeys); authScore > 0 {
			score += authScore
			fields = append(fields, "credential")
		}
		if strings.EqualFold(outbound.Network, inbound.Network) && outbound.Network != "" {
			score += 10
			fields = append(fields, "network")
		}
		if strings.EqualFold(outbound.Security, inbound.Security) && outbound.Security != "" {
			score += 10
			fields = append(fields, "security")
		}
		if outbound.WSPath != "" && outbound.WSPath == inbound.WSPath {
			score += 5
			fields = append(fields, "ws_path")
		}
		endpoint := formatEndpoint(mapping.Address, mapping.ExternalPort)
		internal := inbound.InboundName
		if internal == "" {
			internal = inbound.InboundTag
		}
		if internal == "" {
			internal = fmt.Sprintf("Inbound #%d", inbound.InboundID)
		}
		if mapping.InternalPort != 0 {
			internal = fmt.Sprintf("%s:%d", internal, mapping.InternalPort)
		}
		reason := fmt.Sprintf("入口映射 %s/%s -> %s", endpoint, mappingProtocol, internal)
		explanation := fmt.Sprintf("出站目标 %s 命中 %s 的入口/NAT 映射，按外部端口 %d 转到内部端口 %d。", formatOutboundEndpoint(outbound), inbound.AgentName, mapping.ExternalPort, mapping.InternalPort)
		if mapping.Note != "" {
			explanation += " 备注：" + mapping.Note
		}
		return topologyMatchResult{
			Score:       score,
			Fields:      fields,
			Reason:      reason,
			Explanation: explanation,
		}
	}
	return topologyMatchResult{}
}

func scoreAuthKeyMatch(outboundKeys []string, inboundKeys []string) int {
	if len(outboundKeys) == 0 || len(inboundKeys) == 0 {
		return 0
	}
	inboundSet := make(map[string]struct{}, len(inboundKeys))
	for _, key := range inboundKeys {
		if key != "" {
			inboundSet[key] = struct{}{}
		}
	}
	matches := 0
	for _, key := range outboundKeys {
		if _, ok := inboundSet[key]; ok {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	if matches > 2 {
		matches = 2
	}
	return 160 + (matches-1)*40
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
				Key:              clientChainKey(agentID, client.InboundID, client.Email),
				RootAgentID:      agentID,
				RootAgentName:    agentView.AgentName,
				RootAgentTags:    cloneStrings(agentView.Tags),
				RootClientEmail:  client.Email,
				RootClientRemark: firstNonEmpty(client.Comment, client.SubID),
				RootInboundTag:   client.InboundTag,
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
	// Local cert domains are VPS-level entry aliases, not only TLS-node names.
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

func uniqueNormalizedDomains(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeHost(value); normalized != "" {
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result
}

func uniqueNormalizedIPs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeIP(value); normalized != "" {
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result
}

func mergeStringSets(sets ...[]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, values := range sets {
		for _, value := range values {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func outboundMatchesEntryAddress(outbound model.TopologyOutboundRef, entryAddress string, entryResolvedIPs []string) bool {
	entryHost := extractEndpointHost(entryAddress)
	if entryHost == "" {
		return false
	}
	outboundAddress := extractEndpointHost(outbound.Address)
	entryIP := normalizeIP(entryHost)
	outboundIP := normalizeIP(outboundAddress)
	entryDomain := normalizeHost(entryHost)
	outboundDomain := normalizeHost(outboundAddress)
	entryResolvedSet := make(map[string]struct{}, len(entryResolvedIPs))
	for _, ip := range entryResolvedIPs {
		if normalized := normalizeIP(ip); normalized != "" {
			entryResolvedSet[normalized] = struct{}{}
		}
	}
	if entryIP != "" {
		if outboundIP == entryIP {
			return true
		}
		for _, ip := range outbound.ResolvedIPs {
			if normalizeIP(ip) == entryIP {
				return true
			}
		}
		return false
	}
	if entryDomain != "" && outboundDomain != "" && domainMatchesPattern(outboundDomain, entryDomain) {
		return true
	}
	if outboundIP != "" {
		if _, ok := entryResolvedSet[outboundIP]; ok {
			return true
		}
	}
	for _, ip := range outbound.ResolvedIPs {
		if _, ok := entryResolvedSet[normalizeIP(ip)]; ok {
			return true
		}
	}
	return false
}

func formatTopologyMatchReason(fields []string) string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "address_ip":
			labels = append(labels, "目标 IP")
		case "address_domain":
			labels = append(labels, "目标域名")
		case "resolved_ip":
			labels = append(labels, "解析 IP")
		case "tls_server_name":
			labels = append(labels, "TLS SNI")
		case "ws_host":
			labels = append(labels, "WS Host")
		case "credential":
			labels = append(labels, "客户端凭证")
		case "port":
			labels = append(labels, "端口")
		case "port_mapped":
			labels = append(labels, "端口映射")
		case "protocol":
			labels = append(labels, "节点类型")
		case "network":
			labels = append(labels, "传输协议")
		case "security":
			labels = append(labels, "TLS/Reality")
		case "ws_path":
			labels = append(labels, "WS Path")
		case "entry_mapping":
			labels = append(labels, "入口映射")
		case "entry_address":
			labels = append(labels, "入口地址")
		case "external_port":
			labels = append(labels, "外部端口")
		case "internal_port":
			labels = append(labels, "内部端口")
		default:
			labels = append(labels, field)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return "匹配依据：" + strings.Join(labels, " + ")
}

func formatTopologyMatchExplanation(outbound model.TopologyOutboundRef, inbound model.TopologyInboundRef, fields []string) string {
	return fmt.Sprintf("出站 %s 通过 %s 连接到 %s 的 %s。", firstNonEmpty(outbound.OutboundTag, outbound.Protocol, "unknown"), formatOutboundEndpoint(outbound), inbound.AgentName, firstNonEmpty(inbound.InboundName, inbound.InboundTag, fmt.Sprintf("Inbound #%d", inbound.InboundID))) + " " + formatTopologyMatchReason(fields)
}

func matchConfidence(score int, fields []string) string {
	if containsString(fields, "entry_mapping") || score >= 300 {
		return "high"
	}
	if score >= 180 {
		return "medium"
	}
	return "low"
}

func formatOutboundEndpoint(outbound model.TopologyOutboundRef) string {
	target := firstNonEmpty(outbound.Target, outbound.Address)
	if outbound.Address != "" {
		target = outbound.Address
	}
	if outbound.Port > 0 && target != "" && !strings.Contains(target, ":") {
		return fmt.Sprintf("%s:%d", target, outbound.Port)
	}
	return firstNonEmpty(target, outbound.OutboundTag, "-")
}

func formatEndpoint(address string, port int) string {
	if address == "" {
		return fmt.Sprintf(":%d", port)
	}
	if port <= 0 {
		return address
	}
	return fmt.Sprintf("%s:%d", address, port)
}

func normalizeHost(value string) string {
	value = extractEndpointHost(value)
	if normalizeIP(value) != "" {
		return ""
	}
	return value
}

func extractEndpointHost(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if strings.Contains(value, "/") {
		value = strings.Split(value, "/")[0]
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	return value
}

func matchesDomainPatterns(domain string, patterns []string) bool {
	for _, pattern := range patterns {
		if domainMatchesPattern(domain, pattern) {
			return true
		}
	}
	return false
}

func domainMatchesPattern(domain string, pattern string) bool {
	if domain == "" || pattern == "" {
		return false
	}
	if pattern == domain {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(domain, suffix)
	}
	return false
}

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return ""
}

func inboundTopologyKey(agentID string, inboundID int, inboundTag string) string {
	return fmt.Sprintf("%s::%d::%s", agentID, inboundID, inboundTag)
}

func outboundTopologyKey(agentID string, outboundTag string) string {
	return agentID + "::" + outboundTag
}

func clientChainKey(agentID string, inboundID int, email string) string {
	return fmt.Sprintf("%s::%d::%s", agentID, inboundID, email)
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

func filterEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func NewTopologyResolverData(values []string) TopologyResolverData {
	resolver := newTopologyResolverWithData(TopologyResolverData{}, true)
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		resolved := resolver.lookupHost(value)
		_, _ = resolver.lookupGeo(value, resolved)
	}
	return TopologyResolverData{
		Hosts: cloneResolverHosts(resolver.cache),
		Geos:  cloneResolverGeos(resolver.geoCache),
	}
}

func newTopologyResolver() *topologyResolver {
	return newTopologyResolverWithData(TopologyResolverData{}, true)
}

func newTopologyResolverWithData(data TopologyResolverData, allowNetwork bool) *topologyResolver {
	resolver := &topologyResolver{
		cache:        make(map[string][]string),
		geoCache:     make(map[string]model.IPGeoView),
		allowNetwork: allowNetwork,
	}
	for host, ips := range data.Hosts {
		normalizedHost := normalizeHost(host)
		if normalizedHost == "" {
			continue
		}
		resolver.cache[normalizedHost] = uniqueNormalizedIPs(ips)
	}
	for ip, geo := range data.Geos {
		normalizedIP := normalizeIP(ip)
		if normalizedIP == "" && geo.IP != "" {
			normalizedIP = normalizeIP(geo.IP)
		}
		if normalizedIP == "" {
			continue
		}
		if geo.IP == "" {
			geo.IP = normalizedIP
		}
		resolver.geoCache[normalizedIP] = geo
	}
	return resolver
}

func (r *topologyResolver) lookupAll(values []string) []string {
	result := make([]string, 0)
	for _, value := range values {
		result = append(result, r.lookupHost(value)...)
	}
	return uniqueNormalizedIPs(result)
}

func (r *topologyResolver) lookupHost(value string) []string {
	host := normalizeHost(value)
	if host == "" {
		return nil
	}
	if cached, ok := r.cache[host]; ok {
		return append([]string(nil), cached...)
	}
	if !r.allowNetwork {
		r.cache[host] = nil
		return nil
	}
	resolved := topologyLookupHostIPs(host)
	resolved = uniqueNormalizedIPs(resolved)
	r.cache[host] = resolved
	return append([]string(nil), resolved...)
}

func (r *topologyResolver) lookupGeo(address string, resolvedIPs []string) (string, *model.IPGeoView) {
	candidates := make([]string, 0, len(resolvedIPs)+1)
	if ip := normalizeIP(extractEndpointHost(address)); ip != "" {
		candidates = append(candidates, ip)
	}
	candidates = append(candidates, resolvedIPs...)
	for _, candidate := range uniqueNormalizedIPs(candidates) {
		if !isPublicGeoIP(candidate) {
			continue
		}
		if cached, ok := r.geoCache[candidate]; ok {
			return candidate, cloneGeo(cached)
		}
		if !r.allowNetwork {
			continue
		}
		geo, ok := topologyLookupIPGeo(candidate)
		if !ok {
			r.geoCache[candidate] = model.IPGeoView{IP: candidate}
			continue
		}
		if geo.IP == "" {
			geo.IP = candidate
		}
		r.geoCache[candidate] = geo
		return candidate, cloneGeo(geo)
	}
	return "", nil
}

func cloneResolverHosts(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, item := range values {
		cloned[key] = append([]string(nil), item...)
	}
	return cloned
}

func cloneResolverGeos(values map[string]model.IPGeoView) map[string]model.IPGeoView {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]model.IPGeoView, len(values))
	for key, item := range values {
		cloned[key] = item
	}
	return cloned
}

func cloneGeo(geo model.IPGeoView) *model.IPGeoView {
	if geo == (model.IPGeoView{}) {
		return nil
	}
	cloned := geo
	return &cloned
}

func defaultTopologyLookupHostIPs(host string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(ips))
	for _, ip := range ips {
		result = append(result, ip.String())
	}
	return result
}

func defaultTopologyLookupIPGeo(ip string) (model.IPGeoView, bool) {
	if !isPublicGeoIP(ip) {
		return model.IPGeoView{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	endpoint := "http://ip-api.com/json/" + url.PathEscape(ip) + "?fields=status,message,country,countryCode,regionName,city,query"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return model.IPGeoView{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return model.IPGeoView{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return model.IPGeoView{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return model.IPGeoView{}, false
	}
	var payload struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		RegionName  string `json:"regionName"`
		City        string `json:"city"`
		Query       string `json:"query"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Status != "success" {
		return model.IPGeoView{}, false
	}
	return model.IPGeoView{
		IP:          firstNonEmpty(payload.Query, ip),
		CountryCode: strings.ToUpper(payload.CountryCode),
		CountryName: payload.Country,
		RegionName:  payload.RegionName,
		City:        payload.City,
	}, true
}

func isPublicGeoIP(value string) bool {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return false
	}
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return false
	}
	for _, prefixText := range []string{
		"192.0.2.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	} {
		prefix := netip.MustParsePrefix(prefixText)
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}
