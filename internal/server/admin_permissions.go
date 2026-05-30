package server

import (
	"encoding/json"
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
	return false
}

func (a *App) filterAgentRecordsForAdmin(user model.AdminUser, agents []model.AgentRecord) []model.AgentRecord {
	if isRootAdmin(user) {
		return agents
	}
	allowed := adminAgentSet(user)
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
	allowed := adminAgentSet(user)
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
		return metrics
	}
	allowed := adminAgentSet(user)
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
		return metric
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
	allowed := adminAgentSet(user)
	clientScope := a.areaManagerClientScope(user)
	agentNames := make(map[string]string, len(view.Agents))
	for index := range view.Agents {
		view.Agents[index] = sanitizeDashboardAgentForAreaManager(view.Agents[index], tagMap)
		agentNames[view.Agents[index].AgentID] = view.Agents[index].AgentName
	}
	view.Links = sanitizeTopologyLinksForAreaManager(view.Links, allowed, tagMap, agentNames)
	view.ClientChains = sanitizeClientChainsForAreaManager(view.ClientChains, allowed, tagMap, agentNames, clientScope)
	view.Links = filterTopologyLinksUsedByChains(view.Links, view.ClientChains)
	applyAreaManagerClientCounts(view, clientScope)
	rebuildAreaManagerDashboardStats(view)
}

func (a *App) sanitizeXUIOverviewForAdmin(user model.AdminUser, overview *model.XUIOverview) {
	if overview == nil || isRootAdmin(user) {
		return
	}
	overview.AgentName = areaManagerDisplayName("", overview.AgentName, overview.AgentID)
	overview.BaseURL = ""
	overview.Summary = sanitizeAreaManagerSummary(overview.Summary)
	clientScope := a.areaManagerClientScope(user)
	filteredClients := make([]model.XUIClientView, 0, len(overview.Clients))
	for _, client := range overview.Clients {
		if !clientScope.allowsClient(overview.AgentID, client.InboundID, client.InboundTag, client.Email) &&
			!a.areaManagerCanViewRealmForwardedClient(user, overview.AgentID, client, clientScope) {
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
		node.Up = 0
		node.Down = 0
		node.Total = 0
		node.AllTime = 0
		filteredNodes = append(filteredNodes, node)
	}
	overview.Nodes = filteredNodes
	overview.NodeCount = len(filteredNodes)
	for index := range overview.Outbounds {
		overview.Outbounds[index].Address = redactEndpointIP(overview.Outbounds[index].Address)
		overview.Outbounds[index].Target = redactEndpointIP(overview.Outbounds[index].Target)
		overview.Outbounds[index].SendThrough = ""
	}
}

func (a *App) areaManagerCanViewRealmForwardedClient(user model.AdminUser, sourceAgentID string, client model.XUIClientView, clientScope areaManagerClientScope) bool {
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
	if listenPort <= 0 {
		return false
	}
	return clientScope.allowsClient(sourceAgentID, listenPort, client.InboundTag, client.Email) ||
		clientScope.allowsClient(sourceAgentID, listenPort, "", client.Email)
}

func (a *App) sanitizeXUIOverviewForAreaAssignment(user model.AdminUser, overview *model.XUIOverview) {
	if overview == nil || isRootAdmin(user) {
		return
	}
	overview.AgentName = areaManagerDisplayName("", overview.AgentName, overview.AgentID)
	overview.BaseURL = ""
	overview.Summary = sanitizeAreaManagerSummary(overview.Summary)
	for index := range overview.Clients {
		overview.Clients[index].TotalGB = 0
		overview.Clients[index].ExpiryTime = 0
		overview.Clients[index].CreatedAt = 0
		overview.Clients[index].UpdatedAt = 0
		overview.Clients[index].LastOnline = 0
	}
	overview.ClientCount = len(overview.Clients)
	overview.OnlineClientCount = 0
	for index := range overview.Nodes {
		overview.Nodes[index].ClientCount = 0
		overview.Nodes[index].OnlineCount = 0
		overview.Nodes[index].Up = 0
		overview.Nodes[index].Down = 0
		overview.Nodes[index].Total = 0
		overview.Nodes[index].AllTime = 0
	}
	overview.NodeCount = len(overview.Nodes)
	for index := range overview.Outbounds {
		overview.Outbounds[index].Address = redactEndpointIP(overview.Outbounds[index].Address)
		overview.Outbounds[index].Target = redactEndpointIP(overview.Outbounds[index].Target)
		overview.Outbounds[index].SendThrough = ""
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
	item.Geo = nil
	return item
}

func sanitizeDashboardAgentForAreaManager(agent model.DashboardAgentView, tagMap map[string][]string) model.DashboardAgentView {
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
	agent.NetworkPolicy = nil
	agent.Geo = nil
	return agent
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
		NetTrafficSent:  summary.NetTrafficSent,
		NetTrafficRecv:  summary.NetTrafficRecv,
		NetTrafficTotal: summary.NetTrafficTotal,
		NetIOUp:         summary.NetIOUp,
		NetIODown:       summary.NetIODown,
		DiskUsed:        summary.DiskUsed,
		DiskTotal:       summary.DiskTotal,
	}
}

func sanitizeRealtimeMetricForAreaManager(metric model.AgentRealtimeMetrics) model.AgentRealtimeMetrics {
	metric.AgentName = ""
	metric.ClientVersion = ""
	metric.ClientOS = ""
	metric.ClientArch = ""
	metric.SystemVersion = ""
	metric.Summary = sanitizeAreaManagerSummary(metric.Summary)
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
		link.Target.AgentName = areaManagerDisplayName("", agentNames[link.Target.AgentID], link.Target.AgentID)
		link.Target.AgentTags = cloneStringSlice(tagMap[link.Target.AgentID])
		link.Target.IPs = nil
		link.Target.ResolvedIPs = nil
		link.Target.EntryIPs = nil
		link.Target.EntryAddresses = filterDomainLikeValues(link.Target.EntryAddresses)
		link.Target.EntryMappings = sanitizeEntryMappingsForAreaManager(link.Target.EntryMappings)
		filtered = append(filtered, link)
	}
	return filtered
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
	agents       map[string]struct{}
}

func (a *App) areaManagerClientScope(user model.AdminUser) areaManagerClientScope {
	scope := areaManagerClientScope{
		exactClients: make(map[string]struct{}),
		inbounds:     make(map[string]struct{}),
		agents:       adminAgentSet(user),
	}
	if !isAreaManager(user) || user.ID <= 0 {
		return scope
	}
	customers, err := a.store.ListCustomersForOwner(model.AdminRoleAreaManager, user.ID)
	if err != nil {
		return scope
	}
	assignments, err := a.store.ListAreaManagerAssignments(user.ID)
	if err == nil {
		for _, assignment := range assignments {
			addAreaManagerScopeAssignment(&scope, assignment.AgentID, assignment.InboundID, assignment.InboundTag, assignment.ClientEmail, assignment.Enabled)
		}
	}
	for _, customer := range customers {
		for _, assignment := range customer.Assignments {
			addAreaManagerScopeAssignment(&scope, assignment.AgentID, assignment.InboundID, assignment.InboundTag, assignment.ClientEmail, assignment.Enabled)
		}
	}
	return scope
}

func addAreaManagerScopeAssignment(scope *areaManagerClientScope, agentID string, inboundID int, inboundTag, clientEmail string, enabled bool) {
	if scope == nil || !enabled || agentID == "" {
		return
	}
	if clientEmail != "" {
		scope.exactClients[areaClientExactKey(agentID, inboundID, inboundTag, clientEmail)] = struct{}{}
		if inboundTag != "" {
			scope.exactClients[areaClientExactKey(agentID, inboundID, "", clientEmail)] = struct{}{}
		}
		return
	}
	scope.inbounds[areaClientInboundKey(agentID, inboundID, inboundTag)] = struct{}{}
	if inboundTag != "" {
		scope.inbounds[areaClientInboundKey(agentID, inboundID, "")] = struct{}{}
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

func filterTopologyLinksUsedByChains(links []model.TopologyLinkView, chains []model.ClientChainView) []model.TopologyLinkView {
	used := make(map[string]struct{})
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
		}
	}
	return filtered
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

func (a *App) areaManagerXUIActionAllowed(user model.AdminUser, agentID string, req model.XUIActionRequest) bool {
	if !a.adminCanAccessAgent(user, agentID) {
		return false
	}
	switch strings.TrimSpace(req.Kind) {
	case model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule:
		return true
	case model.XUIActionAddClient:
		return a.areaManagerAddClientPayloadAllowed(user, agentID, req.Payload)
	case model.XUIActionSetClientEnabled, model.XUIActionDeleteClient:
		return a.areaManagerDeleteClientPayloadAllowed(user, agentID, req.Payload)
	default:
		return false
	}
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
	return a.areaManagerClientScope(user).allowsInbound(agentID, inboundID, inboundTag)
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
