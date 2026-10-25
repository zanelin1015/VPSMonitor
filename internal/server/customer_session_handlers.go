package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"bridge-core/internal/model"
)

func (a *App) handleCustomerLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req model.CustomerLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode customer login request: %v", err))
		return
	}
	ipKey, accountKey := a.loginAttemptKeys(r, "customer", req.Username)
	if !a.loginLimiter.allowed(ipKey, accountKey) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	user, ok, err := a.store.AuthenticateCustomer(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		a.loginLimiter.failure(ipKey, accountKey)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	a.loginLimiter.success(accountKey)
	pair, err := a.store.CreateCustomerSessionPair(user.ID, customerSessionTTL, customerRefreshTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setCustomerSessionCookie(w, r, pair.AccessToken, pair.AccessExpiresAt)
	a.setCustomerRefreshCookie(w, r, pair.RefreshToken, pair.RefreshExpiresAt)
	writeJSON(w, http.StatusOK, model.CustomerLoginResponse{User: user})
}

func (a *App) handleCustomerRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !sameOriginWebSocket(r) {
		writeError(w, http.StatusForbidden, "cross-origin refresh is not allowed")
		return
	}
	user, pair, ok, err := a.store.RefreshCustomerSession(readCustomerRefreshToken(r), customerSessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		a.clearCustomerSessionCookie(w, r)
		a.clearCustomerRefreshCookie(w, r)
		writeError(w, http.StatusUnauthorized, "customer login required")
		return
	}
	a.setCustomerSessionCookie(w, r, pair.AccessToken, pair.AccessExpiresAt)
	writeJSON(w, http.StatusOK, model.CustomerLoginResponse{User: user})
}

func (a *App) handleCustomerLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := a.store.RevokeCustomerSessionPair(readCustomerSessionToken(r), readCustomerRefreshToken(r)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.clearCustomerSessionCookie(w, r)
	a.clearCustomerRefreshCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *App) requireCustomer(w http.ResponseWriter, r *http.Request) (model.CustomerUser, string, bool) {
	user, token, ok := a.currentCustomer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "customer login required")
		return model.CustomerUser{}, "", false
	}
	return user, token, true
}

func (a *App) currentCustomer(r *http.Request) (model.CustomerUser, string, bool) {
	token := readCustomerSessionToken(r)
	user, _, ok, err := a.store.ValidateCustomerSession(token)
	if err != nil || !ok {
		return model.CustomerUser{}, "", false
	}
	return user, token, true
}

func readCustomerSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(customerSessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func readCustomerRefreshToken(r *http.Request) string {
	cookie, err := r.Cookie(customerRefreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (a *App) setCustomerRefreshCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     customerRefreshCookieName,
		Value:    token,
		Path:     requestPublicPrefix(r) + "/api/v1/customer/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.isSecureRequest(r),
	})
}

func (a *App) clearCustomerRefreshCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     customerRefreshCookieName,
		Path:     requestPublicPrefix(r) + "/api/v1/customer/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.isSecureRequest(r),
	})
}

func (a *App) setCustomerSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     customerSessionCookieName,
		Value:    token,
		Path:     requestSessionCookiePath(r),
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.isSecureRequest(r),
	})
}

func (a *App) clearCustomerSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     customerSessionCookieName,
		Value:    "",
		Path:     requestSessionCookiePath(r),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.isSecureRequest(r),
	})
}
