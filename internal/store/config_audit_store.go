package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) UpdateAgentConfigWithActor(agentID string, cfg model.ManagedAgentConfig, actor string) (model.AgentRecord, error) {
	before, found, err := s.GetAgentConfig(agentID)
	if err != nil {
		return model.AgentRecord{}, err
	}
	if !found {
		return model.AgentRecord{}, fmt.Errorf("agent not found")
	}

	record, err := s.updateAgentConfig(agentID, cfg)
	if err != nil {
		return model.AgentRecord{}, err
	}
	if err := s.CreateConfigAuditLog(agentID, actor, before, record.Config); err != nil {
		return model.AgentRecord{}, err
	}
	return record, nil
}

func (s *SQLiteStore) CreateConfigAuditLog(agentID, actor string, before, after any) error {
	if agentID == "" {
		return nil
	}
	if actor == "" {
		actor = "system"
	}
	beforeJSON, err := json.Marshal(redactConfigAuditValue(before))
	if err != nil {
		return fmt.Errorf("marshal config audit before: %w", err)
	}
	afterJSON, err := json.Marshal(redactConfigAuditValue(after))
	if err != nil {
		return fmt.Errorf("marshal config audit after: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`
		INSERT INTO config_audit_logs (agent_id, actor, before_json, after_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, agentID, actor, string(beforeJSON), string(afterJSON), now); err != nil {
		return fmt.Errorf("insert config audit log: %w", err)
	}
	return nil
}

func redactConfigAuditValue(value any) any {
	switch typed := value.(type) {
	case model.ManagedAgentConfig:
		return redactManagedAgentConfig(typed)
	case *model.ManagedAgentConfig:
		if typed == nil {
			return nil
		}
		redacted := redactManagedAgentConfig(*typed)
		return redacted
	default:
		return value
	}
}

func redactManagedAgentConfig(cfg model.ManagedAgentConfig) model.ManagedAgentConfig {
	if cfg.XUI.Password != "" {
		cfg.XUI.Password = "******"
	}
	if cfg.XUI.APIToken != "" {
		cfg.XUI.APIToken = "******"
	}
	if cfg.XUI.TwoFactorCode != "" {
		cfg.XUI.TwoFactorCode = "******"
	}
	return cfg
}

func (s *SQLiteStore) ListConfigAuditLogs(agentID string, limit int) ([]model.ConfigAuditLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	query := `
		SELECT id, agent_id, actor, before_json, after_json, created_at
		FROM config_audit_logs
	`
	args := []any{}
	if agentID != "" {
		query += ` WHERE agent_id = ?`
		args = append(args, agentID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query config audit logs: %w", err)
	}
	defer rows.Close()

	items := make([]model.ConfigAuditLog, 0)
	for rows.Next() {
		item, err := scanConfigAuditLog(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate config audit logs: %w", err)
	}
	return items, nil
}

func scanConfigAuditLog(row rowScanner) (model.ConfigAuditLog, error) {
	var (
		item          model.ConfigAuditLog
		beforeJSON    string
		afterJSON     string
		createdAtText string
	)
	if err := row.Scan(&item.ID, &item.AgentID, &item.Actor, &beforeJSON, &afterJSON, &createdAtText); err != nil {
		if err == sql.ErrNoRows {
			return model.ConfigAuditLog{}, err
		}
		return model.ConfigAuditLog{}, fmt.Errorf("scan config audit log: %w", err)
	}
	var before any
	var after any
	_ = json.Unmarshal([]byte(beforeJSON), &before)
	_ = json.Unmarshal([]byte(afterJSON), &after)
	item.Before = before
	item.After = after
	item.CreatedAt = parseTime(createdAtText)
	return item, nil
}
