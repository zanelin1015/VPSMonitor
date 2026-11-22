package store

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestNetworkInterfacePeriodsRestartHotplugReplayAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	card := func(key, epoch string, rx, tx uint64) model.NetworkInterfaceCounter {
		return model.NetworkInterfaceCounter{Key: key, Name: key, Index: 2, Kind: "physical", Up: true, Primary: true, Epoch: epoch, ReceivedBytes: rx, SentBytes: tx}
	}
	save := func(stamp string, component *model.NetworkInterfacesSnapshot) {
		t.Helper()
		at, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if component != nil {
			component.CollectedAt = at
		}
		if err := s.SaveSnapshot(model.AgentSnapshot{AgentID: "node", ReportedAt: at, NetworkInterfaces: component}); err != nil {
			t.Fatal(err)
		}
	}
	sample := func(items ...model.NetworkInterfaceCounter) *model.NetworkInterfacesSnapshot {
		return &model.NetworkInterfacesSnapshot{Interfaces: items}
	}
	save("2026-11-17T23:59:30+08:00", sample(card("ens3", "boot-a:2", 100, 200)))
	save("2026-11-18T00:00:10+08:00", sample(card("ens3", "boot-a:2", 150, 250)))
	save("2026-11-18T00:00:10+08:00", sample(card("ens3", "boot-a:2", 150, 250)))
	save("2026-11-17T23:59:50+08:00", sample(card("ens3", "boot-a:2", 120, 220)))
	save("2026-11-18T10:00:00+08:00", sample(card("ens3", "boot-b:2", 1000, 2000)))
	save("2026-11-18T10:01:00+08:00", &model.NetworkInterfacesSnapshot{Error: "read failed"})
	save("2026-11-18T10:02:00+08:00", nil)
	save("2026-11-18T10:03:00+08:00", sample(card("ens3", "boot-b:2", 1010, 2020)))
	items, err := s.ListNetworkInterfaceTraffic("node", "2026-11-18", "2026-11-18")
	if err != nil || len(items) != 1 || items[0].ReceivedBytes != 1060 || items[0].SentBytes != 2070 {
		t.Fatalf("daily counters: %#v %v", items, err)
	}
	// New interfaces establish their own baselines; disappearing ones keep history.
	save("2026-11-18T11:00:00+08:00", sample(card("ens4", "boot-b:3", 90000, 80000)))
	save("2026-11-18T10:59:00+08:00", sample(card("ens3", "boot-b:2", 1020, 2040)))
	items, err = s.ListNetworkInterfaceTraffic("node", "2026-11-18", "2026-11-18")
	if err != nil || len(items) != 2 || items[0].Name != "ens4" || items[0].TotalBytes != 0 || items[1].Present || items[1].Primary || items[1].TotalBytes != 3130 {
		t.Fatalf("hotplug/replay handling: %#v %v", items, err)
	}
	// A valid empty discovery must also block an older report from resurrecting a NIC.
	save("2026-11-18T12:00:00+08:00", sample())
	save("2026-11-18T11:59:00+08:00", sample(card("ens4", "boot-b:3", 90010, 80010)))
	items, err = s.ListNetworkInterfaceTraffic("node", "2026-11-18", "2026-11-18")
	if err != nil || len(items) != 1 || items[0].Present || items[0].Name != "ens3" {
		t.Fatalf("empty discovery/order: %#v %v", items, err)
	}
	if _, err = s.db.Exec(`DELETE FROM snapshots`); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	items, err = s.ListNetworkInterfaceTraffic("node", "2026-11-18", "2026-11-18")
	if err != nil || len(items) != 1 || items[0].TotalBytes != 3130 {
		t.Fatalf("restart/retention removed history: %#v %v", items, err)
	}
	if err = s.DeleteAgent("node"); err != nil {
		t.Fatal(err)
	}
	if items, err = s.ListNetworkInterfaceTraffic("node", "2026-11-18", "2026-11-18"); err != nil || len(items) != 0 {
		t.Fatalf("deleted interface traffic retained: %#v %v", items, err)
	}
}

func TestNetworkInterfaceTrafficRollbackAndReset(t *testing.T) {
	s, _ := customerSessionFixture(t)
	at := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	snapshot := model.AgentSnapshot{AgentID: "node", ReportedAt: at, NetworkInterfaces: &model.NetworkInterfacesSnapshot{CollectedAt: at, Interfaces: []model.NetworkInterfaceCounter{{Key: "nic", Name: "ens3", Index: 2, Kind: "physical", Up: true, Epoch: "boot:2"}}}}
	if err := s.SaveSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_nic_daily BEFORE INSERT ON network_interface_daily BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	snapshot.ReportedAt = at.Add(time.Minute)
	snapshot.NetworkInterfaces.CollectedAt = snapshot.ReportedAt
	snapshot.NetworkInterfaces.Interfaces[0].ReceivedBytes = 100
	snapshot.NetworkInterfaces.Interfaces[0].SentBytes = 200
	if err := s.SaveSnapshot(snapshot); err == nil {
		t.Fatal("expected failed transaction")
	}
	if _, err := s.db.Exec(`DROP TRIGGER reject_nic_daily`); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.ReportedAt = at.Add(2 * time.Minute)
	snapshot.NetworkInterfaces.CollectedAt = snapshot.ReportedAt
	snapshot.NetworkInterfaces.Interfaces[0].ReceivedBytes = 10
	snapshot.NetworkInterfaces.Interfaces[0].SentBytes = 20
	if err := s.SaveSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListNetworkInterfaceTraffic("node", "2026-11-18", "2026-11-18")
	if err != nil || len(items) != 1 || items[0].ReceivedBytes != 110 || items[0].SentBytes != 220 {
		t.Fatalf("failed write advanced baseline or reset lost: %#v %v", items, err)
	}
}
