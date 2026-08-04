package model

import "time"

const (
	SupportConversationOpen   = "open"
	SupportConversationClosed = "closed"

	SupportSenderCustomer    = "customer"
	SupportSenderAdmin       = AdminRoleRoot
	SupportSenderAreaManager = AdminRoleAreaManager
)

type SupportConversation struct {
	ID                  int64      `json:"id"`
	CustomerID          int64      `json:"customer_id"`
	CustomerUsername    string     `json:"customer_username"`
	CustomerDisplayName string     `json:"customer_display_name,omitempty"`
	OwnerType           string     `json:"owner_type"`
	OwnerID             int64      `json:"owner_id"`
	Status              string     `json:"status"`
	LastMessagePreview  string     `json:"last_message_preview,omitempty"`
	LastSenderRole      string     `json:"last_sender_role,omitempty"`
	LastMessageAt       *time.Time `json:"last_message_at,omitempty"`
	UnreadCount         int        `json:"unread_count"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type SupportMessage struct {
	ID              int64     `json:"id"`
	ConversationID  int64     `json:"conversation_id"`
	SenderRole      string    `json:"sender_role"`
	SenderAccountID int64     `json:"sender_account_id"`
	SenderName      string    `json:"sender_name"`
	Body            string    `json:"body"`
	CreatedAt       time.Time `json:"created_at"`
}

type SupportThreadResponse struct {
	Conversation  SupportConversation `json:"conversation"`
	Messages      []SupportMessage    `json:"messages"`
	SupportOnline bool                `json:"support_online"`
}

type SupportConversationListResponse struct {
	Conversations []SupportConversation `json:"conversations"`
	UnreadCount   int                   `json:"unread_count"`
}

type SupportMessageRequest struct {
	Body string `json:"body"`
}

type SupportStatusRequest struct {
	Status string `json:"status"`
}

type SupportUnreadResponse struct {
	UnreadCount int `json:"unread_count"`
}
