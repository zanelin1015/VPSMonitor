package store

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) saveRealmTrafficTx(tx *sql.Tx, snapshot model.AgentSnapshot) error {
	if snapshot.Realm == nil || snapshot.Realm.Traffic == nil {
		return nil
	}
	traffic := snapshot.Realm.Traffic
	if !traffic.Enabled || traffic.Error != "" || traffic.CollectedAt.IsZero() || traffic.CollectedAt.After(snapshot.ReportedAt.Add(2*time.Minute)) {
		return nil
	}
	at := traffic.CollectedAt.UTC()
	stamp := at.Format(time.RFC3339Nano)
	day := at.In(model.BeijingLocation).Format("2006-01-02")
	for _, counter := range traffic.Counters {
		if counter.Key == "" || counter.RuleID == "" || counter.Epoch == "" || counter.ListenPort <= 0 || counter.ListenPort > 65535 || counter.UploadBytes > math.MaxInt64 || counter.DownloadBytes > math.MaxInt64 {
			continue
		}
		var epoch, previousAt, firstAt string
		var upload, download uint64
		err := tx.QueryRow(`SELECT epoch, upload_bytes, download_bytes, first_observed_at, reported_at
			FROM realm_traffic_counters WHERE agent_id = ? AND counter_key = ?`, snapshot.AgentID, counter.Key).
			Scan(&epoch, &upload, &download, &firstAt, &previousAt)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("load realm traffic baseline: %w", err)
		}
		var upDelta, downDelta uint64
		if err == sql.ErrNoRows {
			// First contact establishes a baseline, without inventing usage for
			// a period before the server started observing this counter.
			firstAt = stamp
		} else {
			if !at.After(parseTime(previousAt)) {
				continue // Duplicate and delayed heartbeats must not double charge.
			}
			if epoch != counter.Epoch {
				upDelta, downDelta = counter.UploadBytes, counter.DownloadBytes
			} else {
				upDelta, downDelta = trafficDelta(upload, counter.UploadBytes), trafficDelta(download, counter.DownloadBytes)
			}
		}
		if _, err := tx.Exec(`INSERT INTO realm_traffic_counters
			(agent_id, counter_key, rule_id, name, listen_address, listen_port, network, epoch, upload_bytes, download_bytes, first_observed_at, reported_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(agent_id, counter_key) DO UPDATE SET rule_id=excluded.rule_id, name=excluded.name,
			listen_address=excluded.listen_address, listen_port=excluded.listen_port, network=excluded.network,
			epoch=excluded.epoch, upload_bytes=excluded.upload_bytes, download_bytes=excluded.download_bytes, reported_at=excluded.reported_at`,
			snapshot.AgentID, counter.Key, counter.RuleID, counter.Name, counter.ListenAddress, counter.ListenPort, counter.Network,
			counter.Epoch, counter.UploadBytes, counter.DownloadBytes, firstAt, stamp); err != nil {
			return fmt.Errorf("save realm traffic baseline: %w", err)
		}
		if upDelta == 0 && downDelta == 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO realm_traffic_daily(agent_id, counter_key, day, upload_bytes, download_bytes) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(agent_id, counter_key, day) DO UPDATE SET upload_bytes=upload_bytes+excluded.upload_bytes, download_bytes=download_bytes+excluded.download_bytes`,
			snapshot.AgentID, counter.Key, day, upDelta, downDelta); err != nil {
			return fmt.Errorf("save realm daily traffic: %w", err)
		}
	}
	return nil
}

// Dates are inclusive Beijing calendar dates. Each delta belongs to the day
// of its latest sample; boundary precision is the Client reporting interval.
func (s *SQLiteStore) ListRealmTrafficUsage(agentID, from, to string) ([]model.RealmTrafficUsage, error) {
	rows, err := s.db.Query(`SELECT c.counter_key, c.rule_id, c.name, c.listen_address, c.listen_port, c.network,
		COALESCE(SUM(d.upload_bytes),0), COALESCE(SUM(d.download_bytes),0), c.first_observed_at, c.reported_at
		FROM realm_traffic_counters c LEFT JOIN realm_traffic_daily d
		ON d.agent_id=c.agent_id AND d.counter_key=c.counter_key AND d.day>=? AND d.day<=?
		WHERE c.agent_id=? GROUP BY c.counter_key ORDER BY c.listen_port, c.counter_key`, from, to, agentID)
	if err != nil {
		return nil, fmt.Errorf("query realm traffic: %w", err)
	}
	defer rows.Close()
	items := make([]model.RealmTrafficUsage, 0)
	for rows.Next() {
		var item model.RealmTrafficUsage
		var first, last string
		if err := rows.Scan(&item.Key, &item.RuleID, &item.Name, &item.ListenAddress, &item.ListenPort, &item.Network,
			&item.UploadBytes, &item.DownloadBytes, &first, &last); err != nil {
			return nil, fmt.Errorf("scan realm traffic: %w", err)
		}
		item.FirstObservedAt, item.LastObservedAt = parseTime(first), parseTime(last)
		item.TotalBytes = item.UploadBytes + item.DownloadBytes
		items = append(items, item)
	}
	return items, rows.Err()
}
