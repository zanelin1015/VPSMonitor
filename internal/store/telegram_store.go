package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

type telegramBotSecret struct {
	model.TelegramBot
	BotToken string
}

func (s *SQLiteStore) ListTelegramBots() ([]model.TelegramBot, error) {
	rows, err := s.db.Query(`
		SELECT id, name, bot_token, chat_id, enabled, created_at, updated_at
		FROM telegram_bots
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query telegram bots: %w", err)
	}
	defer rows.Close()

	items := make([]model.TelegramBot, 0)
	for rows.Next() {
		bot, err := scanTelegramBot(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, bot.TelegramBot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate telegram bots: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) ListEnabledTelegramBotSecrets() ([]telegramBotSecret, error) {
	rows, err := s.db.Query(`
		SELECT id, name, bot_token, chat_id, enabled, created_at, updated_at
		FROM telegram_bots
		WHERE enabled = 1
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query enabled telegram bots: %w", err)
	}
	defer rows.Close()

	items := make([]telegramBotSecret, 0)
	for rows.Next() {
		bot, err := scanTelegramBot(rows)
		if err != nil {
			return nil, err
		}
		bot, err = s.decryptTelegramBotSecret(bot)
		if err != nil {
			return nil, err
		}
		if bot.BotToken == "" || bot.ChatID == "" {
			continue
		}
		items = append(items, bot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled telegram bots: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) GetTelegramBotSecret(id int64) (telegramBotSecret, bool, error) {
	bot, found, err := s.getTelegramBotStoredSecret(id)
	if err != nil || !found {
		return telegramBotSecret{}, found, err
	}
	bot, err = s.decryptTelegramBotSecret(bot)
	if err != nil {
		return telegramBotSecret{}, false, err
	}
	return bot, true, nil
}

func (s *SQLiteStore) getTelegramBotStoredSecret(id int64) (telegramBotSecret, bool, error) {
	if id <= 0 {
		return telegramBotSecret{}, false, nil
	}
	row := s.db.QueryRow(`
		SELECT id, name, bot_token, chat_id, enabled, created_at, updated_at
		FROM telegram_bots
		WHERE id = ?
	`, id)
	bot, err := scanTelegramBot(row)
	if err == sql.ErrNoRows {
		return telegramBotSecret{}, false, nil
	}
	if err != nil {
		return telegramBotSecret{}, false, err
	}
	return bot, true, nil
}

func (s *SQLiteStore) CreateTelegramBot(req model.TelegramBotRequest) (model.TelegramBot, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.BotToken = strings.TrimSpace(req.BotToken)
	req.ChatID = strings.TrimSpace(req.ChatID)
	if req.Name == "" {
		return model.TelegramBot{}, fmt.Errorf("bot name is required")
	}
	if req.BotToken == "" {
		return model.TelegramBot{}, fmt.Errorf("bot token is required")
	}
	if req.ChatID == "" {
		return model.TelegramBot{}, fmt.Errorf("chat id is required")
	}
	token, err := s.secrets.EncryptString(req.BotToken)
	if err != nil {
		return model.TelegramBot{}, fmt.Errorf("encrypt telegram bot token: %w", err)
	}
	now := time.Now().UTC()
	result, err := s.db.Exec(`
		INSERT INTO telegram_bots (name, bot_token, chat_id, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, req.Name, token, req.ChatID, boolInt(req.Enabled), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return model.TelegramBot{}, fmt.Errorf("create telegram bot: %w", err)
	}
	id, _ := result.LastInsertId()
	bot, found, err := s.GetTelegramBotSecret(id)
	if err != nil {
		return model.TelegramBot{}, err
	}
	if !found {
		return model.TelegramBot{}, fmt.Errorf("created telegram bot not found")
	}
	return bot.TelegramBot, nil
}

func (s *SQLiteStore) UpdateTelegramBot(id int64, req model.TelegramBotRequest) (model.TelegramBot, error) {
	current, found, err := s.getTelegramBotStoredSecret(id)
	if err != nil {
		return model.TelegramBot{}, err
	}
	if !found {
		return model.TelegramBot{}, fmt.Errorf("telegram bot not found")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = current.Name
	}
	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		chatID = current.ChatID
	}
	token := current.BotToken
	if strings.TrimSpace(req.BotToken) != "" {
		token, err = s.secrets.EncryptString(strings.TrimSpace(req.BotToken))
		if err != nil {
			return model.TelegramBot{}, fmt.Errorf("encrypt telegram bot token: %w", err)
		}
	}
	now := time.Now().UTC()
	if _, err := s.db.Exec(`
		UPDATE telegram_bots
		SET name = ?, bot_token = ?, chat_id = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, name, token, chatID, boolInt(req.Enabled), now.Format(time.RFC3339Nano), id); err != nil {
		return model.TelegramBot{}, fmt.Errorf("update telegram bot: %w", err)
	}
	updated, found, err := s.GetTelegramBotSecret(id)
	if err != nil {
		return model.TelegramBot{}, err
	}
	if !found {
		return model.TelegramBot{}, fmt.Errorf("updated telegram bot not found")
	}
	return updated.TelegramBot, nil
}

func (s *SQLiteStore) DeleteTelegramBot(id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid telegram bot id")
	}
	result, err := s.db.Exec(`DELETE FROM telegram_bots WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete telegram bot: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("telegram bot not found")
	}
	return nil
}

func scanTelegramBot(row rowScanner) (telegramBotSecret, error) {
	var (
		item          telegramBotSecret
		storedToken   string
		enabled       int
		createdAtText string
		updatedAtText string
	)
	if err := row.Scan(&item.ID, &item.Name, &storedToken, &item.ChatID, &enabled, &createdAtText, &updatedAtText); err != nil {
		return telegramBotSecret{}, err
	}
	item.Enabled = enabled != 0
	item.HasBotToken = storedToken != ""
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	item.BotToken = storedToken
	return item, nil
}

func (s *SQLiteStore) decryptTelegramBotSecret(bot telegramBotSecret) (telegramBotSecret, error) {
	token, err := s.secrets.DecryptString(bot.BotToken)
	if err != nil {
		return telegramBotSecret{}, fmt.Errorf("decrypt telegram bot token: %w", err)
	}
	bot.BotToken = token
	return bot, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
