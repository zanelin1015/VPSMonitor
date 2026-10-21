package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func (a *App) handleXUIClientConfig(w http.ResponseWriter, r *http.Request, agentID string, parts []string) {
	user, _, ok := a.requireAgentAdmin(w, r, agentID)
	if !ok {
		return
	}
	if !isRootAdmin(user) && !isOperatorAdmin(user) {
		writeError(w, http.StatusForbidden, "当前账号没有客户端配置管理权限")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet && len(parts) == 1 {
		save, found, err := a.store.GetXUIClientConfigSave(agentID, parts[0])
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "save record not found")
			return
		}
		if save.Action != nil {
			if _, err := a.store.ExpireStaleXUIActions(save.Action.AgentID, store.XUIActionExecutionLease); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			save, _, err = a.store.GetXUIClientConfigSave(agentID, parts[0])
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, save)
		return
	}
	if r.Method != http.MethodPost || len(parts) != 0 {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req model.XUIClientConfigRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	snapshot, found := a.store.GetLatest(agentID)
	if !found {
		writeError(w, http.StatusConflict, "请先刷新 Client 采集，再编辑客户端")
		return
	}
	overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{Entry: cfg.Entry})
	if overview == nil {
		overview = emptyAgentXUIOverview(snapshot, cfg)
	}
	a.appendForwardedImportURLs(agentID, overview)
	var client *model.XUIClientView
	for i := range overview.Clients {
		c := &overview.Clients[i]
		if c.InboundID == req.InboundID && c.Email == req.Email && (req.InboundTag == "" || c.InboundTag == req.InboundTag) && (req.ClientID == "" || c.ClientID == req.ClientID) {
			client = c
			break
		}
	}
	if client == nil {
		writeError(w, http.StatusConflict, "客户端或关联节点已变化，请刷新后重新编辑")
		return
	}
	target := model.XUIClientConfigTarget{AgentID: agentID, InboundID: client.InboundID, InboundTag: client.InboundTag, ClientID: firstNonEmptyString(client.ClientID, client.AuthUUID), Email: client.Email}
	if client.RealmTargetAgentID != "" {
		target.AgentID, target.InboundID, target.InboundTag = client.RealmTargetAgentID, client.RealmTargetInboundID, client.RealmTargetInboundTag
	}
	req.ClientID, req.InboundTag = client.ClientID, client.InboundTag
	if !req.Changes.Empty() {
		switch strings.ToLower(client.Protocol) {
		case "http", "socks", "socks5":
			writeError(w, http.StatusBadRequest, "节点账号仅支持修改本系统收费配置")
			return
		}
		targetSnapshot, known := a.store.GetLatest(target.AgentID)
		if !known {
			writeError(w, http.StatusConflict, "请先刷新目标 Client 采集")
			return
		}
		if _, known := parseSemver(targetSnapshot.Version); !known || isVersionNewer(model.XUIClientConfigMinVersion, targetSnapshot.Version) {
			writeError(w, http.StatusConflict, "统一保存需要目标 VPSMonitor Client v"+model.XUIClientConfigMinVersion+" 或更高版本，请先升级并刷新采集")
			return
		}
		if req.Changes.TrafficReset != nil && !client.TrafficResetSupported {
			writeError(w, http.StatusConflict, "当前 x-ui 未上报客户端自动重置能力，请刷新采集或升级面板")
			return
		}
		if req.Changes.ExpiryTime != nil && *req.Changes.ExpiryTime != req.Billing.ExpireTime {
			writeError(w, http.StatusBadRequest, "下发到期时间必须与收费配置一致")
			return
		}
	}
	save, err := a.store.SaveXUIClientConfig(agentID, target, req, model.XUIActionActor{Role: user.Role, AccountID: user.ID, Username: user.Username})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrClientConfigConflict) || errors.Is(err, store.ErrClientConfigPending) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	if save.Action != nil && save.Action.Status == model.XUIActionStatusPending && a.realtime != nil {
		action, _ := a.dispatchXUIActionRealtime(save.Action.AgentID, *save.Action)
		save.Action = &action
	}
	a.clearCustomerOverviewCache()
	writeJSON(w, http.StatusOK, save)
}
