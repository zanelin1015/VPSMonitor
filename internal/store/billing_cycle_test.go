package store

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestClientExpiryIsFixedExceptExplicitSettingsEdits(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "fixed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "fixed"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2020, 1, 31, 0, 0, 0, 0, model.BeijingLocation)
	cfg := r.Config
	cfg.Renewal.ClientBillings = []model.XUIClientBillingConfig{{ClientID: "uuid", InboundID: 1, Email: "alice", StartTime: start.UnixMilli(), RevenueCycle: "month", RevenueCurrency: "CNY", TrafficMultiplier: 2}}
	updated, err := s.UpdateAgentConfigWithActor("fixed", cfg, "admin")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2020, 2, 29, 0, 0, 0, 0, model.BeijingLocation).UnixMilli()
	if updated.Config.Renewal.ClientBillings[0].ExpireTime != expected {
		t.Fatalf("expiry rolled instead of first period: %#v", updated.Config.Renewal.ClientBillings[0])
	}
	cfg = updated.Config
	cfg.Renewal.ClientBillings[0].RevenueAmount = 42
	updated, err = s.UpdateAgentConfigWithActor("fixed", cfg, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Config.Renewal.ClientBillings[0].ExpireTime != expected {
		t.Fatal("price-only save changed expiry")
	}
	cfg = updated.Config
	cfg.Renewal.ClientBillings[0].RevenueCycle = "semiannual"
	updated, err = s.UpdateAgentConfigWithActor("fixed", cfg, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Config.Renewal.ClientBillings[0].ExpireTime != model.ClientBillingCycleBoundary(start, 6).UnixMilli() {
		t.Fatal("half-year settings edit did not calculate expiry")
	}
	cfg = updated.Config
	cfg.Renewal.ClientBillings[0].StartTime = start.AddDate(0, 0, 1).UnixMilli()
	cfg.Renewal.ClientBillings[0].ExpireTime = 123456789
	updated, err = s.UpdateAgentConfigWithActor("fixed", cfg, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Config.Renewal.ClientBillings[0].ExpireTime != 123456789 {
		t.Fatal("manual expiry override overwritten")
	}
}
