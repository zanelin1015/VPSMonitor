package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func (a *App) handlePublicSupport(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/public/support"), "/")
	user, _, ok := a.requireGuestSupport(w, r)
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
	if !a.supportPresence.onlineForCustomer(user) {
		claimed, claimErr := a.store.ClaimSupportNotification(conversation.ID, supportNotificationCooldown)
		if claimErr == nil && claimed {
			go a.sendOfflineSupportNotification(user, conversation, message, requestPublicBaseURL(r))
		}
	}
	writeJSON(w, http.StatusCreated, message)
}

func (a *App) requireGuestSupport(w http.ResponseWriter, r *http.Request) (model.CustomerUser, string, bool) {
	ip := a.requestObservedIP(r)
	if strings.TrimSpace(ip) == "" {
		ip = "unknown"
	}
	if cookie, err := r.Cookie(guestSupportCookieName); err == nil && cookie.Value != "" {
		user, _, valid, validateErr := a.store.ValidateCustomerSession(cookie.Value)
		if validateErr == nil && valid && user.OwnerType == model.CustomerOwnerGuest {
			matches, matchErr := a.store.GuestCustomerMatchesIP(user.ID, ip)
			if matchErr == nil && matches {
				return user, cookie.Value, true
			}
		}
	}
	user, err := a.store.GetOrCreateGuestCustomer(ip, guestSupportSessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return model.CustomerUser{}, "", false
	}
	token, session, err := a.store.CreateCustomerSession(user.ID, guestSupportSessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return model.CustomerUser{}, "", false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     guestSupportCookieName,
		Value:    token,
		Path:     requestPublicPrefix(r) + "/api/v1/public/",
		Expires:  session.ExpiresAt,
		MaxAge:   int(guestSupportSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.isSecureRequest(r),
	})
	return user, token, true
}
