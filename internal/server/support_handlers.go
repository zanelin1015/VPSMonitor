package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

const supportNotificationCooldown = 30 * time.Second

func (a *App) handleCustomerSupport(w http.ResponseWriter, r *http.Request, parts []string) {
	user, _, ok := a.requireCustomer(w, r)
	if !ok {
		return
	}
	if len(parts) == 0 {
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
	if len(parts) != 1 || parts[0] != "messages" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req model.SupportMessageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode support message: %v", err))
		return
	}
	message, conversation, err := a.store.AddSupportMessage(user.ID, model.SupportSenderCustomer, user.ID, firstNonEmptyString(user.DisplayName, user.Username), req.Body)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrSupportRateLimit) {
			status = http.StatusTooManyRequests
		}
		writeError(w, status, err.Error())
		return
	}
	supportOnline := a.supportPresence.onlineForCustomer(user)
	if !supportOnline {
		claimed, claimErr := a.store.ClaimSupportNotification(conversation.ID, supportNotificationCooldown)
		if claimErr != nil {
			log.Printf("claim support notification: %v", claimErr)
		} else if claimed {
			baseURL := requestPublicBaseURL(r)
			go a.sendOfflineSupportNotification(user, conversation, message, baseURL)
		}
	}
	writeJSON(w, http.StatusCreated, message)
}

func (a *App) customerSupportThread(user model.CustomerUser, conversation model.SupportConversation, found bool) (model.SupportThreadResponse, error) {
	if !found {
		conversation = model.SupportConversation{
			CustomerID:          user.ID,
			CustomerUsername:    user.Username,
			CustomerDisplayName: user.DisplayName,
			OwnerType:           user.OwnerType,
			OwnerID:             user.OwnerID,
			Status:              model.SupportConversationOpen,
			CreatedAt:           user.CreatedAt,
			UpdatedAt:           user.UpdatedAt,
		}
		return model.SupportThreadResponse{
			Conversation:  conversation,
			Messages:      []model.SupportMessage{},
			SupportOnline: a.supportPresence.onlineForCustomer(user),
		}, nil
	}
	if conversation.CustomerID != user.ID {
		return model.SupportThreadResponse{}, fmt.Errorf("support conversation not found")
	}
	messages, err := a.store.ListSupportMessages(conversation.ID)
	if err != nil {
		return model.SupportThreadResponse{}, err
	}
	return model.SupportThreadResponse{
		Conversation:  conversation,
		Messages:      messages,
		SupportOnline: a.supportPresence.onlineForCustomer(user),
	}, nil
}

func (a *App) handleAdminSupport(w http.ResponseWriter, r *http.Request, parts []string) {
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if len(parts) == 0 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		ownerType, ownerID := supportOwnerScope(user)
		conversations, err := a.store.ListSupportConversations(ownerType, ownerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		unread := 0
		for _, conversation := range conversations {
			unread += conversation.UnreadCount
		}
		writeJSON(w, http.StatusOK, model.SupportConversationListResponse{Conversations: conversations, UnreadCount: unread})
		return
	}
	if len(parts) == 1 && parts[0] == "unread" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		ownerType, ownerID := supportOwnerScope(user)
		count, err := a.store.CountUnreadSupportMessages(ownerType, ownerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.SupportUnreadResponse{UnreadCount: count})
		return
	}
	if len(parts) < 2 || parts[0] != "conversations" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	conversationID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || conversationID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid support conversation id")
		return
	}
	conversation, visible, err := a.supportConversationForAdmin(user, conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !visible {
		writeError(w, http.StatusNotFound, "support conversation not found")
		return
	}

	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := a.store.MarkSupportConversationRead(conversationID, model.SupportSenderAdmin); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		thread, err := a.adminSupportThread(conversationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, thread)
		return
	}

	switch parts[2] {
	case "messages":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req model.SupportMessageRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode support message: %v", err))
			return
		}
		message, _, err := a.store.AddSupportMessage(conversation.CustomerID, user.Role, user.ID, firstNonEmptyString(user.DisplayName, user.Username), req.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.store.MarkSupportConversationRead(conversationID, model.SupportSenderAdmin); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, message)
	case "status":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req model.SupportStatusRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode support status: %v", err))
			return
		}
		updated, err := a.store.UpdateSupportConversationStatus(conversationID, req.Status)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (a *App) adminSupportThread(conversationID int64) (model.SupportThreadResponse, error) {
	conversation, found, err := a.store.GetSupportConversation(conversationID, model.SupportSenderAdmin)
	if err != nil {
		return model.SupportThreadResponse{}, err
	}
	if !found {
		return model.SupportThreadResponse{}, fmt.Errorf("support conversation not found")
	}
	messages, err := a.store.ListSupportMessages(conversationID)
	if err != nil {
		return model.SupportThreadResponse{}, err
	}
	return model.SupportThreadResponse{Conversation: conversation, Messages: messages, SupportOnline: true}, nil
}

func (a *App) supportConversationForAdmin(user model.AdminUser, conversationID int64) (model.SupportConversation, bool, error) {
	conversation, found, err := a.store.GetSupportConversation(conversationID, model.SupportSenderAdmin)
	if err != nil || !found {
		return model.SupportConversation{}, found, err
	}
	if isRootAdmin(user) {
		return conversation, true, nil
	}
	if conversation.OwnerType != model.AdminRoleAreaManager || conversation.OwnerID != user.ID {
		return model.SupportConversation{}, false, nil
	}
	return conversation, true, nil
}

func supportOwnerScope(user model.AdminUser) (string, int64) {
	if isAreaManager(user) {
		return model.AdminRoleAreaManager, user.ID
	}
	return "", 0
}

func (a *App) sendOfflineSupportNotification(customer model.CustomerUser, conversation model.SupportConversation, message model.SupportMessage, baseURL string) {
	if a == nil || a.alerts == nil {
		return
	}
	text := formatOfflineSupportNotification(customer, conversation, message, baseURL)
	if err := a.alerts.sendMessageToEnabledTelegramBots(text); err != nil {
		log.Printf("send offline support notification: %v", err)
	}
}

func formatOfflineSupportNotification(customer model.CustomerUser, conversation model.SupportConversation, message model.SupportMessage, baseURL string) string {
	body := strings.TrimSpace(message.Body)
	if utf8.RuneCountInString(body) > 500 {
		runes := []rune(body)
		body = string(runes[:497]) + "..."
	}
	name := firstNonEmptyString(customer.DisplayName, customer.Username)
	lines := []string{
		"ZaneLin 客服新消息",
		"用户：" + name + " (" + customer.Username + ")",
		"内容：" + body,
		"时间：" + formatBeijingTime(message.CreatedAt),
	}
	if baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/"); baseURL != "" {
		link := baseURL + "/?page=support&conversation=" + url.QueryEscape(strconv.FormatInt(conversation.ID, 10))
		lines = append(lines, "后台："+link)
	}
	return strings.Join(lines, "\n")
}
