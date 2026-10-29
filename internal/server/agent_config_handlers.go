package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (a *App) handleAgentRecord(w http.ResponseWriter, r *http.Request, agentID string) {
	user, _, ok := a.requireAgentAdmin(w, r, agentID)
	if !ok {
		return
	}
	agent, found, err := a.store.GetAgent(agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if snapshot, ok := a.store.GetLatest(agentID); ok {
		agent.Summary = snapshot.Summary
		agent.ReportedAt = &snapshot.ReportedAt
		agent.Version = snapshot.Version
		agent.OS = snapshot.OS
		agent.Arch = snapshot.Arch
		agent.SystemVersion = snapshot.SystemVersion
		agent.Config.Entry = dashboard.MergeRealmSnapshotIntoEntry(agent.Config.Entry, snapshot.Realm)
		agent.Config = inferLegacyAgentFeatures(agent.Config, &snapshot)
	} else {
		agent.Config = inferLegacyAgentFeatures(agent.Config, nil)
	}
	agent = a.sanitizeAgentRecordForAdmin(user, agent)
	writeJSON(w, http.StatusOK, agent)
}

func (a *App) handleAgentConfig(w http.ResponseWriter, r *http.Request, agentID string) {
	switch r.Method {
	case http.MethodGet:
		if !a.isConfigReadAuthorized(agentID, r) {
			writeError(w, http.StatusUnauthorized, "not authorized to read config")
			return
		}
		cfg, found, err := a.store.GetAgentConfig(agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		if user, _, ok := a.currentAdmin(r); ok {
			if snapshot, exists := a.store.GetLatest(agentID); exists {
				cfg.Entry = dashboard.MergeRealmSnapshotIntoEntry(cfg.Entry, snapshot.Realm)
				cfg = inferLegacyAgentFeatures(cfg, &snapshot)
			} else {
				cfg = inferLegacyAgentFeatures(cfg, nil)
			}
			cfg = a.hydrateRealmForwardTargets(cfg)
			cfg, _ = a.hydrateHAProxyTargets(cfg)
			cfg = a.sanitizeManagedConfigForAdmin(user, cfg)
		} else {
			if snapshot, exists := a.store.GetLatest(agentID); exists {
				cfg = inferLegacyAgentFeatures(cfg, &snapshot)
			} else {
				cfg = inferLegacyAgentFeatures(cfg, nil)
			}
			cfg = a.hydrateRealmForwardTargets(cfg)
			cfg, _ = a.hydrateHAProxyTargets(cfg)
		}
		cfg = disableXUIAutoInstall(cfg)
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		user, _, ok := a.requireRootAdmin(w, r)
		if !ok {
			return
		}
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode config: %v", err))
			return
		}
		body, err := json.Marshal(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode config: %v", err))
			return
		}
		var cfg model.ManagedAgentConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode config: %v", err))
			return
		}
		cfg.AgentID = agentID
		if _, ok := raw["customer_display_name"]; !ok {
			existing, found, err := a.store.GetAgentConfig(agentID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !found {
				writeError(w, http.StatusNotFound, "agent not found")
				return
			}
			cfg.CustomerDisplayName = existing.CustomerDisplayName
		}
		cfg = disableXUIAutoInstall(cfg)
		if err := model.ValidateEntryConnection(cfg.Entry); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg = a.hydrateRealmForwardTargets(cfg)
		if featuresRaw, ok := raw["features"]; ok {
			var featureFields map[string]json.RawMessage
			if json.Unmarshal(featuresRaw, &featureFields) == nil {
				_, cfg.Features.RealmExplicitlyConfigured = featureFields["realm"]
				_, cfg.Features.HAProxyExplicitlyConfigured = featureFields["haproxy"]
			}
		}
		cfg, err = a.hydrateHAProxyTargets(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateForwardingFeatureSelection(cfg.Features); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateHAProxyConfig(cfg.Entry.HAProxy, cfg.Entry.PortForwarding); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateRealmForwardTargets(r.Context(), cfg.Entry.PortForwarding); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.validateHAProxyTargetCompatibility(cfg.Entry.HAProxy); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := a.store.UpdateAgentConfigWithActor(agentID, cfg, user.Username)
		if err != nil {
			if err.Error() == "agent not found" {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.clearCustomerOverviewCache()
		a.requestAgentConfigApply(agentID, record.Config)
		writeJSON(w, http.StatusOK, disableXUIAutoInstall(record.Config))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func disableXUIAutoInstall(cfg model.ManagedAgentConfig) model.ManagedAgentConfig {
	cfg.XUI.AutoInstall = false
	cfg.XUI.InstallScriptURL = ""
	cfg.XUI.PanelPort = 0
	cfg.XUI.WebPath = ""
	return cfg
}

func inferLegacyAgentFeatures(cfg model.ManagedAgentConfig, snapshot *model.AgentSnapshot) model.ManagedAgentConfig {
	if cfg.Features.Configured {
		if !cfg.Features.SingBox && legacySingBoxConfigPresent(cfg) {
			cfg.Features.SingBox = true
		}
		return cfg
	}
	entry := cfg.Entry
	cfg.Features = model.AgentFeatureConfig{
		XUI:     legacyXUIConfigPresent(cfg) || snapshotHasXUI(snapshot),
		SingBox: legacySingBoxConfigPresent(cfg) || snapshotHasSingBox(snapshot),
		Realm: entry.PortForwarding.Enabled ||
			len(entry.PortForwarding.Rules) > 0 ||
			strings.TrimSpace(entry.PortForwarding.ConfigPath) != "" ||
			strings.TrimSpace(entry.PortForwarding.ServiceName) != "" ||
			snapshotHasRealm(snapshot),
		HAProxy: entry.HAProxy.Enabled ||
			len(entry.HAProxy.Rules) > 0 ||
			strings.TrimSpace(entry.HAProxy.ConfigPath) != "" ||
			strings.TrimSpace(entry.HAProxy.ServiceName) != "",
		NAT: entry.Connection != (model.EntryConnectionConfig{}) || len(entry.Mappings) > 0 ||
			len(entry.Addresses) > 0 ||
			strings.TrimSpace(entry.ImportDomain) != "",
		PortPolicy: entry.NetworkPolicy.Enabled ||
			len(entry.NetworkPolicy.Rules) > 0 ||
			strings.TrimSpace(entry.NetworkPolicy.Interface) != "",
	}
	return cfg
}

func legacyXUIConfigPresent(cfg model.ManagedAgentConfig) bool {
	xui := cfg.XUI
	return xui.Enabled ||
		strings.TrimSpace(xui.BaseURL) != "" ||
		strings.TrimSpace(xui.DBPath) != "" ||
		strings.TrimSpace(xui.APIToken) != "" ||
		strings.TrimSpace(xui.Username) != ""
}

func legacySingBoxConfigPresent(cfg model.ManagedAgentConfig) bool {
	singBox := cfg.SingBox
	return singBox.Enabled || strings.TrimSpace(singBox.ConfigPath) != "" || strings.TrimSpace(singBox.BinaryPath) != "" || strings.TrimSpace(singBox.ServiceName) != ""
}

func snapshotHasXUI(snapshot *model.AgentSnapshot) bool {
	if snapshot == nil || snapshot.XUI == nil {
		return false
	}
	xui := snapshot.XUI
	return strings.TrimSpace(xui.BaseURL) != "" ||
		len(xui.Inbounds) > 0 ||
		len(xui.Outbounds) > 0 ||
		len(xui.RoutingRules) > 0 ||
		len(xui.Certificates) > 0 ||
		snapshot.Summary.InboundCount > 0 ||
		snapshot.Summary.OutboundCount > 0 ||
		snapshot.Summary.RoutingRuleCount > 0
}

func snapshotHasSingBox(snapshot *model.AgentSnapshot) bool {
	return snapshot != nil && snapshot.SingBox != nil && (len(snapshot.SingBox.Nodes) > 0 || len(snapshot.SingBox.Clients) > 0 || len(snapshot.SingBox.ForwardingRules) > 0 || snapshot.SingBox.Error == "")
}

func snapshotHasRealm(snapshot *model.AgentSnapshot) bool {
	if snapshot == nil || snapshot.Realm == nil {
		return false
	}
	realm := snapshot.Realm
	return len(realm.Rules) > 0 ||
		strings.TrimSpace(realm.ConfigPath) != "" ||
		strings.TrimSpace(realm.ServiceName) != "" ||
		strings.TrimSpace(realm.BinaryPath) != ""
}

func (a *App) requestAgentConfigApply(agentID string, cfg model.ManagedAgentConfig) bool {
	if a.realtime == nil {
		return false
	}
	cfg.AgentID = agentID
	applySent := a.realtime.sendAgentControl(agentID, model.AgentControlMessage{Type: model.AgentControlApplyConfig, Config: &cfg})
	// Older clients do not understand apply_config; collect_now keeps immediate apply compatible.
	collectSent := a.realtime.sendAgentControl(agentID, model.AgentControlMessage{Type: model.AgentControlCollectNow})
	if applySent || collectSent {
		return true
	}
	log.Printf("client %s realtime connection is offline; config will apply on next poll", agentID)
	return false
}

func (a *App) isConfigReadAuthorized(agentID string, r *http.Request) bool {
	if user, _, ok := a.currentAdmin(r); ok {
		return a.adminCanAccessAgent(user, agentID)
	}
	return a.isAuthorized(agentID, r.Header.Get("X-Agent-Token"))
}
