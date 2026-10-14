package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func clientConfigStoreFixture(t *testing.T) (*SQLiteStore, model.XUIClientConfigRequest, model.XUIClientConfigTarget) {
	t.Helper()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, id := range []string{"source", "target"} {
		if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: id}); err != nil {
			t.Fatal(err)
		}
	}
	zero := int64(0)
	limit := int64(50 * 1024 * 1024 * 1024)
	req := model.XUIClientConfigRequest{RequestID: "config-first-request", ClientID: "stable-uuid", InboundID: 1, InboundTag: "entry", Email: "alice", Billing: model.XUIClientBillingConfig{RevenueAmount: 63, RevenueCurrency: "CNY", RevenueCycle: "quarter", TrafficMultiplier: 2}, Changes: model.XUIClientConfigChanges{TotalBytes: &limit}, Expected: model.XUIClientConfigChanges{TotalBytes: &zero}}
	return s, req, model.XUIClientConfigTarget{AgentID: "target", InboundID: 7, InboundTag: "exit", ClientID: "stable-uuid", Email: "alice"}
}

func TestUnifiedClientConfigTransactionAndIdempotency(t *testing.T) {
	s, req, target := clientConfigStoreFixture(t)
	actor := model.XUIActionActor{Role: model.AdminRoleRoot, AccountID: 1, Username: "admin"}
	cfg, _, _ := s.GetAgentConfig("source")
	cfg.Renewal.CostAmount = 123
	cfg.Entry.ImportDomain = "keep.example"
	if _, err := s.UpdateAgentConfig("source", cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveXUIClientConfig("source", target, req, actor)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Action == nil || saved.Action.AgentID != "target" || saved.Action.Payload["inbound_id"] != float64(7) || saved.Billing.InboundID != 1 {
		t.Fatal("forwarded remote target and source billing identity were mixed")
	}
	if saved.Action.XUIAuth != nil {
		t.Fatal("panel credentials must not be persisted in queued jobs")
	}
	stored, _, _ := s.GetAgentConfig("source")
	if stored.Renewal.CostAmount != 123 || stored.Entry.ImportDomain != "keep.example" || len(stored.Renewal.ClientBillings) != 1 {
		t.Fatal("saving one client overwrote other configuration")
	}
	repeated, err := s.SaveXUIClientConfig("source", target, req, actor)
	if err != nil || repeated.Action.ID != saved.Action.ID {
		t.Fatalf("idempotency failed: %v", err)
	}
	actions, _ := s.ListXUIActions("target", 100)
	if len(actions) != 1 {
		t.Fatal("duplicate submission queued extra actions")
	}
	changed := req
	changed.Billing.RevenueAmount++
	if _, err := s.SaveXUIClientConfig("source", target, changed, actor); !errors.Is(err, ErrClientConfigConflict) {
		t.Fatalf("request id reuse accepted changed payload: %v", err)
	}
	changed.RequestID = "config-second-request"
	changed.PreviousBilling = &saved.Billing
	if _, err := s.SaveXUIClientConfig("source", target, changed, actor); !errors.Is(err, ErrClientConfigPending) {
		t.Fatalf("pending save can be overtaken: %v", err)
	}
	// A bare success message is insufficient to claim remote configuration took effect.
	completed, err := s.CompleteXUIAction("target", saved.Action.ID, model.XUIActionResultRequest{Status: "succeeded"})
	if err != nil || completed.Status != "failed" {
		t.Fatalf("unverified result accepted: %v", err)
	}
	latest, err := s.LatestXUIClientConfigSaves("source")
	if err != nil || len(latest) != 1 || latest[0].Action.Status != "failed" {
		t.Fatal("list receipt lost execution state")
	}
	changed.Changes = model.XUIClientConfigChanges{}
	local, err := s.SaveXUIClientConfig("source", target, changed, actor)
	if err != nil || local.Action != nil || local.Billing.RevenueAmount != 64 {
		t.Fatalf("billing-only save failed: %v", err)
	}
	stale := changed
	stale.RequestID = "config-stale-request"
	stale.PreviousBilling = &saved.Billing
	if _, err := s.SaveXUIClientConfig("source", target, stale, actor); !errors.Is(err, ErrClientConfigConflict) {
		t.Fatalf("stale billing overwrote a newer save: %v", err)
	}
}

func TestUnifiedClientConfigRollsBackBillingAndAuditWhenQueueFails(t *testing.T) {
	s, req, target := clientConfigStoreFixture(t)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_config_queue BEFORE INSERT ON xui_actions BEGIN SELECT RAISE(ABORT, 'simulated queue failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveXUIClientConfig("source", target, req, model.XUIActionActor{Role: "admin", AccountID: 1, Username: "admin"}); err == nil {
		t.Fatal("queue failure was swallowed")
	}
	cfg, _, _ := s.GetAgentConfig("source")
	if len(cfg.Renewal.ClientBillings) != 0 {
		t.Fatal("billing persisted without its queued remote patch")
	}
	logs, _ := s.ListConfigAuditLogs("source", 100)
	if len(logs) != 0 {
		t.Fatal("rolled-back save left an audit entry")
	}
	if _, found, err := s.GetXUIClientConfigSave("source", req.RequestID); err != nil || found {
		t.Fatal("rolled-back save left a receipt")
	}
}

func TestUnifiedClientConfigRecoversExpiredExecutionLease(t *testing.T) {
	s, req, target := clientConfigStoreFixture(t)
	if _, err := s.db.Exec(`UPDATE agents SET renewal_config_json = '' WHERE agent_id = 'source'`); err != nil {
		t.Fatal(err)
	}
	save, err := s.SaveXUIClientConfig("source", target, req, model.XUIActionActor{Role: "admin", AccountID: 1, Username: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * XUIActionExecutionLease).UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE xui_actions SET status = 'running', claimed_at = ?, updated_at = ? WHERE id = ?`, old, old, save.Action.ID); err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestXUIClientConfigSaves("source")
	if err != nil || len(latest) != 1 || latest[0].Action.Status != "failed" {
		t.Fatal("list stayed running after the execution lease expired")
	}
	req.RequestID = "config-after-timeout"
	req.PreviousBilling = &save.Billing
	if _, err := s.SaveXUIClientConfig("source", target, req, model.XUIActionActor{Role: "admin", AccountID: 1, Username: "admin"}); err != nil {
		t.Fatal(err)
	}
}

func TestUnifiedClientConfigFeeOnlyPreservesLegacyRenewalFlag(t *testing.T) {
	s, req, target := clientConfigStoreFixture(t)
	cfg, _, _ := s.GetAgentConfig("source")
	old := req.Billing
	old.ClientID, old.InboundID, old.InboundTag, old.Email = req.ClientID, req.InboundID, req.InboundTag, req.Email
	old.ExpireAutoRenew = true
	cfg.Renewal.ClientBillings = []model.XUIClientBillingConfig{old}
	if _, err := s.UpdateAgentConfig("source", cfg); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ = s.GetAgentConfig("source")
	req.PreviousBilling = &cfg.Renewal.ClientBillings[0]
	req.Changes = model.XUIClientConfigChanges{}
	req.Billing.RevenueAmount = 99
	save, err := s.SaveXUIClientConfig("source", target, req, model.XUIActionActor{Role: "admin", AccountID: 1, Username: "admin"})
	if err != nil || !save.Billing.ExpireAutoRenew {
		t.Fatalf("fee-only edit changed a legacy renewal flag: %v", err)
	}
}
