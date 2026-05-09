package store

import (
	"database/sql"
	"fmt"
)

func (s *SQLiteStore) init() error {
	pragmas := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`PRAGMA busy_timeout=5000;`,
	}
	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("apply pragma %q: %w", pragma, err)
		}
	}

	schema := []string{
		`
		CREATE TABLE IF NOT EXISTS admin_accounts (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS admin_sessions (
			token_hash TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS agents (
			agent_id TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL DEFAULT '',
			agent_tags_json TEXT NOT NULL DEFAULT '[]',
			agent_token TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			public_ipv4 TEXT NOT NULL DEFAULT '',
			public_ipv6 TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL DEFAULT '',
			xui_config_json TEXT NOT NULL DEFAULT '{}',
			nezha_config_json TEXT NOT NULL DEFAULT '{}',
			renewal_config_json TEXT NOT NULL DEFAULT '{}',
			entry_config_json TEXT NOT NULL DEFAULT '{}'
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS latest_snapshots (
			agent_id TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL DEFAULT '',
			reported_at TEXT NOT NULL,
			hostname TEXT NOT NULL DEFAULT '',
			public_ipv4 TEXT NOT NULL DEFAULT '',
			public_ipv6 TEXT NOT NULL DEFAULT '',
			cpu REAL NOT NULL DEFAULT 0,
			mem_used INTEGER NOT NULL DEFAULT 0,
			mem_total INTEGER NOT NULL DEFAULT 0,
			xray_state TEXT NOT NULL DEFAULT '',
			inbound_count INTEGER NOT NULL DEFAULT 0,
			outbound_count INTEGER NOT NULL DEFAULT 0,
			routing_rule_count INTEGER NOT NULL DEFAULT 0,
			nezha_server_id INTEGER NOT NULL DEFAULT 0,
			nezha_server_name TEXT NOT NULL DEFAULT '',
			last_collection_err TEXT NOT NULL DEFAULT '',
			snapshot_json TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			agent_name TEXT NOT NULL DEFAULT '',
			reported_at TEXT NOT NULL,
			hostname TEXT NOT NULL DEFAULT '',
			public_ipv4 TEXT NOT NULL DEFAULT '',
			public_ipv6 TEXT NOT NULL DEFAULT '',
			cpu REAL NOT NULL DEFAULT 0,
			mem_used INTEGER NOT NULL DEFAULT 0,
			mem_total INTEGER NOT NULL DEFAULT 0,
			xray_state TEXT NOT NULL DEFAULT '',
			inbound_count INTEGER NOT NULL DEFAULT 0,
			outbound_count INTEGER NOT NULL DEFAULT 0,
			routing_rule_count INTEGER NOT NULL DEFAULT 0,
			nezha_server_id INTEGER NOT NULL DEFAULT 0,
			nezha_server_name TEXT NOT NULL DEFAULT '',
			last_collection_err TEXT NOT NULL DEFAULT '',
			snapshot_json TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS xui_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '{}',
			result_json TEXT NOT NULL DEFAULT '{}',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			claimed_at TEXT NOT NULL DEFAULT '',
			completed_at TEXT NOT NULL DEFAULT ''
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS telegram_bots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			bot_token TEXT NOT NULL DEFAULT '',
			chat_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS alert_states (
			alert_key TEXT PRIMARY KEY,
			fingerprint TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT '',
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			last_sent_at TEXT NOT NULL DEFAULT '',
			resolved_at TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL DEFAULT ''
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS config_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			actor TEXT NOT NULL DEFAULT '',
			before_json TEXT NOT NULL DEFAULT '{}',
			after_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		);
		`,
		`CREATE INDEX IF NOT EXISTS idx_snapshots_agent_reported_at ON snapshots(agent_id, reported_at DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires_at ON admin_sessions(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_xui_actions_agent_status_id ON xui_actions(agent_id, status, id);`,
		`CREATE INDEX IF NOT EXISTS idx_config_audit_agent_id ON config_audit_logs(agent_id, id DESC);`,
	}
	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}
	if err := s.ensureColumn("agents", "agent_tags_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.ensureColumn("agents", "renewal_config_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := s.ensureColumn("agents", "entry_config_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) ensureColumn(tableName, columnName, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		return fmt.Errorf("query table info for %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info for %s: %w", tableName, err)
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table info for %s: %w", tableName, err)
	}

	if _, err := s.db.Exec(`ALTER TABLE ` + tableName + ` ADD COLUMN ` + columnName + ` ` + definition); err != nil {
		return fmt.Errorf("add column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}
