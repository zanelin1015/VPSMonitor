package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		for _, key := range []string{"password", "private_key", "token"} {
			delete(item, key)
		}
		if users, ok := item["users"].([]any); ok {
			for _, raw := range users {
				if user, ok := raw.(map[string]any); ok {
					for _, key := range []string{"password", "uuid", "userkey", "token", "private_key"} {
						delete(user, key)
					}
				}
			}
		}
	}
}

func isSingBoxActionKind(kind string) bool {
	switch kind {
	case model.SingBoxActionAddNode, model.SingBoxActionAddClient, model.SingBoxActionAddUser, model.SingBoxActionDeleteUser, model.SingBoxActionAddForwardingRule, model.SingBoxActionReload:
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
		user, _, ok := a.requireAgentAdmin(w, r, agentID)
		if !ok {
			return
		}
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
		if req.Kind == model.SingBoxActionAddForwardingRule && strings.TrimSpace(stringPayloadValue(req.Payload["outbound_tag"])) != "" {
			if err := a.enrichSingBoxForwardingOutbound(user, agentID, req.Payload); err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not authorized") {
					status = http.StatusForbidden
				} else if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
		} else if req.Kind == model.SingBoxActionAddNode && strings.TrimSpace(stringPayloadValue(req.Payload["outbound_tag"])) != "" {
			if err := a.enrichSingBoxNodeOutbound(user, agentID, req.Payload); err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not authorized") {
					status = http.StatusForbidden
				} else if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
		}
		enrichedRaw, err := json.Marshal(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("encode sing-box action: %v", err))
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(enrichedRaw))
	}
	a.handleXUIActions(w, r, agentID, parts)
}

// enrichSingBoxForwardingOutbound copies the source Client's unredacted
// outbound from its stored snapshot. The overview endpoint intentionally
// redacts passwords for non-root users, so accepting that response directly
// would create an unusable target configuration.
func (a *App) enrichSingBoxForwardingOutbound(user model.AdminUser, targetAgentID string, payload map[string]any) error {
	return a.enrichSingBoxOutbound(user, targetAgentID, payload, "outbound", false)
}

func (a *App) enrichSingBoxNodeOutbound(user model.AdminUser, targetAgentID string, payload map[string]any) error {
	return a.enrichSingBoxOutbound(user, targetAgentID, payload, "node", true)
}

func (a *App) enrichSingBoxOutbound(user model.AdminUser, targetAgentID string, payload map[string]any, payloadKey string, allowSameAgent bool) error {
	if payload == nil {
		return fmt.Errorf("sing-box forwarding payload is empty")
	}
	sourceAgentID := strings.TrimSpace(stringPayloadValue(payload["source_agent_id"]))
	if sourceAgentID == "" || (sourceAgentID == targetAgentID && !allowSameAgent) {
		return nil
	}
	if !a.adminCanAccessAgent(user, sourceAgentID) {
		return fmt.Errorf("source Client %s not authorized", sourceAgentID)
	}
	tag := strings.TrimSpace(stringPayloadValue(payload["outbound_tag"]))
	if tag == "" {
		if outbound, ok := payload["outbound"].(map[string]any); ok {
			tag = strings.TrimSpace(stringPayloadValue(outbound["tag"]))
		}
	}
	if tag == "" {
		return fmt.Errorf("source outbound tag is required")
	}
	snapshot, found := a.store.GetLatest(sourceAgentID)
	if !found || snapshot.SingBox == nil {
		return fmt.Errorf("source Client %s sing-box snapshot not found", sourceAgentID)
	}
	for _, outbound := range snapshot.SingBox.Nodes {
		if strings.TrimSpace(stringPayloadValue(outbound["tag"])) != tag {
			continue
		}
		copied := make(map[string]any, len(outbound))
		for key, value := range outbound {
			copied[key] = value
		}
		if targetTag := strings.TrimSpace(stringPayloadValue(payload["target_tag"])); targetTag != "" {
			copied["tag"] = targetTag
		}
		payload[payloadKey] = copied
		payload["outbound_tag"] = tag
		return nil
	}
	return fmt.Errorf("source Client %s outbound not found: %s", sourceAgentID, tag)
}

func stringPayloadValue(value any) string {
	result, _ := value.(string)
	return result
}
