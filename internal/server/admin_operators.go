package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAdminOperators(w http.ResponseWriter, r *http.Request, parts []string) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
		return
	}
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := a.store.ListOperators()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			var req model.OperatorAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode operator request: %v", err))
				return
			}
			item, err := a.store.CreateOperator(req)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, item)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid operator id")
		return
	}
	if len(parts) == 2 && parts[1] == "reset-password" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		item, err := a.store.UpdateOperator(id, model.OperatorAccountRequest{Password: model.DefaultAccountPassword})
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req model.OperatorAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode operator request: %v", err))
			return
		}
		item, err := a.store.UpdateOperator(id, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := a.store.DeleteOperator(id); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
