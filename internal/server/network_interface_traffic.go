package server

import (
	"net/http"

	"bridge-core/internal/model"
)

func (a *App) handleNetworkInterfaceTraffic(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
		return
	}
	if _, found, err := a.store.GetAgent(agentID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	from, to, err := parseTrafficDates(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := a.store.ListNetworkInterfaceTraffic(agentID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := model.NetworkInterfaceTrafficResponse{From: from, To: to, Timezone: "Asia/Shanghai", Items: items}
	if latest, found := a.store.GetLatest(agentID); found && latest.NetworkInterfaces != nil {
		snapshot := latest.NetworkInterfaces
		response.Reported, response.Error, response.Warnings = true, snapshot.Error, snapshot.Warnings
		response.CollectedAt = &snapshot.CollectedAt
	}
	writeJSON(w, http.StatusOK, response)
}
