package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
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
	if isRootAdmin(user) || isOperatorAdmin(user) {
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
	if isRootAdmin(user) || isOperatorAdmin(user) {
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
	if isRootAdmin(user) || isOperatorAdmin(user) {
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
	if isRootAdmin(user) || isOperatorAdmin(user) {
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
	billings := a.areaManagerVisibleClientBillings(user, cfg)
	cfg.Renewal = sanitizeAreaManagerRenewal(cfg.Renewal)
	cfg.Renewal.ClientBillings = billings
	cfg.Entry = model.AgentEntryConfig{}
	cfg.XUI = config.XUIConfig{
		Enabled: cfg.XUI.Enabled,
	}
	return cfg
}

// Only per-client display settings are shared; VPS costs and unrelated client prices stay private.
func (a *App) areaManagerVisibleClientBillings(user model.AdminUser, cfg model.ManagedAgentConfig) []model.XUIClientBillingConfig {
	if !isAreaManager(user) || len(cfg.Renewal.ClientBillings) == 0 {
		return nil
	}
	scope := a.areaManagerClientScope(user)
	var clients []model.XUIClientView
	if overview := a.xuiOverviewForOutboundAuthorization(cfg.AgentID); overview != nil {
		clients = overview.Clients
	}
	result := make([]model.XUIClientBillingConfig, 0)
	for _, billing := range cfg.Renewal.ClientBillings {
		allowed := billing.Email != "" && scope.allowsClient(cfg.AgentID, billing.InboundID, billing.InboundTag, billing.Email)
		if !allowed && billing.ClientID != "" {
			for _, client := range clients {
				if client.ClientID == billing.ClientID && (scope.allowsClient(cfg.AgentID, client.InboundID, client.InboundTag, client.Email) || a.areaManagerCanViewForwardedClient(user, cfg.AgentID, client, scope)) {
					allowed = true
					break
				}
			}
		}
		if allowed {
			result = append(result, billing)
		}
	}
	return result
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
	if isOperatorAdmin(user) {
		for index := range items {
			items[index].ClientVersion = ""
			items[index].ClientOS = ""
			items[index].ClientArch = ""
			items[index].SystemVersion = ""
			items[index].Entry = model.AgentEntryConfig{}
			items[index].Realm = nil
			items[index].HAProxy = nil
			items[index].NetworkPolicy = nil
		}
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
	if isOperatorAdmin(user) {
		sanitizeDashboardForOperator(view)
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
		publicAgent.FinanceClients = areaManagerVisibleFinanceClients(agent, clientScope)
		publicAgent.FinanceClientsReady = agent.FinanceClientsReady
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

func sanitizeDashboardForOperator(view *model.GlobalDashboardView) {
	if view == nil {
		return
	}
	view.Links = nil
	view.ClientChains = nil
	view.Tags = nil
	view.Totals.LinkCount = 0
	view.Totals.ChainCount = 0
	for index := range view.Agents {
		agent := &view.Agents[index]
		agent.ClientVersion = ""
		agent.ClientOS = ""
		agent.ClientArch = ""
		agent.SystemVersion = ""
		agent.Entry = model.AgentEntryConfig{}
		agent.Realm = nil
		agent.HAProxy = nil
		agent.NetworkPolicy = nil
		agent.Geo = nil
		agent.FinanceClients = nil
		agent.FinanceClientsReady = false
	}
}

func sanitizeXUIOverviewForOperator(overview *model.XUIOverview) {
	if overview == nil {
		return
	}
	overview.BaseURL = ""
	overview.EditableRoutingRules = nil
	overview.Outbounds = nil
	overview.Balancers = nil
	overview.RoutingRules = nil
	overview.Certificates = nil
	overview.OutboundsFingerprint = ""
	overview.RoutingRulesFingerprint = ""
	for index := range overview.Clients {
		client := &overview.Clients[index]
		client.ExportEndpoint = nil
		client.ImportURL = ""
		client.ShadowrocketImportURL = ""
		client.AuthUUID = ""
		client.AuthPassword = ""
		client.Route = model.XUIRouteTrace{}
	}
	for index := range overview.Nodes {
		node := &overview.Nodes[index]
		node.ExportEndpoint = nil
		node.Route = model.XUIRouteTrace{}
		node.AuthKeys = nil
	}
}

func (a *App) sanitizeXUIOverviewForAdmin(user model.AdminUser, overview *model.XUIOverview) {
	if overview == nil || isRootAdmin(user) {
		return
	}
	if isOperatorAdmin(user) {
		sanitizeXUIOverviewForOperator(overview)
		return
	}
	overview.EditableRoutingRules = nil
	overview.RoutingRulesFingerprint = ""
	sanitizeOutboundDeletionMetadata(overview)
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

func sanitizeOutboundDeletionMetadata(overview *model.XUIOverview) {
	overview.OutboundsFingerprint = ""
	for i := range overview.Outbounds {
		overview.Outbounds[i].Fingerprint = ""
		overview.Outbounds[i].DeleteBlockedReason = ""
	}
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
		// Reasons may contain matchers belonging to another area account.
		next.ValidityReasons = nil
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
	overview.EditableRoutingRules = nil
	overview.RoutingRulesFingerprint = ""
	sanitizeOutboundDeletionMetadata(overview)
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
		filteredClients = append(filteredClients, client)
		visibleInbounds[overviewInboundKey(client.InboundID, client.InboundTag)] = struct{}{}
	}
	overview.Clients = filteredClients
	overview.ClientCount = len(filteredClients)
	overview.OnlineClientCount = 0
	filteredNodes := make([]model.XUINodeView, 0, len(overview.Nodes))
	for _, node := range overview.Nodes {
		canAssignAllClients := clientScope.allowsWholeInbound(overview.AgentID, node.ID, node.Tag)
		if _, visible := visibleInbounds[overviewInboundKey(node.ID, node.Tag)]; !clientScope.allowsInbound(overview.AgentID, node.ID, node.Tag) && !visible {
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

func areaManagerVisibleFinanceClients(agent model.DashboardAgentView, scope areaManagerClientScope) []model.FinanceClientView {
	if !agent.FinanceClientsReady || len(agent.FinanceClients) == 0 {
		return []model.FinanceClientView{}
	}
	visible := make([]model.FinanceClientView, 0, len(agent.FinanceClients))
	for _, client := range agent.FinanceClients {
		if !scope.allowsClient(agent.AgentID, client.InboundID, client.InboundTag, client.Email) {
			continue
		}
		visible = append(visible, model.FinanceClientView{
			ClientID:    client.ClientID,
			InboundID:   client.InboundID,
			InboundTag:  client.InboundTag,
			Email:       client.Email,
			Comment:     client.Comment,
			NodeEnabled: client.NodeEnabled,
			Enabled:     client.Enabled,
		})
	}
	return visible
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

func outboundLinkKey(agentID, outboundTag string) string {
	return strings.TrimSpace(agentID) + "::" + strings.TrimSpace(outboundTag)
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
