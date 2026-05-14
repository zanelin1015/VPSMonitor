package server

import (
	"net/http"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/"), "/")
	if strings.HasPrefix(path, "telegram-bots") {
		a.handleAdminTelegramBots(w, r, strings.Split(path, "/")[1:])
		return
	}
	if path == "area-managers" || strings.HasPrefix(path, "area-managers/") {
		a.handleAdminAreaManagers(w, r, strings.Split(path, "/")[1:])
		return
	}
	if path == "customers" || strings.HasPrefix(path, "customers/") {
		a.handleAdminCustomers(w, r, strings.Split(path, "/")[1:])
		return
	}
	if path == "area-agent-tags" || strings.HasPrefix(path, "area-agent-tags/") {
		a.handleAdminAreaAgentTags(w, r, strings.Split(path, "/")[1:])
		return
	}
	if strings.HasPrefix(path, "updates") {
		a.handleAdminUpdates(w, r, strings.Split(path, "/")[1:])
		return
	}
	switch path {
	case "login":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminLogin(w, r)
	case "session":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		user, _, ok := a.requireAdmin(w, r)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user, System: serverSystemInfo()})
	case "logout":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminLogout(w, r)
	case "account":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminAccountUpdate(w, r)
	case "audit":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleConfigAuditLogs(w, r)
	case "client-install":
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleClientInstallInfo(w, r)
	case "tags":
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminTags(w, r)
	case "frontend-settings":
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminFrontendSettings(w, r)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
