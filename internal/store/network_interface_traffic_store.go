package store

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) saveNetworkInterfaceTrafficTx(tx *sql.Tx, snapshot model.AgentSnapshot) error {
	traffic := snapshot.NetworkInterfaces
	if traffic == nil || traffic.Error != "" || traffic.CollectedAt.IsZero() || traffic.CollectedAt.After(snapshot.ReportedAt.Add(2*time.Minute)) {
		return nil
	}
	at := traffic.CollectedAt.UTC()
	stamp := at.Format(time.RFC3339Nano)
	var previousSnapshot string
	err := tx.QueryRow(`SELECT reported_at FROM network_interface_observations WHERE agent_id=?`, snapshot.AgentID).Scan(&previousSnapshot)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load interface observation: %w", err)
	}
	if err == nil && !at.After(parseTime(previousSnapshot)) {
		return nil
	}
	if _, err := tx.Exec(`INSERT INTO network_interface_observations(agent_id,reported_at) VALUES (?,?)
		ON CONFLICT(agent_id) DO UPDATE SET reported_at=excluded.reported_at`, snapshot.AgentID, stamp); err != nil {
		return err
	}
	// A complete, valid discovery marks absent interfaces without erasing their
	// historical totals. Missing/error components leave the last discovery alone.
	if _, err := tx.Exec(`UPDATE network_interface_counters SET present=0,is_primary=0 WHERE agent_id=?`, snapshot.AgentID); err != nil {
		return err
	}
	seen := map[string]bool{}
	day := at.In(model.BeijingLocation).Format("2006-01-02")
	for _, item := range traffic.Interfaces {
		if item.Key == "" || item.Name == "" || item.Index <= 0 || item.Epoch == "" || seen[item.Key] || item.ReceivedBytes > math.MaxInt64 || item.SentBytes > math.MaxInt64 {
			continue
		}
		seen[item.Key] = true
		var epoch, firstAt string
		var received, sent uint64
		err := tx.QueryRow(`SELECT epoch,received_bytes,sent_bytes,first_observed_at FROM network_interface_counters WHERE agent_id=? AND counter_key=?`, snapshot.AgentID, item.Key).
			Scan(&epoch, &received, &sent, &firstAt)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("load interface baseline: %w", err)
		}
		var rxDelta, txDelta uint64
		if err == sql.ErrNoRows {
			firstAt = stamp
		} else if epoch != item.Epoch {
			rxDelta, txDelta = item.ReceivedBytes, item.SentBytes
		} else {
			rxDelta, txDelta = trafficDelta(received, item.ReceivedBytes), trafficDelta(sent, item.SentBytes)
		}
		if _, err := tx.Exec(`INSERT INTO network_interface_counters
			(agent_id,counter_key,name,interface_index,mac,kind,is_up,loopback,is_primary,present,epoch,received_bytes,sent_bytes,first_observed_at,reported_at)
			VALUES (?,?,?,?,?,?,?,?,?,1,?,?,?,?,?) ON CONFLICT(agent_id,counter_key) DO UPDATE SET
			name=excluded.name,interface_index=excluded.interface_index,mac=excluded.mac,kind=excluded.kind,is_up=excluded.is_up,loopback=excluded.loopback,
			is_primary=excluded.is_primary,present=1,epoch=excluded.epoch,received_bytes=excluded.received_bytes,sent_bytes=excluded.sent_bytes,reported_at=excluded.reported_at`,
			snapshot.AgentID, item.Key, item.Name, item.Index, item.MAC, item.Kind, boolInt(item.Up), boolInt(item.Loopback), boolInt(item.Primary), item.Epoch, item.ReceivedBytes, item.SentBytes, firstAt, stamp); err != nil {
			return fmt.Errorf("save interface baseline: %w", err)
		}
		if rxDelta == 0 && txDelta == 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO network_interface_daily(agent_id,counter_key,day,received_bytes,sent_bytes) VALUES (?,?,?,?,?)
			ON CONFLICT(agent_id,counter_key,day) DO UPDATE SET received_bytes=received_bytes+excluded.received_bytes,sent_bytes=sent_bytes+excluded.sent_bytes`,
			snapshot.AgentID, item.Key, day, rxDelta, txDelta); err != nil {
			return fmt.Errorf("save daily interface traffic: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) ListNetworkInterfaceTraffic(agentID, from, to string) ([]model.NetworkInterfaceTrafficUsage, error) {
	rows, err := s.db.Query(`SELECT c.counter_key,c.name,c.interface_index,c.mac,c.kind,c.is_up,c.loopback,c.is_primary,c.present,
		COALESCE(SUM(d.received_bytes),0),COALESCE(SUM(d.sent_bytes),0),c.first_observed_at,c.reported_at
		FROM network_interface_counters c LEFT JOIN network_interface_daily d
		ON d.agent_id=c.agent_id AND d.counter_key=c.counter_key AND d.day>=? AND d.day<=?
		WHERE c.agent_id=? GROUP BY c.counter_key HAVING c.present=1 OR SUM(d.received_bytes)>0 OR SUM(d.sent_bytes)>0
		ORDER BY c.present DESC,c.is_primary DESC,c.name,c.counter_key`, from, to, agentID)
	if err != nil {
		return nil, fmt.Errorf("query interface traffic: %w", err)
	}
	defer rows.Close()
	items := make([]model.NetworkInterfaceTrafficUsage, 0)
	for rows.Next() {
		var item model.NetworkInterfaceTrafficUsage
		var up, loopback, primary, present int
		var first, last string
		if err := rows.Scan(&item.Key, &item.Name, &item.Index, &item.MAC, &item.Kind, &up, &loopback, &primary, &present, &item.ReceivedBytes, &item.SentBytes, &first, &last); err != nil {
			return nil, err
		}
		item.Up, item.Loopback, item.Primary, item.Present = up != 0, loopback != 0, primary != 0, present != 0
		item.FirstObservedAt, item.LastObservedAt = parseTime(first), parseTime(last)
		item.TotalBytes = item.ReceivedBytes + item.SentBytes
		items = append(items, item)
	}
	return items, rows.Err()
}
