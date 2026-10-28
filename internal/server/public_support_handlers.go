package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func (a *App) handlePublicSupport(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/public/support"), "/")
	ownerType := model.CustomerOwnerGuest
	var ownerID int64
	if scopeToken := strings.TrimSpace(r.URL.Query().Get("area")); scopeToken != "" {
		managerID, found, enabled, err := a.store.ResolveAreaManagerRecommendationToken(scopeToken)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !found || !enabled {
			writeError(w, http.StatusNotFound, "recommendation link not found")
			return
		}
		ownerType = model.AdminRoleAreaManager
		ownerID = managerID
	}
	user, _, ok := a.requireGuestSupport(w, r, ownerType, ownerID)
	if !ok {
		return
	}

	if path == "" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		conversation, found, err := a.store.GetSupportConversationByCustomer(user.ID, model.SupportSenderCustomer)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if found && r.URL.Query().Get("mark_read") == "1" {
			if err := a.store.MarkSupportConversationRead(conversation.ID, model.SupportSenderCustomer); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			conversation, found, err = a.store.GetSupportConversationByCustomer(user.ID, model.SupportSenderCustomer)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		thread, err := a.customerSupportThread(user, conversation, found)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, thread)
		return
	}

	if path != "messages" || r.Method != http.MethodPost {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	var req model.SupportMessageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode support message: %v", err))
		return
	}
	message, conversation, err := a.store.AddSupportMessage(user.ID, model.SupportSenderCustomer, user.ID, user.DisplayName, req.Body)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrSupportRateLimit) {
			status = http.StatusTooManyRequests
		}
		writeError(w, status, err.Error())
		return
	}
	a.notifySupportMessage(user, conversation, message, requestPublicBaseURL(r))
	writeJSON(w, http.StatusCreated, message)
}

func (a *App) requireGuestSupport(w http.ResponseWriter, r *http.Request, ownerType string, ownerID int64) (model.CustomerUser, string, bool) {
	ip := a.requestObservedIP(r)
	if strings.TrimSpace(ip) == "" {
		ip = "unknown"
	}
	if cookie, err := r.Cookie(guestSupportCookieName); err == nil && cookie.Value != "" {
		user, session, valid, validateErr := a.store.ValidateCustomerSession(cookie.Value)
		if validateErr == nil && valid {
			matches, matchErr := a.store.GuestCustomerMatchesIP(user.ID, ip, ownerType, ownerID)
			if matchErr == nil && matches {
				// Older releases scoped this cookie to /api/v1/public/, which
				// prevents the login endpoint from seeing it. Re-issue a root API
				// cookie whenever the visitor returns so the next login can claim
				// the conversation without requiring a second browser flow.
				a.setGuestSupportCookie(w, r, cookie.Value, session.ExpiresAt)
				return user, cookie.Value, true
			}
		}
	}
	user, err := a.store.GetOrCreateGuestCustomer(ip, guestSupportSessionTTL, ownerType, ownerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return model.CustomerUser{}, "", false
	}
	token, session, err := a.store.CreateCustomerSession(user.ID, guestSupportSessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return model.CustomerUser{}, "", false
	}
	a.setGuestSupportCookie(w, r, token, session.ExpiresAt)
	return user, token, true
}

func (a *App) setGuestSupportCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     guestSupportCookieName,
		Value:    token,
		Path:     requestPublicPrefix(r) + "/api/v1/",
		Expires:  expiresAt,
		MaxAge:   maxCookieAge(expiresAt),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.isSecureRequest(r),
	})
}

func maxCookieAge(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
