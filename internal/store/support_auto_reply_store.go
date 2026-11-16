package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"bridge-core/internal/model"
)

const supportAutoReplyMaxRunes = 2000

func (s *SQLiteStore) GetSupportAutoReply(scopeType string, scopeID int64) (model.SupportAutoReplySettings, bool, error) {
	scopeType, scopeID, err := normalizeSupportAutoReplyScope(scopeType, scopeID)
	if err != nil {
		return model.SupportAutoReplySettings{}, false, err
	}
	var item model.SupportAutoReplySettings
	var updatedAt string
	err = s.db.QueryRow(`
		SELECT scope_type, scope_id, mode, sender_name, message,
		       updated_by_role, updated_by_account_id, updated_at
		FROM support_auto_reply_settings
		WHERE scope_type = ? AND scope_id = ?
	`, scopeType, scopeID).Scan(
		&item.ScopeType, &item.ScopeID, &item.Mode, &item.SenderName, &item.Message,
		&item.UpdatedByRole, &item.UpdatedByAccountID, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return model.SupportAutoReplySettings{ScopeType: scopeType, ScopeID: scopeID, Mode: defaultSupportAutoReplyMode(scopeType)}, false, nil
	}
	if err != nil {
		return model.SupportAutoReplySettings{}, false, fmt.Errorf("load support auto reply: %w", err)
	}
	item.UpdatedAt = parseTime(updatedAt)
	return item, true, nil
}

func (s *SQLiteStore) SaveSupportAutoReply(item model.SupportAutoReplySettings) (model.SupportAutoReplySettings, error) {
	scopeType, scopeID, err := normalizeSupportAutoReplyScope(item.ScopeType, item.ScopeID)
	if err != nil {
		return model.SupportAutoReplySettings{}, err
	}
	mode := strings.ToLower(strings.TrimSpace(item.Mode))
	if scopeType == model.SupportAutoReplyScopeGlobal {
		if mode != model.SupportAutoReplyDisabled && mode != model.SupportAutoReplyCustom {
			return model.SupportAutoReplySettings{}, fmt.Errorf("global auto reply mode must be disabled or custom")
		}
	} else if mode != model.SupportAutoReplyDisabled && mode != model.SupportAutoReplyCustom && mode != model.SupportAutoReplyInherit {
		return model.SupportAutoReplySettings{}, fmt.Errorf("area auto reply mode must be disabled, custom or inherit")
	}
	senderName := strings.TrimSpace(item.SenderName)
	message := strings.TrimSpace(item.Message)
	if utf8.RuneCountInString(senderName) > 80 {
		return model.SupportAutoReplySettings{}, fmt.Errorf("auto reply sender name exceeds 80 characters")
	}
	if !utf8.ValidString(message) || utf8.RuneCountInString(message) > supportAutoReplyMaxRunes {
		return model.SupportAutoReplySettings{}, fmt.Errorf("auto reply message exceeds %d characters or is invalid utf-8", supportAutoReplyMaxRunes)
	}
	if mode == model.SupportAutoReplyCustom && message == "" {
		return model.SupportAutoReplySettings{}, fmt.Errorf("custom auto reply message is required")
	}
	if mode != model.SupportAutoReplyCustom {
		senderName = ""
		message = ""
	}
	now := time.Now().UTC()
	if _, err := s.db.Exec(`
		INSERT INTO support_auto_reply_settings(
			scope_type, scope_id, mode, sender_name, message,
			updated_by_role, updated_by_account_id, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id) DO UPDATE SET
			mode = excluded.mode,
			sender_name = excluded.sender_name,
			message = excluded.message,
			updated_by_role = excluded.updated_by_role,
			updated_by_account_id = excluded.updated_by_account_id,
			updated_at = excluded.updated_at
	`, scopeType, scopeID, mode, senderName, message, item.UpdatedByRole, item.UpdatedByAccountID, now.Format(time.RFC3339Nano)); err != nil {
		return model.SupportAutoReplySettings{}, fmt.Errorf("save support auto reply: %w", err)
	}
	item.ScopeType, item.ScopeID, item.Mode = scopeType, scopeID, mode
	item.SenderName, item.Message, item.UpdatedAt = senderName, message, now
	return item, nil
}

func normalizeSupportAutoReplyScope(scopeType string, scopeID int64) (string, int64, error) {
	scopeType = strings.ToLower(strings.TrimSpace(scopeType))
	switch scopeType {
	case model.SupportAutoReplyScopeGlobal:
		return scopeType, 0, nil
	case model.SupportAutoReplyScopeArea:
		if scopeID <= 0 {
			return "", 0, fmt.Errorf("area auto reply scope requires an account id")
		}
		return scopeType, scopeID, nil
	default:
		return "", 0, fmt.Errorf("invalid auto reply scope")
	}
}

func defaultSupportAutoReplyMode(scopeType string) string {
	if scopeType == model.SupportAutoReplyScopeArea {
		return model.SupportAutoReplyInherit
	}
	return model.SupportAutoReplyDisabled
}
