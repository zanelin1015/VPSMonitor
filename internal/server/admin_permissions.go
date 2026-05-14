package server

import (
	"net/http"
	"strings"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func (a *App) requireAgentAdmin(w http.ResponseWriter, r *http.Request, agentID string) (model.AdminUser, string, bool) {
	user, token, ok := a.requireAdmin(w, r)
	if !ok {
		return model.AdminUser{}, "", false
	}
	if !a.adminCanAccessAgent(user, agentID) {
		writeError(w, http.StatusForbidden, "agent is not assigned to this account")
		return model.AdminUser{}, "", false
	}
	return user, token, true
}

func (a *App) adminCanAccessAgent(user model.AdminUser, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	if isRootAdmin(user) {
		return true
	}
	for _, allowed := range user.AgentIDs {
		if allowed == agentID {
			return true
		}
	}
	return false
}

func (a *App) filterAgentRecordsForAdmin(user model.AdminUser, agents []model.AgentRecord) []model.AgentRecord {
	if isRootAdmin(user) {
		return agents
	}
	allowed := adminAgentSet(user)
	filtered := make([]model.AgentRecord, 0, len(agents))
	for _, agent := range agents {
		if _, ok := allowed[agent.AgentID]; ok {
			filtered = append(filtered, agent)
		}
	}
	return filtered
}

func (a *App) filterSnapshotsForAdmin(user model.AdminUser, snapshots []model.AgentSnapshot) []model.AgentSnapshot {
	if isRootAdmin(user) {
		return snapshots
	}
	allowed := adminAgentSet(user)
	filtered := make([]model.AgentSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if _, ok := allowed[snapshot.AgentID]; ok {
			filtered = append(filtered, snapshot)
		}
	}
	return filtered
}

func (a *App) filterRealtimeMetricsForAdmin(user model.AdminUser, metrics []model.AgentRealtimeMetrics) []model.AgentRealtimeMetrics {
	if isRootAdmin(user) {
		return metrics
	}
	allowed := adminAgentSet(user)
	filtered := make([]model.AgentRealtimeMetrics, 0, len(metrics))
	for _, metric := range metrics {
		if _, ok := allowed[metric.AgentID]; ok {
			filtered = append(filtered, metric)
		}
	}
	return filtered
}

func (a *App) sanitizeManagedConfigForAdmin(user model.AdminUser, cfg model.ManagedAgentConfig) model.ManagedAgentConfig {
	if isRootAdmin(user) {
		return cfg
	}
	cfg.XUI = config.XUIConfig{
		Enabled:       cfg.XUI.Enabled,
		BaseURL:       cfg.XUI.BaseURL,
		SkipTLSVerify: cfg.XUI.SkipTLSVerify,
	}
	return cfg
}

func (a *App) customerVisibleToAdmin(user model.AdminUser, customerID int64) (bool, error) {
	if isRootAdmin(user) {
		_, found, err := a.store.GetCustomer(customerID)
		return found, err
	}
	return a.store.CustomerOwnedBy(customerID, model.AdminRoleAreaManager, user.ID)
}

func (a *App) areaManagerXUIActionAllowed(kind string) bool {
	switch strings.TrimSpace(kind) {
	case model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule:
		return true
	default:
		return false
	}
}

func adminAgentSet(user model.AdminUser) map[string]struct{} {
	set := make(map[string]struct{}, len(user.AgentIDs))
	for _, agentID := range user.AgentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID != "" {
			set[agentID] = struct{}{}
		}
	}
	return set
}
