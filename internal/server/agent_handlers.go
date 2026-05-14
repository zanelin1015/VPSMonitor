package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
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

	result, err := a.store.RegisterAgent(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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

	items := make([]model.AgentListItem, 0, len(agents))
	for _, agent := range agents {
		items = append(items, model.AgentListItem{
			AgentID:       agent.AgentID,
			AgentName:     agent.AgentName,
			ClientVersion: agent.Version,
			ClientOS:      agent.OS,
			ClientArch:    agent.Arch,
			SystemVersion: agent.SystemVersion,
			SortOrder:     agent.SortOrder,
			Tags:          agent.Tags,
			Renewal:       agent.Config.Renewal,
			Entry:         agent.Config.Entry,
			ReportedAt:    agent.ReportedAt,
			RegisteredAt:  &agent.RegisteredAt,
			UpdatedAt:     &agent.UpdatedAt,
			LastSeenAt:    agent.LastSeenAt,
			Summary:       agent.Summary,
			HasConfig:     agent.HasConfig,
		})
	}
	a.realtime.applyToAgentItems(items)
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
		if _, _, ok := a.requireAgentAdmin(w, r, agentID); !ok {
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
		writeJSON(w, http.StatusOK, overview)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (a *App) handleAgentRefresh(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, _, ok := a.requireAgentAdmin(w, r, agentID); !ok {
		return
	}
	if _, found, err := a.store.GetAgent(agentID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "agent not found")
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
			if isAreaManager(user) && !a.areaManagerXUIActionAllowed(req.Kind) {
				writeError(w, http.StatusForbidden, "area manager can only create routing rule actions")
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
	}
	agent.Config = a.sanitizeManagedConfigForAdmin(user, agent.Config)
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
			cfg = a.sanitizeManagedConfigForAdmin(user, cfg)
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
	if snapshot.Summary.ObservedIP == "" {
		snapshot.Summary.ObservedIP = requestObservedIP(r)
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
	go a.alerts.EvaluateAgent(agentID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
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
