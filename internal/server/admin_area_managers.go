package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAdminAreaManagers(w http.ResponseWriter, r *http.Request, parts []string) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
		return
	}
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := a.store.ListAreaManagers()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			var req model.AreaManagerAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode area manager request: %v", err))
				return
			}
			item, err := a.store.CreateAreaManager(req)
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
		writeError(w, http.StatusBadRequest, "invalid area manager id")
		return
	}
	if len(parts) >= 2 {
		if parts[1] != "assignments" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		if len(parts) == 2 {
			switch r.Method {
			case http.MethodGet:
				assignments, err := a.store.ListAreaManagerAssignments(id)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, assignments)
			case http.MethodPost:
				var req model.AreaManagerAssignmentBatchRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeError(w, http.StatusBadRequest, fmt.Sprintf("decode area manager assignment request: %v", err))
					return
				}
				assignments, err := a.store.CreateAreaManagerAssignments(id, req.Assignments)
				if err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, assignments)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}
		if len(parts) == 3 {
			if r.Method != http.MethodDelete {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			assignmentID, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || assignmentID <= 0 {
				writeError(w, http.StatusBadRequest, "invalid assignment id")
				return
			}
			if err := a.store.DeleteAreaManagerAssignment(id, assignmentID); err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
		writeError(w, http.StatusNotFound, "route not found")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req model.AreaManagerAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode area manager request: %v", err))
			return
		}
		item, err := a.store.UpdateAreaManager(id, req)
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
		if err := a.store.DeleteAreaManager(id); err != nil {
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
