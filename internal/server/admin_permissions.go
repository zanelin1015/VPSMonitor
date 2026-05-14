package server

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
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
	agentNames := make(map[string]string, len(view.Agents))
	for index := range view.Agents {
		view.Agents[index] = sanitizeDashboardAgentForAreaManager(view.Agents[index], tagMap)
		agentNames[view.Agents[index].AgentID] = view.Agents[index].AgentName
	}
	view.Links = sanitizeTopologyLinksForAreaManager(view.Links, allowed, tagMap, agentNames)
	view.ClientChains = sanitizeClientChainsForAreaManager(view.ClientChains, allowed, tagMap, agentNames)
	rebuildAreaManagerDashboardStats(view)
}

func (a *App) sanitizeXUIOverviewForAdmin(user model.AdminUser, overview *model.XUIOverview) {
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
		link.Source.TargetGeo = nil
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

func sanitizeClientChainsForAreaManager(chains []model.ClientChainView, allowed map[string]struct{}, tagMap map[string][]string, agentNames map[string]string) []model.ClientChainView {
	filtered := make([]model.ClientChainView, 0, len(chains))
	for _, chain := range chains {
		if _, ok := allowed[chain.RootAgentID]; !ok {
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
			step.TargetGeo = nil
		}
		filtered = append(filtered, chain)
	}
	return filtered
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

func (a *App) areaManagerXUIActionAllowed(kind string) bool {
	switch strings.TrimSpace(kind) {
	case model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule:
		return true
	default:
		return false
	}
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
