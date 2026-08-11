package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func (a *App) requireAgentAdmin(w http.ResponseWriter, r *http.Request, agentID string) (model.AdminUser, string, bool) {
	user, token, ok := a.requireAdmin(w, r)
	if !ok {
		return model.AdminUser{}, "", false
	}
	if !a.adminCanAccessAgent(user, agentID) {
		writeError(w, http.StatusForbidden, "agent is not assigned to this account")
		return model.AdminUser{}, "", false
	}
	return user, token, true
}

func (a *App) adminCanAccessAgent(user model.AdminUser, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	if isRootAdmin(user) {
		return true
	}
	for _, allowed := range user.AgentIDs {
		if allowed == agentID {
			return true
		}
	}
	if isAreaManager(user) && user.ID > 0 {
		if _, ok := a.areaManagerClientScope(user).agents[agentID]; ok {
			return true
		}
	}
	return false
}

func (a *App) filterAgentRecordsForAdmin(user model.AdminUser, agents []model.AgentRecord) []model.AgentRecord {
	if isRootAdmin(user) {
		return agents
	}
	allowed := a.adminVisibleAgentSet(user)
	filtered := make([]model.AgentRecord, 0, len(agents))
	for _, agent := range agents {
		if _, ok := allowed[agent.AgentID]; ok {
			filtered = append(filtered, agent)
		}
	}
	return filtered
}

func (a *App) filterSnapshotsForAdmin(user model.AdminUser, snapshots []model.AgentSnapshot) []model.AgentSnapshot {
	if isRootAdmin(user) {
		return snapshots
	}
	allowed := a.adminVisibleAgentSet(user)
	filtered := make([]model.AgentSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if _, ok := allowed[snapshot.AgentID]; ok {
			filtered = append(filtered, snapshot)
		}
	}
	return filtered
}

func (a *App) filterRealtimeMetricsForAdmin(user model.AdminUser, metrics []model.AgentRealtimeMetrics) []model.AgentRealtimeMetrics {
	if isRootAdmin(user) {
		filtered := make([]model.AgentRealtimeMetrics, 0, len(metrics))
		for _, metric := range metrics {
			filtered = append(filtered, sanitizeRealtimeMetricForBrowser(metric))
		}
		return filtered
	}
	allowed := a.adminVisibleAgentSet(user)
	filtered := make([]model.AgentRealtimeMetrics, 0, len(metrics))
	for _, metric := range metrics {
		if _, ok := allowed[metric.AgentID]; ok {
			filtered = append(filtered, sanitizeRealtimeMetricForAreaManager(metric))
		}
	}
	return filtered
}

func (a *App) sanitizeRealtimeMetricForAdmin(user model.AdminUser, metric model.AgentRealtimeMetrics) model.AgentRealtimeMetrics {
	if isRootAdmin(user) {
		return sanitizeRealtimeMetricForBrowser(metric)
	}
	return sanitizeRealtimeMetricForAreaManager(metric)
}

func (a *App) sanitizeManagedConfigForAdmin(user model.AdminUser, cfg model.ManagedAgentConfig) model.ManagedAgentConfig {
	if isRootAdmin(user) {
		return cfg
	}
	cfg.AgentName = areaManagerDisplayName(cfg.CustomerDisplayName, cfg.AgentName, cfg.AgentID)
	cfg.Tags = a.areaManagerTagsForAgent(user, cfg.AgentID)
	cfg.Renewal = sanitizeAreaManagerRenewal(cfg.Renewal)
	cfg.Entry = model.AgentEntryConfig{}
	cfg.XUI = config.XUIConfig{
		Enabled: cfg.XUI.Enabled,
	}
	return cfg
}

func (a *App) sanitizeAgentRecordForAdmin(user model.AdminUser, agent model.AgentRecord) model.AgentRecord {
	if isRootAdmin(user) {
		return agent
	}
	tags := a.areaManagerTagsForAgent(user, agent.AgentID)
	agent.AgentName = areaManagerDisplayName(agent.CustomerDisplayName, agent.AgentName, agent.AgentID)
	agent.Tags = cloneStringSlice(tags)
	agent.Version = ""
	agent.OS = ""
	agent.Arch = ""
	agent.SystemVersion = ""
	agent.Hostname = ""
	agent.PublicIPv4 = ""
	agent.PublicIPv6 = ""
	agent.Summary = sanitizeAreaManagerSummary(agent.Summary)
	agent.Config = a.sanitizeManagedConfigForAdmin(user, agent.Config)
	agent.HasConfig = false
	return agent
}

func (a *App) sanitizeAgentListItemsForAdmin(user model.AdminUser, items []model.AgentListItem) []model.AgentListItem {
	if isRootAdmin(user) {
		return items
	}
	tagMap, _ := a.store.ListAreaManagerAgentTags(user.ID)
	for index := range items {
		items[index] = sanitizeAgentListItemForAreaManager(items[index], tagMap)
	}
	return items
}

func (a *App) sanitizeDashboardForAdmin(user model.AdminUser, view *model.GlobalDashboardView) {
	if view == nil || isRootAdmin(user) {
		return
	}
	tagMap, _ := a.store.ListAreaManagerAgentTags(user.ID)
	clientScope := a.areaManagerClientScope(user)
	allowed := cloneAgentSet(clientScope.agents)
	expandAreaManagerForwardingPathAgents(allowed, view.Links, clientScope)
	agentNames := make(map[string]string, len(view.Agents))
	filteredAgents := make([]model.DashboardAgentView, 0, len(clientScope.agents))
	for _, agent := range view.Agents {
		if _, directlyAssigned := clientScope.agents[agent.AgentID]; !directlyAssigned {
			agentNames[agent.AgentID] = firstNonEmptyString(agent.CustomerDisplayName, "转发节点")
			continue
		}
		publicAgent := sanitizeDashboardAgentForAreaManager(agent, tagMap)
		agentNames[agent.AgentID] = publicAgent.AgentName
		filteredAgents = append(filteredAgents, publicAgent)
	}
	view.Agents = filteredAgents
	view.Links = sanitizeTopologyLinksForAreaManager(view.Links, allowed, tagMap, agentNames)
	view.ClientChains = sanitizeClientChainsForAreaManager(view.ClientChains, allowed, tagMap, agentNames, clientScope)
	view.Links = filterTopologyLinksVisibleToAreaManager(view.Links, view.ClientChains, clientScope)
	applyAreaManagerClientCounts(view, clientScope)
	a.applyAreaManagerDashboardTraffic(user, view, clientScope)
	rebuildAreaManagerDashboardStats(view)
}

func (a *App) sanitizeXUIOverviewForAdmin(user model.AdminUser, overview *model.XUIOverview) {
	if overview == nil || isRootAdmin(user) {
		return
	}
	a.sanitizeRealmTargetNamesForAreaManager(overview)
	overview.AgentName = areaManagerDisplayName("", overview.AgentName, overview.AgentID)
	overview.BaseURL = ""
	overview.Summary = sanitizeAreaManagerSummary(overview.Summary)
	clientScope := a.areaManagerClientScope(user)
	filteredClients := make([]model.XUIClientView, 0, len(overview.Clients))
	for _, client := range overview.Clients {
		if !clientScope.allowsClient(overview.AgentID, client.InboundID, client.InboundTag, client.Email) &&
			!a.areaManagerCanViewForwardedClient(user, overview.AgentID, client, clientScope) {
			continue
		}
		client.TotalGB = 0
		client.ExpiryTime = 0
		client.CreatedAt = 0
		client.UpdatedAt = 0
		client.LastOnline = 0
		filteredClients = append(filteredClients, client)
	}
	overview.Clients = filteredClients
	overview.ClientCount = len(filteredClients)
	overview.OnlineClientCount = 0
	visibleNodes := make(map[int]struct{})
	for _, client := range filteredClients {
		visibleNodes[client.InboundID] = struct{}{}
	}
	filteredNodes := make([]model.XUINodeView, 0, len(overview.Nodes))
	for _, node := range overview.Nodes {
		if !clientScope.allowsInbound(overview.AgentID, node.ID, node.Tag) {
			if _, ok := visibleNodes[node.ID]; !ok {
				continue
			}
		}
		node.ClientCount = 0
		node.OnlineCount = 0
		filteredNodes = append(filteredNodes, node)
	}
	overview.Nodes = filteredNodes
	overview.NodeCount = len(filteredNodes)
	applyScopedClientTrafficToNodes(overview.AgentID, overview.Nodes, filteredClients, clientScope)
	overview.Summary = a.areaManagerScopedTrafficSummary(user, overview)
	outboundScope := a.areaManagerOutboundScope(user)
	overview.Outbounds = filterOutboundsForAreaManager(overview.Outbounds, overview.AgentID, outboundScope)
	applyScopedClientTrafficToOutbounds(overview.Outbounds, filteredClients)
	overview.Balancers = nil
	overview.RoutingRules = filterRoutingRulesForAreaManager(overview.RoutingRules, overview.AgentID, filteredClients, filteredNodes, clientScope, outboundScope)
	for index := range overview.Outbounds {
		overview.Outbounds[index].SendThrough = ""
	}
}

func filterOutboundsForAreaManager(outbounds []model.XUIOutboundView, agentID string, scope areaManagerOutboundScope) []model.XUIOutboundView {
	filtered := make([]model.XUIOutboundView, 0, len(outbounds))
	for _, outbound := range outbounds {
		if scope.allows(agentID, outbound.Tag) {
			filtered = append(filtered, outbound)
		}
	}
	return filtered
}

func filterRoutingRulesForAreaManager(rules []model.XUIRoutingRuleView, agentID string, visibleClients []model.XUIClientView, visibleNodes []model.XUINodeView, clientScope areaManagerClientScope, outboundScope areaManagerOutboundScope) []model.XUIRoutingRuleView {
	if len(rules) == 0 {
		return rules
	}
	allowedUsers := make(map[string]struct{})
	visibleInboundTags := make(map[string]struct{})
	for _, client := range visibleClients {
		if email := strings.ToLower(strings.TrimSpace(client.Email)); email != "" {
			allowedUsers[email] = struct{}{}
		}
		if tag := strings.TrimSpace(client.InboundTag); tag != "" {
			visibleInboundTags[strings.ToLower(tag)] = struct{}{}
		}
	}
	for _, node := range visibleNodes {
		if !clientScope.allowsInbound(agentID, node.ID, node.Tag) && !clientScope.hasExactClientOnInbound(agentID, node.ID, node.Tag) {
			continue
		}
		if tag := strings.TrimSpace(node.Tag); tag != "" {
			visibleInboundTags[strings.ToLower(tag)] = struct{}{}
		}
	}

	filtered := make([]model.XUIRoutingRuleView, 0, len(rules))
	for _, rule := range rules {
		next := rule
		if next.BalancerTag != "" || !outboundScope.allows(agentID, next.OutboundTag) {
			continue
		}
		if len(next.Users) > 0 {
			next.Users = filterStringsBySet(next.Users, allowedUsers)
			if len(next.Users) == 0 {
				continue
			}
		}
		if len(next.InboundTags) > 0 {
			next.InboundTags = filterStringsBySet(next.InboundTags, visibleInboundTags)
			if len(next.InboundTags) == 0 {
				continue
			}
		}
		// Rebuild the frontend summary from sanitized fields to avoid leaking hidden users.
		next.Summary = ""
		filtered = append(filtered, next)
	}
	return filtered
}

func filterStringsBySet(values []string, allowed map[string]struct{}) []string {
	if len(values) == 0 {
		return values
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(value))]; ok {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func (a *App) areaManagerCanViewForwardedClient(user model.AdminUser, sourceAgentID string, client model.XUIClientView, clientScope areaManagerClientScope) bool {
	if client.RealmSourceAgentID == "" || !strings.EqualFold(client.RealmSourceAgentID, sourceAgentID) {
		return false
	}
	if !a.adminCanAccessAgent(user, sourceAgentID) {
		return false
	}
	listenPort := client.RealmListenPort
	if listenPort <= 0 {
		listenPort = client.InboundID
	}
	if listenPort <= 0 || client.RealmTargetAgentID == "" || client.RealmTargetInboundID <= 0 {
		return false
	}
	forwardType := strings.ToLower(strings.TrimSpace(client.ForwardType))
	if forwardType == "" && (client.IsRealmForwarded || client.RealmSourceAgentID != "") {
		forwardType = "realm"
	}
	if !clientScope.allowsForwardingPort(sourceAgentID, listenPort, forwardType) {
		return false
	}
	return clientScope.allowsClient(client.RealmTargetAgentID, client.RealmTargetInboundID, client.RealmTargetInboundTag, client.Email)
}

func (a *App) sanitizeXUIOverviewForAreaAssignment(user model.AdminUser, overview *model.XUIOverview) {
	if overview == nil || isRootAdmin(user) {
		return
	}
	a.sanitizeRealmTargetNamesForAreaManager(overview)
	overview.AgentName = areaManagerDisplayName("", overview.AgentName, overview.AgentID)
	overview.BaseURL = ""
	overview.Summary = sanitizeAreaManagerSummary(overview.Summary)
	clientScope := a.areaManagerAssignmentClientScope(user, overview.AgentID)
	filteredClients := make([]model.XUIClientView, 0, len(overview.Clients))
	visibleInbounds := make(map[string]struct{})
	for _, client := range overview.Clients {
		if !clientScope.allowsClient(overview.AgentID, client.InboundID, client.InboundTag, client.Email) &&
			!a.areaManagerCanViewForwardedClient(user, overview.AgentID, client, clientScope) {
			continue
		}
		client.TotalGB = 0
		client.ExpiryTime = 0
		client.CreatedAt = 0
		client.UpdatedAt = 0
		client.LastOnline = 0
		filteredClients = append(filteredClients, client)
		visibleInbounds[overviewInboundKey(client.InboundID, client.InboundTag)] = struct{}{}
	}
	overview.Clients = filteredClients
	overview.ClientCount = len(filteredClients)
	overview.OnlineClientCount = 0
	filteredNodes := make([]model.XUINodeView, 0, len(overview.Nodes))
	for _, node := range overview.Nodes {
		canAssignAllClients := clientScope.allowsInbound(overview.AgentID, node.ID, node.Tag)
		if _, visible := visibleInbounds[overviewInboundKey(node.ID, node.Tag)]; !canAssignAllClients && !visible {
			continue
		}
		node.CanAssignAllClients = &canAssignAllClients
		node.ClientCount = 0
		node.OnlineCount = 0
		node.Up = 0
		node.Down = 0
		node.Total = 0
		node.AllTime = 0
		filteredNodes = append(filteredNodes, node)
	}
	overview.Nodes = filteredNodes
	overview.NodeCount = len(filteredNodes)
	outboundScope := a.areaManagerOutboundScope(user)
	overview.Outbounds = filterOutboundsForAreaManager(overview.Outbounds, overview.AgentID, outboundScope)
	applyScopedClientTrafficToOutbounds(overview.Outbounds, filteredClients)
	overview.Balancers = nil
	overview.RoutingRules = filterRoutingRulesForAreaManager(overview.RoutingRules, overview.AgentID, filteredClients, filteredNodes, clientScope, outboundScope)
	for index := range overview.Outbounds {
		overview.Outbounds[index].SendThrough = ""
	}
}

func (a *App) sanitizeRealmTargetNamesForAreaManager(overview *model.XUIOverview) {
	if overview == nil || a == nil || a.store == nil {
		return
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return
	}
	displayNames := make(map[string]string, len(agents))
	for _, agent := range agents {
		displayNames[agent.AgentID] = firstNonEmptyString(agent.CustomerDisplayName, agent.AgentID)
	}
	publicName := func(agentID string) string {
		agentID = strings.TrimSpace(agentID)
		if name := strings.TrimSpace(displayNames[agentID]); name != "" {
			return name
		}
		return agentID
	}
	forwardedLabel := func(listenPort int, targetAgentID string, targetInboundID int) string {
		sourceName := firstNonEmptyString(publicName(overview.AgentID), "当前 Client")
		targetName := firstNonEmptyString(publicName(targetAgentID), "目标 Client")
		source := sourceName
		if listenPort > 0 {
			source = fmt.Sprintf("%s:%d", source, listenPort)
		}
		target := targetName
		if targetInboundID > 0 {
			target = fmt.Sprintf("%s:%d", target, targetInboundID)
		}
		return source + " -> " + target
	}
	for index := range overview.Nodes {
		node := &overview.Nodes[index]
		if node.RealmTargetAgentID != "" || node.RealmTargetAgentName != "" {
			node.RealmTargetAgentName = publicName(node.RealmTargetAgentID)
			node.Remark = forwardedLabel(node.Port, node.RealmTargetAgentID, node.RealmTargetInboundID)
			node.Route.Note = ""
		}
	}
	for index := range overview.Clients {
		client := &overview.Clients[index]
		if client.RealmTargetAgentID != "" || client.RealmTargetAgentName != "" {
			client.RealmTargetAgentName = publicName(client.RealmTargetAgentID)
			client.InboundRemark = forwardedLabel(client.RealmListenPort, client.RealmTargetAgentID, client.RealmTargetInboundID)
			client.Route.Note = ""
		}
	}
}

func sanitizeAgentListItemForAreaManager(item model.AgentListItem, tagMap map[string][]string) model.AgentListItem {
	item.AgentName = areaManagerDisplayName(item.CustomerDisplayName, item.AgentName, item.AgentID)
	item.Tags = cloneStringSlice(tagMap[item.AgentID])
	item.ClientVersion = ""
	item.ClientOS = ""
	item.ClientArch = ""
	item.SystemVersion = ""
	item.Renewal = sanitizeAreaManagerRenewal(item.Renewal)
	item.Entry = model.AgentEntryConfig{}
	item.HasConfig = false
	item.Summary = sanitizeAreaManagerSummary(item.Summary)
	item.Realm = nil
	item.HAProxy = nil
	item.Geo = nil
	return item
}

func sanitizeDashboardAgentForAreaManager(agent model.DashboardAgentView, tagMap map[string][]string) model.DashboardAgentView {
	agent.LineEntry = isCNLineEntryDashboardAgent(agent)
	agent.AgentName = areaManagerDisplayName(agent.CustomerDisplayName, agent.AgentName, agent.AgentID)
	agent.Tags = cloneStringSlice(tagMap[agent.AgentID])
	agent.ClientVersion = ""
	agent.ClientOS = ""
	agent.ClientArch = ""
	agent.SystemVersion = ""
	agent.Renewal = sanitizeAreaManagerRenewal(agent.Renewal)
	agent.Entry = model.AgentEntryConfig{}
	agent.HasConfig = false
	agent.Summary = sanitizeAreaManagerSummary(agent.Summary)
	agent.Realm = nil
	agent.HAProxy = nil
	agent.NetworkPolicy = nil
	agent.Geo = nil
	agent.FinanceClients = []model.FinanceClientView{}
	agent.FinanceClientsReady = false
	return agent
}

func isCNLineEntryDashboardAgent(agent model.DashboardAgentView) bool {
	if dashboardAgentCountryCode(agent) != "CN" {
		return false
	}
	if realmForwardingActive(agent.Entry.PortForwarding) {
		for _, rule := range agent.Entry.PortForwarding.Rules {
			if rule.Enabled {
				return true
			}
		}
	}
	if agent.Entry.HAProxy.Enabled {
		for _, rule := range agent.Entry.HAProxy.Rules {
			if rule.Enabled {
				return true
			}
		}
	}
	for _, tag := range agent.Tags {
		normalized := strings.NewReplacer(" ", "", "\t", "", "_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(tag)))
		if strings.Contains(normalized, "国内入口") || normalized == "cn入口" || normalized == "cnentry" {
			return true
		}
	}
	return false
}

func dashboardAgentCountryCode(agent model.DashboardAgentView) string {
	candidates := append([]string{agent.AgentName}, agent.Tags...)
	candidates = append(candidates, agent.Summary.Hostname, agent.AgentID)
	for _, candidate := range candidates {
		if code := explicitDashboardCountryCode(candidate); code != "" {
			return code
		}
	}
	if agent.Geo != nil {
		return strings.ToUpper(strings.TrimSpace(agent.Geo.CountryCode))
	}
	return ""
}

func explicitDashboardCountryCode(value string) string {
	for _, token := range strings.FieldsFunc(strings.ToUpper(strings.TrimSpace(value)), func(char rune) bool {
		return !((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9'))
	}) {
		switch token {
		case "TH", "MY", "VN", "IN", "SG", "HK", "MO", "TW", "JP", "KR", "CA", "US", "CN", "PH", "DE", "FR", "GB", "AU":
			return token
		}
	}
	return ""
}

func sanitizeAreaManagerRenewal(cfg model.VPSRenewalConfig) model.VPSRenewalConfig {
	return model.VPSRenewalConfig{
		TrafficBaselineBytes:       cfg.TrafficBaselineBytes,
		TrafficSentBaselineBytes:   cfg.TrafficSentBaselineBytes,
		TrafficRecvBaselineBytes:   cfg.TrafficRecvBaselineBytes,
		TrafficBaselinePeriodStart: cfg.TrafficBaselinePeriodStart,
	}
}

func sanitizeAreaManagerSummary(summary model.VPSSummary) model.VPSSummary {
	return model.VPSSummary{
		DiskUsed:  summary.DiskUsed,
		DiskTotal: summary.DiskTotal,
	}
}

func sanitizeRealtimeMetricForAreaManager(metric model.AgentRealtimeMetrics) model.AgentRealtimeMetrics {
	metric.AgentName = ""
	metric.ClientVersion = ""
	metric.ClientOS = ""
	metric.ClientArch = ""
	metric.SystemVersion = ""
	metric.Summary = sanitizeAreaManagerSummary(metric.Summary)
	metric.HAProxy = nil
	metric.XUITraffic = nil
	return metric
}

func sanitizeRealtimeMetricForBrowser(metric model.AgentRealtimeMetrics) model.AgentRealtimeMetrics {
	metric.XUITraffic = nil
	return metric
}

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

type areaManagerClientScope struct {
	exactClients map[string]struct{}
	inbounds     map[string]struct{}
	realmPorts   map[string]struct{}
	haProxyPorts map[string]struct{}
	agents       map[string]struct{}
}

type areaManagerOutboundScope struct {
	tags map[string]struct{}
}

func (a *App) areaManagerOutboundScope(user model.AdminUser) areaManagerOutboundScope {
	scope := areaManagerOutboundScope{tags: make(map[string]struct{})}
	if !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	grants, err := a.store.ListAreaManagerOutboundGrants(user.ID)
	if err != nil {
		return scope
	}
	for _, grant := range grants {
		scope.tags[outboundGrantKey(grant.AgentID, grant.OutboundTag)] = struct{}{}
	}
	return scope
}

func (s areaManagerOutboundScope) allows(agentID, outboundTag string) bool {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(outboundTag) == "" {
		return false
	}
	_, ok := s.tags[outboundGrantKey(agentID, outboundTag)]
	return ok
}

func outboundGrantKey(agentID, outboundTag string) string {
	return strings.TrimSpace(agentID) + "\x00" + strings.TrimSpace(outboundTag)
}

func (a *App) areaManagerClientScope(user model.AdminUser) areaManagerClientScope {
	scope := a.areaManagerGrantedClientScope(user)
	if !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	customers, err := a.store.ListCustomersForOwner(model.AdminRoleAreaManager, user.ID)
	if err != nil {
		return scope
	}
	for _, customer := range customers {
		for _, assignment := range customer.Assignments {
			addAreaManagerScopeAssignment(&scope, assignment.AgentID, assignment.InboundID, assignment.InboundTag, assignment.ClientEmail, assignment.Enabled)
		}
	}
	return scope
}

func (a *App) areaManagerGrantedClientScope(user model.AdminUser) areaManagerClientScope {
	scope := areaManagerClientScope{
		exactClients: make(map[string]struct{}),
		inbounds:     make(map[string]struct{}),
		realmPorts:   make(map[string]struct{}),
		haProxyPorts: make(map[string]struct{}),
		agents:       adminAgentSet(user),
	}
	if !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	assignments, err := a.store.ListAreaManagerAssignments(user.ID)
	if err == nil {
		configCache := make(map[string]*model.ManagedAgentConfig)
		for _, assignment := range assignments {
			inboundTag := assignment.InboundTag
			if assignment.ClientEmail == "" {
				inboundTag = a.normalizeAreaManagerForwardingAssignmentTag(assignment.AgentID, assignment.InboundID, inboundTag, configCache)
			}
			addAreaManagerScopeAssignment(&scope, assignment.AgentID, assignment.InboundID, inboundTag, assignment.ClientEmail, assignment.Enabled)
		}
	}
	return scope
}

func (a *App) normalizeAreaManagerForwardingAssignmentTag(agentID string, listenPort int, inboundTag string, cache map[string]*model.ManagedAgentConfig) string {
	if a == nil || a.store == nil || listenPort <= 0 || (!isRealmAssignmentTag(inboundTag) && !isHAProxyAssignmentTag(inboundTag)) {
		return inboundTag
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return inboundTag
	}
	cfg, cached := cache[agentID]
	if !cached {
		loaded, found, err := a.store.GetAgentConfig(agentID)
		if err != nil || !found {
			cache[agentID] = nil
			return inboundTag
		}
		cfg = &loaded
		cache[agentID] = cfg
	}
	if cfg == nil {
		return inboundTag
	}
	if cfg.Entry.HAProxy.Enabled {
		for _, rule := range cfg.Entry.HAProxy.Rules {
			if rule.Enabled && rule.ListenPort == listenPort {
				return "haproxy:" + strconv.Itoa(listenPort)
			}
		}
	}
	if realmForwardingActive(cfg.Entry.PortForwarding) {
		for _, rule := range cfg.Entry.PortForwarding.Rules {
			if rule.Enabled && rule.ListenPort == listenPort {
				return "realm:" + strconv.Itoa(listenPort)
			}
		}
	}
	return inboundTag
}

func (a *App) areaManagerAssignmentClientScope(user model.AdminUser, agentID string) areaManagerClientScope {
	scope := a.areaManagerGrantedClientScope(user)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	actions, err := a.store.ListSucceededXUIActionsByActorKind(agentID, user.Role, user.ID, model.XUIActionAddClient)
	if err != nil {
		return scope
	}
	for _, action := range actions {
		inboundID, inboundTag, email, ok := xuiAddClientTarget(action.Payload)
		if !ok || !scope.allowsManageInbound(agentID, inboundID, inboundTag) {
			continue
		}
		addAreaManagerExactClient(&scope, agentID, inboundID, inboundTag, email)
	}
	return scope
}

func xuiAddClientTarget(payload map[string]any) (int, string, string, bool) {
	if payload == nil {
		return 0, "", "", false
	}
	inboundID := intFromAny(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromAny(payload["inbound_tag"]))
	client, ok := payload["client"].(map[string]any)
	if !ok || (inboundID <= 0 && inboundTag == "") {
		return 0, "", "", false
	}
	email := strings.TrimSpace(stringFromAny(client["email"]))
	return inboundID, inboundTag, email, email != ""
}

func addAreaManagerExactClient(scope *areaManagerClientScope, agentID string, inboundID int, inboundTag, email string) {
	if scope == nil || strings.TrimSpace(agentID) == "" || strings.TrimSpace(email) == "" {
		return
	}
	scope.agents[strings.TrimSpace(agentID)] = struct{}{}
	scope.exactClients[areaClientExactKey(agentID, inboundID, inboundTag, email)] = struct{}{}
	if strings.TrimSpace(inboundTag) != "" {
		scope.exactClients[areaClientExactKey(agentID, inboundID, "", email)] = struct{}{}
	}
}

func addAreaManagerScopeAssignment(scope *areaManagerClientScope, agentID string, inboundID int, inboundTag, clientEmail string, enabled bool) {
	if scope == nil || !enabled || agentID == "" {
		return
	}
	scope.agents[agentID] = struct{}{}
	if clientEmail != "" {
		scope.exactClients[areaClientExactKey(agentID, inboundID, inboundTag, clientEmail)] = struct{}{}
		if inboundTag != "" {
			scope.exactClients[areaClientExactKey(agentID, inboundID, "", clientEmail)] = struct{}{}
		}
		removeAreaManagerInboundGrant(scope, agentID, inboundID, inboundTag)
		return
	}
	if isRealmAssignmentTag(inboundTag) {
		scope.realmPorts[areaRealmPortKey(agentID, inboundID)] = struct{}{}
		return
	}
	if isHAProxyAssignmentTag(inboundTag) {
		scope.haProxyPorts[areaForwardingPortKey(agentID, inboundID)] = struct{}{}
		return
	}
	if scope.hasExactClientOnInbound(agentID, inboundID, inboundTag) {
		return
	}
	scope.inbounds[areaClientInboundKey(agentID, inboundID, inboundTag)] = struct{}{}
	if inboundTag != "" {
		scope.inbounds[areaClientInboundKey(agentID, inboundID, "")] = struct{}{}
	}
}

func removeAreaManagerInboundGrant(scope *areaManagerClientScope, agentID string, inboundID int, inboundTag string) {
	if scope == nil {
		return
	}
	delete(scope.inbounds, areaClientInboundKey(agentID, inboundID, inboundTag))
	if inboundTag != "" {
		delete(scope.inbounds, areaClientInboundKey(agentID, inboundID, ""))
		return
	}
	normalizedAgentID := strings.TrimSpace(agentID)
	normalizedInboundID := strconv.Itoa(inboundID)
	for key := range scope.inbounds {
		parts := strings.Split(key, "\x00")
		if len(parts) == 3 && strings.EqualFold(parts[0], normalizedAgentID) && parts[1] == normalizedInboundID {
			delete(scope.inbounds, key)
		}
	}
}

func (s areaManagerClientScope) allowsClient(agentID string, inboundID int, inboundTag, email string) bool {
	if agentID == "" {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	if email == "" {
		return s.allowsInbound(agentID, inboundID, inboundTag)
	}
	if _, ok := s.exactClients[areaClientExactKey(agentID, inboundID, inboundTag, email)]; ok {
		return true
	}
	if inboundTag != "" {
		if _, ok := s.exactClients[areaClientExactKey(agentID, inboundID, "", email)]; ok {
			return true
		}
	}
	return s.allowsInbound(agentID, inboundID, inboundTag)
}

func (s areaManagerClientScope) allowsInbound(agentID string, inboundID int, inboundTag string) bool {
	if agentID == "" {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	if _, ok := s.inbounds[areaClientInboundKey(agentID, inboundID, inboundTag)]; ok {
		return true
	}
	if inboundTag != "" {
		if _, ok := s.inbounds[areaClientInboundKey(agentID, inboundID, "")]; ok {
			return true
		}
	}
	return false
}

func (s areaManagerClientScope) allowsRealmPort(agentID string, listenPort int) bool {
	return s.allowsForwardingPort(agentID, listenPort, "realm")
}

func (s areaManagerClientScope) allowsForwardingPort(agentID string, listenPort int, forwardType string) bool {
	if agentID == "" || listenPort <= 0 {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	portKey := areaForwardingPortKey(agentID, listenPort)
	ports := s.realmPorts
	assignmentTagMatch := isRealmAssignmentTag
	if strings.EqualFold(strings.TrimSpace(forwardType), "haproxy") {
		ports = s.haProxyPorts
		assignmentTagMatch = isHAProxyAssignmentTag
	}
	if _, ok := ports[portKey]; ok {
		return true
	}
	for key := range s.inbounds {
		parts := strings.Split(key, "\x00")
		if len(parts) != 3 || !strings.EqualFold(parts[0], strings.TrimSpace(agentID)) || parts[1] != strconv.Itoa(listenPort) {
			continue
		}
		if assignmentTagMatch(parts[2]) {
			return true
		}
	}
	return false
}

func areaClientExactKey(agentID string, inboundID int, inboundTag, email string) string {
	return strings.Join([]string{
		strings.TrimSpace(agentID),
		strconv.Itoa(inboundID),
		strings.ToLower(strings.TrimSpace(inboundTag)),
		strings.ToLower(strings.TrimSpace(email)),
	}, "\x00")
}

func areaClientInboundKey(agentID string, inboundID int, inboundTag string) string {
	return strings.Join([]string{
		strings.TrimSpace(agentID),
		strconv.Itoa(inboundID),
		strings.ToLower(strings.TrimSpace(inboundTag)),
	}, "\x00")
}

func areaRealmPortKey(agentID string, listenPort int) string {
	return areaForwardingPortKey(agentID, listenPort)
}

func areaForwardingPortKey(agentID string, listenPort int) string {
	return strings.Join([]string{strings.TrimSpace(agentID), strconv.Itoa(listenPort)}, "\x00")
}

func isRealmAssignmentTag(tag string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(tag)), "realm:")
}

func isHAProxyAssignmentTag(tag string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(tag)), "haproxy:")
}

func filterTopologyLinksUsedByChains(links []model.TopologyLinkView, chains []model.ClientChainView) []model.TopologyLinkView {
	return filterTopologyLinksVisibleToAreaManager(links, chains, areaManagerClientScope{})
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

func (s areaManagerClientScope) hasExactClientOnInbound(agentID string, inboundID int, inboundTag string) bool {
	if agentID == "" || inboundID <= 0 {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	normalizedAgentID := strings.TrimSpace(agentID)
	normalizedInboundID := strconv.Itoa(inboundID)
	normalizedTag := strings.ToLower(strings.TrimSpace(inboundTag))
	for key := range s.exactClients {
		parts := strings.Split(key, "\x00")
		if len(parts) != 4 || !strings.EqualFold(parts[0], normalizedAgentID) || parts[1] != normalizedInboundID {
			continue
		}
		if parts[2] == normalizedTag || parts[2] == "" || normalizedTag == "" {
			return true
		}
	}
	return false
}

func (s areaManagerClientScope) allowsManageInbound(agentID string, inboundID int, inboundTag string) bool {
	return s.allowsInbound(agentID, inboundID, inboundTag) || s.hasExactClientOnInbound(agentID, inboundID, inboundTag)
}

func outboundLinkKey(agentID, outboundTag string) string {
	return strings.TrimSpace(agentID) + "::" + strings.TrimSpace(outboundTag)
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

func (a *App) areaManagerTagsForAgent(user model.AdminUser, agentID string) []string {
	tags, _, err := a.store.GetAreaManagerAgentTags(user.ID, agentID)
	if err != nil {
		return nil
	}
	return tags
}

func areaManagerDisplayName(customerDisplayName, agentName, agentID string) string {
	return firstNonEmptyString(customerDisplayName, agentName, agentID)
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func filterDomainLikeValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if redactEndpointIP(value) == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func redactEndpointIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	host := endpointHost(value)
	if host == "" {
		return value
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return ""
	}
	return value
}

func endpointHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return strings.Trim(host, "[]")
	}
	if strings.HasPrefix(value, "[") {
		if end := strings.Index(value, "]"); end > 0 {
			return value[1:end]
		}
	}
	if strings.Count(value, ":") == 1 {
		if host, _, ok := strings.Cut(value, ":"); ok {
			return host
		}
	}
	return strings.Trim(value, "[]")
}

func (a *App) customerVisibleToAdmin(user model.AdminUser, customerID int64) (bool, error) {
	if isRootAdmin(user) {
		_, found, err := a.store.GetCustomer(customerID)
		return found, err
	}
	return a.store.CustomerOwnedBy(customerID, model.AdminRoleAreaManager, user.ID)
}

func (a *App) areaManagerCustomerAssignmentAllowed(user model.AdminUser, req model.CustomerAssignmentRequest) bool {
	if !isAreaManager(user) || strings.TrimSpace(req.AgentID) == "" || req.InboundID <= 0 {
		return false
	}
	if a.areaManagerCustomerAssignmentAgentTargetsForwarding(user, req.AgentID) {
		return false
	}
	clientScope := a.areaManagerAssignmentClientScope(user, req.AgentID)
	if strings.TrimSpace(req.ClientEmail) == "" {
		return clientScope.allowsInbound(req.AgentID, req.InboundID, req.InboundTag)
	}
	if clientScope.allowsClient(req.AgentID, req.InboundID, req.InboundTag, req.ClientEmail) {
		return true
	}
	overview := a.xuiOverviewForOutboundAuthorization(req.AgentID)
	if overview == nil {
		return false
	}
	for _, client := range overview.Clients {
		if client.InboundID != req.InboundID ||
			!strings.EqualFold(strings.TrimSpace(client.InboundTag), strings.TrimSpace(req.InboundTag)) ||
			!strings.EqualFold(strings.TrimSpace(client.Email), strings.TrimSpace(req.ClientEmail)) {
			continue
		}
		return a.areaManagerCanViewForwardedClient(user, req.AgentID, client, clientScope)
	}
	return false
}

func (a *App) areaManagerXUIActionAllowed(user model.AdminUser, agentID string, req model.XUIActionRequest) bool {
	if !a.adminCanAccessAgent(user, agentID) {
		return false
	}
	switch strings.TrimSpace(req.Kind) {
	case model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule:
		return a.areaManagerRoutingPayloadAllowed(user, agentID, req.Payload)
	case model.XUIActionAddOutbound:
		return a.areaManagerCanCreateOutbound(user, agentID, req.Payload)
	case model.XUIActionAddClient:
		return a.areaManagerAddClientPayloadAllowed(user, agentID, req.Payload)
	case model.XUIActionSetClientEnabled, model.XUIActionDeleteClient:
		return a.areaManagerDeleteClientPayloadAllowed(user, agentID, req.Payload)
	default:
		return false
	}
}

func (a *App) areaManagerRoutingPayloadAllowed(user model.AdminUser, agentID string, payload map[string]any) bool {
	if payload == nil {
		return false
	}
	rule, ok := payload["rule"].(map[string]any)
	if !ok || rule == nil || strings.TrimSpace(stringFromAny(rule["balancerTag"])) != "" {
		return false
	}
	outboundTag := strings.TrimSpace(stringFromAny(rule["outboundTag"]))
	if outboundTag == "" {
		return false
	}
	outboundScope := a.areaManagerOutboundScope(user)
	previousTag := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(payload["previous_outbound_tag"]),
		stringFromAny(payload["old_outbound_tag"]),
	))
	if previousTag != "" && !outboundScope.allows(agentID, previousTag) {
		return false
	}
	if createdTag := outboundTagFromPayload(payload); createdTag != "" {
		return createdTag == outboundTag && a.areaManagerCanCreateOutbound(user, agentID, payload)
	}
	return outboundScope.allows(agentID, outboundTag)
}

func outboundTagFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	outbound, ok := payload["outbound"].(map[string]any)
	if !ok || outbound == nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(outbound["tag"]))
}

func (a *App) areaManagerAddClientPayloadAllowed(user model.AdminUser, agentID string, payload map[string]any) bool {
	if payload == nil {
		return false
	}
	inboundID := intFromAny(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromAny(payload["inbound_tag"]))
	if inboundID <= 0 && inboundTag == "" {
		return false
	}
	client, ok := payload["client"].(map[string]any)
	if !ok || client == nil {
		return false
	}
	if strings.TrimSpace(stringFromAny(client["email"])) == "" {
		return false
	}
	return a.areaManagerClientScope(user).allowsManageInbound(agentID, inboundID, inboundTag)
}

func (a *App) areaManagerDeleteClientPayloadAllowed(user model.AdminUser, agentID string, payload map[string]any) bool {
	if payload == nil {
		return false
	}
	inboundID := intFromAny(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromAny(payload["inbound_tag"]))
	if inboundID <= 0 && inboundTag == "" {
		return false
	}
	email := strings.TrimSpace(stringFromAny(payload["email"]))
	clientID := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(payload["client_id"]),
		stringFromAny(payload["client_uuid"]),
		stringFromAny(payload["auth_uuid"]),
	))
	if email == "" && clientID == "" {
		return false
	}
	if email != "" {
		return a.areaManagerClientScope(user).allowsClient(agentID, inboundID, inboundTag, email)
	}
	return a.areaManagerClientScope(user).allowsInbound(agentID, inboundID, inboundTag)
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		number, _ := typed.Int64()
		return int(number)
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func adminAgentSet(user model.AdminUser) map[string]struct{} {
	set := make(map[string]struct{}, len(user.AgentIDs))
	for _, agentID := range user.AgentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID != "" {
			set[agentID] = struct{}{}
		}
	}
	return set
}

func (a *App) adminVisibleAgentSet(user model.AdminUser) map[string]struct{} {
	if isAreaManager(user) && user.ID > 0 && a != nil && a.store != nil {
		return a.areaManagerClientScope(user).agents
	}
	return adminAgentSet(user)
}
