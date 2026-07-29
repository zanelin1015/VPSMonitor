package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"bridge-core/internal/model"
)

type topologyMatchResult struct {
	Score       int
	Fields      []string
	Reason      string
	Explanation string
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
	annotateRealmTopologyLinks(links, linkByOutbound)
	return links, linkByOutbound
}

func annotateRealmTopologyLinks(links []model.TopologyLinkView, linkByOutbound map[string]model.TopologyLinkView) {
	for index := range links {
		link := links[index]
		sourceProtocol := normalizedTopologyProtocol(link.Source.Protocol)
		if sourceProtocol != "realm" && sourceProtocol != "haproxy" {
			continue
		}
		visited := map[string]struct{}{link.Key: {}}
		current := link
		for isTopologyForwardingProtocol(current.Target.Protocol) {
			link.RealmHops = append(link.RealmHops, current.Target)
			nextKey := outboundTopologyKey(current.Target.AgentID, current.Target.InboundTag)
			if _, seen := visited[nextKey]; seen {
				link.LoopDetected = true
				link.UnresolvedReason = "detected a forwarding loop"
				break
			}
			visited[nextKey] = struct{}{}
			next, ok := linkByOutbound[nextKey]
			if !ok {
				link.UnresolvedReason = "forwarding target did not resolve to another forwarding listener or x-ui inbound"
				break
			}
			current = next
		}
		if !link.LoopDetected && link.UnresolvedReason == "" && current.Target.AgentID != "" {
			finalTarget := current.Target
			link.FinalTarget = &finalTarget
		}
		links[index] = link
		linkByOutbound[link.Key] = link
	}
}

func isTopologyForwardingProtocol(protocol string) bool {
	protocol = normalizedTopologyProtocol(protocol)
	return protocol == "realm" || protocol == "haproxy"
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
