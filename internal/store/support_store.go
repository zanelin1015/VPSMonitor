package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"bridge-core/internal/model"
)

const (
	supportMessageMaxRunes   = 2000
	supportMessageRateLimit  = 20
	supportMessageRateWindow = time.Minute
	supportMessageListLimit  = 200
)

var ErrSupportRateLimit = errors.New("support message rate limit exceeded")

func (s *SQLiteStore) ListSupportConversations(ownerType string, ownerID int64) ([]model.SupportConversation, error) {
	query := supportConversationSelect(model.SupportSenderAdmin)
	args := make([]any, 0, 2)
	if normalizeCustomerOwnerType(ownerType) == model.AdminRoleAreaManager {
		if ownerID <= 0 {
			return []model.SupportConversation{}, nil
		}
		query += ` WHERE customer.owner_type = ? AND customer.owner_id = ?`
		args = append(args, model.AdminRoleAreaManager, ownerID)
	}
	query += ` ORDER BY CASE WHEN conversation.status = 'open' THEN 0 ELSE 1 END, conversation.updated_at DESC, conversation.id DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list support conversations: %w", err)
	}
	defer rows.Close()
	items := make([]model.SupportConversation, 0)
	for rows.Next() {
		item, err := scanSupportConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan support conversation: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate support conversations: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) CountUnreadSupportMessages(ownerType string, ownerID int64) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM support_messages AS message
		JOIN support_conversations AS conversation ON conversation.id = message.conversation_id
		JOIN customer_accounts AS customer ON customer.id = conversation.customer_id
		WHERE message.sender_role = ? AND message.id > conversation.admin_read_message_id
	`
	args := []any{model.SupportSenderCustomer}
	if normalizeCustomerOwnerType(ownerType) == model.AdminRoleAreaManager {
		if ownerID <= 0 {
			return 0, nil
		}
		query += ` AND customer.owner_type = ? AND customer.owner_id = ?`
		args = append(args, model.AdminRoleAreaManager, ownerID)
	}
	var count int
	if err := s.db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unread support messages: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) GetSupportConversation(id int64, audience string) (model.SupportConversation, bool, error) {
	if id <= 0 {
		return model.SupportConversation{}, false, nil
	}
	row := s.db.QueryRow(supportConversationSelect(audience)+` WHERE conversation.id = ?`, id)
	item, err := scanSupportConversation(row)
	if err == sql.ErrNoRows {
		return model.SupportConversation{}, false, nil
	}
	if err != nil {
		return model.SupportConversation{}, false, fmt.Errorf("load support conversation: %w", err)
	}
	return item, true, nil
}

func (s *SQLiteStore) GetSupportConversationByCustomer(customerID int64, audience string) (model.SupportConversation, bool, error) {
	if customerID <= 0 {
		return model.SupportConversation{}, false, nil
	}
	row := s.db.QueryRow(supportConversationSelect(audience)+` WHERE conversation.customer_id = ?`, customerID)
	item, err := scanSupportConversation(row)
	if err == sql.ErrNoRows {
		return model.SupportConversation{}, false, nil
	}
	if err != nil {
		return model.SupportConversation{}, false, fmt.Errorf("load customer support conversation: %w", err)
	}
	return item, true, nil
}

func (s *SQLiteStore) ListSupportMessages(conversationID int64) ([]model.SupportMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, conversation_id, sender_role, sender_account_id, sender_name, body, created_at
		FROM (
			SELECT id, conversation_id, sender_role, sender_account_id, sender_name, body, created_at
			FROM support_messages
			WHERE conversation_id = ?
			ORDER BY id DESC
			LIMIT ?
		)
		ORDER BY id ASC
	`, conversationID, supportMessageListLimit)
	if err != nil {
		return nil, fmt.Errorf("list support messages: %w", err)
	}
	defer rows.Close()
	items := make([]model.SupportMessage, 0)
	for rows.Next() {
		item, err := scanSupportMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan support message: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate support messages: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) AddSupportMessage(customerID int64, senderRole string, senderAccountID int64, senderName, body string) (model.SupportMessage, model.SupportConversation, error) {
	body, err := normalizeSupportMessageBody(body)
	if err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, err
	}
	senderRole = normalizeSupportSenderRole(senderRole)
	senderName = strings.TrimSpace(senderName)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)

	tx, err := s.db.Begin()
	if err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("begin support message: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO support_conversations (customer_id, status, created_at, updated_at)
		SELECT id, ?, ?, ? FROM customer_accounts WHERE id = ?
	`, model.SupportConversationOpen, nowText, nowText, customerID); err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("ensure support conversation: %w", err)
	}
	var conversationID int64
	if err := tx.QueryRow(`SELECT id FROM support_conversations WHERE customer_id = ?`, customerID).Scan(&conversationID); err == sql.ErrNoRows {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("customer not found")
	} else if err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("load support conversation id: %w", err)
	}
	if senderRole == model.SupportSenderCustomer {
		var recent int
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM support_messages
			WHERE conversation_id = ? AND sender_role = ? AND created_at >= ?
		`, conversationID, model.SupportSenderCustomer, now.Add(-supportMessageRateWindow).Format(time.RFC3339Nano)).Scan(&recent); err != nil {
			return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("check support message rate: %w", err)
		}
		if recent >= supportMessageRateLimit {
			return model.SupportMessage{}, model.SupportConversation{}, ErrSupportRateLimit
		}
	}
	result, err := tx.Exec(`
		INSERT INTO support_messages (conversation_id, sender_role, sender_account_id, sender_name, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, conversationID, senderRole, senderAccountID, senderName, body, nowText)
	if err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("create support message: %w", err)
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("load support message id: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE support_conversations
		SET status = ?, last_message_id = ?, last_message_preview = ?, last_sender_role = ?, last_message_at = ?, updated_at = ?
		WHERE id = ?
	`, model.SupportConversationOpen, messageID, supportMessagePreview(body), senderRole, nowText, nowText, conversationID); err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("update support conversation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("commit support message: %w", err)
	}

	message := model.SupportMessage{
		ID:              messageID,
		ConversationID:  conversationID,
		SenderRole:      senderRole,
		SenderAccountID: senderAccountID,
		SenderName:      senderName,
		Body:            body,
		CreatedAt:       now,
	}
	conversation, found, err := s.GetSupportConversation(conversationID, model.SupportSenderAdmin)
	if err != nil {
		return model.SupportMessage{}, model.SupportConversation{}, err
	}
	if !found {
		return model.SupportMessage{}, model.SupportConversation{}, fmt.Errorf("support conversation not found")
	}
	return message, conversation, nil
}

func (s *SQLiteStore) MarkSupportConversationRead(conversationID int64, audience string) error {
	column := "admin_read_message_id"
	if audience == model.SupportSenderCustomer {
		column = "customer_read_message_id"
	}
	result, err := s.db.Exec(`
		UPDATE support_conversations
		SET `+column+` = COALESCE((SELECT MAX(id) FROM support_messages WHERE conversation_id = ?), 0)
		WHERE id = ?
	`, conversationID, conversationID)
	if err != nil {
		return fmt.Errorf("mark support conversation read: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("support conversation not found")
	}
	return nil
}

func (s *SQLiteStore) UpdateSupportConversationStatus(conversationID int64, status string) (model.SupportConversation, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != model.SupportConversationOpen && status != model.SupportConversationClosed {
		return model.SupportConversation{}, fmt.Errorf("invalid support conversation status")
	}
	result, err := s.db.Exec(`UPDATE support_conversations SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().UTC().Format(time.RFC3339Nano), conversationID)
	if err != nil {
		return model.SupportConversation{}, fmt.Errorf("update support conversation status: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return model.SupportConversation{}, fmt.Errorf("support conversation not found")
	}
	item, found, err := s.GetSupportConversation(conversationID, model.SupportSenderAdmin)
	if err != nil {
		return model.SupportConversation{}, err
	}
	if !found {
		return model.SupportConversation{}, fmt.Errorf("support conversation not found")
	}
	return item, nil
}

func (s *SQLiteStore) ClaimSupportNotification(conversationID int64, cooldown time.Duration) (bool, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-cooldown).Format(time.RFC3339Nano)
	result, err := s.db.Exec(`
		UPDATE support_conversations
		SET last_notified_at = ?
		WHERE id = ? AND (last_notified_at = '' OR last_notified_at < ?)
	`, now.Format(time.RFC3339Nano), conversationID, cutoff)
	if err != nil {
		return false, fmt.Errorf("claim support notification: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func supportConversationSelect(audience string) string {
	readColumn := "conversation.admin_read_message_id"
	unreadCondition := `message.sender_role = 'customer'`
	if audience == model.SupportSenderCustomer {
		readColumn = "conversation.customer_read_message_id"
		unreadCondition = `message.sender_role <> 'customer'`
	}
	return `
		SELECT
			conversation.id,
			conversation.customer_id,
			customer.username,
			customer.display_name,
			customer.owner_type,
			customer.owner_id,
			conversation.status,
			conversation.last_message_preview,
			conversation.last_sender_role,
			conversation.last_message_at,
			(SELECT COUNT(*) FROM support_messages AS message WHERE message.conversation_id = conversation.id AND ` + unreadCondition + ` AND message.id > ` + readColumn + `),
			conversation.created_at,
			conversation.updated_at
		FROM support_conversations AS conversation
		JOIN customer_accounts AS customer ON customer.id = conversation.customer_id
	`
}

func scanSupportConversation(scanner rowScanner) (model.SupportConversation, error) {
	var (
		item              model.SupportConversation
		lastMessageAtText string
		createdAtText     string
		updatedAtText     string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.CustomerID,
		&item.CustomerUsername,
		&item.CustomerDisplayName,
		&item.OwnerType,
		&item.OwnerID,
		&item.Status,
		&item.LastMessagePreview,
		&item.LastSenderRole,
		&lastMessageAtText,
		&item.UnreadCount,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return model.SupportConversation{}, err
	}
	if parsed := parseTime(lastMessageAtText); !parsed.IsZero() {
		item.LastMessageAt = &parsed
	}
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	return item, nil
}

func scanSupportMessage(scanner rowScanner) (model.SupportMessage, error) {
	var item model.SupportMessage
	var createdAtText string
	if err := scanner.Scan(
		&item.ID,
		&item.ConversationID,
		&item.SenderRole,
		&item.SenderAccountID,
		&item.SenderName,
		&item.Body,
		&createdAtText,
	); err != nil {
		return model.SupportMessage{}, err
	}
	item.CreatedAt = parseTime(createdAtText)
	return item, nil
}

func normalizeSupportSenderRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.SupportSenderCustomer:
		return model.SupportSenderCustomer
	case model.SupportSenderAreaManager:
		return model.SupportSenderAreaManager
	default:
		return model.SupportSenderAdmin
	}
}

func normalizeSupportMessageBody(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("message body is required")
	}
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("message body is invalid utf-8")
	}
	if utf8.RuneCountInString(value) > supportMessageMaxRunes {
		return "", fmt.Errorf("message body exceeds %d characters", supportMessageMaxRunes)
	}
	return value, nil
}

func supportMessagePreview(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:157]) + "..."
	}
	return value
}
