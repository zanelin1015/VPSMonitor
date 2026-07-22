package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestSQLiteStoreStoresCompactHistoryAndFullLatestSnapshot(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	reportedAt := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	snapshot := model.AgentSnapshot{
		AgentID:    "hk-compact-01",
		AgentName:  "HK Compact 01",
		ReportedAt: reportedAt,
		Summary: model.VPSSummary{
			Hostname:          "hk-compact",
			PublicIPv4:        "203.0.113.10",
			CPU:               12.5,
			MemUsed:           100,
			MemTotal:          200,
			DiskUsed:          300,
			DiskTotal:         400,
			NetTrafficSent:    500,
			NetTrafficRecv:    600,
			NetTrafficTotal:   1100,
			NetIOUp:           700,
			NetIODown:         800,
			XrayState:         "running",
			InboundCount:      1,
			OutboundCount:     2,
			RoutingRuleCount:  3,
			LastCollectionErr: "sample warning",
		},
		XUI: &model.XUISnapshot{
			BaseURL:      "http://127.0.0.1:2053",
			CollectedAt:  reportedAt,
			Inbounds:     []map[string]any{{"id": 1, "remark": "HK-01", "up": 10, "down": 20}},
			Outbounds:    []map[string]any{{"tag": "direct", "protocol": "freedom"}},
			RoutingRules: []map[string]any{{"outboundTag": "direct"}},
		},
		Realm: &model.RealmSnapshot{
			CollectedAt: reportedAt,
			Rules:       []model.RealmForwardRule{{ListenPort: 20001, TargetAddress: "198.51.100.20", TargetPort: 20001}},
		},
		NetworkPolicy: &model.NetworkPolicySnapshot{
			CollectedAt: reportedAt,
			Interface:   "eth0",
			Rules:       []model.NetworkPortPolicyRule{{Port: 20001, RateLimitMbps: 100}},
		},
		Logs: []model.AgentLogEntry{{Time: reportedAt, Level: "warn", Message: "sample"}},
	}
	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	var (
		historyVersion int
		historyJSON    string
		diskUsed       int64
		netSent        int64
	)
	if err := store.db.QueryRow(`
		SELECT history_version, snapshot_json, disk_used, net_traffic_sent
		FROM snapshots
		WHERE agent_id = ?
	`, snapshot.AgentID).Scan(&historyVersion, &historyJSON, &diskUsed, &netSent); err != nil {
		t.Fatalf("read compact history row: %v", err)
	}
	if historyVersion != compactSnapshotHistoryVersion || historyJSON != emptySnapshotHistoryJSON {
		t.Fatalf("unexpected compact history marker: version=%d json=%q", historyVersion, historyJSON)
	}
	if diskUsed != 300 || netSent != 500 {
		t.Fatalf("unexpected compact metrics: disk=%d sent=%d", diskUsed, netSent)
	}

	latest, ok := store.GetLatest(snapshot.AgentID)
	if !ok || latest.XUI == nil || latest.Realm == nil || latest.NetworkPolicy == nil || len(latest.Logs) != 1 {
		t.Fatalf("latest snapshot lost full component data: %#v", latest)
	}
	history, err := store.ListHistory(snapshot.AgentID, 10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one history row, got %d", len(history))
	}
	got := history[0]
	if got.XUI != nil || got.Realm != nil || got.NetworkPolicy != nil || len(got.Logs) != 0 {
		t.Fatalf("history row should contain metrics only: %#v", got)
	}
	if got.Summary.DiskUsed != 300 || got.Summary.NetTrafficSent != 500 || got.Summary.NetIODown != 800 {
		t.Fatalf("history metrics were not reconstructed: %#v", got.Summary)
	}
}

func TestSQLiteStoreSnapshotComponentEventsIgnoreRuntimeChanges(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	base := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	makeSnapshot := func(at time.Time, remark string, traffic int64, realmPort int, rate float64) model.AgentSnapshot {
		return model.AgentSnapshot{
			AgentID:    "event-01",
			ReportedAt: at,
			XUI: &model.XUISnapshot{
				BaseURL:     "http://127.0.0.1:2053",
				CollectedAt: at,
				Inbounds: []map[string]any{{
					"id":       1,
					"remark":   remark,
					"up":       traffic,
					"down":     traffic * 2,
					"all_time": traffic * 3,
					"clientStats": []map[string]any{{
						"email":      "customer@example.com",
						"enable":     true,
						"up":         traffic,
						"down":       traffic * 2,
						"allTime":    traffic * 3,
						"lastOnline": at.UnixMilli(),
					}},
				}},
				OutboundTraffic: []map[string]any{{"tag": "direct", "up": traffic, "down": traffic * 2}},
			},
			Realm: &model.RealmSnapshot{
				CollectedAt: at,
				Rules:       []model.RealmForwardRule{{ListenPort: realmPort, TargetAddress: "198.51.100.20", TargetPort: realmPort}},
			},
			NetworkPolicy: &model.NetworkPolicySnapshot{
				CollectedAt: at,
				Interface:   "eth0",
				Rules:       []model.NetworkPortPolicyRule{{Port: 20001, RateLimitMbps: rate}},
			},
		}
	}

	if err := store.SaveSnapshot(makeSnapshot(base, "HK-01", 10, 20001, 100)); err != nil {
		t.Fatalf("save initial snapshot: %v", err)
	}
	if err := store.SaveSnapshot(makeSnapshot(base.Add(time.Minute), "HK-01", 999, 20001, 100)); err != nil {
		t.Fatalf("save runtime-only update: %v", err)
	}
	if err := store.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "event-01",
		ReportedAt: base.Add(90 * time.Second),
		XUI:        &model.XUISnapshot{Error: "temporary API error"},
		Realm:      &model.RealmSnapshot{Error: "temporary read error"},
		NetworkPolicy: &model.NetworkPolicySnapshot{
			Error: "temporary tc error",
		},
	}); err != nil {
		t.Fatalf("save collection error snapshot: %v", err)
	}
	assertSnapshotComponentEventCounts(t, store.db, map[string]int{
		snapshotComponentXUI:           1,
		snapshotComponentRealm:         1,
		snapshotComponentNetworkPolicy: 1,
	})

	if err := store.SaveSnapshot(makeSnapshot(base.Add(2*time.Minute), "HK-02", 1000, 20002, 50)); err != nil {
		t.Fatalf("save configuration update: %v", err)
	}
	assertSnapshotComponentEventCounts(t, store.db, map[string]int{
		snapshotComponentXUI:           2,
		snapshotComponentRealm:         2,
		snapshotComponentNetworkPolicy: 2,
	})
}

func TestSQLiteStoreReadsLegacyAndCompactTrafficHistory(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	base := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	legacy := model.AgentSnapshot{
		AgentID:    "mixed-01",
		AgentName:  "Mixed 01",
		ReportedAt: base,
		Summary: model.VPSSummary{
			NetTrafficSent: 100,
			NetTrafficRecv: 200,
		},
		XUI: &model.XUISnapshot{Inbounds: []map[string]any{{"id": 1}}},
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy snapshot: %v", err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO snapshots (
			agent_id, agent_name, reported_at, hostname, public_ipv4, public_ipv6, cpu, mem_used, mem_total,
			xray_state, inbound_count, outbound_count, routing_rule_count, nezha_server_id, nezha_server_name,
			last_collection_err, snapshot_json
		) VALUES (?, ?, ?, '', '', '', 0, 0, 0, '', 0, 0, 0, 0, '', '', ?)
	`, legacy.AgentID, legacy.AgentName, legacy.ReportedAt.Format(time.RFC3339Nano), string(legacyJSON)); err != nil {
		t.Fatalf("insert legacy snapshot: %v", err)
	}

	compact := model.AgentSnapshot{
		AgentID:    legacy.AgentID,
		AgentName:  legacy.AgentName,
		ReportedAt: base.Add(time.Hour),
		Summary: model.VPSSummary{
			NetTrafficSent: 180,
			NetTrafficRecv: 260,
		},
	}
	if err := store.SaveSnapshot(compact); err != nil {
		t.Fatalf("save compact snapshot: %v", err)
	}

	history, err := store.ListHistory(legacy.AgentID, 10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(history) != 2 || history[1].XUI == nil || history[0].XUI != nil {
		t.Fatalf("legacy/compact history compatibility failed: %#v", history)
	}
	usage, err := store.ListDailyTrafficUsage(base)
	if err != nil {
		t.Fatalf("ListDailyTrafficUsage: %v", err)
	}
	if len(usage) != 1 || usage[0].Upload != 80 || usage[0].Download != 60 {
		t.Fatalf("unexpected mixed daily traffic usage: %#v", usage)
	}
}

func TestSQLiteStoreMigratesLegacySnapshotHistoryColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE snapshots (
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
			snapshot_json TEXT NOT NULL
		);
	`); err != nil {
		_ = db.Close()
		t.Fatalf("create legacy snapshots table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	defer store.Close()
	for _, column := range []string{
		"disk_used",
		"disk_total",
		"net_traffic_sent",
		"net_traffic_recv",
		"net_traffic_total",
		"net_io_up",
		"net_io_down",
		"history_version",
	} {
		if !sqliteColumnExists(t, store.db, "snapshots", column) {
			t.Fatalf("expected migrated snapshots.%s column", column)
		}
	}
}

func assertSnapshotComponentEventCounts(t *testing.T, db *sql.DB, want map[string]int) {
	t.Helper()
	for component, expected := range want {
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*)
			FROM snapshot_component_events
			WHERE component = ?
		`, component).Scan(&count); err != nil {
			t.Fatalf("count %s events: %v", component, err)
		}
		if count != expected {
			t.Fatalf("unexpected %s event count: got %d want %d", component, count, expected)
		}
	}
}
