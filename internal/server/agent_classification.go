package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAgentClassification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireRootAdmin(w, r)
	if !ok {
		return
	}
	var req struct {
		AgentIDs []string                       `json:"agent_ids"`
		Patch    model.AgentClassificationPatch `json:"patch"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := make([]string, 0, len(req.AgentIDs))
	seen := make(map[string]bool)
	for _, id := range req.AgentIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 || (req.Patch.Purposes == nil && req.Patch.CountryCode == nil && req.Patch.City == nil) {
		writeError(w, http.StatusBadRequest, "Client IDs and classification changes are required")
		return
	}
	if _, err := req.Patch.Apply(nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		impact, err := a.previewAgentClassification(ids, req.Patch)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, impact)
		return
	}
	if err := a.store.UpdateAgentClassifications(ids, req.Patch, user.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.clearDashboardCache()
	a.clearCustomerOverviewCache()
	writeJSON(w, http.StatusOK, map[string]int{"updated": len(ids)})
}

type agentClassificationImpact struct {
	AssignmentID int64  `json:"assignment_id"`
	CustomerName string `json:"customer_name"`
	AgentName    string `json:"agent_name"`
	InboundID    int    `json:"inbound_id"`
}

func (a *App) previewAgentClassification(ids []string, patch model.AgentClassificationPatch) ([]agentClassificationImpact, error) {
	becomingInternal := make(map[string]model.DashboardAgentView)
	for _, id := range ids {
		stored, found, err := a.store.GetAgent(id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("Client %q not found", id)
		}
		agent := model.DashboardAgentView{AgentID: id, AgentName: stored.AgentName, Entry: stored.Config.Entry}
		wasInternal := internalCustomerEntrance(agent, model.CustomerRouteConfig{})
		agent.Entry.Classification, err = patch.Apply(agent.Entry.Classification)
		if err != nil {
			return nil, err
		}
		if !wasInternal && internalCustomerEntrance(agent, model.CustomerRouteConfig{}) {
			becomingInternal[id] = agent
		}
	}
	impact := make([]agentClassificationImpact, 0)
	if len(becomingInternal) == 0 {
		return impact, nil
	}
	customers, err := a.store.ListCustomers()
	if err != nil {
		return nil, err
	}
	for _, customer := range customers {
		for _, assignment := range customer.Assignments {
			agent, affected := becomingInternal[assignment.AgentID]
			if !affected || !assignment.Enabled {
				continue
			}
			route := findCustomerEntranceRule(agent.Entry, assignment.InboundID)
			if route.publication.Visibility == "" {
				impact = append(impact, agentClassificationImpact{AssignmentID: assignment.ID, CustomerName: customer.Username,
					AgentName: firstNonEmptyString(agent.AgentName, agent.AgentID), InboundID: assignment.InboundID})
			}
		}
	}
	return impact, nil
}
