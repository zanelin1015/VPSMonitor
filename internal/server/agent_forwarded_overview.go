package server

import (
	"fmt"
	"sort"
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func emptyAgentXUIOverview(snapshot model.AgentSnapshot, cfg model.ManagedAgentConfig) *model.XUIOverview {
	return &model.XUIOverview{
		AgentID:     snapshot.AgentID,
		AgentName:   firstNonEmptyString(snapshot.AgentName, cfg.AgentName),
		ReportedAt:  snapshot.ReportedAt,
		CollectedAt: snapshot.ReportedAt,
		Summary:     snapshot.Summary,
	}
}

func (a *App) appendForwardedImportURLs(agentID string, overview *model.XUIOverview) {
	if overview == nil {
		return
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return
	}
	snapshots := a.store.ListLatest()
	appendForwardedImportURLsWithContext(agentID, overview, buildForwardedOverviewContext(agents, snapshots))
}

type forwardedOverviewContext struct {
	agentMap              map[string]model.DashboardAgentView
	targetOverviewByAgent map[string]*model.XUIOverview
	templateURLs          map[string]map[string]string
	xrayVersions          map[string]string
}

func buildForwardedOverviewContext(agents []model.AgentRecord, snapshots []model.AgentSnapshot) forwardedOverviewContext {
	agentMap := buildRealmForwardAgentMap(agents, snapshots)
	snapshotMap := make(map[string]model.AgentSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotMap[snapshot.AgentID] = snapshot
	}
	entryByAgent := make(map[string]model.AgentEntryConfig, len(agents))
	for agentID, agent := range agentMap {
		entryByAgent[agentID] = agent.Entry
	}
	targetOverviewByAgent := make(map[string]*model.XUIOverview, len(snapshotMap))
	templateURLs := make(map[string]map[string]string, len(snapshotMap))
	xrayVersions := make(map[string]string, len(snapshotMap))
	for targetAgentID, targetSnapshot := range snapshotMap {
		targetOverviewByAgent[targetAgentID] = dashboard.BuildXUIOverviewWithOptions(targetSnapshot, dashboard.XUIOverviewOptions{Entry: entryByAgent[targetAgentID]})
		if targetSnapshot.XUI != nil {
			xrayVersions[targetAgentID] = targetSnapshot.XUI.ServerStatus.Xray.Version
		}
		if target := targetOverviewByAgent[targetAgentID]; target != nil {
			for _, client := range target.Clients {
				if client.ImportURL == "" {
					templateURLs[targetAgentID] = customerTemplateImportURLs(targetSnapshot)
					break
				}
			}
		}
	}
	return forwardedOverviewContext{
		agentMap:              agentMap,
		targetOverviewByAgent: targetOverviewByAgent,
		templateURLs:          templateURLs,
		xrayVersions:          xrayVersions,
	}
}

func cloneForwardedBaseOverview(overview *model.XUIOverview) *model.XUIOverview {
	if overview == nil {
		return nil
	}
	cloned := *overview
	cloned.Nodes = append([]model.XUINodeView(nil), overview.Nodes...)
	cloned.Clients = append([]model.XUIClientView(nil), overview.Clients...)
	return &cloned
}

type forwardedXUIEntry struct {
	RuleID      string
	ForwardType string
	SourceAgent model.DashboardAgentView
	ListenPort  int
	Listen      string
	Network     string
	Enabled     bool
	Resolution  realmForwardResolution
	InboundTag  func(model.XUIClientView) string
	InboundName func(model.XUIClientView) string
	RouteNote   string
}

func appendForwardedImportURLsWithContext(agentID string, overview *model.XUIOverview, context forwardedOverviewContext) {
	if overview == nil {
		return
	}
	sourceAgent, ok := context.agentMap[agentID]
	if !ok {
		return
	}
	added := 0
	forwardedNodes := make(map[string]int)
	for index, node := range overview.Nodes {
		forwardedNodes[overviewInboundKey(node.ID, node.Tag)] = index
	}
	realmRules := sourceAgent.Entry.PortForwarding.Rules
	if !realmForwardingActive(sourceAgent.Entry.PortForwarding) {
		realmRules = nil
	}
	for _, rule := range realmRules {
		if !rule.Enabled || rule.ListenPort <= 0 || rule.TargetPort <= 0 {
			continue
		}
		resolution := resolveRealmForwardTarget(agentID, rule, context.agentMap, context.targetOverviewByAgent)
		if !resolution.Resolved || resolution.FinalAgentID == "" {
			continue
		}
		host := customerRealmSourceHost(sourceAgent, rule)
		if host == "" {
			continue
		}
		added += appendResolvedForwardedClients(overview, context, forwardedNodes, forwardedXUIEntry{
			ForwardType: "realm",
			RuleID:      rule.ID,
			SourceAgent: sourceAgent,
			ListenPort:  rule.ListenPort,
			Listen:      rule.ListenAddress,
			Network:     rule.Network,
			Enabled:     rule.Enabled,
			Resolution:  resolution,
			InboundTag: func(client model.XUIClientView) string {
				return realmForwardedInboundTag(rule, client)
			},
			InboundName: func(client model.XUIClientView) string {
				return realmForwardedInboundRemark(sourceAgent, context.agentMap[resolution.FinalAgentID], rule, client)
			},
			RouteNote: realmForwardResolutionNote(host, rule.ListenPort, resolution, context.agentMap),
		})
	}
	for _, rule := range sourceAgent.Entry.HAProxy.Rules {
		if !sourceAgent.Entry.HAProxy.Enabled || !rule.Enabled || rule.ListenPort <= 0 {
			continue
		}
		paths, err := resolveHAProxyRulePaths(rule, context)
		if err != nil || validateHAProxyResolvedPaths(rule, paths) != nil || len(paths) == 0 {
			continue
		}
		host := customerHAProxySourceHost(sourceAgent, rule)
		if host == "" {
			continue
		}
		resolution := paths[0].Resolution
		added += appendResolvedForwardedClients(overview, context, forwardedNodes, forwardedXUIEntry{
			ForwardType: "haproxy",
			RuleID:      rule.ID,
			SourceAgent: sourceAgent,
			ListenPort:  rule.ListenPort,
			Listen:      rule.ListenAddress,
			Network:     "tcp",
			Enabled:     rule.Enabled,
			Resolution:  resolution,
			InboundTag: func(client model.XUIClientView) string {
				return haProxyForwardedInboundTag(rule, client)
			},
			InboundName: func(client model.XUIClientView) string {
				return haProxyForwardedInboundRemark(sourceAgent, context.agentMap[resolution.FinalAgentID], rule, client)
			},
			RouteNote: haProxyForwardResolutionNote(host, rule.ListenPort, resolution, context.agentMap),
		})
	}
	if added > 0 {
		sortXUIClients(overview.Clients)
		sort.SliceStable(overview.Nodes, func(i, j int) bool {
			if overview.Nodes[i].Port != overview.Nodes[j].Port {
				return overview.Nodes[i].Port < overview.Nodes[j].Port
			}
			return overview.Nodes[i].Tag < overview.Nodes[j].Tag
		})
		overview.NodeCount = len(overview.Nodes)
		overview.ClientCount = len(overview.Clients)
		overview.OnlineClientCount = countOnlineXUIClients(overview.Clients)
	}
}

func appendResolvedForwardedClients(overview *model.XUIOverview, context forwardedOverviewContext, nodeIndexes map[string]int, entry forwardedXUIEntry) int {
	targetAgentID := entry.Resolution.FinalAgentID
	targetOverview := context.targetOverviewByAgent[targetAgentID]
	if targetOverview == nil || entry.ListenPort <= 0 {
		return 0
	}
	endpoint := model.ResolveEntryEndpoint(entry.SourceAgent.Entry, model.EntryTarget{Type: entry.ForwardType, ID: entry.RuleID, Port: entry.ListenPort, Transport: forwardEntryTransport(entry.Network)}, customerForwardSourceHost(entry.SourceAgent, entry.Listen))
	added := 0
	for _, client := range targetOverview.Clients {
		if !realmClientMatchesNode(client, entry.Resolution.FinalNode) {
			continue
		}
		sourceURL := firstNonEmptyString(client.ImportURL, context.templateURLs[targetAgentID][customerAssignmentKey(targetAgentID, client.InboundID, client.Email)])
		sourceClient := client
		sourceClient.ExportEndpoint = &endpoint
		sourceClient.ImportURL = ""
		sourceClient.ShadowrocketImportURL = ""
		if endpoint.Error == "" {
			sourceClient.ImportURL = rewriteCustomerImportURL(sourceURL, endpoint.Address, endpoint.Port)
			shadowURL := firstNonEmptyString(client.ShadowrocketImportURL, model.ShadowrocketImportURL(sourceURL, context.xrayVersions[targetAgentID]))
			sourceClient.ShadowrocketImportURL = rewriteCustomerImportURL(shadowURL, endpoint.Address, endpoint.Port)
		}
		sourceClient.InboundID = entry.ListenPort
		sourceClient.InboundTag = entry.InboundTag(client)
		sourceClient.InboundRemark = entry.InboundName(client)
		sourceClient.ForwardType = entry.ForwardType
		sourceClient.IsRealmForwarded = entry.ForwardType == "realm"
		sourceClient.RealmListenTag = sourceClient.InboundTag
		sourceClient.RealmSourceAgentID = entry.SourceAgent.AgentID
		sourceClient.RealmTargetAgentID = targetAgentID
		sourceClient.RealmTargetAgentName = context.agentMap[targetAgentID].AgentName
		sourceClient.RealmTargetInboundID = client.InboundID
		sourceClient.RealmTargetInboundTag = client.InboundTag
		sourceClient.RealmListenPort = entry.ListenPort
		sourceClient.Route.Note = entry.RouteNote
		overview.Clients = append(overview.Clients, sourceClient)

		nodeKey := overviewInboundKey(entry.ListenPort, sourceClient.InboundTag)
		if nodeIndex, exists := nodeIndexes[nodeKey]; exists {
			overview.Nodes[nodeIndex].ClientCount++
		} else {
			overview.Nodes = append(overview.Nodes, forwardedNodeView(sourceClient, entry))
			nodeIndexes[nodeKey] = len(overview.Nodes) - 1
		}
		added++
	}
	return added
}

func buildRealmForwardAgentMap(agents []model.AgentRecord, snapshots []model.AgentSnapshot) map[string]model.DashboardAgentView {
	realmByAgent := make(map[string]*model.RealmSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.Realm != nil {
			realmByAgent[snapshot.AgentID] = snapshot.Realm
		}
	}
	agentMap := make(map[string]model.DashboardAgentView, len(agents))
	for _, agent := range agents {
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
		agentMap[agent.AgentID] = model.DashboardAgentView{
			AgentID:             agent.AgentID,
			AgentName:           agent.AgentName,
			CustomerDisplayName: agent.CustomerDisplayName,
			Tags:                append([]string(nil), agent.Tags...),
			Entry:               dashboard.MergeRealmSnapshotIntoEntry(agent.Config.Entry, realmByAgent[agent.AgentID]),
			Summary:             summary,
		}
	}
	return agentMap
}

func overviewInboundKey(inboundID int, inboundTag string) string {
	return fmt.Sprintf("%d\x00%s", inboundID, inboundTag)
}

func findRealmTargetAgentID(rule model.RealmForwardRule, agentMap map[string]model.DashboardAgentView) string {
	targetAgentID := strings.TrimSpace(rule.TargetAgentID)
	if targetAgentID != "" {
		if _, ok := agentMap[targetAgentID]; ok {
			return targetAgentID
		}
		for agentID := range agentMap {
			if strings.EqualFold(agentID, targetAgentID) {
				return agentID
			}
		}
		return ""
	}
	var mappedMatches, hostMatches []string
	for _, agent := range agentMap {
		for _, mapping := range agent.Entry.Mappings {
			if !mapping.UseForExport {
				continue
			}
			single := agent.Entry
			single.Mappings = []model.AgentEntryMapping{mapping}
			endpoint := model.ResolveEntryEndpoint(single, model.EntryTarget{Type: mapping.TargetType, ID: mapping.TargetID, Port: mapping.InternalPort, Transport: mapping.Transport}, "")
			if endpoint.Error == "" && endpoint.Port == rule.TargetPort && strings.EqualFold(normalizeCustomerShareHost(endpoint.Address), normalizeCustomerShareHost(rule.TargetAddress)) {
				mappedMatches = append(mappedMatches, agent.AgentID)
				break
			}
		}
		if customerRealmRuleTargetsAgent(rule, agent.AgentID, customerAgentAddressSet(agent)) {
			hostMatches = append(hostMatches, agent.AgentID)
		}
	}
	if len(mappedMatches) == 1 {
		return mappedMatches[0]
	}
	if len(mappedMatches) == 0 && len(hostMatches) == 1 {
		return hostMatches[0]
	}
	return ""
}

func realmForwardedInboundTag(rule model.RealmForwardRule, client model.XUIClientView) string {
	if name := strings.TrimSpace(rule.Name); name != "" {
		return name
	}
	target := firstNonEmptyString(client.InboundRemark, client.InboundTag, fmt.Sprintf(":%d", rule.TargetPort))
	return fmt.Sprintf("Realm %d -> %s", rule.ListenPort, target)
}

func forwardedNodeView(client model.XUIClientView, entry forwardedXUIEntry) model.XUINodeView {
	route := client.Route
	route.MatchScope = firstNonEmptyString(entry.ForwardType, route.MatchScope)
	route.Note = firstNonEmptyString(entry.RouteNote, route.Note)
	return model.XUINodeView{
		ExportEndpoint:        client.ExportEndpoint,
		ID:                    entry.ListenPort,
		Tag:                   client.InboundTag,
		Remark:                firstNonEmptyString(client.InboundRemark, client.InboundTag),
		Protocol:              client.Protocol,
		Listen:                firstNonEmptyString(entry.Listen, "0.0.0.0"),
		Port:                  entry.ListenPort,
		Network:               entry.Network,
		Enabled:               entry.Enabled,
		ClientCount:           1,
		Route:                 route,
		RealmTargetAgentID:    client.RealmTargetAgentID,
		RealmTargetAgentName:  client.RealmTargetAgentName,
		RealmTargetInboundID:  client.RealmTargetInboundID,
		RealmTargetInboundTag: client.RealmTargetInboundTag,
	}
}

func haProxyForwardedInboundTag(rule model.HAProxyRule, client model.XUIClientView) string {
	if name := strings.TrimSpace(rule.Name); name != "" {
		return name
	}
	target := firstNonEmptyString(client.InboundRemark, client.InboundTag, fmt.Sprintf(":%d", rule.ListenPort))
	return fmt.Sprintf("HAProxy %d -> %s", rule.ListenPort, target)
}

func haProxyForwardedInboundRemark(sourceAgent, targetAgent model.DashboardAgentView, rule model.HAProxyRule, client model.XUIClientView) string {
	name := strings.TrimSpace(rule.Name)
	if name == "" {
		name = fmt.Sprintf("%s:%d -> %s:%d",
			firstNonEmptyString(sourceAgent.AgentName, sourceAgent.AgentID),
			rule.ListenPort,
			firstNonEmptyString(targetAgent.AgentName, targetAgent.AgentID),
			client.InboundID,
		)
	}
	if client.InboundRemark != "" || client.InboundTag != "" {
		name = fmt.Sprintf("%s / %s", name, firstNonEmptyString(client.InboundRemark, client.InboundTag))
	}
	return name
}

func realmForwardedInboundRemark(sourceAgent model.DashboardAgentView, targetAgent model.DashboardAgentView, rule model.RealmForwardRule, client model.XUIClientView) string {
	name := strings.TrimSpace(rule.Name)
	if name == "" {
		name = fmt.Sprintf("%s:%d -> %s:%d",
			firstNonEmptyString(sourceAgent.AgentName, sourceAgent.AgentID),
			rule.ListenPort,
			firstNonEmptyString(targetAgent.AgentName, targetAgent.AgentID),
			rule.TargetPort,
		)
	}
	if client.InboundRemark != "" || client.InboundTag != "" {
		name = fmt.Sprintf("%s / %s", name, firstNonEmptyString(client.InboundRemark, client.InboundTag))
	}
	return name
}

func sortXUIClients(clients []model.XUIClientView) {
	sort.SliceStable(clients, func(i, j int) bool {
		if clients[i].InboundTag != clients[j].InboundTag {
			return clients[i].InboundTag < clients[j].InboundTag
		}
		return clients[i].Email < clients[j].Email
	})
}

func countOnlineXUIClients(clients []model.XUIClientView) int {
	total := 0
	for _, client := range clients {
		if client.LastOnline > 0 {
			total++
		}
	}
	return total
}
