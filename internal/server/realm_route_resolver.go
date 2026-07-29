package server

import (
	"fmt"
	"strings"

	"bridge-core/internal/model"
)

const maxRealmForwardDepth = 16

type realmForwardHop struct {
	SourceAgentID string
	Rule          model.RealmForwardRule
}

type realmForwardResolution struct {
	Resolved         bool
	LoopDetected     bool
	UnresolvedReason string
	FinalAgentID     string
	FinalPort        int
	FinalNode        model.XUINodeView
	Hops             []realmForwardHop
}

func resolveRealmForwardTarget(
	sourceAgentID string,
	rule model.RealmForwardRule,
	agentMap map[string]model.DashboardAgentView,
	overviewByAgent map[string]*model.XUIOverview,
) realmForwardResolution {
	resolution := realmForwardResolution{
		Hops: []realmForwardHop{{SourceAgentID: sourceAgentID, Rule: rule}},
	}
	visited := map[string]struct{}{
		realmForwardEndpointKey(sourceAgentID, rule.ListenPort): {},
	}
	currentRule := rule
	for depth := 0; depth < maxRealmForwardDepth; depth++ {
		targetAgentID := findRealmTargetAgentID(currentRule, agentMap)
		if targetAgentID == "" {
			resolution.UnresolvedReason = fmt.Sprintf("Realm target %s:%d does not match a registered Client", currentRule.TargetAddress, currentRule.TargetPort)
			return resolution
		}
		targetKey := realmForwardEndpointKey(targetAgentID, currentRule.TargetPort)
		if _, seen := visited[targetKey]; seen {
			resolution.LoopDetected = true
			resolution.UnresolvedReason = "detected a Realm forwarding loop"
			return resolution
		}
		visited[targetKey] = struct{}{}

		if targetOverview := overviewByAgent[targetAgentID]; targetOverview != nil {
			if node, ok := realmTargetNode(targetOverview.Nodes, currentRule.TargetPort); ok {
				resolution.Resolved = true
				resolution.FinalAgentID = targetAgentID
				resolution.FinalPort = currentRule.TargetPort
				resolution.FinalNode = node
				return resolution
			}
		}

		targetAgent, ok := agentMap[targetAgentID]
		if !ok {
			resolution.UnresolvedReason = "Realm target Client is not present in the current dashboard"
			return resolution
		}
		if !realmForwardingActive(targetAgent.Entry.PortForwarding) {
			resolution.UnresolvedReason = fmt.Sprintf("%s has Realm forwarding disabled", firstNonEmptyString(targetAgent.AgentName, targetAgentID))
			return resolution
		}
		nextRule, ok := realmRuleListeningOnPort(targetAgent.Entry.PortForwarding.Rules, currentRule.TargetPort)
		if !ok {
			resolution.UnresolvedReason = fmt.Sprintf("%s:%d is neither an x-ui inbound nor an enabled Realm listener", firstNonEmptyString(targetAgent.AgentName, targetAgentID), currentRule.TargetPort)
			return resolution
		}
		resolution.Hops = append(resolution.Hops, realmForwardHop{SourceAgentID: targetAgentID, Rule: nextRule})
		currentRule = nextRule
	}
	resolution.UnresolvedReason = fmt.Sprintf("Realm forwarding exceeded %d hops", maxRealmForwardDepth)
	return resolution
}

func realmForwardingActive(cfg model.RealmForwardConfig) bool {
	return cfg.Enabled && !strings.EqualFold(strings.TrimSpace(cfg.Backend), "none")
}

func realmForwardEndpointKey(agentID string, port int) string {
	return fmt.Sprintf("%s\x00%d", strings.TrimSpace(agentID), port)
}

func realmRuleListeningOnPort(rules []model.RealmForwardRule, port int) (model.RealmForwardRule, bool) {
	for _, rule := range rules {
		if rule.Enabled && rule.ListenPort == port && rule.TargetPort > 0 &&
			(strings.TrimSpace(rule.TargetAddress) != "" || strings.TrimSpace(rule.TargetAgentID) != "") {
			return rule, true
		}
	}
	return model.RealmForwardRule{}, false
}

func realmTargetNode(nodes []model.XUINodeView, port int) (model.XUINodeView, bool) {
	for _, node := range nodes {
		if node.Port == port {
			return node, true
		}
	}
	for _, node := range nodes {
		if node.Port == 0 && node.ID == port {
			return node, true
		}
	}
	return model.XUINodeView{}, false
}

func realmClientMatchesNode(client model.XUIClientView, node model.XUINodeView) bool {
	if node.ID > 0 && client.InboundID > 0 {
		return node.ID == client.InboundID
	}
	return node.Tag != "" && client.InboundTag == node.Tag
}

func realmForwardResolutionNote(host string, listenPort int, resolution realmForwardResolution, agentMap map[string]model.DashboardAgentView) string {
	parts := []string{fmt.Sprintf("%s:%d", host, listenPort)}
	for _, hop := range resolution.Hops[1:] {
		agent := agentMap[hop.SourceAgentID]
		parts = append(parts, fmt.Sprintf("%s:%d", firstNonEmptyString(agent.AgentName, hop.SourceAgentID), hop.Rule.ListenPort))
	}
	finalAgent := agentMap[resolution.FinalAgentID]
	parts = append(parts, fmt.Sprintf("%s:%d", firstNonEmptyString(finalAgent.AgentName, resolution.FinalAgentID), resolution.FinalPort))
	return "Realm 入口 " + strings.Join(parts, " -> ")
}

func haProxyForwardResolutionNote(host string, listenPort int, resolution realmForwardResolution, agentMap map[string]model.DashboardAgentView) string {
	parts := []string{fmt.Sprintf("%s:%d", host, listenPort)}
	for _, hop := range resolution.Hops {
		agent := agentMap[hop.SourceAgentID]
		parts = append(parts, fmt.Sprintf("%s:%d", firstNonEmptyString(agent.AgentName, hop.SourceAgentID), hop.Rule.ListenPort))
	}
	finalAgent := agentMap[resolution.FinalAgentID]
	parts = append(parts, fmt.Sprintf("%s:%d", firstNonEmptyString(finalAgent.AgentName, resolution.FinalAgentID), resolution.FinalPort))
	return "HAProxy 入口 " + strings.Join(parts, " -> ")
}
