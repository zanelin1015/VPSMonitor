package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) ListDailyTrafficUsage(day time.Time) ([]model.DailyTrafficUsage, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)
	rows, err := s.db.Query(`
		WITH agent_ids AS (
			SELECT DISTINCT agent_id
			FROM snapshots
			WHERE reported_at >= ? AND reported_at < ?
		), boundaries AS (
			SELECT
				agent_id,
				(
					SELECT id
					FROM snapshots AS first_snapshot
					WHERE first_snapshot.agent_id = agent_ids.agent_id
					  AND first_snapshot.reported_at >= ? AND first_snapshot.reported_at < ?
					ORDER BY first_snapshot.reported_at ASC, first_snapshot.id ASC
					LIMIT 1
				) AS first_id,
				(
					SELECT id
					FROM snapshots AS last_snapshot
					WHERE last_snapshot.agent_id = agent_ids.agent_id
					  AND last_snapshot.reported_at >= ? AND last_snapshot.reported_at < ?
					ORDER BY last_snapshot.reported_at DESC, last_snapshot.id DESC
					LIMIT 1
				) AS last_id
			FROM agent_ids
		)
		SELECT
			snapshot.agent_id,
			snapshot.agent_name,
			snapshot.net_traffic_sent,
			snapshot.net_traffic_recv,
			snapshot.history_version,
			snapshot.snapshot_json
		FROM boundaries
		JOIN snapshots AS snapshot ON snapshot.id = boundaries.first_id OR snapshot.id = boundaries.last_id
		ORDER BY snapshot.agent_id ASC, snapshot.reported_at ASC, snapshot.id ASC
	`,
		start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano),
		start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano),
		start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("query daily traffic snapshots: %w", err)
	}
	defer rows.Close()

	type pair struct {
		first trafficHistoryPoint
		last  trafficHistoryPoint
	}
	byAgent := map[string]*pair{}
	for rows.Next() {
		var (
			agentID        string
			agentName      string
			sent           sql.NullInt64
			received       sql.NullInt64
			historyVersion sql.NullInt64
			snapshotJSON   string
		)
		if err := rows.Scan(&agentID, &agentName, &sent, &received, &historyVersion, &snapshotJSON); err != nil {
			return nil, fmt.Errorf("scan daily traffic snapshot: %w", err)
		}
		point, ok := decodeTrafficHistoryPoint(agentID, agentName, sent, received, historyVersion, snapshotJSON)
		if !ok {
			continue
		}
		current := byAgent[point.agentID]
		if current == nil {
			byAgent[point.agentID] = &pair{first: point, last: point}
			continue
		}
		current.last = point
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily traffic snapshots: %w", err)
	}

	items := make([]model.DailyTrafficUsage, 0, len(byAgent))
	for agentID, item := range byAgent {
		upload := trafficDelta(item.first.sent, item.last.sent)
		download := trafficDelta(item.first.received, item.last.received)
		items = append(items, model.DailyTrafficUsage{
			AgentID:   agentID,
			AgentName: item.last.agentName,
			Upload:    upload,
			Download:  download,
			Total:     upload + download,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Total != items[j].Total {
			return items[i].Total > items[j].Total
		}
		return items[i].AgentID < items[j].AgentID
	})
	return items, nil
}

type trafficHistoryPoint struct {
	agentID   string
	agentName string
	sent      uint64
	received  uint64
}

func decodeTrafficHistoryPoint(
	agentID string,
	agentName string,
	sent sql.NullInt64,
	received sql.NullInt64,
	historyVersion sql.NullInt64,
	snapshotJSON string,
) (trafficHistoryPoint, bool) {
	if historyVersion.Valid {
		return trafficHistoryPoint{
			agentID:   agentID,
			agentName: agentName,
			sent:      nullableUint64(sent),
			received:  nullableUint64(received),
		}, agentID != ""
	}

	var snapshot model.AgentSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil || snapshot.AgentID == "" {
		return trafficHistoryPoint{}, false
	}
	return trafficHistoryPoint{
		agentID:   snapshot.AgentID,
		agentName: snapshot.AgentName,
		sent:      snapshot.Summary.NetTrafficSent,
		received:  snapshot.Summary.NetTrafficRecv,
	}, true
}

func trafficDelta(first, last uint64) uint64 {
	if last >= first {
		return last - first
	}
	return last
}
