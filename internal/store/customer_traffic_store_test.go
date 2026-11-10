package store

import (
	"path/filepath"
	"reflect"
	"strings"
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

func TestCustomerTrafficPointsIndexedIdentityAndBaseline(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "traffic.db"), WithMaxOpenConns(1))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent", AgentName: "Agent"}); err != nil {
		t.Fatal(err)
	}
	customer, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "customer", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "other", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	assignment := func(customerID int64, clientID, email string, enabled bool) int64 {
		t.Helper()
		a, err := s.CreateCustomerAssignment(customerID, model.CustomerAssignmentRequest{
			AgentID: "agent", InboundID: 1, ClientID: clientID, ClientEmail: email, Enabled: boolPtr(enabled),
		})
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	emailID := assignment(customer.ID, "id-a", "Alice@example.com", true)
	clientID := assignment(customer.ID, "id-b", "", true)
	assignment(customer.ID, "disabled", "disabled@example.com", false)
	assignment(other.ID, "other", "other@example.com", true)
	start := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	for _, sample := range []struct {
		hour   int
		id     string
		email  string
		upload int
	}{
		{-2, "old-id", "Alice@example.com", 10},
		{-1, "old-id", "Alice@example.com", 20},
		{-1, "id-a", "alice@example.com", 30},  // Tie: the later sample ID wins.
		{0, "new-id", "Alice@example.com", 35}, // Exact email survives credential rotation.
		{1, "wrong-id", "ALICE@example.com", 999},
		{1, "id-a", "alice@example.com", 45},
		{2, "id-a", "Alice@example.com", 100}, // End boundary is exclusive.
		{-1, "id-b", "old@example.com", 100},
		{0, "id-b", "new@example.com", 150}, // ID-only assignment follows the ID.
		{1, "id-b", "new@example.com", 5},   // Counter reset remains a valid point.
		{0, "disabled", "disabled@example.com", 1000},
		{0, "other", "other@example.com", 2000},
	} {
		_, err := s.db.Exec(`INSERT INTO customer_traffic_samples
			(agent_id, inbound_id, client_id, client_email, upload_bytes, download_bytes, reported_at)
			VALUES ('agent', 1, ?, ?, ?, 0, ?)`, sample.id, sample.email, sample.upload, start.Add(time.Duration(sample.hour)*time.Hour).Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
	}
	points, err := s.listCustomerTrafficPoints(customer.ID, start, start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[int64][]uint64)
	for id, items := range points {
		for _, item := range items {
			got[id] = append(got[id], item.upload)
		}
	}
	want := map[int64][]uint64{emailID: {30, 35, 45}, clientID: {100, 150, 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("traffic points=%v, want %v", got, want)
	}

	// Guard against the production regression: scanning all of an agent's
	// samples instead of seeking the customer's identity and time range.
	from, to := start.Format(time.RFC3339Nano), start.Add(2*time.Hour).Format(time.RFC3339Nano)
	rows, err := s.db.Query("EXPLAIN QUERY PLAN "+customerTrafficPointsQuery, customer.ID, from, from, from, to, from, to)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.String(), "idx_customer_traffic_samples_agent_reported_at") ||
		strings.Count(plan.String(), "COVERING INDEX idx_customer_traffic_samples_email_nocase_cover") != 2 ||
		strings.Count(plan.String(), "COVERING INDEX idx_customer_traffic_samples_client_id_cover") != 2 {
		t.Fatalf("traffic query lost its bounded identity lookups:\n%s", plan.String())
	}
}
