package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func (s *SQLiteStore) SeedAgents(agents []config.ServerAgentAuth) error {
	for _, agent := range agents {
		if agent.ID == "" {
			continue
		}
		xuiJSON, err := s.storedXUIConfigJSON(config.XUIConfig{})
		if err != nil {
			return err
		}
		sortOrder, err := s.nextAgentSortOrder()
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = s.db.Exec(`
			INSERT INTO agents (
				agent_id, agent_name, sort_order, agent_tags_json, agent_token, hostname, public_ipv4, public_ipv6,
				created_at, updated_at, last_seen_at, xui_config_json, nezha_config_json, renewal_config_json, entry_config_json
			) VALUES (?, ?, ?, '[]', ?, '', '', '', ?, ?, '', ?, ?, ?, ?)
			ON CONFLICT(agent_id) DO UPDATE SET
				agent_name = CASE WHEN excluded.agent_name <> '' THEN excluded.agent_name ELSE agents.agent_name END,
				agent_token = CASE WHEN excluded.agent_token <> '' THEN excluded.agent_token ELSE agents.agent_token END,
				updated_at = excluded.updated_at
		`,
			agent.ID,
			agent.Name,
			sortOrder,
			agent.Token,
			now,
			now,
			xuiJSON,
			mustJSON(config.NezhaConfig{}),
			mustJSON(model.VPSRenewalConfig{}),
			mustJSON(model.AgentEntryConfig{}),
		)
		if err != nil {
			return fmt.Errorf("seed agent %s: %w", agent.ID, err)
		}
	}
	return nil
}

func (s *SQLiteStore) RegisterAgent(req model.AgentRegisterRequest) (model.AgentRegisterResponse, error) {
	if req.AgentID == "" {
		return model.AgentRegisterResponse{}, fmt.Errorf("agent_id is required")
	}

	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)

	tx, err := s.db.Begin()
	if err != nil {
		return model.AgentRegisterResponse{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	record, found, err := s.loadAgentTx(tx, req.AgentID)
	if err != nil {
		return model.AgentRegisterResponse{}, err
	}

	if !found {
		token, tokenErr := randomToken()
		if tokenErr != nil {
			return model.AgentRegisterResponse{}, tokenErr
		}
		sortOrder, sortErr := s.nextAgentSortOrderTx(tx)
		if sortErr != nil {
			return model.AgentRegisterResponse{}, sortErr
		}
		record = model.AgentRecord{
			AgentID:      req.AgentID,
			AgentName:    firstNonEmpty(req.AgentName, req.SeedConfig.AgentName, req.AgentID),
			SortOrder:    sortOrder,
			Tags:         normalizeTags(req.SeedConfig.Tags),
			AgentToken:   token,
			Hostname:     req.Hostname,
			PublicIPv4:   req.PublicIPv4,
			PublicIPv6:   req.PublicIPv6,
			RegisteredAt: now,
			UpdatedAt:    now,
			Config: model.ManagedAgentConfig{
				AgentID:   req.AgentID,
				AgentName: firstNonEmpty(req.AgentName, req.SeedConfig.AgentName, req.AgentID),
				SortOrder: sortOrder,
				Tags:      normalizeTags(req.SeedConfig.Tags),
				Renewal:   normalizeRenewalConfig(req.SeedConfig.Renewal),
				Entry:     normalizeEntryConfig(req.SeedConfig.Entry),
				XUI:       req.SeedConfig.XUI,
			},
			HasConfig: hasManagedConfig(req.SeedConfig),
		}
		xuiJSON, xuiErr := s.storedXUIConfigJSON(record.Config.XUI)
		if xuiErr != nil {
			return model.AgentRegisterResponse{}, xuiErr
		}
		_, err = tx.Exec(`
			INSERT INTO agents (
				agent_id, agent_name, sort_order, agent_tags_json, agent_token, hostname, public_ipv4, public_ipv6,
				created_at, updated_at, last_seen_at, xui_config_json, nezha_config_json, renewal_config_json, entry_config_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?)
		`,
			record.AgentID,
			record.AgentName,
			record.SortOrder,
			mustJSON(record.Config.Tags),
			record.AgentToken,
			record.Hostname,
			record.PublicIPv4,
			record.PublicIPv6,
			nowText,
			nowText,
			xuiJSON,
			mustJSON(config.NezhaConfig{}),
			mustJSON(record.Config.Renewal),
			mustJSON(record.Config.Entry),
		)
		if err != nil {
			return model.AgentRegisterResponse{}, fmt.Errorf("insert registered agent: %w", err)
		}
	} else {
		if record.AgentToken == "" {
			record.AgentToken, err = randomToken()
			if err != nil {
				return model.AgentRegisterResponse{}, err
			}
		}
		if record.AgentName == "" {
			record.AgentName = firstNonEmpty(req.AgentName, req.SeedConfig.AgentName, req.AgentID)
		}
		if record.Hostname == "" {
			record.Hostname = req.Hostname
		}
		if record.PublicIPv4 == "" {
			record.PublicIPv4 = req.PublicIPv4
		}
		if record.PublicIPv6 == "" {
			record.PublicIPv6 = req.PublicIPv6
		}
		if !hasXUIConfig(record.Config.XUI) && hasXUIConfig(req.SeedConfig.XUI) {
			record.Config.XUI = req.SeedConfig.XUI
		}
		if len(record.Config.Tags) == 0 && len(req.SeedConfig.Tags) > 0 {
			record.Config.Tags = normalizeTags(req.SeedConfig.Tags)
		}
		if !hasRenewalConfig(record.Config.Renewal) && hasRenewalConfig(req.SeedConfig.Renewal) {
			record.Config.Renewal = normalizeRenewalConfig(req.SeedConfig.Renewal)
		}
		if !hasEntryConfig(record.Config.Entry) && hasEntryConfig(req.SeedConfig.Entry) {
			record.Config.Entry = normalizeEntryConfig(req.SeedConfig.Entry)
		}
		record.Config.AgentID = req.AgentID
		record.Config.AgentName = record.AgentName
		record.Tags = cloneStrings(record.Config.Tags)
		record.UpdatedAt = now
		record.HasConfig = hasManagedConfig(record.Config)

		xuiJSON, xuiErr := s.storedXUIConfigJSON(record.Config.XUI)
		if xuiErr != nil {
			return model.AgentRegisterResponse{}, xuiErr
		}
		_, err = tx.Exec(`
			UPDATE agents
			SET
				agent_name = ?,
				agent_tags_json = ?,
				agent_token = ?,
				hostname = CASE WHEN ? <> '' THEN ? ELSE hostname END,
				public_ipv4 = CASE WHEN ? <> '' THEN ? ELSE public_ipv4 END,
				public_ipv6 = CASE WHEN ? <> '' THEN ? ELSE public_ipv6 END,
				updated_at = ?,
				xui_config_json = ?,
				nezha_config_json = ?,
				renewal_config_json = ?,
				entry_config_json = ?
			WHERE agent_id = ?
			`,
			record.AgentName,
			mustJSON(record.Config.Tags),
			record.AgentToken,
			req.Hostname,
			req.Hostname,
			req.PublicIPv4,
			req.PublicIPv4,
			req.PublicIPv6,
			req.PublicIPv6,
			nowText,
			xuiJSON,
			mustJSON(config.NezhaConfig{}),
			mustJSON(record.Config.Renewal),
			mustJSON(record.Config.Entry),
			req.AgentID,
		)
		if err != nil {
			return model.AgentRegisterResponse{}, fmt.Errorf("update registered agent: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return model.AgentRegisterResponse{}, fmt.Errorf("commit transaction: %w", err)
	}

	return model.AgentRegisterResponse{
		AgentID:      record.AgentID,
		AgentName:    record.AgentName,
		AgentToken:   record.AgentToken,
		RegisteredAt: record.RegisteredAt,
		Config: model.ManagedAgentConfig{
			AgentID:   record.AgentID,
			AgentName: record.AgentName,
			SortOrder: record.SortOrder,
			Tags:      cloneStrings(record.Config.Tags),
			Renewal:   record.Config.Renewal,
			Entry:     record.Config.Entry,
			XUI:       record.Config.XUI,
		},
	}, nil
}

func (s *SQLiteStore) GetAgent(agentID string) (model.AgentRecord, bool, error) {
	return s.loadAgent(agentID)
}

func (s *SQLiteStore) GetAgentConfig(agentID string) (model.ManagedAgentConfig, bool, error) {
	record, found, err := s.GetAgent(agentID)
	if err != nil || !found {
		return model.ManagedAgentConfig{}, found, err
	}
	cfg := record.Config
	cfg.AgentID = record.AgentID
	cfg.AgentName = record.AgentName
	return cfg, true, nil
}

func (s *SQLiteStore) UpdateAgentConfig(agentID string, cfg model.ManagedAgentConfig) (model.AgentRecord, error) {
	return s.UpdateAgentConfigWithActor(agentID, cfg, "system")
}

func (s *SQLiteStore) updateAgentConfig(agentID string, cfg model.ManagedAgentConfig) (model.AgentRecord, error) {
	record, found, err := s.GetAgent(agentID)
	if err != nil {
		return model.AgentRecord{}, err
	}
	if !found {
		return model.AgentRecord{}, fmt.Errorf("agent not found")
	}

	record.AgentName = firstNonEmpty(cfg.AgentName, record.AgentName, agentID)
	if cfg.SortOrder > 0 {
		record.SortOrder = cfg.SortOrder
	}
	if record.SortOrder <= 0 {
		record.SortOrder, err = s.nextAgentSortOrder()
		if err != nil {
			return model.AgentRecord{}, err
		}
	}
	record.Config = model.ManagedAgentConfig{
		AgentID:   agentID,
		AgentName: record.AgentName,
		SortOrder: record.SortOrder,
		Tags:      normalizeTags(cfg.Tags),
		Renewal:   normalizeRenewalConfig(cfg.Renewal),
		Entry:     normalizeEntryConfig(cfg.Entry),
		XUI:       cfg.XUI,
	}
	record.Tags = cloneStrings(record.Config.Tags)
	record.UpdatedAt = time.Now().UTC()
	if snapshot, ok := s.GetLatest(agentID); ok {
		summary := snapshot.Summary
		applyRenewalTrafficBaseline(&record.Config.Renewal, summary.NetTrafficSent, summary.NetTrafficRecv, summary.NetTrafficTotal, snapshot.ReportedAt)
	}
	record.HasConfig = hasManagedConfig(record.Config)

	xuiJSON, err := s.storedXUIConfigJSON(record.Config.XUI)
	if err != nil {
		return model.AgentRecord{}, err
	}
	_, err = s.db.Exec(`
		UPDATE agents
		SET agent_name = ?, sort_order = ?, agent_tags_json = ?, updated_at = ?, xui_config_json = ?, nezha_config_json = ?, renewal_config_json = ?, entry_config_json = ?
		WHERE agent_id = ?
	`,
		record.AgentName,
		record.SortOrder,
		mustJSON(record.Config.Tags),
		record.UpdatedAt.Format(time.RFC3339Nano),
		xuiJSON,
		mustJSON(config.NezhaConfig{}),
		mustJSON(record.Config.Renewal),
		mustJSON(record.Config.Entry),
		agentID,
	)
	if err != nil {
		return model.AgentRecord{}, fmt.Errorf("update agent config: %w", err)
	}

	return record, nil
}

func (s *SQLiteStore) ValidateAgentToken(agentID, token string) bool {
	if agentID == "" || token == "" {
		return false
	}

	var storedToken string
	err := s.db.QueryRow(`SELECT agent_token FROM agents WHERE agent_id = ?`, agentID).Scan(&storedToken)
	if err != nil {
		return false
	}
	return storedToken != "" && storedToken == token
}

func (s *SQLiteStore) nextAgentSortOrder() (int, error) {
	return nextAgentSortOrderQuery(s.db)
}

func (s *SQLiteStore) nextAgentSortOrderTx(tx *sql.Tx) (int, error) {
	return nextAgentSortOrderQuery(tx)
}

func nextAgentSortOrderQuery(db queryer) (int, error) {
	var maxOrder int
	if err := db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM agents`).Scan(&maxOrder); err != nil {
		return 0, fmt.Errorf("load next agent sort order: %w", err)
	}
	return maxOrder + 1, nil
}

func (s *SQLiteStore) ListAgents() ([]model.AgentRecord, error) {
	rows, err := s.db.Query(`
		SELECT
			a.agent_id,
			a.agent_name,
			a.sort_order,
			a.agent_tags_json,
			a.agent_token,
			a.hostname,
			a.public_ipv4,
			a.public_ipv6,
			a.created_at,
			a.updated_at,
			a.last_seen_at,
			a.xui_config_json,
			a.nezha_config_json,
			a.renewal_config_json,
			a.entry_config_json,
			ls.reported_at,
			ls.snapshot_json
		FROM agents a
		LEFT JOIN latest_snapshots ls ON ls.agent_id = a.agent_id
		ORDER BY CASE WHEN a.sort_order > 0 THEN a.sort_order ELSE 2147483647 END ASC, a.created_at ASC, a.agent_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	agents := make([]model.AgentRecord, 0)
	for rows.Next() {
		var (
			record         model.AgentRecord
			createdAtText  string
			updatedAtText  string
			lastSeenText   string
			tagsJSON       string
			xuiJSON        string
			nezhaJSON      string
			renewalJSON    string
			entryJSON      string
			reportedAtText sql.NullString
			snapshotJSON   sql.NullString
		)
		if err := rows.Scan(
			&record.AgentID,
			&record.AgentName,
			&record.SortOrder,
			&tagsJSON,
			&record.AgentToken,
			&record.Hostname,
			&record.PublicIPv4,
			&record.PublicIPv6,
			&createdAtText,
			&updatedAtText,
			&lastSeenText,
			&xuiJSON,
			&nezhaJSON,
			&renewalJSON,
			&entryJSON,
			&reportedAtText,
			&snapshotJSON,
		); err != nil {
			return nil, fmt.Errorf("scan agent row: %w", err)
		}

		record.RegisteredAt = parseTime(createdAtText)
		record.UpdatedAt = parseTime(updatedAtText)
		if lastSeenText != "" {
			lastSeen := parseTime(lastSeenText)
			record.LastSeenAt = &lastSeen
		}
		record.Config, err = s.parseManagedConfig(record.AgentID, record.AgentName, record.SortOrder, tagsJSON, xuiJSON, nezhaJSON, renewalJSON, entryJSON)
		if err != nil {
			return nil, err
		}
		record.Tags = cloneStrings(record.Config.Tags)
		record.HasConfig = hasManagedConfig(record.Config)

		if reportedAtText.Valid && reportedAtText.String != "" {
			reportedAt := parseTime(reportedAtText.String)
			record.ReportedAt = &reportedAt
		}
		if snapshotJSON.Valid && snapshotJSON.String != "" {
			var snapshot model.AgentSnapshot
			if err := json.Unmarshal([]byte(snapshotJSON.String), &snapshot); err == nil {
				record.Version = snapshot.Version
				record.OS = snapshot.OS
				record.Arch = snapshot.Arch
				record.SystemVersion = snapshot.SystemVersion
				record.Summary = snapshot.Summary
				if record.Hostname == "" {
					record.Hostname = snapshot.Summary.Hostname
				}
				if record.PublicIPv4 == "" {
					record.PublicIPv4 = snapshot.Summary.PublicIPv4
				}
				if record.PublicIPv6 == "" {
					record.PublicIPv6 = snapshot.Summary.PublicIPv6
				}
			}
		}

		agents = append(agents, record)
	}
	return agents, nil
}

func (s *SQLiteStore) loadAgent(agentID string) (model.AgentRecord, bool, error) {
	return s.loadAgentQuery(s.db, agentID)
}

func (s *SQLiteStore) loadAgentTx(tx *sql.Tx, agentID string) (model.AgentRecord, bool, error) {
	return s.loadAgentQuery(tx, agentID)
}

func (s *SQLiteStore) loadAgentQuery(db queryer, agentID string) (model.AgentRecord, bool, error) {
	var (
		record        model.AgentRecord
		createdAtText string
		updatedAtText string
		lastSeenText  string
		tagsJSON      string
		xuiJSON       string
		nezhaJSON     string
		renewalJSON   string
		entryJSON     string
	)
	err := db.QueryRow(`
		SELECT agent_id, agent_name, sort_order, agent_tags_json, agent_token, hostname, public_ipv4, public_ipv6,
		       created_at, updated_at, last_seen_at, xui_config_json, nezha_config_json, renewal_config_json, entry_config_json
		FROM agents
		WHERE agent_id = ?
	`, agentID).Scan(
		&record.AgentID,
		&record.AgentName,
		&record.SortOrder,
		&tagsJSON,
		&record.AgentToken,
		&record.Hostname,
		&record.PublicIPv4,
		&record.PublicIPv6,
		&createdAtText,
		&updatedAtText,
		&lastSeenText,
		&xuiJSON,
		&nezhaJSON,
		&renewalJSON,
		&entryJSON,
	)
	if err == sql.ErrNoRows {
		return model.AgentRecord{}, false, nil
	}
	if err != nil {
		return model.AgentRecord{}, false, fmt.Errorf("load agent %s: %w", agentID, err)
	}

	record.RegisteredAt = parseTime(createdAtText)
	record.UpdatedAt = parseTime(updatedAtText)
	if lastSeenText != "" {
		lastSeen := parseTime(lastSeenText)
		record.LastSeenAt = &lastSeen
	}
	record.Config, err = s.parseManagedConfig(record.AgentID, record.AgentName, record.SortOrder, tagsJSON, xuiJSON, nezhaJSON, renewalJSON, entryJSON)
	if err != nil {
		return model.AgentRecord{}, false, err
	}
	record.Tags = cloneStrings(record.Config.Tags)
	record.HasConfig = hasManagedConfig(record.Config)
	return record, true, nil
}
