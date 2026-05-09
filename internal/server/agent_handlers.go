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
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	agents, err := a.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]model.AgentListItem, 0, len(agents))
	for _, agent := range agents {
		items = append(items, model.AgentListItem{
			AgentID:      agent.AgentID,
			AgentName:    agent.AgentName,
			SortOrder:    agent.SortOrder,
			Tags:         agent.Tags,
			Renewal:      agent.Config.Renewal,
			Entry:        agent.Config.Entry,
			ReportedAt:   agent.ReportedAt,
			RegisteredAt: &agent.RegisteredAt,
			UpdatedAt:    &agent.UpdatedAt,
			LastSeenAt:   agent.LastSeenAt,
			Summary:      agent.Summary,
			HasConfig:    agent.HasConfig,
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
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	agents, err := a.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view := dashboard.BuildGlobalDashboard(agents, a.store.ListLatest())
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
	case "history":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if _, _, ok := a.requireAdmin(w, r); !ok {
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
		if _, _, ok := a.requireAdmin(w, r); !ok {
			return
		}
		snapshot, ok := a.store.GetLatest(agentID)
		if !ok {
			writeError(w, http.StatusNotFound, "snapshot not found")
			return
		}
		overview := dashboard.BuildXUIOverview(snapshot)
		if overview == nil {
			writeError(w, http.StatusNotFound, "x-ui snapshot not found")
			return
		}
		writeJSON(w, http.StatusOK, overview)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (a *App) handleXUIActions(w http.ResponseWriter, r *http.Request, agentID string, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			if _, _, ok := a.currentAdmin(r); ok {
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
			if _, _, ok := a.requireAdmin(w, r); !ok {
				return
			}
			var req model.XUIActionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode x-ui action: %v", err))
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
	if _, _, ok := a.requireAdmin(w, r); !ok {
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
	}
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
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		user, _, ok := a.requireAdmin(w, r)
		if !ok {
			return
		}
		var cfg model.ManagedAgentConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode config: %v", err))
			return
		}
		cfg.AgentID = agentID
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
	if _, _, ok := a.currentAdmin(r); ok {
		return true
	}
	return a.isAuthorized(agentID, r.Header.Get("X-Agent-Token"))
}
