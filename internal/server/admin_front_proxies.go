package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAdminFrontProxies(w http.ResponseWriter, r *http.Request, parts []string) {
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			var (
				items []model.FrontProxyNode
				err   error
			)
			if isRootAdmin(user) {
				items, err = a.store.ListFrontProxyNodes()
			} else {
				items, err = a.store.ListFrontProxyNodesForGrantee(model.FrontProxyGranteeAreaManager, user.ID)
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			if !isRootAdmin(user) {
				writeError(w, http.StatusForbidden, "only admin can manage front proxy nodes")
				return
			}
			var req model.FrontProxyNodeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode front proxy request: %v", err))
				return
			}
			if _, ok := parseMihomoProxy(req.ShareURL, req.Name); !ok {
				writeError(w, http.StatusBadRequest, "unsupported front proxy share_url")
				return
			}
			item, err := a.store.CreateFrontProxyNode(req)
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

	if len(parts) == 1 && parts[0] == "grants" {
		if !isRootAdmin(user) {
			writeError(w, http.StatusForbidden, "only admin can manage front proxy grants")
			return
		}
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req model.FrontProxyGrantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode front proxy grant request: %v", err))
			return
		}
		if err := a.store.ReplaceFrontProxyGrants(req.GranteeType, req.GranteeID, req.NodeIDs); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid front proxy node id")
		return
	}
	if !isRootAdmin(user) {
		writeError(w, http.StatusForbidden, "only admin can manage front proxy nodes")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req model.FrontProxyNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode front proxy request: %v", err))
			return
		}
		if _, ok := parseMihomoProxy(req.ShareURL, req.Name); !ok {
			writeError(w, http.StatusBadRequest, "unsupported front proxy share_url")
			return
		}
		item, err := a.store.UpdateFrontProxyNode(id, req)
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
		if err := a.store.DeleteFrontProxyNode(id); err != nil {
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
