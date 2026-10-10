package store

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestCustomerTrafficUsageDerivesDailyClientDeltas(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "usage-agent", AgentName: "Usage Agent"}); err != nil {
		t.Fatal(err)
	}
	customer, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "usage-customer", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{
		AgentID: "usage-agent", InboundID: 1, ClientEmail: "alice@example.com", Enabled: boolPtr(true),
	}); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	for _, sample := range []struct {
		at   time.Time
		up   int
		down int
	}{{base, 100, 200}, {base.Add(2 * time.Hour), 150, 260}, {base.AddDate(0, 0, 1), 190, 300}} {
		if err := s.SaveSnapshot(model.AgentSnapshot{
			AgentID: "usage-agent", ReportedAt: sample.at,
			XUI: &model.XUISnapshot{CollectedAt: sample.at, Inbounds: []map[string]any{{
				"id":          1,
				"clientStats": []map[string]any{{"email": "alice@example.com", "up": sample.up, "down": sample.down}},
			}}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	items, err := s.ListCustomerTrafficUsage(customer.ID, base, base.AddDate(0, 0, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two daily usage items, got %#v", items)
	}
	if items[0].Date != "2026-10-02" || items[0].UploadBytes != 50 || items[0].DownloadBytes != 60 {
		t.Fatalf("unexpected first daily usage: %#v", items[0])
	}
	if items[1].Date != "2026-10-03" || items[1].UploadBytes != 40 || items[1].DownloadBytes != 40 {
		t.Fatalf("unexpected second daily usage: %#v", items[1])
	}
}

func boolPtr(value bool) *bool { return &value }
