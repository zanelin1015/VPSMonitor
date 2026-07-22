package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"bridge-core/internal/model"
)

const (
	compactSnapshotHistoryVersion = 1
	emptySnapshotHistoryJSON      = "{}"
)

type snapshotHistoryScanner interface {
	Scan(dest ...any) error
}

type snapshotHistoryRow struct {
	agentID             string
	agentName           string
	reportedAt          string
	hostname            string
	publicIPv4          string
	publicIPv6          string
	cpu                 float64
	memUsed             int64
	memTotal            int64
	diskUsed            sql.NullInt64
	diskTotal           sql.NullInt64
	netTrafficSent      sql.NullInt64
	netTrafficRecv      sql.NullInt64
	netTrafficTotal     sql.NullInt64
	netIOUp             sql.NullInt64
	netIODown           sql.NullInt64
	xrayState           string
	inboundCount        int
	outboundCount       int
	routingRuleCount    int
	nezhaServerID       int64
	nezhaServerName     string
	lastCollectionError string
	historyVersion      sql.NullInt64
	legacySnapshotJSON  string
}

func (s *SQLiteStore) ListHistory(agentID string, limit int) ([]model.AgentSnapshot, error) {
	query := `
		SELECT
			agent_id, agent_name, reported_at, hostname, public_ipv4, public_ipv6, cpu, mem_used, mem_total,
			disk_used, disk_total, net_traffic_sent, net_traffic_recv, net_traffic_total, net_io_up, net_io_down,
			xray_state, inbound_count, outbound_count, routing_rule_count, nezha_server_id, nezha_server_name,
			last_collection_err, history_version, snapshot_json
		FROM snapshots
		WHERE agent_id = ?
		ORDER BY reported_at DESC, id DESC
	`
	args := []any{agentID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var snapshots []model.AgentSnapshot
	for rows.Next() {
		snapshot, err := scanSnapshotHistory(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
	}
	return snapshots, nil
}

func scanSnapshotHistory(scanner snapshotHistoryScanner) (model.AgentSnapshot, error) {
	var row snapshotHistoryRow
	if err := scanner.Scan(
		&row.agentID,
		&row.agentName,
		&row.reportedAt,
		&row.hostname,
		&row.publicIPv4,
		&row.publicIPv6,
		&row.cpu,
		&row.memUsed,
		&row.memTotal,
		&row.diskUsed,
		&row.diskTotal,
		&row.netTrafficSent,
		&row.netTrafficRecv,
		&row.netTrafficTotal,
		&row.netIOUp,
		&row.netIODown,
		&row.xrayState,
		&row.inboundCount,
		&row.outboundCount,
		&row.routingRuleCount,
		&row.nezhaServerID,
		&row.nezhaServerName,
		&row.lastCollectionError,
		&row.historyVersion,
		&row.legacySnapshotJSON,
	); err != nil {
		return model.AgentSnapshot{}, fmt.Errorf("scan history row: %w", err)
	}

	if !row.historyVersion.Valid {
		var snapshot model.AgentSnapshot
		if err := json.Unmarshal([]byte(row.legacySnapshotJSON), &snapshot); err != nil {
			return model.AgentSnapshot{}, fmt.Errorf("decode legacy history snapshot: %w", err)
		}
		return snapshot, nil
	}

	return model.AgentSnapshot{
		AgentID:    row.agentID,
		AgentName:  row.agentName,
		ReportedAt: parseTime(row.reportedAt),
		Summary: model.VPSSummary{
			Hostname:          row.hostname,
			PublicIPv4:        row.publicIPv4,
			PublicIPv6:        row.publicIPv6,
			CPU:               row.cpu,
			MemUsed:           nonNegativeUint64(row.memUsed),
			MemTotal:          nonNegativeUint64(row.memTotal),
			DiskUsed:          nullableUint64(row.diskUsed),
			DiskTotal:         nullableUint64(row.diskTotal),
			NetTrafficSent:    nullableUint64(row.netTrafficSent),
			NetTrafficRecv:    nullableUint64(row.netTrafficRecv),
			NetTrafficTotal:   nullableUint64(row.netTrafficTotal),
			NetIOUp:           nullableUint64(row.netIOUp),
			NetIODown:         nullableUint64(row.netIODown),
			XrayState:         row.xrayState,
			InboundCount:      row.inboundCount,
			OutboundCount:     row.outboundCount,
			RoutingRuleCount:  row.routingRuleCount,
			NezhaServerID:     nonNegativeUint64(row.nezhaServerID),
			NezhaServerName:   row.nezhaServerName,
			LastCollectionErr: row.lastCollectionError,
		},
	}, nil
}

func nullableUint64(value sql.NullInt64) uint64 {
	if !value.Valid {
		return 0
	}
	return nonNegativeUint64(value.Int64)
}

func nonNegativeUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}
