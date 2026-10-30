package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func (a *App) handleXUIActions(w http.ResponseWriter, r *http.Request, agentID string, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			if user, _, ok := a.currentAdmin(r); ok {
				if !a.adminCanAccessAgent(user, agentID) {
					writeError(w, http.StatusForbidden, "agent is not assigned to this account")
					return
				}
				limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
				var actions []model.XUIAction
				var err error
				if isRootAdmin(user) {
					actions, err = a.store.ListXUIActions(agentID, limit)
				} else {
					actions, err = a.store.ListXUIActionsByActor(agentID, user.Role, user.ID, limit)
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				if !isRootAdmin(user) && !isOperatorAdmin(user) {
					actions = filterRootOnlyXUIActions(actions)
				}
				if !isRootAdmin(user) {
					for index := range actions {
						if strings.HasPrefix(actions[index].Kind, "singbox_") {
							actions[index].Payload = nil
							actions[index].Result = nil
						}
					}
				}
				writeJSON(w, http.StatusOK, actions)
				return
			}
			if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
				writeError(w, http.StatusUnauthorized, "invalid agent token")
				return
			}
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			actions, err := a.store.ClaimPendingXUIActions(agentID, limit)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			a.attachXUIActionAuth(agentID, actions)
			writeJSON(w, http.StatusOK, actions)
		case http.MethodPost:
			user, _, ok := a.requireAgentAdmin(w, r, agentID)
			if !ok {
				return
			}
			var req model.XUIActionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode x-ui action: %v", err))
				return
			}
			if req.Kind == model.XUIActionResetClientTraffic {
				writeError(w, http.StatusGone, model.LegacyTrafficResetRemovedMessage)
				return
			}
			if req.Kind == model.XUIActionUpdateClientConfig {
				writeError(w, http.StatusBadRequest, "请使用客户端统一配置接口保存")
				return
			}
			if isAreaManager(user) && !a.areaManagerXUIActionAllowed(user, agentID, req) {
				writeError(w, http.StatusForbidden, "outbound or node is outside the area manager authorization scope")
				return
			}
			if isRootOnlyXUIActionKind(req.Kind) && !isRootAdmin(user) && (!isOperatorAdmin(user) || !isOperatorXUIActionKind(req.Kind)) {
				writeError(w, http.StatusForbidden, "only root admin can create this x-ui action")
				return
			}
			if req.Kind == model.XUIActionManualResetClientTraffic {
				if _, err := model.ParseXUIManualTrafficResetPayload(req.Payload); err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				snapshot, found := a.store.GetLatest(agentID)
				if _, known := parseSemver(snapshot.Version); !found || !known || isVersionNewer(model.XUIManualTrafficResetMinVersion, snapshot.Version) {
					writeError(w, http.StatusConflict, "请先将目标 VPSMonitor Client 升级到 v"+model.XUIManualTrafficResetMinVersion+" 或更高版本，并刷新采集")
					return
				}
			}
			if req.Kind == model.XUIActionDeleteRoutingRules {
				if _, err := model.ParseRoutingRulesDeletePayload(req.Payload); err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
			}
			if req.Kind == model.XUIActionUpdateRoutingRule {
				if _, err := model.ParseRoutingRuleUpdatePayload(req.Payload); err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				snapshot, found := a.store.GetLatest(agentID)
				if !found || snapshot.XUI == nil || snapshot.XUI.RoutingTemplate == nil {
					writeError(w, http.StatusConflict, "请升级目标 VPSMonitor Client 并刷新采集，以读取面板保存的路由规则")
					return
				}
				if _, known := parseSemver(snapshot.Version); !known || isVersionNewer(model.XUIRoutingRuleUpdateMinVersion, snapshot.Version) {
					writeError(w, http.StatusConflict, "请先将目标 VPSMonitor Client 升级到 v"+model.XUIRoutingRuleUpdateMinVersion+" 或更高版本，并刷新采集")
					return
				}
			}
			if req.Kind == model.XUIActionDeleteOutbounds {
				if _, err := model.ParseOutboundsDeletePayload(req.Payload); err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				snapshot, found := a.store.GetLatest(agentID)
				if !found || snapshot.XUI == nil || snapshot.XUI.OutboundTemplate == nil {
					writeError(w, http.StatusConflict, "请升级目标 VPSMonitor Client 并刷新采集，以读取可编辑出站")
					return
				}
				if _, known := parseSemver(snapshot.Version); !known || isVersionNewer(model.XUIOutboundsDeleteMinVersion, snapshot.Version) {
					writeError(w, http.StatusConflict, "请先将目标 VPSMonitor Client 升级到 v"+model.XUIOutboundsDeleteMinVersion+" 或更高版本，并刷新采集")
					return
				}
			}
			if req.Kind == model.XUIActionUpdateClientTrafficReset {
				if _, err := model.ParseXUIClientTrafficResetPayload(req.Payload); err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				_, found, err := a.store.GetAgent(agentID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				if !found {
					writeError(w, http.StatusBadRequest, "agent not found")
					return
				}
				snapshot, _ := a.store.GetLatest(agentID)
				if _, known := parseSemver(snapshot.Version); !known || isVersionNewer(model.XUIClientTrafficResetMinVersion, snapshot.Version) {
					writeError(w, http.StatusConflict, "请先将目标 VPSMonitor Client 升级到 v"+model.XUIClientTrafficResetMinVersion+" 或更高版本，并刷新采集")
					return
				}
			}
			if isAreaManager(user) && user.OutboundCreateEnabled {
				if outboundTag := outboundTagFromPayload(req.Payload); outboundTag != "" {
					if err := a.store.UpsertAreaManagerOutboundGrant(user.ID, model.AreaManagerOutboundGrantRequest{
						AgentID:     agentID,
						OutboundTag: outboundTag,
					}); err != nil {
						writeError(w, http.StatusInternalServerError, err.Error())
						return
					}
				}
			}
			action, err := a.store.CreateXUIActionWithActor(agentID, req, model.XUIActionActor{
				Role:      user.Role,
				AccountID: user.ID,
				Username:  user.Username,
			})
			if err != nil {
				status := http.StatusInternalServerError
				if strings.Contains(err.Error(), "traffic reset is already pending") {
					status = http.StatusConflict
				} else if err.Error() == "agent not found" || strings.Contains(err.Error(), "unsupported") {
					status = http.StatusBadRequest
				}
				writeError(w, status, err.Error())
				return
			}
			action, _ = a.dispatchXUIActionRealtime(agentID, action)
			writeJSON(w, http.StatusOK, action)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "result" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
			writeError(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		actionID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || actionID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid action id")
			return
		}
		var req model.XUIActionResultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode x-ui action result: %v", err))
			return
		}
		action, err := a.store.CompleteXUIAction(agentID, actionID, req)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid") {
				status = http.StatusBadRequest
			} else if strings.Contains(err.Error(), "not executable") {
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}
		if (action.Kind == model.XUIActionDeleteRoutingRules || action.Kind == model.XUIActionDeleteOutbounds || action.Kind == model.XUIActionUpdateRoutingRule) && (action.Result["saved"] == true || action.Result["submitted"] == true) && a.realtime != nil {
			// Refresh the full saved-template snapshot, including partial success
			// where saving worked but restarting Xray did not.
			a.realtime.sendAgentControl(agentID, model.AgentControlMessage{Type: model.AgentControlCollectNow})
		}
		if (action.Kind == model.XUIActionAddClient || action.Kind == model.SingBoxActionAddNode || action.Kind == model.SingBoxActionAddClient || action.Kind == model.SingBoxActionAddUser || action.Kind == model.SingBoxActionDeleteUser || action.Kind == model.SingBoxActionAddForwardingRule) && action.Status == model.XUIActionStatusSucceeded && a.realtime != nil {
			a.realtime.sendAgentControl(agentID, model.AgentControlMessage{Type: model.AgentControlCollectNow})
		}
		if (action.Kind == model.XUIActionUpdateClientConfig || action.Kind == model.XUIActionManualResetClientTraffic) && a.realtime != nil {
			a.dispatchXUITrafficCollectionRealtime(agentID)
		}
		writeJSON(w, http.StatusOK, action)
		return
	}

	writeError(w, http.StatusNotFound, "route not found")
}

func filterRootOnlyXUIActions(actions []model.XUIAction) []model.XUIAction {
	filtered := make([]model.XUIAction, 0, len(actions))
	for _, action := range actions {
		if isRootOnlyXUIActionKind(action.Kind) {
			continue
		}
		filtered = append(filtered, action)
	}
	return filtered
}

func isRootOnlyXUIActionKind(kind string) bool {
	switch kind {
	case model.XUIActionManualResetClientTraffic, model.XUIActionExecuteCommand, model.XUIActionUpdate3XUI, model.XUIActionDeleteRoutingRules, model.XUIActionDeleteOutbounds, model.XUIActionUpdateRoutingRule, model.XUIActionUpdateClientConfig, model.SingBoxActionAddNode, model.SingBoxActionAddClient, model.SingBoxActionAddUser, model.SingBoxActionDeleteUser, model.SingBoxActionAddForwardingRule, model.SingBoxActionReload:
		return true
	default:
		return false
	}
}

func isOperatorXUIActionKind(kind string) bool {
	switch kind {
	case model.XUIActionManualResetClientTraffic, model.XUIActionUpdateClientConfig:
		return true
	default:
		return false
	}
}

func realtimeXUIActionAllowed(kind string) bool {
	switch kind {
	case model.XUIActionAddOutbound,
		model.XUIActionAddClient,
		model.XUIActionAddRoutingRule,
		model.XUIActionUpsertRoutingRule,
		model.XUIActionUpdateRoutingRule,
		model.XUIActionDeleteRoutingRules,
		model.XUIActionDeleteOutbounds,
		model.XUIActionUpdateClientExpiry,
		model.XUIActionUpdateClientTraffic,
		model.XUIActionUpdateClientTrafficReset,
		model.XUIActionManualResetClientTraffic,
		model.XUIActionUpdateClientConfig,
		model.XUIActionSetClientEnabled,
		model.XUIActionDeleteClient,
		model.XUIActionUpdateClient,
		model.XUIActionExecuteCommand,
		model.XUIActionUpdate3XUI:
		return true
	case model.SingBoxActionAddNode,
		model.SingBoxActionAddClient,
		model.SingBoxActionAddUser,
		model.SingBoxActionDeleteUser,
		model.SingBoxActionAddForwardingRule,
		model.SingBoxActionReload:
		return true
	default:
		return false
	}
}

func (a *App) dispatchXUIActionRealtime(agentID string, action model.XUIAction) (model.XUIAction, bool) {
	control := model.AgentControlMessage{
		ActionID: action.ID,
	}
	switch {
	case action.Kind == model.XUIActionRestartXUI:
		control.Type = model.AgentControlRestartXUI
	case realtimeXUIActionAllowed(action.Kind):
		control.Type = model.AgentControlExecuteXUI
		control.Kind = action.Kind
	default:
		return action, false
	}
	running, claimed, err := a.store.ClaimXUIAction(agentID, action.ID)
	if err != nil || !claimed {
		return action, false
	}
	action = running
	control.Payload = action.Payload
	control.XUIAuth = a.xuiActionAuth(agentID, action.Kind)
	if !a.realtime.sendAgentControl(agentID, control) {
		if releaseErr := a.store.ReleaseXUIAction(agentID, action.ID); releaseErr != nil {
			log.Printf("release undelivered x-ui action %d failed: %v", action.ID, releaseErr)
		}
		return action, false
	}
	return running, true
}

func (a *App) attachXUIActionAuth(agentID string, actions []model.XUIAction) {
	var auth *model.XUIActionAuth
	for index := range actions {
		if !xuiActionUsesPanelAuth(actions[index].Kind) {
			continue
		}
		if auth == nil {
			auth = a.xuiActionAuth(agentID, actions[index].Kind)
		}
		if auth != nil {
			actions[index].XUIAuth = &model.XUIActionAuth{APIToken: auth.APIToken}
		}
	}
}

func (a *App) xuiActionAuth(agentID, kind string) *model.XUIActionAuth {
	if !xuiActionUsesPanelAuth(kind) {
		return nil
	}
	cfg, found, err := a.store.GetAgentConfig(agentID)
	if err != nil || !found || strings.TrimSpace(cfg.XUI.APIToken) == "" {
		return nil
	}
	return &model.XUIActionAuth{APIToken: cfg.XUI.APIToken}
}

func (a *App) dispatchXUITrafficCollectionRealtime(agentID string) {
	if a == nil || a.realtime == nil || a.store == nil {
		return
	}
	cfg, found, err := a.store.GetAgentConfig(agentID)
	if err != nil || !found || !cfg.XUI.Enabled {
		return
	}
	var auth *model.XUIActionAuth
	if token := strings.TrimSpace(cfg.XUI.APIToken); token != "" {
		auth = &model.XUIActionAuth{APIToken: token}
	}
	a.realtime.sendAgentControl(agentID, model.AgentControlMessage{
		Type:    model.AgentControlCollectXUI,
		XUIAuth: auth,
	})
}

func xuiActionUsesPanelAuth(kind string) bool {
	switch kind {
	case model.XUIActionAddOutbound,
		model.XUIActionAddClient,
		model.XUIActionAddRoutingRule,
		model.XUIActionUpsertRoutingRule,
		model.XUIActionUpdateRoutingRule,
		model.XUIActionDeleteRoutingRules,
		model.XUIActionDeleteOutbounds,
		model.XUIActionUpdateClientExpiry,
		model.XUIActionUpdateClientTraffic,
		model.XUIActionUpdateClientTrafficReset,
		model.XUIActionManualResetClientTraffic,
		model.XUIActionUpdateClientConfig,
		model.XUIActionSetClientEnabled,
		model.XUIActionDeleteClient:
		return true
	default:
		return false
	}
}

func (a *App) dispatchPendingXUIActionsRealtime(agentID string) {
	if _, err := a.store.ExpireStaleXUIActions(agentID, store.XUIActionExecutionLease); err != nil {
		log.Printf("expire stale x-ui actions for %s: %v", agentID, err)
	}
	actions, err := a.store.ListXUIActions(agentID, 100)
	if err != nil {
		return
	}
	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if action.Status != model.XUIActionStatusPending {
			continue
		}
		a.dispatchXUIActionRealtime(agentID, action)
	}
}
