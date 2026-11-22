package server

import (
	"net/http"

	"bridge-core/internal/model"
)

func (a *App) handleRealmTraffic(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAgentAdmin(w, r, agentID)
	if !ok {
		return
	}
	if isOperatorAdmin(user) {
		writeError(w, http.StatusForbidden, "permission denied")
		return
	}
	fromText, toText, err := parseTrafficDates(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := a.store.ListRealmTrafficUsage(agentID, fromText, toText)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isAreaManager(user) {
		scope := a.areaManagerGrantedClientScope(user)
		visible := make([]model.RealmTrafficUsage, 0)
		for _, item := range items {
			if scope.allowsForwardingPort(agentID, item.ListenPort, "realm") {
				visible = append(visible, item)
			}
		}
		items = visible
	}
	response := model.RealmTrafficUsageResponse{From: fromText, To: toText, Timezone: "Asia/Shanghai", Items: items}
	if latest, found := a.store.GetLatest(agentID); found && latest.Realm != nil && latest.Realm.Traffic != nil {
		traffic := latest.Realm.Traffic
		response.Reported, response.Enabled, response.Error = true, traffic.Enabled, traffic.Error
		if !isRootAdmin(user) && response.Error != "" {
			response.Error = "Client 流量采集失败，请联系管理员检查"
		}
		response.CollectedAt = &traffic.CollectedAt
	}
	writeJSON(w, http.StatusOK, response)
}
