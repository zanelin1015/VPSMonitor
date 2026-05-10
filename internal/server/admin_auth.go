package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

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
	writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user, System: serverSystemInfo()})
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
	writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user, System: serverSystemInfo()})
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
