package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

type AccessLogFilter struct {
	AgentID     string
	SourceIP    string
	Target      string
	ClientEmail string
	Limit       int
	Offset      int
}

func (s *SQLiteStore) SaveAccessLogs(agentID string, entries []model.AccessLogEntry) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin access logs: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.Prepare(`
		INSERT INTO access_logs (
			agent_id, agent_name, inbound_id, inbound_tag, client_email, client_id,
			source_ip, source_port, target_host, target_ip, target_port, network, protocol, outbound_tag,
			upload_bytes, download_bytes, duration_ms, raw_summary, started_at, ended_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare access logs: %w", err)
	}
	defer stmt.Close()
	now := time.Now().UTC()
	for _, entry := range entries {
		entry = normalizeAccessLogEntry(agentID, entry, now)
		_, err = stmt.Exec(
			entry.AgentID, entry.AgentName, entry.InboundID, entry.InboundTag, entry.ClientEmail, entry.ClientID,
			entry.SourceIP, entry.SourcePort, entry.TargetHost, entry.TargetIP, entry.TargetPort, entry.Network, entry.Protocol, entry.OutboundTag,
			entry.UploadBytes, entry.DownloadBytes, entry.DurationMS, entry.RawSummary,
			formatOptionalAccessLogTime(entry.StartedAt), formatOptionalAccessLogTime(entry.EndedAt), entry.CreatedAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			return fmt.Errorf("insert access log: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit access logs: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListAccessLogs(filter AccessLogFilter) ([]model.AccessLogEntry, int, error) {
	where, args := accessLogWhere(filter)
	countSQL := `SELECT COUNT(*) FROM access_logs` + where
	var total int
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count access logs: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query := `
		SELECT id, agent_id, agent_name, inbound_id, inbound_tag, client_email, client_id,
			source_ip, source_port, target_host, target_ip, target_port, network, protocol, outbound_tag,
			upload_bytes, download_bytes, duration_ms, raw_summary, started_at, ended_at, created_at
		FROM access_logs` + where + `
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`
	queryArgs := append(args, limit, offset)
	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query access logs: %w", err)
	}
	defer rows.Close()
	items := make([]model.AccessLogEntry, 0, limit)
	for rows.Next() {
		item, err := scanAccessLog(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate access logs: %w", err)
	}
	return items, total, nil
}

func (s *SQLiteStore) PruneAccessLogs(retentionDays int) error {
	if retentionDays <= 0 {
		retentionDays = 7
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`DELETE FROM access_logs WHERE created_at < ?`, cutoff); err != nil {
		return fmt.Errorf("prune access logs: %w", err)
	}
	return nil
}

func accessLogWhere(filter AccessLogFilter) (string, []any) {
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if strings.TrimSpace(filter.AgentID) != "" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, strings.TrimSpace(filter.AgentID))
	}
	if strings.TrimSpace(filter.SourceIP) != "" {
		conditions = append(conditions, "source_ip = ?")
		args = append(args, strings.TrimSpace(filter.SourceIP))
	}
	if strings.TrimSpace(filter.ClientEmail) != "" {
		conditions = append(conditions, "client_email LIKE ?")
		args = append(args, "%"+strings.TrimSpace(filter.ClientEmail)+"%")
	}
	if strings.TrimSpace(filter.Target) != "" {
		conditions = append(conditions, "(target_host LIKE ? OR target_ip LIKE ?)")
		target := "%" + strings.TrimSpace(filter.Target) + "%"
		args = append(args, target, target)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func normalizeAccessLogEntry(agentID string, entry model.AccessLogEntry, now time.Time) model.AccessLogEntry {
	entry.AgentID = agentID
	entry.AgentName = truncateAccessLogField(entry.AgentName, 160)
	entry.InboundTag = truncateAccessLogField(entry.InboundTag, 160)
	entry.ClientEmail = truncateAccessLogField(entry.ClientEmail, 320)
	entry.ClientID = truncateAccessLogField(entry.ClientID, 160)
	entry.SourceIP = truncateAccessLogField(entry.SourceIP, 80)
	entry.TargetHost = truncateAccessLogField(entry.TargetHost, 320)
	entry.TargetIP = truncateAccessLogField(entry.TargetIP, 80)
	entry.Network = truncateAccessLogField(strings.ToLower(entry.Network), 16)
	entry.Protocol = truncateAccessLogField(strings.ToLower(entry.Protocol), 32)
	entry.OutboundTag = truncateAccessLogField(entry.OutboundTag, 160)
	entry.RawSummary = truncateAccessLogField(entry.RawSummary, 500)
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	} else {
		entry.CreatedAt = entry.CreatedAt.UTC()
	}
	if !entry.StartedAt.IsZero() {
		entry.StartedAt = entry.StartedAt.UTC()
	}
	if !entry.EndedAt.IsZero() {
		entry.EndedAt = entry.EndedAt.UTC()
	}
	return entry
}

func truncateAccessLogField(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func formatOptionalAccessLogTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func scanAccessLog(scanner rowScanner) (model.AccessLogEntry, error) {
	var item model.AccessLogEntry
	var startedAt, endedAt, createdAt string
	if err := scanner.Scan(
		&item.ID, &item.AgentID, &item.AgentName, &item.InboundID, &item.InboundTag, &item.ClientEmail, &item.ClientID,
		&item.SourceIP, &item.SourcePort, &item.TargetHost, &item.TargetIP, &item.TargetPort, &item.Network, &item.Protocol, &item.OutboundTag,
		&item.UploadBytes, &item.DownloadBytes, &item.DurationMS, &item.RawSummary, &startedAt, &endedAt, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.AccessLogEntry{}, err
		}
		return model.AccessLogEntry{}, fmt.Errorf("scan access log: %w", err)
	}
	item.StartedAt = parseTime(startedAt)
	item.EndedAt = parseTime(endedAt)
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}
