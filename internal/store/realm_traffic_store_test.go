package store

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestRealmTrafficDailyDeltasResetsReplayAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	save := func(stamp, epoch string, up, down uint64, failure string) {
		t.Helper()
		at, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SaveSnapshot(model.AgentSnapshot{AgentID: "realm-entry", ReportedAt: at, Realm: &model.RealmSnapshot{
			Running: true, Traffic: &model.RealmTrafficSnapshot{Enabled: true, CollectedAt: at, Error: failure, Counters: []model.RealmTrafficCounter{{
				Key: "listener-a", RuleID: "rule-a", Name: "GZ -> HK", ListenAddress: "0.0.0.0", ListenPort: 20002, Network: "both", Epoch: epoch, UploadBytes: up, DownloadBytes: down,
			}}},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	save("2026-11-17T23:59:30+08:00", "a", 100, 200, "")
	save("2026-11-18T00:00:10+08:00", "a", 150, 300, "")
	save("2026-11-18T00:00:10+08:00", "a", 150, 300, "") // Replay.
	save("2026-11-17T23:59:50+08:00", "a", 120, 220, "") // Late delivery.
	save("2026-11-18T10:00:00+08:00", "a", 20, 10, "")   // Counter reset.
	save("2026-11-18T10:01:00+08:00", "b", 500, 600, "") // New generation already exceeds the old counters.
	save("2026-11-18T10:02:00+08:00", "b", 99999, 99999, "permission error")
	save("2026-11-18T10:03:00+08:00", "b", 510, 620, "")
	check := func(day string, wantUp, wantDown uint64) {
		t.Helper()
		items, err := s.ListRealmTrafficUsage("realm-entry", day, day)
		if err != nil || len(items) != 1 || items[0].UploadBytes != wantUp || items[0].DownloadBytes != wantDown || items[0].TotalBytes != wantUp+wantDown {
			t.Fatalf("%s usage: %#v err=%v", day, items, err)
		}
	}
	check("2026-11-17", 0, 0)
	check("2026-11-18", 580, 730)
	// Raw snapshot retention must not remove the compact daily ledger.
	if _, err := s.db.Exec(`DELETE FROM snapshots`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	check("2026-11-18", 580, 730)
	if err := s.DeleteAgent("realm-entry"); err != nil {
		t.Fatal(err)
	}
	if items, err := s.ListRealmTrafficUsage("realm-entry", "2026-11-18", "2026-11-18"); err != nil || len(items) != 0 {
		t.Fatalf("deleted agent retained traffic: %#v %v", items, err)
	}
}

func TestRealmTrafficNameChangesAndMissingComponentKeepBaseline(t *testing.T) {
	s, _ := customerSessionFixture(t)
	first := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	counter := model.RealmTrafficCounter{Key: "stable-key", RuleID: "stable-id", Name: "Old name", ListenAddress: "::", ListenPort: 443, Network: "tcp", Epoch: "same", UploadBytes: 10, DownloadBytes: 20}
	for index := 0; index < 3; index++ {
		snapshot := model.AgentSnapshot{AgentID: "relay", ReportedAt: first.Add(time.Duration(index) * time.Minute)}
		if index != 1 {
			if index == 2 {
				counter.Name, counter.UploadBytes, counter.DownloadBytes = "New name", 40, 60
			}
			snapshot.Realm = &model.RealmSnapshot{Traffic: &model.RealmTrafficSnapshot{Enabled: true, CollectedAt: snapshot.ReportedAt, Counters: []model.RealmTrafficCounter{counter}}}
		}
		if err := s.SaveSnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.ListRealmTrafficUsage("relay", "2026-11-18", "2026-11-18")
	if err != nil || len(items) != 1 || items[0].Name != "New name" || items[0].UploadBytes != 30 || items[0].DownloadBytes != 40 {
		t.Fatalf("rename/collection gap reset baseline: %#v %v", items, err)
	}
}
