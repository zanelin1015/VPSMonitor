package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func (s *SQLiteStore) SaveSnapshot(snapshot model.AgentSnapshot) error {
	if snapshot.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if snapshot.ReportedAt.IsZero() {
		snapshot.ReportedAt = time.Now().UTC()
	} else {
		snapshot.ReportedAt = snapshot.ReportedAt.UTC()
	}
	reportedAt := snapshot.ReportedAt.Format(time.RFC3339Nano)

	latestBody, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	summary := snapshot.Summary
	_, err = tx.Exec(`
		INSERT INTO snapshots (
			agent_id, agent_name, reported_at, hostname, public_ipv4, public_ipv6, cpu, mem_used, mem_total,
			xray_state, inbound_count, outbound_count, routing_rule_count, nezha_server_id, nezha_server_name,
			last_collection_err, disk_used, disk_total, net_traffic_sent, net_traffic_recv, net_traffic_total,
			net_io_up, net_io_down, history_version, snapshot_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		snapshot.AgentID,
		snapshot.AgentName,
		reportedAt,
		summary.Hostname,
		summary.PublicIPv4,
		summary.PublicIPv6,
		summary.CPU,
		summary.MemUsed,
		summary.MemTotal,
		summary.XrayState,
		summary.InboundCount,
		summary.OutboundCount,
		summary.RoutingRuleCount,
		summary.NezhaServerID,
		summary.NezhaServerName,
		summary.LastCollectionErr,
		summary.DiskUsed,
		summary.DiskTotal,
		summary.NetTrafficSent,
		summary.NetTrafficRecv,
		summary.NetTrafficTotal,
		summary.NetIOUp,
		summary.NetIODown,
		compactSnapshotHistoryVersion,
		emptySnapshotHistoryJSON,
	)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO latest_snapshots (
			agent_id, agent_name, reported_at, hostname, public_ipv4, public_ipv6, cpu, mem_used, mem_total,
			xray_state, inbound_count, outbound_count, routing_rule_count, nezha_server_id, nezha_server_name,
			last_collection_err, snapshot_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			agent_name = excluded.agent_name,
			reported_at = excluded.reported_at,
			hostname = excluded.hostname,
			public_ipv4 = excluded.public_ipv4,
			public_ipv6 = excluded.public_ipv6,
			cpu = excluded.cpu,
			mem_used = excluded.mem_used,
			mem_total = excluded.mem_total,
			xray_state = excluded.xray_state,
			inbound_count = excluded.inbound_count,
			outbound_count = excluded.outbound_count,
			routing_rule_count = excluded.routing_rule_count,
			nezha_server_id = excluded.nezha_server_id,
			nezha_server_name = excluded.nezha_server_name,
			last_collection_err = excluded.last_collection_err,
			snapshot_json = excluded.snapshot_json
	`,
		snapshot.AgentID,
		snapshot.AgentName,
		reportedAt,
		summary.Hostname,
		summary.PublicIPv4,
		summary.PublicIPv6,
		summary.CPU,
		summary.MemUsed,
		summary.MemTotal,
		summary.XrayState,
		summary.InboundCount,
		summary.OutboundCount,
		summary.RoutingRuleCount,
		summary.NezhaServerID,
		summary.NezhaServerName,
		summary.LastCollectionErr,
		string(latestBody),
	)
	if err != nil {
		return fmt.Errorf("upsert latest snapshot: %w", err)
	}
	if err = s.saveSnapshotComponentEventsTx(tx, snapshot); err != nil {
		return err
	}

	nowText := time.Now().UTC().Format(time.RFC3339Nano)
	emptyXUIJSON, err := s.storedXUIConfigJSON(config.XUIConfig{})
	if err != nil {
		return err
	}
	sortOrder, err := s.nextAgentSortOrderTx(tx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO agents (
			agent_id, agent_name, sort_order, agent_tags_json, agent_token, hostname, public_ipv4, public_ipv6,
			created_at, updated_at, last_seen_at, xui_config_json, nezha_config_json, renewal_config_json, entry_config_json
		) VALUES (?, ?, ?, '[]', '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			agent_name = CASE WHEN excluded.agent_name <> '' THEN excluded.agent_name ELSE agents.agent_name END,
			hostname = CASE WHEN excluded.hostname <> '' THEN excluded.hostname ELSE agents.hostname END,
			public_ipv4 = CASE WHEN excluded.public_ipv4 <> '' THEN excluded.public_ipv4 ELSE agents.public_ipv4 END,
			public_ipv6 = CASE WHEN excluded.public_ipv6 <> '' THEN excluded.public_ipv6 ELSE agents.public_ipv6 END,
			updated_at = excluded.updated_at,
			last_seen_at = excluded.last_seen_at
	`,
		snapshot.AgentID,
		snapshot.AgentName,
		sortOrder,
		summary.Hostname,
		summary.PublicIPv4,
		summary.PublicIPv6,
		nowText,
		nowText,
		reportedAt,
		emptyXUIJSON,
		mustJSON(config.NezhaConfig{}),
		mustJSON(model.VPSRenewalConfig{}),
		mustJSON(model.AgentEntryConfig{}),
	)
	if err != nil {
		return fmt.Errorf("upsert agent heartbeat metadata: %w", err)
	}

	if err = updateRenewalTrafficBaselineTx(tx, snapshot.AgentID, summary.NetTrafficSent, summary.NetTrafficRecv, summary.NetTrafficTotal, snapshot.ReportedAt); err != nil {
		return fmt.Errorf("update renewal traffic baseline: %w", err)
	}

	if err = s.pruneSnapshotHistoryTx(tx, snapshot.AgentID, snapshot.ReportedAt); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListLatest() []model.AgentSnapshot {
	rows, err := s.db.Query(`
		SELECT snapshot_json
		FROM latest_snapshots
		ORDER BY reported_at DESC, agent_id ASC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var snapshots []model.AgentSnapshot
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			continue
		}
		var snapshot model.AgentSnapshot
		if err := json.Unmarshal([]byte(body), &snapshot); err != nil {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ReportedAt.After(snapshots[j].ReportedAt)
	})
	return snapshots
}

func (s *SQLiteStore) GetLatest(agentID string) (model.AgentSnapshot, bool) {
	var body string
	err := s.db.QueryRow(`
		SELECT snapshot_json
		FROM latest_snapshots
		WHERE agent_id = ?
	`, agentID).Scan(&body)
	if err != nil {
		return model.AgentSnapshot{}, false
	}

	var snapshot model.AgentSnapshot
	if err := json.Unmarshal([]byte(body), &snapshot); err != nil {
		return model.AgentSnapshot{}, false
	}
	return snapshot, true
}

func (s *SQLiteStore) pruneSnapshotHistoryTx(tx *sql.Tx, agentID string, referenceTime time.Time) error {
	if agentID == "" {
		return nil
	}
	if s.retention.MaxAge > 0 {
		cutoff := referenceTime.UTC().Add(-s.retention.MaxAge).Format(time.RFC3339Nano)
		if _, err := tx.Exec(`
			DELETE FROM snapshots
			WHERE agent_id = ? AND reported_at < ?
		`, agentID, cutoff); err != nil {
			return fmt.Errorf("prune old snapshots: %w", err)
		}
	}
	if s.retention.MaxPerAgent > 0 {
		if _, err := tx.Exec(`
			DELETE FROM snapshots
			WHERE agent_id = ?
			  AND id NOT IN (
				SELECT id
				FROM snapshots
				WHERE agent_id = ?
				ORDER BY reported_at DESC, id DESC
				LIMIT ?
			  )
		`, agentID, agentID, s.retention.MaxPerAgent); err != nil {
			return fmt.Errorf("prune excess snapshots: %w", err)
		}
	}
	return nil
}
