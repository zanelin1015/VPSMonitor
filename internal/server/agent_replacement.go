package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"bridge-core/internal/model"
	storepkg "bridge-core/internal/store"
)

func (a *App) handleAgentReplacement(w http.ResponseWriter, r *http.Request, sourceAgentID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireRootAdmin(w, r)
	if !ok {
		return
	}
	var req model.AgentReplacementRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent replacement request: "+err.Error())
		return
	}
	req.ReplacementAgentID = strings.TrimSpace(req.ReplacementAgentID)
	if req.ReplacementAgentID == "" {
		writeError(w, http.StatusBadRequest, "replacement_agent_id is required")
		return
	}

	result, err := a.store.ReplaceAgentReferences(sourceAgentID, req.ReplacementAgentID, user.Username)
	if err != nil {
		var conflict *storepkg.AgentReplacementConflictError
		switch {
		case strings.Contains(err.Error(), "source agent not found"), strings.Contains(err.Error(), "replacement agent not found"):
			writeError(w, http.StatusNotFound, err.Error())
		case strings.Contains(err.Error(), "must be different"), strings.Contains(err.Error(), "are required"):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.As(err, &conflict):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	a.clearAgentReplacementCaches()
	for _, agentID := range result.UpdatedConfigAgentIDs {
		cfg, found, err := a.store.GetAgentConfig(agentID)
		if err != nil || !found {
			continue
		}
		if a.requestAgentConfigApply(agentID, cfg) {
			result.ConfigApplySentAgentIDs = append(result.ConfigApplySentAgentIDs, agentID)
		}
	}
	sort.Strings(result.ConfigApplySentAgentIDs)
	writeJSON(w, http.StatusOK, result)
}

func (a *App) clearAgentReplacementCaches() {
	a.dashboardCacheMu.Lock()
	a.dashboardCache = make(map[string]dashboardCacheEntry)
	a.topologyCache = make(map[string]dashboardCacheEntry)
	a.customerViewCache = make(map[string]customerOverviewCacheEntry)
	a.dashboardCacheMu.Unlock()

	a.areaTrafficMu.Lock()
	a.areaTrafficSamples = make(map[string]areaManagerTrafficSample)
	a.areaTrafficMu.Unlock()
}
