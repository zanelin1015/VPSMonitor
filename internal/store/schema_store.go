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
			avatar_url TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS admin_sessions (
			token_hash TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'admin',
			account_id INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS area_manager_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL COLLATE NOCASE UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			billing_enabled INTEGER NOT NULL DEFAULT 0,
			revenue_amount REAL NOT NULL DEFAULT 0,
			revenue_currency TEXT NOT NULL DEFAULT 'CNY',
			revenue_cycle TEXT NOT NULL DEFAULT 'month',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS area_manager_agents (
			manager_id INTEGER NOT NULL,
			agent_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(manager_id, agent_id),
			FOREIGN KEY(manager_id) REFERENCES area_manager_accounts(id) ON DELETE CASCADE,
			FOREIGN KEY(agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS area_manager_agent_tags (
			manager_id INTEGER NOT NULL,
			agent_id TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			updated_at TEXT NOT NULL,
			PRIMARY KEY(manager_id, agent_id),
			FOREIGN KEY(manager_id) REFERENCES area_manager_accounts(id) ON DELETE CASCADE,
			FOREIGN KEY(agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS area_manager_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			manager_id INTEGER NOT NULL,
			agent_id TEXT NOT NULL,
			inbound_id INTEGER NOT NULL DEFAULT 0,
			inbound_tag TEXT NOT NULL DEFAULT '',
			client_email TEXT NOT NULL DEFAULT '',
			public_client_name TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(manager_id) REFERENCES area_manager_accounts(id) ON DELETE CASCADE,
			FOREIGN KEY(agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE,
			UNIQUE(manager_id, agent_id, inbound_id, inbound_tag, client_email)
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS area_manager_outbound_grants (
			manager_id INTEGER NOT NULL,
			agent_id TEXT NOT NULL,
			outbound_tag TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(manager_id, agent_id, outbound_tag),
			FOREIGN KEY(manager_id) REFERENCES area_manager_accounts(id) ON DELETE CASCADE,
			FOREIGN KEY(agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS front_proxy_nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			protocol TEXT NOT NULL DEFAULT '',
			share_url TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			remark TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS front_proxy_grants (
			node_id INTEGER NOT NULL,
			grantee_type TEXT NOT NULL,
			grantee_id INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(node_id, grantee_type, grantee_id),
			FOREIGN KEY(node_id) REFERENCES front_proxy_nodes(id) ON DELETE CASCADE
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS customer_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL COLLATE NOCASE UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			style_code TEXT NOT NULL DEFAULT '',
			subscription_token TEXT NOT NULL DEFAULT '',
			owner_type TEXT NOT NULL DEFAULT 'admin',
			owner_id INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS customer_sessions (
			token_hash TEXT PRIMARY KEY,
			customer_id INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			FOREIGN KEY(customer_id) REFERENCES customer_accounts(id) ON DELETE CASCADE
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS agents (
			agent_id TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL DEFAULT '',
			customer_display_name TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
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
		CREATE TABLE IF NOT EXISTS customer_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_id INTEGER NOT NULL,
			agent_id TEXT NOT NULL,
			inbound_id INTEGER NOT NULL DEFAULT 0,
			inbound_tag TEXT NOT NULL DEFAULT '',
			client_email TEXT NOT NULL DEFAULT '',
			public_client_name TEXT NOT NULL DEFAULT '',
			customer_remark TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(customer_id) REFERENCES customer_accounts(id) ON DELETE CASCADE,
			FOREIGN KEY(agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE,
			UNIQUE(customer_id, agent_id, inbound_id, inbound_tag, client_email)
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS customer_assignment_front_proxies (
			assignment_id INTEGER NOT NULL,
			node_id INTEGER NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			PRIMARY KEY(assignment_id, node_id),
			FOREIGN KEY(assignment_id) REFERENCES customer_assignments(id) ON DELETE CASCADE,
			FOREIGN KEY(node_id) REFERENCES front_proxy_nodes(id) ON DELETE CASCADE
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
			disk_used INTEGER,
			disk_total INTEGER,
			net_traffic_sent INTEGER,
			net_traffic_recv INTEGER,
			net_traffic_total INTEGER,
			net_io_up INTEGER,
			net_io_down INTEGER,
			history_version INTEGER,
			snapshot_json TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS snapshot_component_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			component TEXT NOT NULL,
			observed_at TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			payload_json TEXT NOT NULL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS xui_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			created_by_role TEXT NOT NULL DEFAULT '',
			created_by_account_id INTEGER NOT NULL DEFAULT 0,
			created_by_username TEXT NOT NULL DEFAULT '',
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
		`
		CREATE TABLE IF NOT EXISTS access_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			agent_name TEXT NOT NULL DEFAULT '',
			inbound_id INTEGER NOT NULL DEFAULT 0,
			inbound_tag TEXT NOT NULL DEFAULT '',
			client_email TEXT NOT NULL DEFAULT '',
			client_id TEXT NOT NULL DEFAULT '',
			source_ip TEXT NOT NULL DEFAULT '',
			source_port INTEGER NOT NULL DEFAULT 0,
			target_host TEXT NOT NULL DEFAULT '',
			target_ip TEXT NOT NULL DEFAULT '',
			target_port INTEGER NOT NULL DEFAULT 0,
			network TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT '',
			outbound_tag TEXT NOT NULL DEFAULT '',
			upload_bytes INTEGER NOT NULL DEFAULT 0,
			download_bytes INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			raw_summary TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL DEFAULT '',
			ended_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS support_conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_id INTEGER NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'open',
			last_message_id INTEGER NOT NULL DEFAULT 0,
			last_message_preview TEXT NOT NULL DEFAULT '',
			last_sender_role TEXT NOT NULL DEFAULT '',
			last_message_at TEXT NOT NULL DEFAULT '',
			admin_read_message_id INTEGER NOT NULL DEFAULT 0,
			customer_read_message_id INTEGER NOT NULL DEFAULT 0,
			last_notified_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(customer_id) REFERENCES customer_accounts(id) ON DELETE CASCADE
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS support_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL,
			sender_role TEXT NOT NULL,
			sender_account_id INTEGER NOT NULL DEFAULT 0,
			sender_name TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(conversation_id) REFERENCES support_conversations(id) ON DELETE CASCADE
		);
		`,
	}
	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}
	if err := s.ensureColumn("agents", "agent_tags_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.ensureColumn("agents", "customer_display_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("agents", "sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureAgentSortOrders(); err != nil {
		return err
	}
	if err := s.ensureColumn("agents", "renewal_config_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := s.ensureColumn("agents", "entry_config_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := s.ensureColumn("admin_accounts", "avatar_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("admin_sessions", "role", "TEXT NOT NULL DEFAULT 'admin'"); err != nil {
		return err
	}
	if err := s.ensureColumn("admin_sessions", "account_id", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn("customer_accounts", "style_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("customer_accounts", "subscription_token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("customer_accounts", "owner_type", "TEXT NOT NULL DEFAULT 'admin'"); err != nil {
		return err
	}
	if err := s.ensureColumn("customer_accounts", "owner_id", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn("area_manager_accounts", "billing_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("area_manager_accounts", "revenue_amount", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("area_manager_accounts", "revenue_currency", "TEXT NOT NULL DEFAULT 'CNY'"); err != nil {
		return err
	}
	if err := s.ensureColumn("area_manager_accounts", "revenue_cycle", "TEXT NOT NULL DEFAULT 'month'"); err != nil {
		return err
	}
	if err := s.ensureColumn("area_manager_accounts", "outbound_create_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("xui_actions", "created_by_role", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("xui_actions", "created_by_account_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("xui_actions", "created_by_username", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "disk_used", definition: "INTEGER"},
		{name: "disk_total", definition: "INTEGER"},
		{name: "net_traffic_sent", definition: "INTEGER"},
		{name: "net_traffic_recv", definition: "INTEGER"},
		{name: "net_traffic_total", definition: "INTEGER"},
		{name: "net_io_up", definition: "INTEGER"},
		{name: "net_io_down", definition: "INTEGER"},
		{name: "history_version", definition: "INTEGER"},
	} {
		if err := s.ensureColumn("snapshots", column.name, column.definition); err != nil {
			return err
		}
	}
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_snapshots_agent_reported_at ON snapshots(agent_id, reported_at DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_snapshot_component_events_agent_component ON snapshot_component_events(agent_id, component, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires_at ON admin_sessions(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_area_manager_agents_agent ON area_manager_agents(agent_id, manager_id);`,
		`CREATE INDEX IF NOT EXISTS idx_area_manager_agent_tags_agent ON area_manager_agent_tags(agent_id, manager_id);`,
		`CREATE INDEX IF NOT EXISTS idx_area_manager_assignments_manager ON area_manager_assignments(manager_id, enabled, id);`,
		`CREATE INDEX IF NOT EXISTS idx_area_manager_assignments_agent ON area_manager_assignments(agent_id, inbound_id, client_email);`,
		`CREATE INDEX IF NOT EXISTS idx_area_manager_outbound_grants_agent ON area_manager_outbound_grants(agent_id, outbound_tag, manager_id);`,
		`CREATE INDEX IF NOT EXISTS idx_front_proxy_nodes_enabled ON front_proxy_nodes(enabled, id);`,
		`CREATE INDEX IF NOT EXISTS idx_front_proxy_grants_grantee ON front_proxy_grants(grantee_type, grantee_id, node_id);`,
		`CREATE INDEX IF NOT EXISTS idx_customer_assignment_front_proxies_node ON customer_assignment_front_proxies(node_id, assignment_id);`,
		`CREATE INDEX IF NOT EXISTS idx_customer_accounts_owner ON customer_accounts(owner_type, owner_id, id);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_accounts_subscription_token ON customer_accounts(subscription_token) WHERE subscription_token <> '';`,
		`CREATE INDEX IF NOT EXISTS idx_customer_sessions_expires_at ON customer_sessions(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_customer_assignments_customer ON customer_assignments(customer_id, enabled, id);`,
		`CREATE INDEX IF NOT EXISTS idx_customer_assignments_agent ON customer_assignments(agent_id, inbound_id, client_email);`,
		`CREATE INDEX IF NOT EXISTS idx_xui_actions_agent_status_id ON xui_actions(agent_id, status, id);`,
		`CREATE INDEX IF NOT EXISTS idx_xui_actions_agent_actor_id ON xui_actions(agent_id, created_by_role, created_by_account_id, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_config_audit_agent_id ON config_audit_logs(agent_id, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_created ON access_logs(created_at DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_agent_created ON access_logs(agent_id, created_at DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_source ON access_logs(source_ip, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_target ON access_logs(target_host, target_ip, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_support_conversations_updated ON support_conversations(status, updated_at DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_support_messages_conversation ON support_messages(conversation_id, id DESC);`,
	}
	for _, stmt := range indexes {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init index: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) ensureAgentSortOrders() error {
	rows, err := s.db.Query(`
		SELECT agent_id
		FROM agents
		WHERE sort_order <= 0
		ORDER BY created_at ASC, agent_id ASC
	`)
	if err != nil {
		return fmt.Errorf("query agents without sort order: %w", err)
	}
	defer rows.Close()

	agentIDs := make([]string, 0)
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return fmt.Errorf("scan agent without sort order: %w", err)
		}
		agentIDs = append(agentIDs, agentID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate agents without sort order: %w", err)
	}
	if len(agentIDs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sort order backfill: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	sortOrder, err := s.nextAgentSortOrderTx(tx)
	if err != nil {
		return err
	}
	for _, agentID := range agentIDs {
		if _, err = tx.Exec(`UPDATE agents SET sort_order = ? WHERE agent_id = ?`, sortOrder, agentID); err != nil {
			return fmt.Errorf("backfill sort order for %s: %w", agentID, err)
		}
		sortOrder++
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sort order backfill: %w", err)
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
