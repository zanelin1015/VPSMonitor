package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
	"bridge-core/internal/realmconfig"
)

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.isRegistrationAuthorized(r.Header.Get("X-Registration-Token")) {
		writeError(w, http.StatusUnauthorized, "invalid registration token")
		return
	}

	var req model.AgentRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode register request: %v", err))
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	if req.AgentName == "" {
		req.AgentName = req.AgentID
	}
	req.SeedConfig.AgentID = req.AgentID
	if req.SeedConfig.AgentName == "" {
		req.SeedConfig.AgentName = req.AgentName
	}
	a.applyDefaultXUIBootstrap(&req)

	result, err := a.store.RegisterAgent(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	observedIP := requestObservedIP(r)
	go a.refreshTopologyLookupCacheFromRegister(req, observedIP)
	writeJSON(w, http.StatusOK, result)
}

func (a *App) applyDefaultXUIBootstrap(req *model.AgentRegisterRequest) {
	// x-ui auto-install is currently disabled globally. Do not seed new clients
	// with auto_install even if old install settings still have it enabled.
	_ = req
}

func latestSnapshotsByAgent(snapshots []model.AgentSnapshot) map[string]model.AgentSnapshot {
	result := make(map[string]model.AgentSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.AgentID == "" {
			continue
		}
		result[snapshot.AgentID] = snapshot
	}
	return result
}

func (a *App) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}

	agents, err := a.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	agents = a.filterAgentRecordsForAdmin(user, agents)
	latestByAgent := latestSnapshotsByAgent(a.filterSnapshotsForAdmin(user, a.store.ListLatest()))

	items := make([]model.AgentListItem, 0, len(agents))
	for _, agent := range agents {
		var networkPolicy *model.NetworkPolicySnapshot
		if snapshot, ok := latestByAgent[agent.AgentID]; ok {
			agent.Config.Entry = dashboard.MergeRealmSnapshotIntoEntry(agent.Config.Entry, snapshot.Realm)
			networkPolicy = snapshot.NetworkPolicy
		}
		items = append(items, model.AgentListItem{
			AgentID:             agent.AgentID,
			AgentName:           agent.AgentName,
			CustomerDisplayName: agent.CustomerDisplayName,
			ClientVersion:       agent.Version,
			ClientOS:            agent.OS,
			ClientArch:          agent.Arch,
			SystemVersion:       agent.SystemVersion,
			SortOrder:           agent.SortOrder,
			Tags:                agent.Tags,
			Renewal:             agent.Config.Renewal,
			Entry:               agent.Config.Entry,
			ReportedAt:          agent.ReportedAt,
			RegisteredAt:        &agent.RegisteredAt,
			UpdatedAt:           &agent.UpdatedAt,
			LastSeenAt:          agent.LastSeenAt,
			Summary:             agent.Summary,
			NetworkPolicy:       networkPolicy,
			HasConfig:           agent.HasConfig,
		})
	}
	a.realtime.applyToAgentItems(items)
	items = a.sanitizeAgentListItemsForAdmin(user, items)
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}

	view, err := a.dashboardViewForAdmin(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) handleDashboardTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}

	view, err := a.dashboardTopologyViewForAdmin(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) dashboardViewForAdmin(user model.AdminUser) (model.GlobalDashboardView, error) {
	cacheKey := dashboardCacheKey(user)
	now := time.Now()

	a.dashboardCacheMu.Lock()
	if a.dashboardCache == nil {
		a.dashboardCache = make(map[string]dashboardCacheEntry)
	}
	if entry, ok := a.dashboardCache[cacheKey]; ok && now.Before(entry.expiresAt) {
		view, err := cloneDashboardView(entry.view)
		a.dashboardCacheMu.Unlock()
		return view, err
	}
	a.dashboardCacheMu.Unlock()

	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return model.GlobalDashboardView{}, err
	}
	agents = a.filterAgentRecordsForAdmin(user, agents)
	snapshots = a.filterSnapshotsForAdmin(user, snapshots)

	view := dashboard.BuildGlobalDashboardWithOptions(agents, snapshots, dashboard.GlobalDashboardOptions{
		IncludeTopology:    false,
		IncludeGeo:         true,
		AllowNetworkLookup: false,
		ResolverData:       a.dashboardTopologyResolverData(),
	})
	if a.realtime != nil {
		a.realtime.applyToDashboard(&view)
	}
	a.sanitizeDashboardForAdmin(user, &view)

	cachedView, err := cloneDashboardView(view)
	if err != nil {
		return view, nil
	}
	a.dashboardCacheMu.Lock()
	a.dashboardCache[cacheKey] = dashboardCacheEntry{
		expiresAt: time.Now().Add(dashboardCacheTTL),
		view:      cachedView,
	}
	a.dashboardCacheMu.Unlock()
	return view, nil
}

func (a *App) dashboardTopologyViewForAdmin(user model.AdminUser) (model.GlobalDashboardView, error) {
	cacheKey := dashboardCacheKey(user)
	now := time.Now()

	for {
		a.dashboardCacheMu.Lock()
		if a.topologyCache == nil {
			a.topologyCache = make(map[string]dashboardCacheEntry)
		}
		if a.topologyBuilds == nil {
			a.topologyBuilds = make(map[string]chan struct{})
		}
		if entry, ok := a.topologyCache[cacheKey]; ok && now.Before(entry.expiresAt) {
			view, err := cloneDashboardView(entry.view)
			a.dashboardCacheMu.Unlock()
			return view, err
		}
		if ch, ok := a.topologyBuilds[cacheKey]; ok {
			if entry, hasStale := a.topologyCache[cacheKey]; hasStale {
				view, err := cloneDashboardView(entry.view)
				a.dashboardCacheMu.Unlock()
				return view, err
			}
			a.dashboardCacheMu.Unlock()
			<-ch
			continue
		}
		a.topologyBuilds[cacheKey] = make(chan struct{})
		a.dashboardCacheMu.Unlock()
		break
	}

	view, err := a.buildDashboardViewForAdmin(user, true)

	a.dashboardCacheMu.Lock()
	if ch, ok := a.topologyBuilds[cacheKey]; ok {
		close(ch)
		delete(a.topologyBuilds, cacheKey)
	}
	if err == nil {
		if cachedView, cloneErr := cloneDashboardView(view); cloneErr == nil {
			a.topologyCache[cacheKey] = dashboardCacheEntry{
				expiresAt: time.Now().Add(topologyCacheTTL),
				view:      cachedView,
			}
		}
	}
	a.dashboardCacheMu.Unlock()

	return view, err
}

func (a *App) buildDashboardViewForAdmin(user model.AdminUser, includeTopology bool) (model.GlobalDashboardView, error) {
	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return model.GlobalDashboardView{}, err
	}
	agents = a.filterAgentRecordsForAdmin(user, agents)
	snapshots = a.filterSnapshotsForAdmin(user, snapshots)

	view := dashboard.BuildGlobalDashboardWithOptions(agents, snapshots, dashboard.GlobalDashboardOptions{
		IncludeTopology:    includeTopology,
		IncludeGeo:         true,
		AllowNetworkLookup: false,
		ResolverData:       a.dashboardTopologyResolverData(),
	})
	if a.realtime != nil {
		a.realtime.applyToDashboard(&view)
	}
	a.sanitizeDashboardForAdmin(user, &view)
	return view, nil
}

func (a *App) dashboardTopologyResolverData() dashboard.TopologyResolverData {
	cache, found, err := a.store.GetTopologyLookupCache()
	if err != nil {
		log.Printf("load topology lookup cache failed: %v", err)
		return dashboard.TopologyResolverData{}
	}
	if !found {
		return dashboard.TopologyResolverData{}
	}
	data := dashboard.TopologyResolverData{
		Hosts: make(map[string][]string, len(cache.Hosts)),
		Geos:  make(map[string]model.IPGeoView, len(cache.Geos)),
	}
	for key, entry := range cache.Hosts {
		data.Hosts[key] = append([]string(nil), entry.IPs...)
	}
	for key, entry := range cache.Geos {
		data.Geos[key] = entry.Geo
	}
	return data
}

func (a *App) refreshTopologyLookupCacheFromRegister(req model.AgentRegisterRequest, observedIP string) {
	values := []string{req.PublicIPv4, req.PublicIPv6, req.Hostname}
	if isUsableObservedIP(observedIP) {
		values = append(values, observedIP)
	}
	a.refreshTopologyLookupCache(req.AgentID, values)
}

func (a *App) refreshTopologyLookupCacheFromSnapshot(agentID string, snapshot model.AgentSnapshot) {
	values := []string{
		snapshot.Summary.ObservedIP,
		snapshot.Summary.ServerSeenIP,
		snapshot.Summary.PublicIPv4,
		snapshot.Summary.PublicIPv6,
	}
	if snapshot.XUI != nil {
		values = append(values, snapshot.XUI.BaseURL)
		overview := dashboard.BuildXUIOverview(snapshot)
		if overview != nil {
			for _, outbound := range overview.Outbounds {
				values = append(values, outbound.Address, outbound.TLSServerName, outbound.WSHost, outbound.SendThrough)
			}
		}
	}
	if snapshot.Realm != nil {
		for _, rule := range snapshot.Realm.Rules {
			values = append(values, rule.ListenAddress, rule.TargetAddress)
		}
	}
	a.refreshTopologyLookupCache(agentID, values)
}

func (a *App) refreshTopologyLookupCache(agentID string, values []string) {
	a.lookupCacheMu.Lock()
	defer a.lookupCacheMu.Unlock()

	cache, _, err := a.store.GetTopologyLookupCache()
	if err != nil {
		log.Printf("refresh topology lookup cache for %s failed: load cache: %v", agentID, err)
		return
	}
	values = pendingTopologyLookupValues(cache, values)
	if len(values) == 0 {
		return
	}
	resolved := dashboard.NewTopologyResolverData(values)
	if len(resolved.Hosts) == 0 && len(resolved.Geos) == 0 {
		return
	}
	now := time.Now().UTC()
	if cache.Hosts == nil {
		cache.Hosts = make(map[string]model.TopologyHostCacheEntry)
	}
	if cache.Geos == nil {
		cache.Geos = make(map[string]model.TopologyGeoCacheEntry)
	}
	for host, ips := range resolved.Hosts {
		cache.Hosts[host] = model.TopologyHostCacheEntry{
			IPs:       append([]string(nil), ips...),
			UpdatedAt: now,
			ExpiresAt: now.Add(20 * time.Minute),
		}
	}
	for ip, geo := range resolved.Geos {
		if geo.CountryCode == "" && geo.CountryName == "" {
			continue
		}
		cache.Geos[ip] = model.TopologyGeoCacheEntry{
			Geo:       geo,
			UpdatedAt: now,
			ExpiresAt: now.Add(7 * 24 * time.Hour),
		}
	}
	if err := a.store.SaveTopologyLookupCache(cache); err != nil {
		log.Printf("refresh topology lookup cache for %s failed: save cache: %v", agentID, err)
		return
	}
	a.clearDashboardTopologyCache()
}

func (a *App) clearDashboardTopologyCache() {
	a.dashboardCacheMu.Lock()
	a.topologyCache = make(map[string]dashboardCacheEntry)
	a.dashboardCacheMu.Unlock()
}

func pendingTopologyLookupValues(cache model.TopologyLookupCache, values []string) []string {
	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(values))
	pending := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeTopologyLookupValue(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		if ip := normalizeTopologyLookupIP(normalized); ip != "" {
			entry, ok := cache.Geos[ip]
			if ok && now.Before(entry.ExpiresAt) && (entry.Geo.CountryCode != "" || entry.Geo.CountryName != "") {
				continue
			}
		} else {
			entry, ok := cache.Hosts[normalized]
			if ok && now.Before(entry.ExpiresAt) && len(entry.IPs) > 0 {
				continue
			}
		}
		pending = append(pending, normalized)
	}
	return pending
}

func normalizeTopologyLookupValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if strings.Contains(value, "/") {
		value = strings.Split(value, "/")[0]
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(strings.TrimSuffix(value, "."), "[]")
	return value
}

func normalizeTopologyLookupIP(value string) string {
	value = normalizeTopologyLookupValue(value)
	if value == "" {
		return ""
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return ""
	}
	return addr.String()
}

func dashboardCacheKey(user model.AdminUser) string {
	return fmt.Sprintf("%s:%d:%s:%d", user.Role, user.ID, user.Username, user.UpdatedAt.UnixNano())
}

func cloneDashboardView(view model.GlobalDashboardView) (model.GlobalDashboardView, error) {
	body, err := json.Marshal(view)
	if err != nil {
		return model.GlobalDashboardView{}, err
	}
	var cloned model.GlobalDashboardView
	if err := json.Unmarshal(body, &cloned); err != nil {
		return model.GlobalDashboardView{}, err
	}
	return cloned, nil
}

func (a *App) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	agentID := parts[0]

	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			a.handleDeleteAgent(w, r, agentID)
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAgentRecord(w, r, agentID)
		return
	}

	switch parts[1] {
	case "heartbeat":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleHeartbeat(w, r, agentID)
	case "metrics":
		if len(parts) < 3 || parts[2] != "ws" {
			writeError(w, http.StatusNotFound, "metrics endpoint not found")
			return
		}
		a.handleAgentMetricsWS(w, r, agentID)
	case "terminal":
		if len(parts) < 3 || parts[2] != "ws" {
			writeError(w, http.StatusNotFound, "terminal endpoint not found")
			return
		}
		a.handleAgentTerminalWS(w, r, agentID)
	case "refresh":
		a.handleAgentRefresh(w, r, agentID)
	case "history":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if _, _, ok := a.requireAgentAdmin(w, r, agentID); !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		history, err := a.store.ListHistory(agentID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, history)
	case "config":
		a.handleAgentConfig(w, r, agentID)
	case "realm":
		if len(parts) != 3 || parts[2] != "copy" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		a.handleRealmConfigCopy(w, r, agentID)
	case "logs":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAgentLogs(w, r, agentID)
	case "xui":
		if len(parts) >= 3 && parts[2] == "actions" {
			a.handleXUIActions(w, r, agentID, parts[3:])
			return
		}
		if len(parts) != 3 || parts[2] != "overview" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		user, _, ok := a.requireAgentAdmin(w, r, agentID)
		if !ok {
			return
		}
		snapshot, ok := a.store.GetLatest(agentID)
		if !ok {
			writeError(w, http.StatusNotFound, "snapshot not found")
			return
		}
		cfg, _, err := a.store.GetAgentConfig(agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{Entry: cfg.Entry})
		if overview == nil {
			overview = emptyAgentXUIOverview(snapshot, cfg)
		}
		a.appendRealmForwardedImportURLs(agentID, overview)
		if isAreaManager(user) {
			overview.AgentName = areaManagerDisplayName(cfg.CustomerDisplayName, cfg.AgentName, agentID)
		}
		if isAreaManager(user) && r.URL.Query().Get("assignment_scope") == "1" {
			a.sanitizeXUIOverviewForAreaAssignment(user, overview)
		} else {
			a.sanitizeXUIOverviewForAdmin(user, overview)
		}
		writeJSON(w, http.StatusOK, overview)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (a *App) handleDeleteAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	user, _, ok := a.requireRootAdmin(w, r)
	if !ok {
		return
	}
	_ = user
	if err := a.store.DeleteAgent(agentID); err != nil {
		if err.Error() == "agent not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.realtime.removeAgent(agentID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "agent_id": agentID})
}

func (a *App) handleAgentRefresh(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAgentAdmin(w, r, agentID)
	if !ok {
		return
	}
	if _, found, err := a.store.GetAgent(agentID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if isAreaManager(user) {
		writeError(w, http.StatusForbidden, "area manager cannot trigger client refresh")
		return
	}
	if !a.realtime.sendAgentControl(agentID, model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		writeError(w, http.StatusConflict, "Client 实时连接不在线，无法立即采集；请确认 Client 已更新到新版并保持在线")
		return
	}
	writeJSON(w, http.StatusOK, model.AgentRefreshResponse{
		Status:  "sent",
		Mode:    "websocket",
		Message: "已通知 Client 立即采集并上报",
	})
}

func (a *App) handleAgentLogs(w http.ResponseWriter, r *http.Request, agentID string) {
	if _, _, ok := a.requireAgentAdmin(w, r, agentID); !ok {
		return
	}
	snapshot, ok := a.store.GetLatest(agentID)
	if !ok {
		writeError(w, http.StatusNotFound, "snapshot not found")
		return
	}
	logs := snapshot.Logs
	if logs == nil {
		logs = []model.AgentLogEntry{}
	}
	writeJSON(w, http.StatusOK, model.AgentLogsResponse{
		AgentID:           agentID,
		ReportedAt:        snapshot.ReportedAt,
		LastCollectionErr: snapshot.Summary.LastCollectionErr,
		Logs:              logs,
	})
}

func emptyAgentXUIOverview(snapshot model.AgentSnapshot, cfg model.ManagedAgentConfig) *model.XUIOverview {
	return &model.XUIOverview{
		AgentID:     snapshot.AgentID,
		AgentName:   firstNonEmptyString(snapshot.AgentName, cfg.AgentName),
		ReportedAt:  snapshot.ReportedAt,
		CollectedAt: snapshot.ReportedAt,
		Summary:     snapshot.Summary,
	}
}

func (a *App) appendRealmForwardedImportURLs(agentID string, overview *model.XUIOverview) {
	if overview == nil {
		return
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return
	}
	snapshots := a.store.ListLatest()
	agentMap := buildRealmForwardAgentMap(agents, snapshots)
	snapshotMap := make(map[string]model.AgentSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotMap[snapshot.AgentID] = snapshot
	}
	entryByAgent := make(map[string]model.AgentEntryConfig, len(agents))
	for agentID, agent := range agentMap {
		entryByAgent[agentID] = agent.Entry
	}
	sourceAgent, ok := agentMap[agentID]
	if !ok {
		return
	}
	added := 0
	targetOverviewByAgent := make(map[string]*model.XUIOverview)
	for _, rule := range sourceAgent.Entry.PortForwarding.Rules {
		if !rule.Enabled || rule.ListenPort <= 0 || rule.TargetPort <= 0 {
			continue
		}
		targetAgentID := findRealmTargetAgentID(rule, agentMap)
		if targetAgentID == "" || strings.EqualFold(targetAgentID, agentID) {
			continue
		}
		targetSnapshot, ok := snapshotMap[targetAgentID]
		if !ok {
			continue
		}
		targetOverview := targetOverviewByAgent[targetAgentID]
		if targetOverview == nil {
			targetOverview = dashboard.BuildXUIOverviewWithOptions(targetSnapshot, dashboard.XUIOverviewOptions{Entry: entryByAgent[targetAgentID]})
			targetOverviewByAgent[targetAgentID] = targetOverview
		}
		if targetOverview == nil {
			continue
		}
		host := customerRealmSourceHost(sourceAgent, rule)
		if host == "" {
			continue
		}
		targetPorts := overviewNodePorts(targetOverview.Nodes)
		for _, client := range targetOverview.Clients {
			targetPort := targetPorts[overviewInboundKey(client.InboundID, client.InboundTag)]
			if targetPort > 0 && targetPort != rule.TargetPort {
				continue
			}
			if targetPort == 0 && rule.TargetPort <= 0 {
				continue
			}
			rewritten := rewriteCustomerImportURL(client.ImportURL, host, rule.ListenPort)
			if rewritten == "" {
				continue
			}
			sourceClient := client
			sourceClient.ImportURL = rewritten
			sourceClient.InboundID = rule.ListenPort
			sourceClient.InboundTag = realmForwardedInboundTag(rule, client)
			sourceClient.InboundRemark = realmForwardedInboundRemark(sourceAgent, agentMap[targetAgentID], rule, client)
			sourceClient.IsRealmForwarded = true
			sourceClient.RealmListenTag = sourceClient.InboundTag
			sourceClient.RealmSourceAgentID = agentID
			sourceClient.RealmTargetAgentID = targetAgentID
			sourceClient.RealmTargetInboundID = client.InboundID
			sourceClient.RealmTargetInboundTag = client.InboundTag
			sourceClient.RealmListenPort = rule.ListenPort
			sourceClient.Route.Note = fmt.Sprintf("Realm 入口 %s:%d -> %s:%d", host, rule.ListenPort, firstNonEmptyString(agentMap[targetAgentID].AgentName, targetAgentID), rule.TargetPort)
			overview.Clients = append(overview.Clients, sourceClient)
			added++
		}
	}
	if added > 0 {
		sortXUIClients(overview.Clients)
		overview.ClientCount = len(overview.Clients)
		overview.OnlineClientCount = countOnlineXUIClients(overview.Clients)
	}
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

func overviewNodePorts(nodes []model.XUINodeView) map[string]int {
	ports := make(map[string]int, len(nodes))
	for _, node := range nodes {
		ports[overviewInboundKey(node.ID, node.Tag)] = node.Port
	}
	return ports
}

func findRealmTargetAgentID(rule model.RealmForwardRule, agentMap map[string]model.DashboardAgentView) string {
	if strings.TrimSpace(rule.TargetAgentID) != "" {
		if _, ok := agentMap[rule.TargetAgentID]; ok {
			return rule.TargetAgentID
		}
		return ""
	}
	for _, agent := range agentMap {
		if customerRealmRuleTargetsAgent(rule, agent.AgentID, customerAgentAddressSet(agent)) {
			return agent.AgentID
		}
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

func (a *App) handleXUIActions(w http.ResponseWriter, r *http.Request, agentID string, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			if user, _, ok := a.currentAdmin(r); ok {
				if !a.adminCanAccessAgent(user, agentID) {
					writeError(w, http.StatusForbidden, "agent is not assigned to this account")
					return
				}
				limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
				actions, err := a.store.ListXUIActions(agentID, limit)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				if !isRootAdmin(user) {
					actions = filterRootOnlyXUIActions(actions)
				}
				writeJSON(w, http.StatusOK, actions)
				return
			}
			if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
				writeError(w, http.StatusUnauthorized, "invalid agent token")
				return
			}
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			actions, err := a.store.ClaimPendingXUIActions(agentID, limit)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, actions)
		case http.MethodPost:
			user, _, ok := a.requireAgentAdmin(w, r, agentID)
			if !ok {
				return
			}
			var req model.XUIActionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode x-ui action: %v", err))
				return
			}
			if isAreaManager(user) && !a.areaManagerXUIActionAllowed(user, agentID, req) {
				writeError(w, http.StatusForbidden, "area manager can only create routing rule actions or add/delete clients under assigned nodes")
				return
			}
			if isRootOnlyXUIActionKind(req.Kind) && !isRootAdmin(user) {
				writeError(w, http.StatusForbidden, "only root admin can create this x-ui action")
				return
			}
			action, err := a.store.CreateXUIAction(agentID, req)
			if err != nil {
				status := http.StatusInternalServerError
				if err.Error() == "agent not found" || strings.Contains(err.Error(), "unsupported") {
					status = http.StatusBadRequest
				}
				writeError(w, status, err.Error())
				return
			}
			action, _ = a.dispatchXUIActionRealtime(agentID, action)
			writeJSON(w, http.StatusOK, action)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "result" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
			writeError(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		actionID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || actionID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid action id")
			return
		}
		var req model.XUIActionResultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode x-ui action result: %v", err))
			return
		}
		action, err := a.store.CompleteXUIAction(agentID, actionID, req)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid") {
				status = http.StatusBadRequest
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, action)
		return
	}

	writeError(w, http.StatusNotFound, "route not found")
}

func (a *App) handleAgentRecord(w http.ResponseWriter, r *http.Request, agentID string) {
	user, _, ok := a.requireAgentAdmin(w, r, agentID)
	if !ok {
		return
	}
	agent, found, err := a.store.GetAgent(agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if snapshot, ok := a.store.GetLatest(agentID); ok {
		agent.Summary = snapshot.Summary
		agent.ReportedAt = &snapshot.ReportedAt
		agent.Version = snapshot.Version
		agent.OS = snapshot.OS
		agent.Arch = snapshot.Arch
		agent.SystemVersion = snapshot.SystemVersion
		agent.Config.Entry = dashboard.MergeRealmSnapshotIntoEntry(agent.Config.Entry, snapshot.Realm)
		agent.Config = inferLegacyAgentFeatures(agent.Config, &snapshot)
	} else {
		agent.Config = inferLegacyAgentFeatures(agent.Config, nil)
	}
	agent = a.sanitizeAgentRecordForAdmin(user, agent)
	writeJSON(w, http.StatusOK, agent)
}

func (a *App) handleAgentConfig(w http.ResponseWriter, r *http.Request, agentID string) {
	switch r.Method {
	case http.MethodGet:
		if !a.isConfigReadAuthorized(agentID, r) {
			writeError(w, http.StatusUnauthorized, "not authorized to read config")
			return
		}
		cfg, found, err := a.store.GetAgentConfig(agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		if user, _, ok := a.currentAdmin(r); ok {
			if snapshot, exists := a.store.GetLatest(agentID); exists {
				cfg.Entry = dashboard.MergeRealmSnapshotIntoEntry(cfg.Entry, snapshot.Realm)
				cfg = inferLegacyAgentFeatures(cfg, &snapshot)
			} else {
				cfg = inferLegacyAgentFeatures(cfg, nil)
			}
			cfg = a.hydrateRealmForwardTargets(cfg)
			cfg = a.sanitizeManagedConfigForAdmin(user, cfg)
		} else {
			if snapshot, exists := a.store.GetLatest(agentID); exists {
				cfg = inferLegacyAgentFeatures(cfg, &snapshot)
			} else {
				cfg = inferLegacyAgentFeatures(cfg, nil)
			}
			cfg = a.hydrateRealmForwardTargets(cfg)
		}
		cfg = disableXUIAutoInstall(cfg)
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		user, _, ok := a.requireRootAdmin(w, r)
		if !ok {
			return
		}
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode config: %v", err))
			return
		}
		body, err := json.Marshal(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode config: %v", err))
			return
		}
		var cfg model.ManagedAgentConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode config: %v", err))
			return
		}
		cfg.AgentID = agentID
		if _, ok := raw["customer_display_name"]; !ok {
			existing, found, err := a.store.GetAgentConfig(agentID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !found {
				writeError(w, http.StatusNotFound, "agent not found")
				return
			}
			cfg.CustomerDisplayName = existing.CustomerDisplayName
		}
		cfg = disableXUIAutoInstall(cfg)
		record, err := a.store.UpdateAgentConfigWithActor(agentID, cfg, user.Username)
		if err != nil {
			if err.Error() == "agent not found" {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.syncAgentClientExpiryRules(record, "config_save")
		a.requestAgentConfigApply(agentID, record.Config)
		writeJSON(w, http.StatusOK, disableXUIAutoInstall(record.Config))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func disableXUIAutoInstall(cfg model.ManagedAgentConfig) model.ManagedAgentConfig {
	cfg.XUI.AutoInstall = false
	cfg.XUI.InstallScriptURL = ""
	cfg.XUI.PanelPort = 0
	cfg.XUI.WebPath = ""
	return cfg
}

func inferLegacyAgentFeatures(cfg model.ManagedAgentConfig, snapshot *model.AgentSnapshot) model.ManagedAgentConfig {
	if cfg.Features.Configured {
		return cfg
	}
	entry := cfg.Entry
	cfg.Features = model.AgentFeatureConfig{
		XUI: legacyXUIConfigPresent(cfg) || snapshotHasXUI(snapshot),
		Realm: entry.PortForwarding.Enabled ||
			len(entry.PortForwarding.Rules) > 0 ||
			strings.TrimSpace(entry.PortForwarding.ConfigPath) != "" ||
			strings.TrimSpace(entry.PortForwarding.ServiceName) != "" ||
			snapshotHasRealm(snapshot),
		NAT: len(entry.Mappings) > 0 ||
			len(entry.Addresses) > 0 ||
			strings.TrimSpace(entry.ImportDomain) != "",
		PortPolicy: entry.NetworkPolicy.Enabled ||
			len(entry.NetworkPolicy.Rules) > 0 ||
			strings.TrimSpace(entry.NetworkPolicy.Interface) != "",
	}
	return cfg
}

func legacyXUIConfigPresent(cfg model.ManagedAgentConfig) bool {
	xui := cfg.XUI
	return xui.Enabled ||
		strings.TrimSpace(xui.BaseURL) != "" ||
		strings.TrimSpace(xui.DBPath) != "" ||
		strings.TrimSpace(xui.APIToken) != "" ||
		strings.TrimSpace(xui.Username) != ""
}

func snapshotHasXUI(snapshot *model.AgentSnapshot) bool {
	if snapshot == nil || snapshot.XUI == nil {
		return false
	}
	xui := snapshot.XUI
	return strings.TrimSpace(xui.BaseURL) != "" ||
		len(xui.Inbounds) > 0 ||
		len(xui.Outbounds) > 0 ||
		len(xui.RoutingRules) > 0 ||
		len(xui.Certificates) > 0 ||
		snapshot.Summary.InboundCount > 0 ||
		snapshot.Summary.OutboundCount > 0 ||
		snapshot.Summary.RoutingRuleCount > 0
}

func snapshotHasRealm(snapshot *model.AgentSnapshot) bool {
	if snapshot == nil || snapshot.Realm == nil {
		return false
	}
	realm := snapshot.Realm
	return len(realm.Rules) > 0 ||
		strings.TrimSpace(realm.ConfigPath) != "" ||
		strings.TrimSpace(realm.ServiceName) != "" ||
		strings.TrimSpace(realm.BinaryPath) != ""
}

func (a *App) requestAgentConfigApply(agentID string, cfg model.ManagedAgentConfig) bool {
	if a.realtime == nil {
		return false
	}
	cfg.AgentID = agentID
	applySent := a.realtime.sendAgentControl(agentID, model.AgentControlMessage{Type: model.AgentControlApplyConfig, Config: &cfg})
	// Older clients do not understand apply_config; collect_now keeps immediate apply compatible.
	collectSent := a.realtime.sendAgentControl(agentID, model.AgentControlMessage{Type: model.AgentControlCollectNow})
	if applySent || collectSent {
		return true
	}
	log.Printf("client %s realtime connection is offline; config will apply on next poll", agentID)
	return false
}

func filterRootOnlyXUIActions(actions []model.XUIAction) []model.XUIAction {
	filtered := make([]model.XUIAction, 0, len(actions))
	for _, action := range actions {
		if isRootOnlyXUIActionKind(action.Kind) {
			continue
		}
		filtered = append(filtered, action)
	}
	return filtered
}

func isRootOnlyXUIActionKind(kind string) bool {
	switch kind {
	case model.XUIActionExecuteCommand, model.XUIActionUpdate3XUI:
		return true
	default:
		return false
	}
}

func realtimeXUIActionAllowed(kind string) bool {
	switch kind {
	case model.XUIActionAddOutbound,
		model.XUIActionAddClient,
		model.XUIActionAddRoutingRule,
		model.XUIActionUpsertRoutingRule,
		model.XUIActionUpdateClientExpiry,
		model.XUIActionSetClientEnabled,
		model.XUIActionDeleteClient,
		model.XUIActionUpdateClient,
		model.XUIActionExecuteCommand,
		model.XUIActionUpdate3XUI:
		return true
	default:
		return false
	}
}

func (a *App) dispatchXUIActionRealtime(agentID string, action model.XUIAction) (model.XUIAction, bool) {
	control := model.AgentControlMessage{
		ActionID: action.ID,
		Payload:  action.Payload,
	}
	switch {
	case action.Kind == model.XUIActionRestartXUI:
		control.Type = model.AgentControlRestartXUI
	case realtimeXUIActionAllowed(action.Kind):
		control.Type = model.AgentControlExecuteXUI
		control.Kind = action.Kind
	default:
		return action, false
	}
	if !a.realtime.sendAgentControl(agentID, control) {
		return action, false
	}
	running, err := a.store.MarkXUIActionRunning(agentID, action.ID)
	if err != nil {
		return action, true
	}
	return running, true
}

func (a *App) dispatchPendingXUIActionsRealtime(agentID string) {
	actions, err := a.store.ListXUIActions(agentID, 100)
	if err != nil {
		return
	}
	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if action.Status != model.XUIActionStatusPending {
			continue
		}
		a.dispatchXUIActionRealtime(agentID, action)
	}
}

func (a *App) handleHeartbeat(w http.ResponseWriter, r *http.Request, agentID string) {
	if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}

	var snapshot model.AgentSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode snapshot: %v", err))
		return
	}
	if snapshot.AgentID == "" {
		snapshot.AgentID = agentID
	}
	if snapshot.AgentID != agentID {
		writeError(w, http.StatusBadRequest, "agent id mismatch")
		return
	}
	if serverSeenIP := requestObservedIP(r); isUsableObservedIP(serverSeenIP) {
		snapshot.Summary.ServerSeenIP = serverSeenIP
	}
	if snapshot.AgentName == "" {
		agent, found, err := a.store.GetAgent(agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if found {
			snapshot.AgentName = agent.AgentName
		}
	}
	if err := a.store.SaveSnapshot(snapshot); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	go a.refreshTopologyLookupCacheFromSnapshot(agentID, snapshot)
	a.syncRealmConfigFromSnapshot(agentID, snapshot.Realm)
	go a.alerts.EvaluateAgent(agentID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (a *App) syncRealmConfigFromSnapshot(agentID string, snapshot *model.RealmSnapshot) {
	if snapshot == nil || len(snapshot.Rules) == 0 {
		return
	}
	cfg, found, err := a.store.GetAgentConfig(agentID)
	if err != nil {
		log.Printf("sync realm config for %s failed: load config: %v", agentID, err)
		return
	}
	if !found {
		return
	}
	merged := cfg
	merged.Entry = realmconfig.MergeSnapshotIntoEntry(cfg.Entry, snapshot)
	if reflect.DeepEqual(cfg.Entry.PortForwarding, merged.Entry.PortForwarding) {
		return
	}
	if _, err := a.store.UpdateAgentConfigWithActor(agentID, merged, "system:realm-config"); err != nil {
		log.Printf("sync realm config for %s failed: save config: %v", agentID, err)
	}
}

func (a *App) isAuthorized(agentID, token string) bool {
	return a.store.ValidateAgentToken(agentID, token)
}

func (a *App) isRegistrationAuthorized(token string) bool {
	if a.config.RegistrationToken == "" {
		return true
	}
	return token != "" && token == a.config.RegistrationToken
}

func (a *App) isConfigReadAuthorized(agentID string, r *http.Request) bool {
	if user, _, ok := a.currentAdmin(r); ok {
		return a.adminCanAccessAgent(user, agentID)
	}
	return a.isAuthorized(agentID, r.Header.Get("X-Agent-Token"))
}
