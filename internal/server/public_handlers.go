package server

import (
	"fmt"
	"net/http"
	"sort"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (a *App) handlePublicTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view := dashboard.BuildGlobalDashboardWithOptions(agents, snapshots, dashboard.GlobalDashboardOptions{
		IncludeTopology:    true,
		IncludeGeo:         true,
		AllowNetworkLookup: false,
		ResolverData:       a.dashboardTopologyResolverData(),
	})
	if a.realtime != nil {
		a.realtime.applyToDashboard(&view)
	}
	writeJSON(w, http.StatusOK, sanitizePublicTopologyView(view))
}

func sanitizePublicTopologyView(view model.GlobalDashboardView) model.GlobalDashboardView {
	idMap := make(map[string]string, len(view.Agents))
	titleMap := make(map[string]string, len(view.Agents))
	sort.Slice(view.Agents, func(i, j int) bool {
		if view.Agents[i].SortOrder != view.Agents[j].SortOrder {
			return view.Agents[i].SortOrder < view.Agents[j].SortOrder
		}
		return view.Agents[i].AgentID < view.Agents[j].AgentID
	})
	for index := range view.Agents {
		publicID := fmt.Sprintf("node-%02d", index+1)
		idMap[view.Agents[index].AgentID] = publicID
		titleMap[view.Agents[index].AgentID] = publicAgentTitle(view.Agents[index], index+1)
		view.Agents[index] = sanitizePublicAgent(view.Agents[index], publicID, index+1)
	}
	for index := range view.Links {
		view.Links[index] = sanitizePublicTopologyLink(view.Links[index], idMap, titleMap)
		view.Links[index].Key = fmt.Sprintf("public-link-%02d", index+1)
	}
	publicChains := make([]model.ClientChainView, 0, len(view.ClientChains))
	for index, chain := range view.ClientChains {
		publicChains = append(publicChains, sanitizePublicClientChain(chain, index+1, idMap, titleMap))
	}
	for index := range view.Tags {
		view.Tags[index].ClientCount = 0
		view.Tags[index].OnlineClientCount = 0
	}
	view.ClientChains = publicChains
	view.Totals.ClientCount = 0
	view.Totals.OnlineClientCount = 0
	view.Totals.ChainCount = len(publicChains)
	return view
}

func sanitizePublicAgent(agent model.DashboardAgentView, publicID string, ordinal int) model.DashboardAgentView {
	title := publicAgentTitle(agent, ordinal)
	return model.DashboardAgentView{
		AgentID:             publicID,
		AgentName:           title,
		CustomerDisplayName: title,
		Tags:                append([]string(nil), agent.Tags...),
		ReportedAt:          agent.ReportedAt,
		RealtimeAt:          agent.RealtimeAt,
		LastSeenAt:          agent.LastSeenAt,
		HasConfig:           agent.HasConfig,
		Geo:                 cloneIPGeo(agent.Geo),
		NodeCount:           agent.NodeCount,
		OutboundCount:       agent.OutboundCount,
		RoutingRuleCount:    agent.RoutingRuleCount,
		Summary: model.VPSSummary{
			NetIOUp:          agent.Summary.NetIOUp,
			NetIODown:        agent.Summary.NetIODown,
			NetTrafficSent:   agent.Summary.NetTrafficSent,
			NetTrafficRecv:   agent.Summary.NetTrafficRecv,
			NetTrafficTotal:  agent.Summary.NetTrafficTotal,
			XrayState:        agent.Summary.XrayState,
			InboundCount:     agent.Summary.InboundCount,
			OutboundCount:    agent.Summary.OutboundCount,
			RoutingRuleCount: agent.Summary.RoutingRuleCount,
		},
	}
}

func sanitizePublicTopologyLink(link model.TopologyLinkView, idMap map[string]string, titleMap map[string]string) model.TopologyLinkView {
	link.Source = sanitizePublicOutboundRef(link.Source, idMap, titleMap)
	link.Target = sanitizePublicInboundRef(link.Target, idMap, titleMap)
	link.MatchFields = nil
	link.MatchExplanation = ""
	return link
}

func sanitizePublicOutboundRef(ref model.TopologyOutboundRef, idMap map[string]string, titleMap map[string]string) model.TopologyOutboundRef {
	publicID := publicTopologyAgentID(ref.AgentID, idMap)
	return model.TopologyOutboundRef{
		AgentID:     publicID,
		AgentName:   publicTopologyAgentTitle(ref.AgentID, publicID, titleMap),
		AgentTags:   append([]string(nil), ref.AgentTags...),
		OutboundTag: publicOutboundLabel(ref.OutboundTag),
		Protocol:    ref.Protocol,
		Port:        ref.Port,
		ListenPort:  ref.ListenPort,
		Network:     ref.Network,
		Security:    ref.Security,
		TargetGeo:   cloneIPGeo(ref.TargetGeo),
	}
}

func sanitizePublicInboundRef(ref model.TopologyInboundRef, idMap map[string]string, titleMap map[string]string) model.TopologyInboundRef {
	publicID := publicTopologyAgentID(ref.AgentID, idMap)
	return model.TopologyInboundRef{
		AgentID:       publicID,
		AgentName:     publicTopologyAgentTitle(ref.AgentID, publicID, titleMap),
		AgentTags:     append([]string(nil), ref.AgentTags...),
		InboundID:     ref.InboundID,
		InboundTag:    publicInboundLabel(ref.InboundTag, ref.InboundName, ref.InboundID),
		InboundName:   publicInboundLabel(ref.InboundTag, ref.InboundName, ref.InboundID),
		Protocol:      ref.Protocol,
		Port:          ref.Port,
		Network:       ref.Network,
		Security:      ref.Security,
		ResolvedIPs:   nil,
		Domains:       nil,
		IPs:           nil,
		EntryIPs:      nil,
		EntryMappings: nil,
	}
}

func sanitizePublicClientChain(chain model.ClientChainView, ordinal int, idMap map[string]string, titleMap map[string]string) model.ClientChainView {
	publicRootID := publicTopologyAgentID(chain.RootAgentID, idMap)
	result := model.ClientChainView{
		Key:               fmt.Sprintf("public-chain-%02d", ordinal),
		RootAgentID:       publicRootID,
		RootAgentName:     publicTopologyAgentTitle(chain.RootAgentID, publicRootID, titleMap),
		RootAgentTags:     append([]string(nil), chain.RootAgentTags...),
		RootClientEnabled: chain.RootClientEnabled,
		RootInboundTag:    publicInboundLabel("", "", 0),
		MatchedLinkCount:  chain.MatchedLinkCount,
		LoopDetected:      chain.LoopDetected,
		Steps:             make([]model.ClientChainStep, 0, len(chain.Steps)),
	}
	for _, step := range chain.Steps {
		if step.StepType == "client" {
			continue
		}
		publicID := publicTopologyAgentID(step.AgentID, idMap)
		publicStep := model.ClientChainStep{
			StepType:   step.StepType,
			AgentID:    publicID,
			AgentName:  publicTopologyAgentTitle(step.AgentID, publicID, titleMap),
			AgentTags:  append([]string(nil), step.AgentTags...),
			Protocol:   step.Protocol,
			Port:       step.Port,
			RouteScope: step.RouteScope,
			RuleIndex:  step.RuleIndex,
			TargetGeo:  cloneIPGeo(step.TargetGeo),
		}
		switch step.StepType {
		case "inbound", "match":
			publicStep.Label = publicInboundLabel("", "", step.Port)
			publicStep.Detail = publicProtocolDetail(step.Protocol, step.Port)
		case "outbound", "balancer":
			publicStep.Label = publicOutboundLabel(step.OutboundTag)
			publicStep.Detail = publicProtocolDetail(step.Protocol, step.Port)
			publicStep.OutboundTag = publicOutboundLabel(step.OutboundTag)
		default:
			publicStep.Label = "公开链路"
		}
		result.Steps = append(result.Steps, publicStep)
	}
	return result
}

func publicAgentTitle(agent model.DashboardAgentView, ordinal int) string {
	return firstNonEmptyString(agent.CustomerDisplayName, fmt.Sprintf("拓扑节点 %02d", ordinal))
}

func publicTopologyAgentID(agentID string, idMap map[string]string) string {
	if id := idMap[agentID]; id != "" {
		return id
	}
	return "node-00"
}

func publicTopologyAgentTitle(agentID string, publicID string, titleMap map[string]string) string {
	if title := titleMap[agentID]; title != "" {
		return title
	}
	if publicID == "node-00" {
		return "拓扑节点"
	}
	return "拓扑节点 " + publicID[len("node-"):]
}

func publicOutboundLabel(value string) string {
	if value == "" {
		return "出站规则"
	}
	return "出站规则"
}

func publicInboundLabel(tag, name string, id int) string {
	if id > 0 {
		return fmt.Sprintf("入站节点 %d", id)
	}
	return "入站节点"
}

func publicProtocolDetail(protocol string, port int) string {
	if protocol == "" && port <= 0 {
		return "公开链路"
	}
	if port <= 0 {
		return protocol
	}
	if protocol == "" {
		return fmt.Sprintf("端口 %d", port)
	}
	return fmt.Sprintf("%s · 端口 %d", protocol, port)
}

func cloneIPGeo(geo *model.IPGeoView) *model.IPGeoView {
	if geo == nil {
		return nil
	}
	cloned := *geo
	cloned.IP = ""
	return &cloned
}
