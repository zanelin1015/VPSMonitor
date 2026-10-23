package server

import (
	"encoding/json"
	"net/http"

	"bridge-core/internal/model"
)

func agentRoleView(agent model.AgentRecord) model.AgentRoleView {
	v := model.AgentRoleView{AgentID: agent.AgentID, AgentName: agent.AgentName}
	for _, tag := range agent.Tags {
		switch tag {
		case model.AgentRoleLine:
			v.LineEnabled = true
		case model.AgentRoleLanding:
			v.LandingEnabled = true
		}
	}
	return v
}
func (a *App) handleAdminAgentRoles(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if !isRootAdmin(user) && !isOperatorAdmin(user) {
		writeError(w, 403, "admin or operator permission required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		agents, err := a.store.ListAgents()
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		result := make([]model.AgentRoleView, 0, len(agents))
		for _, agent := range agents {
			result = append(result, agentRoleView(agent))
		}
		writeJSON(w, 200, result)
	case http.MethodPut:
		var req model.AgentRoleUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		cfg, found, err := a.store.GetAgentConfig(req.AgentID)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		if !found {
			writeError(w, 404, "agent not found")
			return
		}
		tags := make([]string, 0, len(cfg.Tags)+2)
		for _, tag := range cfg.Tags {
			if tag != model.AgentRoleLine && tag != model.AgentRoleLanding {
				tags = append(tags, tag)
			}
		}
		if req.LineEnabled {
			tags = append(tags, model.AgentRoleLine)
		}
		if req.LandingEnabled {
			tags = append(tags, model.AgentRoleLanding)
		}
		cfg.Tags = tags
		record, err := a.store.UpdateAgentConfigWithActor(req.AgentID, cfg, user.Username)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		a.clearCustomerOverviewCache()
		a.dashboardCacheMu.Lock()
		a.dashboardCache = make(map[string]dashboardCacheEntry)
		a.topologyCache = make(map[string]dashboardCacheEntry)
		a.dashboardCacheMu.Unlock()
		writeJSON(w, 200, agentRoleView(record))
	default:
		writeError(w, 405, "method not allowed")
	}
}
