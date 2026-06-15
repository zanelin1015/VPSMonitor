package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

type realmConfigCopyRequest struct {
	TargetAgentID string `json:"target_agent_id"`
}

type realmConfigCopyResponse struct {
	SourceAgentID string                   `json:"source_agent_id"`
	TargetAgentID string                   `json:"target_agent_id"`
	Config        model.RealmForwardConfig `json:"config"`
	ApplySent     bool                     `json:"apply_sent"`
}

func (a *App) handleRealmConfigCopy(w http.ResponseWriter, r *http.Request, sourceAgentID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireRootAdmin(w, r)
	if !ok {
		return
	}
	var req realmConfigCopyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode realm copy request: %v", err))
		return
	}
	targetAgentID := strings.TrimSpace(req.TargetAgentID)
	if targetAgentID == "" {
		writeError(w, http.StatusBadRequest, "target_agent_id is required")
		return
	}
	if targetAgentID == sourceAgentID {
		writeError(w, http.StatusBadRequest, "target_agent_id must be different from source agent")
		return
	}

	sourceCfg, found, err := a.store.GetAgentConfig(sourceAgentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "source agent not found")
		return
	}
	sourceForwarding := sourceCfg.Entry.PortForwarding
	if snapshot, exists := a.store.GetLatest(sourceAgentID); exists {
		sourceForwarding = dashboard.MergeRealmSnapshotIntoEntry(sourceCfg.Entry, snapshot.Realm).PortForwarding
	}
	if !hasCopyableRealmForwarding(sourceForwarding) {
		writeError(w, http.StatusBadRequest, "source agent has no realm forwarding config to copy")
		return
	}

	targetCfg, found, err := a.store.GetAgentConfig(targetAgentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "target agent not found")
		return
	}
	targetCfg.Entry.PortForwarding = sourceForwarding
	targetCfg.Features.Realm = true
	targetCfg.Features.Configured = true
	targetCfg = inferLegacyAgentFeatures(targetCfg, nil)
	record, err := a.store.UpdateAgentConfigWithActor(targetAgentID, targetCfg, user.Username+":realm-copy:"+sourceAgentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.clearCustomerOverviewCache()
	applySent := a.requestAgentConfigApply(targetAgentID, record.Config)
	writeJSON(w, http.StatusOK, realmConfigCopyResponse{
		SourceAgentID: sourceAgentID,
		TargetAgentID: targetAgentID,
		Config:        record.Config.Entry.PortForwarding,
		ApplySent:     applySent,
	})
}

func hasCopyableRealmForwarding(cfg model.RealmForwardConfig) bool {
	return cfg.Enabled ||
		strings.TrimSpace(cfg.Backend) != "" ||
		strings.TrimSpace(cfg.BinaryPath) != "" ||
		strings.TrimSpace(cfg.ConfigPath) != "" ||
		strings.TrimSpace(cfg.ServiceName) != "" ||
		len(cfg.Rules) > 0
}
