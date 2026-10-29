package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (a *App) handleSingBoxOverview(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAgentAdmin(w, r, agentID)
	if !ok {
		return
	}
	snapshot, ok := a.store.GetLatest(agentID)
	if !ok || snapshot.SingBox == nil {
		writeError(w, http.StatusNotFound, "sing-box snapshot not found")
		return
	}
	overview := dashboard.BuildSingBoxOverview(snapshot)
	if overview == nil {
		writeError(w, http.StatusNotFound, "sing-box snapshot not found")
		return
	}
	if !isRootAdmin(user) {
		redactSingBoxOverview(overview)
		overview.RawConfig = nil
	}
	writeJSON(w, http.StatusOK, overview)
}

func redactSingBoxOverview(overview *model.SingBoxOverview) {
	if overview == nil {
		return
	}
	for _, item := range overview.Nodes {
		for _, key := range []string{"psk", "password", "private_key", "private_key_path", "token"} {
			delete(item, key)
		}
	}
	for _, item := range overview.Clients {
		for _, key := range []string{"password", "users", "private_key", "token"} {
			delete(item, key)
		}
	}
}

func isSingBoxActionKind(kind string) bool {
	switch kind {
	case model.SingBoxActionAddNode, model.SingBoxActionAddClient, model.SingBoxActionAddForwardingRule, model.SingBoxActionReload:
		return true
	default:
		return false
	}
}

// handleSingBoxActions reuses the durable action queue implementation.  The
// wrapper only narrows creation to sing-box kinds; result delivery and polling
// remain compatible with Clients that already understand the x-ui queue.
func (a *App) handleSingBoxActions(w http.ResponseWriter, r *http.Request, agentID string, parts []string) {
	if len(parts) == 0 && r.Method == http.MethodPost {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("read sing-box action: %v", err))
			return
		}
		var req model.XUIActionRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode sing-box action: %v", err))
			return
		}
		if !isSingBoxActionKind(req.Kind) {
			writeError(w, http.StatusBadRequest, "unsupported sing-box action kind")
			return
		}
		if cfg, found, err := a.store.GetAgentConfig(agentID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		} else if !found || !cfg.SingBox.Enabled {
			writeError(w, http.StatusConflict, "请先启用并保存 sing-box 托管配置")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
	}
	a.handleXUIActions(w, r, agentID, parts)
}
