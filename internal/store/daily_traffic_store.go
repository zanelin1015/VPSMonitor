package store

import (
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
		SELECT snapshot_json
		FROM snapshots
		WHERE reported_at >= ? AND reported_at < ?
		ORDER BY agent_id ASC, reported_at ASC, id ASC
	`, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("query daily traffic snapshots: %w", err)
	}
	defer rows.Close()

	type pair struct {
		first model.AgentSnapshot
		last  model.AgentSnapshot
	}
	byAgent := map[string]*pair{}
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, fmt.Errorf("scan daily traffic snapshot: %w", err)
		}
		var snapshot model.AgentSnapshot
		if err := json.Unmarshal([]byte(body), &snapshot); err != nil || snapshot.AgentID == "" {
			continue
		}
		current := byAgent[snapshot.AgentID]
		if current == nil {
			byAgent[snapshot.AgentID] = &pair{first: snapshot, last: snapshot}
			continue
		}
		current.last = snapshot
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily traffic snapshots: %w", err)
	}

	items := make([]model.DailyTrafficUsage, 0, len(byAgent))
	for agentID, item := range byAgent {
		upload := trafficDelta(item.first.Summary.NetTrafficSent, item.last.Summary.NetTrafficSent)
		download := trafficDelta(item.first.Summary.NetTrafficRecv, item.last.Summary.NetTrafficRecv)
		items = append(items, model.DailyTrafficUsage{
			AgentID:   agentID,
			AgentName: item.last.AgentName,
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

func trafficDelta(first, last uint64) uint64 {
	if last >= first {
		return last - first
	}
	return last
}
