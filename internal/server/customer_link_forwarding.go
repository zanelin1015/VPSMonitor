package server

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

type customerPublicEntry struct {
	Host  string
	Port  int
	Error string
}

func resolveForwardPublicEntry(agent model.DashboardAgentView, kind, id string, port int, network, host string) customerPublicEntry {
	endpoint := model.ResolveEntryEndpoint(agent.Entry, model.EntryTarget{Type: kind, ID: id, Port: port, Transport: forwardEntryTransport(network)}, host)
	return customerPublicEntry{Host: endpoint.Address, Port: endpoint.Port, Error: endpoint.Error}
}

func forwardEntryTransport(network string) string {
	switch strings.ToLower(network) {
	case "udp":
		return "udp"
	case "both", "tcp+udp":
		return "both"
	default:
		return "tcp"
	}
}

func customerForwardingPublicEntry(assignment model.CustomerAssignment, chain model.ClientChainView, agentMap map[string]model.DashboardAgentView) (customerPublicEntry, bool) {
	if assignment.AgentID == "" || agentMap == nil {
		return customerPublicEntry{}, false
	}
	assignmentAgent, ok := agentMap[assignment.AgentID]
	if !ok {
		return customerPublicEntry{}, false
	}
	if !strings.EqualFold(chain.RootAgentID, assignment.AgentID) {
		if rule, found := haProxyRuleListeningOnPort(assignmentAgent.Entry.HAProxy.Rules, assignment.InboundID); assignmentAgent.Entry.HAProxy.Enabled && found {
			return resolveForwardPublicEntry(assignmentAgent, "haproxy", rule.ID, rule.ListenPort, "tcp", customerHAProxySourceHost(assignmentAgent, rule)), true
		}
		if rule, found := realmRuleListeningOnPort(assignmentAgent.Entry.PortForwarding.Rules, assignment.InboundID); realmForwardingActive(assignmentAgent.Entry.PortForwarding) && found {
			return resolveForwardPublicEntry(assignmentAgent, "realm", rule.ID, rule.ListenPort, rule.Network, customerRealmSourceHost(assignmentAgent, rule)), true
		}
	}
	inboundPort := customerChainRootInboundPort(assignment, chain)
	if inboundPort <= 0 {
		return customerPublicEntry{}, false
	}
	entry, _, ok := customerOutermostForwardingEntry(assignment.AgentID, inboundPort, agentMap, make(map[string]struct{}))
	return entry, ok
}

func customerOutermostForwardingEntry(
	targetAgentID string,
	targetPort int,
	agentMap map[string]model.DashboardAgentView,
	visited map[string]struct{},
) (customerPublicEntry, int, bool) {
	endpointKey := realmForwardEndpointKey(targetAgentID, targetPort)
	if _, seen := visited[endpointKey]; seen {
		return customerPublicEntry{}, 0, false
	}
	visited[endpointKey] = struct{}{}
	defer delete(visited, endpointKey)

	targetAgent, ok := agentMap[targetAgentID]
	if !ok {
		return customerPublicEntry{}, 0, false
	}
	targetAddresses := customerAgentAddressSet(targetAgent)
	agentIDs := make([]string, 0, len(agentMap))
	for agentID := range agentMap {
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)

	best := customerPublicEntry{}
	bestDepth := 0
	for _, sourceAgentID := range agentIDs {
		sourceAgent := agentMap[sourceAgentID]
		realmRules := sourceAgent.Entry.PortForwarding.Rules
		if !realmForwardingActive(sourceAgent.Entry.PortForwarding) {
			realmRules = nil
		}
		for _, rule := range realmRules {
			if !rule.Enabled || rule.ListenPort <= 0 || !customerTargetPortMatches(targetAgent.Entry, targetPort, rule.TargetPort) {
				continue
			}
			if !customerRealmRuleTargetsAgent(rule, targetAgentID, targetAddresses) {
				continue
			}
			candidate := resolveForwardPublicEntry(sourceAgent, "realm", rule.ID, rule.ListenPort, rule.Network, customerRealmSourceHost(sourceAgent, rule))
			depth := 1
			if upstream, upstreamDepth, found := customerOutermostForwardingEntry(sourceAgentID, rule.ListenPort, agentMap, visited); found {
				candidate = upstream
				depth += upstreamDepth
			}
			if depth > bestDepth {
				best = candidate
				bestDepth = depth
			}
		}
		for _, rule := range sourceAgent.Entry.HAProxy.Rules {
			if !sourceAgent.Entry.HAProxy.Enabled || !rule.Enabled || rule.ListenPort <= 0 ||
				!customerHAProxyRuleTargetsMappedEndpoint(rule, targetAgentID, targetPort, targetAgent.Entry) {
				continue
			}
			candidate := resolveForwardPublicEntry(sourceAgent, "haproxy", rule.ID, rule.ListenPort, "tcp", customerHAProxySourceHost(sourceAgent, rule))
			depth := 1
			if upstream, upstreamDepth, found := customerOutermostForwardingEntry(sourceAgentID, rule.ListenPort, agentMap, visited); found {
				candidate = upstream
				depth += upstreamDepth
			}
			if depth > bestDepth {
				best = candidate
				bestDepth = depth
			}
		}
	}
	return best, bestDepth, bestDepth > 0
}

func haProxyRuleListeningOnPort(rules []model.HAProxyRule, port int) (model.HAProxyRule, bool) {
	for _, rule := range rules {
		if rule.Enabled && rule.ListenPort == port && rule.Primary.Port > 0 &&
			(strings.TrimSpace(rule.Primary.AgentID) != "" || strings.TrimSpace(rule.Primary.Address) != "") {
			return rule, true
		}
	}
	return model.HAProxyRule{}, false
}

func customerTargetPortMatches(entry model.AgentEntryConfig, internalPort, destinationPort int) bool {
	if internalPort == destinationPort {
		return true
	}
	for _, m := range entry.Mappings {
		if !m.UseForExport || m.InternalPort != internalPort {
			continue
		}
		if entryTargetMatchesPublicPort(entry, model.EntryTarget{Type: m.TargetType, ID: m.TargetID, Port: internalPort, Transport: m.Transport}, destinationPort) {
			return true
		}
	}
	return false
}

func customerHAProxyRuleTargetsMappedEndpoint(rule model.HAProxyRule, agentID string, port int, entry model.AgentEntryConfig) bool {
	for _, target := range append([]model.HAProxyRealmTarget{rule.Primary}, rule.Backups...) {
		if strings.EqualFold(strings.TrimSpace(target.AgentID), agentID) && customerTargetPortMatches(entry, port, target.Port) {
			return true
		}
	}
	return false
}

func customerChainRootInboundPort(assignment model.CustomerAssignment, chain model.ClientChainView) int {
	for _, step := range chain.Steps {
		if step.AgentID != assignment.AgentID || step.Port <= 0 {
			continue
		}
		if step.StepType == "inbound" || step.StepType == "match" {
			return step.Port
		}
	}
	return 0
}

func customerRealmRuleTargetsAgent(rule model.RealmForwardRule, targetAgentID string, targetAddresses map[string]struct{}) bool {
	if strings.TrimSpace(rule.TargetAgentID) != "" {
		return strings.EqualFold(strings.TrimSpace(rule.TargetAgentID), targetAgentID)
	}
	targetHost := normalizeCustomerShareHost(rule.TargetAddress)
	if targetHost == "" {
		return false
	}
	_, ok := targetAddresses[strings.ToLower(targetHost)]
	return ok
}

func customerAgentAddressSet(agent model.DashboardAgentView) map[string]struct{} {
	values := append([]string{}, agent.Entry.Addresses...)
	values = append(values, agent.Entry.ImportDomain, agent.Entry.Connection.PublicAddress)
	for _, mapping := range agent.Entry.Mappings {
		values = append(values, mapping.Address)
	}
	values = append(values, agent.Summary.ObservedIP, agent.Summary.PublicIPv4, agent.Summary.PublicIPv6)
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		host := normalizeCustomerShareHost(value)
		if host == "" {
			continue
		}
		result[strings.ToLower(host)] = struct{}{}
	}
	return result
}

func customerRealmSourceHost(agent model.DashboardAgentView, rule model.RealmForwardRule) string {
	return customerForwardSourceHost(agent, rule.ListenAddress)
}

func customerHAProxySourceHost(agent model.DashboardAgentView, rule model.HAProxyRule) string {
	return customerForwardSourceHost(agent, rule.ListenAddress)
}

func customerForwardSourceHost(agent model.DashboardAgentView, listenAddress string) string {
	for _, candidate := range []string{agent.Entry.Connection.PublicAddress, agent.Entry.ImportDomain} {
		if host := normalizeCustomerShareHost(candidate); host != "" && !isWildcardListenAddress(host) {
			return host
		}
	}
	for _, candidate := range agent.Entry.Addresses {
		if host := normalizeCustomerShareHost(candidate); host != "" && !isWildcardListenAddress(host) {
			return host
		}
	}
	for _, candidate := range []string{listenAddress, agent.Summary.ObservedIP, agent.Summary.PublicIPv4, agent.Summary.PublicIPv6} {
		if host := normalizeCustomerShareHost(candidate); host != "" && !isWildcardListenAddress(host) {
			return host
		}
	}
	return ""
}

func rewriteCustomerImportURL(rawURL string, host string, port int) string {
	rawURL = strings.TrimSpace(rawURL)
	host = normalizeCustomerShareHost(host)
	if rawURL == "" || host == "" || port <= 0 || port > 65535 {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "vmess://") {
		return rewriteCustomerVMessImportURL(rawURL, host, port)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return ""
	}
	parsed.Host = net.JoinHostPort(host, strconv.Itoa(port))
	return parsed.String()
}

func rewriteCustomerVMessImportURL(rawURL string, host string, port int) string {
	payload := strings.TrimSpace(rawURL[len("vmess://"):])
	decoded, err := decodeCustomerVMessPayload(payload)
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(decoded, &data); err != nil {
		return ""
	}
	data["add"] = host
	data["port"] = port
	next, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(next)
}

func decodeCustomerVMessPayload(payload string) ([]byte, error) {
	normalized := strings.NewReplacer("-", "+", "_", "/").Replace(payload)
	if padding := len(normalized) % 4; padding > 0 {
		normalized += strings.Repeat("=", 4-padding)
	}
	return base64.StdEncoding.DecodeString(normalized)
}

func normalizeCustomerShareHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Host
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(strings.TrimSpace(value), "[]")
	if slash := strings.Index(value, "/"); slash >= 0 {
		value = value[:slash]
	}
	return strings.TrimSpace(value)
}

func isWildcardListenAddress(host string) bool {
	switch strings.TrimSpace(strings.ToLower(host)) {
	case "", "0.0.0.0", "::", "[::]", "*":
		return true
	default:
		return false
	}
}
