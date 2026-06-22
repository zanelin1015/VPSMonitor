package server

import (
	"net/http"
	"strconv"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func (a *App) handleAdminAccessLogs(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
		return
	}
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))
	items, total, err := a.store.ListAccessLogs(store.AccessLogFilter{
		AgentID:     query.Get("agent_id"),
		SourceIP:    query.Get("source_ip"),
		Target:      query.Get("target"),
		ClientEmail: query.Get("client_email"),
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.AccessLogListResponse{Items: items, Total: total})
}
