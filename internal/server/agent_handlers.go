package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
	"bridge-core/internal/realmconfig"
)

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
		var realmSnapshot *model.RealmSnapshot
		var haProxySnapshot *model.HAProxySnapshot
		if snapshot, ok := latestByAgent[agent.AgentID]; ok {
			agent.Config.Entry = dashboard.MergeRealmSnapshotIntoEntry(agent.Config.Entry, snapshot.Realm)
			realmSnapshot = snapshot.Realm
			haProxySnapshot = snapshot.HAProxy
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
			Realm:               realmSnapshot,
			HAProxy:             haProxySnapshot,
			NetworkPolicy:       networkPolicy,
			HasConfig:           agent.HasConfig,
		})
	}
	a.realtime.applyToAgentItems(items)
	items = a.sanitizeAgentListItemsForAdmin(user, items)
	writeJSON(w, http.StatusOK, items)
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
	case "traffic-reset-validation":
		if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
			writeError(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		writeError(w, http.StatusGone, model.LegacyTrafficResetRemovedMessage)
	case "replace":
		if len(parts) != 2 {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		a.handleAgentReplacement(w, r, agentID)
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
	case "access-logs":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAgentAccessLogs(w, r, agentID)
	case "xui":
		if len(parts) >= 3 && parts[2] == "client-config" {
			a.handleXUIClientConfig(w, r, agentID, parts[3:])
			return
		}
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
		a.appendForwardedImportURLs(agentID, overview)
		if isAreaManager(user) {
			overview.AgentName = areaManagerDisplayName(cfg.CustomerDisplayName, cfg.AgentName, agentID)
		}
		if isAreaManager(user) && r.URL.Query().Get("assignment_scope") == "1" {
			a.sanitizeXUIOverviewForAreaAssignment(user, overview)
		} else {
			a.sanitizeXUIOverviewForAdmin(user, overview)
		}
		if isRootAdmin(user) || isAreaManager(user) {
			overview.ClientConfigSaves, err = a.store.LatestXUIClientConfigSaves(agentID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if isAreaManager(user) {
			visible := make([]model.XUIClientConfigSave, 0)
			for _, save := range overview.ClientConfigSaves {
				for _, client := range overview.Clients {
					if clientConfigSaveMatchesClient(save, client) {
						visible = append(visible, save)
						break
					}
				}
			}
			overview.ClientConfigSaves = visible
		}
		writeJSON(w, http.StatusOK, overview)
	case "singbox", "sing-box":
		if len(parts) >= 3 && parts[2] == "actions" {
			a.handleSingBoxActions(w, r, agentID, parts[3:])
			return
		}
		if len(parts) == 3 && parts[2] == "overview" {
			a.handleSingBoxOverview(w, r, agentID)
			return
		}
		writeError(w, http.StatusNotFound, "route not found")
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
	agent, found, err := a.store.GetAgent(agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err := a.store.DeleteAgent(agentID); err != nil {
		if err.Error() == "agent not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	clientStopSent := a.dispatchDisableClientService(agent)
	a.removeAgentRealtimeAfterDisable(agentID, clientStopSent)
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "agent_id": agentID, "client_stop_sent": clientStopSent})
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
	if isOperatorAdmin(user) {
		writeError(w, http.StatusForbidden, "当前账号不能触发 Client 立即刷新")
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
	// Online freshness must be based on the server receive time; VPS clocks can drift.
	snapshot.ReportedAt = time.Now().UTC()
	if snapshot.HAProxy != nil {
		snapshot.HAProxy.CollectedAt = snapshot.ReportedAt
	}
	// Older clients may still report an IP obtained through their own proxy.
	snapshot.Summary.ObservedIP = ""
	snapshot.Summary.ServerSeenIP = ""
	if serverSeenIP := a.requestObservedIP(r); isUsableObservedIP(serverSeenIP) {
		snapshot.Summary.ObservedIP = serverSeenIP
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
	a.clearCustomerOverviewCache()
	go a.refreshTopologyLookupCacheFromSnapshot(agentID, snapshot)
	a.syncRealmConfigFromSnapshot(agentID, snapshot.Realm)
	go a.alerts.EvaluateAgent(agentID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (a *App) handleAgentAccessLogs(w http.ResponseWriter, r *http.Request, agentID string) {
	if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}
	var req model.AccessLogBatchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode access logs: %v", err))
		return
	}
	if len(req.Entries) > 500 {
		req.Entries = req.Entries[:500]
	}
	if err := a.store.SaveAccessLogs(agentID, req.Entries); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	retentionDays := 7
	if cfg, found, err := a.store.GetAgentConfig(agentID); err == nil && found && cfg.XUI.AccessLogRetentionDays > 0 {
		retentionDays = cfg.XUI.AccessLogRetentionDays
	}
	go func() {
		if err := a.store.PruneAccessLogs(retentionDays); err != nil {
			log.Printf("prune access logs failed: %v", err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted", "count": len(req.Entries)})
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
	if cfg.Features.HAProxy || cfg.Entry.HAProxy.Enabled {
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
