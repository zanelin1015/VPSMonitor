package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (a *App) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/"), "/")
	if strings.HasPrefix(path, "telegram-bots") {
		a.handleAdminTelegramBots(w, r, strings.Split(path, "/")[1:])
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
		writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user})
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
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req model.AdminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode login request: %v", err))
		return
	}
	user, ok, err := a.store.AuthenticateAdmin(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	token, session, err := a.store.CreateAdminSession(user.Username, adminSessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setAdminSessionCookie(w, r, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user})
}

func (a *App) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if token := readAdminSessionToken(r); token != "" {
		_ = a.store.DeleteAdminSession(token)
	}
	clearAdminSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *App) handleAdminAccountUpdate(w http.ResponseWriter, r *http.Request) {
	_, token, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var req model.AdminAccountUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode account request: %v", err))
		return
	}
	user, err := a.store.UpdateAdminAccount(req, token)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not initialized") {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user})
}

func (a *App) handleAdminTelegramBots(w http.ResponseWriter, r *http.Request, parts []string) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			bots, err := a.store.ListTelegramBots()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, bots)
		case http.MethodPost:
			var req model.TelegramBotRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode telegram bot: %v", err))
				return
			}
			bot, err := a.store.CreateTelegramBot(req)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, bot)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid telegram bot id")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			var req model.TelegramBotRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode telegram bot: %v", err))
				return
			}
			bot, err := a.store.UpdateTelegramBot(id, req)
			if err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, bot)
		case http.MethodDelete:
			if err := a.store.DeleteTelegramBot(id); err != nil {
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
		return
	}

	if len(parts) == 2 && parts[1] == "test" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		bot, found, err := a.store.GetTelegramBotSecret(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "telegram bot not found")
			return
		}
		text := fmt.Sprintf("✅ NanFengMonitor Telegram 测试消息\n机器人：%s\n时间：%s", bot.Name, time.Now().Format("2006-01-02 15:04:05"))
		if err := a.alerts.sendTelegramMessage(bot.BotToken, bot.ChatID, text); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
		return
	}

	writeError(w, http.StatusNotFound, "route not found")
}

func (a *App) handleConfigAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	agentID := r.URL.Query().Get("agent_id")
	items, err := a.store.ListConfigAuditLogs(agentID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleClientInstallInfo(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	if r.Method == http.MethodPut {
		var req model.ClientInstallSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode client install settings: %v", err))
			return
		}
		settings, err := validateClientInstallSettings(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := a.store.SaveClientInstallSettings(settings)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a.clientInstallInfo(r, saved))
		return
	}

	settings, found, err := a.store.GetClientInstallSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		settings = model.ClientInstallSettingsRequest{}
	}
	writeJSON(w, http.StatusOK, a.clientInstallInfo(r, settings))
}

const defaultClientInstallScriptURL = "https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh"

func (a *App) clientInstallInfo(r *http.Request, settings model.ClientInstallSettingsRequest) model.ClientInstallInfo {
	serverURL := firstNonEmptyString(settings.ServerURL, requestPublicBaseURL(r))
	installScriptURL := firstNonEmptyString(settings.InstallScriptURL, defaultClientInstallScriptURL)
	pollInterval := firstNonEmptyString(settings.PollInterval, "30s")
	requestTimeoutSeconds := settings.RequestTimeoutSeconds
	if requestTimeoutSeconds <= 0 {
		requestTimeoutSeconds = 15
	}
	return model.ClientInstallInfo{
		ServerURL:             serverURL,
		RegistrationToken:     a.config.RegistrationToken,
		InstallScriptURL:      installScriptURL,
		PollInterval:          pollInterval,
		RequestTimeoutSeconds: requestTimeoutSeconds,
		ServerSkipTLSVerify:   settings.ServerSkipTLSVerify,
	}
}

func validateClientInstallSettings(req model.ClientInstallSettingsRequest) (model.ClientInstallSettingsRequest, error) {
	req.ServerURL = strings.TrimSpace(req.ServerURL)
	req.InstallScriptURL = strings.TrimSpace(req.InstallScriptURL)
	req.PollInterval = strings.TrimSpace(req.PollInterval)
	if req.ServerURL == "" {
		return req, fmt.Errorf("server url is required")
	}
	if err := validateHTTPURL(req.ServerURL, "server url"); err != nil {
		return req, err
	}
	if req.InstallScriptURL == "" {
		return req, fmt.Errorf("install script url is required")
	}
	if err := validateHTTPURL(req.InstallScriptURL, "install script url"); err != nil {
		return req, err
	}
	if req.PollInterval == "" {
		req.PollInterval = "30s"
	}
	if d, err := time.ParseDuration(req.PollInterval); err != nil || d <= 0 {
		return req, fmt.Errorf("poll interval must be a positive Go duration, e.g. 30s")
	}
	if req.RequestTimeoutSeconds <= 0 {
		req.RequestTimeoutSeconds = 15
	}
	return req, nil
}

func validateHTTPURL(value, label string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be a valid URL", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must start with http:// or https://", label)
	}
	return nil
}

func requestPublicBaseURL(r *http.Request) string {
	proto := firstNonEmptyString(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0], r.Header.Get("X-Forwarded-Protocol"), r.Header.Get("X-Scheme"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := firstNonEmptyString(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0], r.Host)
	if host == "" {
		host = "SERVER_IP:8090"
	}
	return strings.TrimRight(strings.TrimSpace(proto), ":/") + "://" + strings.TrimSpace(host)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (model.AdminUser, string, bool) {
	user, token, ok := a.currentAdmin(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "admin login required")
		return model.AdminUser{}, "", false
	}
	return user, token, true
}

func (a *App) currentAdmin(r *http.Request) (model.AdminUser, string, bool) {
	token := readAdminSessionToken(r)
	user, _, ok, err := a.store.ValidateAdminSession(token)
	if err != nil || !ok {
		return model.AdminUser{}, "", false
	}
	return user, token, true
}

func readAdminSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setAdminSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

func clearAdminSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
