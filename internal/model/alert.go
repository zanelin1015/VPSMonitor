package model

import "time"

type TelegramBot struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	ChatID      string    `json:"chat_id"`
	Enabled     bool      `json:"enabled"`
	HasBotToken bool      `json:"has_bot_token"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TelegramBotRequest struct {
	Name     string `json:"name"`
	BotToken string `json:"bot_token,omitempty"`
	ChatID   string `json:"chat_id"`
	Enabled  bool   `json:"enabled"`
}

type ConfigAuditLog struct {
	ID        int64     `json:"id"`
	AgentID   string    `json:"agent_id"`
	Actor     string    `json:"actor"`
	Before    any       `json:"before,omitempty"`
	After     any       `json:"after,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
