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
		SELECT
			agent_id, agent_name, net_traffic_sent, net_traffic_recv, history_version, snapshot_json
		FROM snapshots
		WHERE reported_at >= ? AND reported_at < ?
		ORDER BY agent_id ASC, reported_at ASC, id ASC
	`,
		start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("query daily traffic snapshots: %w", err)
	}
	defer rows.Close()

	type pair struct {
		last     trafficHistoryPoint
		upload   uint64
		download uint64
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
			byAgent[point.agentID] = &pair{last: point}
			continue
		}
		current.upload += trafficDelta(current.last.sent, point.sent)
		current.download += trafficDelta(current.last.received, point.received)
		current.last = point
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily traffic snapshots: %w", err)
	}

	items := make([]model.DailyTrafficUsage, 0, len(byAgent))
	for agentID, item := range byAgent {
		items = append(items, model.DailyTrafficUsage{
			AgentID:   agentID,
			AgentName: item.last.agentName,
			Upload:    item.upload,
			Download:  item.download,
			Total:     item.upload + item.download,
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
