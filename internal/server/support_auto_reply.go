package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAdminSupportAutoReply(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	scopeType, scopeID, scopeName, allowed := supportAutoReplyAdminScope(user)
	if !allowed {
		writeError(w, http.StatusForbidden, "当前账号没有自动回复配置权限")
		return
	}
	global, _, err := a.store.GetSupportAutoReply(model.SupportAutoReplyScopeGlobal, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings, _, err := a.store.GetSupportAutoReply(scopeType, scopeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if r.Method == http.MethodGet {
		effective := global
		if scopeType == model.SupportAutoReplyScopeArea && (settings.Mode == model.SupportAutoReplyCustom || settings.Mode == model.SupportAutoReplyDisabled) {
			effective = settings
		}
		if scopeType == model.SupportAutoReplyScopeGlobal {
			effective = settings
		}
		writeJSON(w, http.StatusOK, model.SupportAutoReplyConfigResponse{
			ScopeType: scopeType, ScopeID: scopeID, ScopeName: scopeName,
			Settings: settings, Global: global, Effective: effective,
		})
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req model.SupportAutoReplyConfigRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode auto reply config: %v", err))
		return
	}
	saved, err := a.store.SaveSupportAutoReply(model.SupportAutoReplySettings{
		ScopeType: scopeType, ScopeID: scopeID, Mode: strings.TrimSpace(req.Mode),
		SenderName: req.SenderName, Message: req.Message,
		UpdatedByRole: user.Role, UpdatedByAccountID: user.ID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	global, _, err = a.store.GetSupportAutoReply(model.SupportAutoReplyScopeGlobal, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	effective := global
	if scopeType == model.SupportAutoReplyScopeGlobal || saved.Mode == model.SupportAutoReplyCustom || saved.Mode == model.SupportAutoReplyDisabled {
		effective = saved
	}
	writeJSON(w, http.StatusOK, model.SupportAutoReplyConfigResponse{
		ScopeType: scopeType, ScopeID: scopeID, ScopeName: scopeName,
		Settings: saved, Global: global, Effective: effective,
	})
}

func supportAutoReplyAdminScope(user model.AdminUser) (string, int64, string, bool) {
	if isAreaManager(user) {
		if user.ID <= 0 {
			return "", 0, "", false
		}
		return model.SupportAutoReplyScopeArea, user.ID, firstNonEmptyString(user.DisplayName, user.Username), true
	}
	if isRootAdmin(user) || isOperatorAdmin(user) {
		return model.SupportAutoReplyScopeGlobal, 0, "全局管理员", true
	}
	return "", 0, "", false
}

func (a *App) guestSupportAutoReply(user model.CustomerUser) (*model.SupportAutoReplySettings, error) {
	global, _, err := a.store.GetSupportAutoReply(model.SupportAutoReplyScopeGlobal, 0)
	if err != nil {
		return nil, err
	}
	if user.OwnerType == model.AdminRoleAreaManager && user.OwnerID > 0 {
		area, _, err := a.store.GetSupportAutoReply(model.SupportAutoReplyScopeArea, user.OwnerID)
		if err != nil {
			return nil, err
		}
		switch area.Mode {
		case model.SupportAutoReplyDisabled:
			return nil, nil
		case model.SupportAutoReplyCustom:
			return &area, nil
		}
	}
	if global.Mode == model.SupportAutoReplyCustom && strings.TrimSpace(global.Message) != "" {
		return &global, nil
	}
	return nil, nil
}
