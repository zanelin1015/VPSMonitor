package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"bridge-core/internal/config"
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
	writeJSON(w, http.StatusOK, result)
}

func (a *App) applyDefaultXUIBootstrap(req *model.AgentRegisterRequest) {
	settings, found, err := a.store.GetClientInstallSettings()
	if err != nil || !found || !settings.XUIAutoInstall || registerSeedHasXUIConfig(req.SeedConfig.XUI) {
		return
	}
	panelPort := settings.XUIPanelPort
	if panelPort <= 0 {
		return
	}
	webPath := normalizeXUIWebPath(settings.XUIWebPath)
	req.SeedConfig.XUI.Enabled = true
	req.SeedConfig.XUI.BaseURL = fmt.Sprintf("http://127.0.0.1:%d%s", panelPort, webPath)
	req.SeedConfig.XUI.DBPath = config.DefaultXUIDBPathForOS(req.OS)
	req.SeedConfig.XUI.Username = settings.XUIUsername
	req.SeedConfig.XUI.Password = settings.XUIPassword
	req.SeedConfig.XUI.AutoInstall = true
	req.SeedConfig.XUI.InstallScriptURL = firstNonEmptyString(settings.XUIInstallScriptURL, defaultXUIInstallScriptURL)
	req.SeedConfig.XUI.PanelPort = panelPort
	req.SeedConfig.XUI.WebPath = webPath
}

func registerSeedHasXUIConfig(cfg config.XUIConfig) bool {
	return cfg.Enabled || cfg.BaseURL != "" || cfg.Username != "" || cfg.Password != "" || cfg.APIToken != "" || cfg.TwoFactorCode != "" || cfg.SkipTLSVerify || cfg.AutoInstall
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
		if snapshot, ok := latestByAgent[agent.AgentID]; ok {
			agent.Config.Entry = dashboard.MergeRealmSnapshotIntoEntry(agent.Config.Entry, snapshot.Realm)
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

	agents, err := a.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	agents = a.filterAgentRecordsForAdmin(user, agents)
	view := dashboard.BuildGlobalDashboard(agents, a.filterSnapshotsForAdmin(user, a.store.ListLatest()))
	a.realtime.applyToDashboard(&view)
	a.sanitizeDashboardForAdmin(user, &view)
	writeJSON(w, http.StatusOK, view)
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
			writeError(w, http.StatusNotFound, "x-ui snapshot not found")
			return
		}
		a.applyRealmPublicImportURLs(agentID, overview)
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

func (a *App) applyRealmPublicImportURLs(agentID string, overview *model.XUIOverview) {
	if overview == nil || len(overview.Clients) == 0 {
		return
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return
	}
	view := dashboard.BuildGlobalDashboard(agents, a.store.ListLatest())
	agentMap := make(map[string]model.DashboardAgentView, len(view.Agents))
	for _, agent := range view.Agents {
		agentMap[agent.AgentID] = agent
	}
	chainMap := make(map[string]model.ClientChainView, len(view.ClientChains))
	for _, chain := range view.ClientChains {
		chainMap[chain.Key] = chain
	}
	inboundPorts := make(map[string]int, len(overview.Nodes))
	for _, node := range overview.Nodes {
		inboundPorts[overviewInboundKey(node.ID, node.Tag)] = node.Port
	}
	for index := range overview.Clients {
		client := &overview.Clients[index]
		if client.ImportURL == "" {
			continue
		}
		assignment := model.CustomerAssignment{
			AgentID:     agentID,
			InboundID:   client.InboundID,
			InboundTag:  client.InboundTag,
			ClientEmail: client.Email,
		}
		chain, found := findCustomerChain(assignment, chainMap)
		if !found {
			chain = model.ClientChainView{
				RootAgentID:     agentID,
				RootInboundTag:  client.InboundTag,
				RootClientEmail: client.Email,
				Steps: []model.ClientChainStep{{
					StepType: "inbound",
					AgentID:  agentID,
					Label:    client.InboundTag,
					Port:     inboundPorts[overviewInboundKey(client.InboundID, client.InboundTag)],
				}},
			}
		}
		publicEntry, ok := customerRealmPublicEntry(assignment, chain, agentMap)
		if !ok {
			continue
		}
		if rewritten := rewriteCustomerImportURL(client.ImportURL, publicEntry.Host, publicEntry.Port); rewritten != "" {
			client.ImportURL = rewritten
		}
	}
}

func overviewInboundKey(inboundID int, inboundTag string) string {
	return fmt.Sprintf("%d\x00%s", inboundID, inboundTag)
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
			}
			cfg = a.hydrateRealmForwardTargets(cfg)
			cfg = a.sanitizeManagedConfigForAdmin(user, cfg)
		} else {
			cfg = a.hydrateRealmForwardTargets(cfg)
		}
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
		record, err := a.store.UpdateAgentConfigWithActor(agentID, cfg, user.Username)
		if err != nil {
			if err.Error() == "agent not found" {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, record.Config)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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
