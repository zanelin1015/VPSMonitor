package store

import (
	"bridge-core/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestTrafficResetDurableDedupAndAtomicDispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reset.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "reset-agent"}); err != nil {
		t.Fatal(err)
	}
	settings, _, err := s.GetScheduledTaskSettings()
	if err != nil || settings.TrafficReset.Enabled || settings.TrafficReset.CatchUpMinutes != 360 {
		t.Fatalf("unsafe default: %#v %v", settings, err)
	}
	p, err := s.SaveTrafficResetPolicy(model.TrafficResetPolicy{AgentID: "reset-agent", InboundID: 1, ClientID: "uuid", Email: "alice", StartTime: 1, Cycle: "semiannual", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	job := model.TrafficResetJob{ID: TrafficResetID("reset-agent", "uuid", 1), AgentID: p.AgentID, PolicyID: p.ID, ClientID: p.ClientID, Email: p.Email, Boundary: 1, Status: "prepared"}
	for i := 0; i < 3; i++ {
		if err := s.PrepareTrafficResetJob(job); err != nil {
			t.Fatal(err)
		}
		if err := s.QueueTrafficResetJob(job, map[string]any{"job_id": job.ID}); err != nil {
			t.Fatal(err)
		}
	}
	actions, err := s.ListXUIActions(p.AgentID, 100)
	if err != nil || len(actions) != 1 {
		t.Fatalf("duplicate actions: %v %v", actions, err)
	}
	s.Close()
	s, err = NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetTrafficResetJob(job.ID)
	if err != nil || got.Status != "dispatched" || got.ActionID != actions[0].ID {
		t.Fatalf("lost durable state: %#v %v", got, err)
	}
	if err := s.QueueTrafficResetJob(job, map[string]any{"job_id": job.ID}); err != nil {
		t.Fatal(err)
	}
	actions, err = s.ListXUIActions(p.AgentID, 100)
	if err != nil || len(actions) != 1 {
		t.Fatal("reopened store dispatched twice")
	}
}

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
	start := time.Date(2020, 1, 31, 0, 0, 0, 0, model.TrafficResetLocation)
	cfg := r.Config
	cfg.Renewal.ClientBillings = []model.XUIClientBillingConfig{{ClientID: "uuid", InboundID: 1, Email: "alice", StartTime: start.UnixMilli(), RevenueCycle: "month", RevenueCurrency: "CNY", TrafficMultiplier: 2}}
	updated, err := s.UpdateAgentConfigWithActor("fixed", cfg, "admin")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2020, 2, 29, 0, 0, 0, 0, model.TrafficResetLocation).UnixMilli()
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
	if updated.Config.Renewal.ClientBillings[0].ExpireTime != model.ClientCycleBoundary(start, 6).UnixMilli() {
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

func TestTrafficResetAuditIsDurableDeduplicatedAndNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "audit-agent"}); err != nil {
		t.Fatal(err)
	}
	job := model.TrafficResetJob{ID: TrafficResetID("audit"), AgentID: "audit-agent", AgentName: "HK", ClientID: "uuid", ClientName: "Alice", Email: "alice@example.com", Status: "prepared"}
	if err := s.PrepareTrafficResetJob(job); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueTrafficResetJob(job, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	job, err = s.GetTrafficResetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	job.Status = "needs_enable"
	entry := model.TrafficResetLog{JobID: job.ID, ActionID: job.ActionID, AgentID: job.AgentID, AgentName: job.AgentName, ClientName: job.ClientName, Email: job.Email, Operation: "reset", Outcome: "unverified", StartedAt: "2026-10-02T00:01:03+08:00", CompletedAt: "2026-10-02T00:01:04+08:00", TimeSource: "client", Message: "reset acknowledged, verification failed"}
	if err := s.FinishTrafficResetJob(job, "dispatched", entry); err != nil {
		t.Fatal(err)
	}
	entry.Message = "must not replace the original log"
	if err := s.FinishTrafficResetJob(job, "dispatched", entry); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueTrafficResetJob(job, map[string]any{"resume_only": true}); err != nil {
		t.Fatal(err)
	}
	job, err = s.GetTrafficResetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	job.Status = "succeeded"
	entry.ActionID = job.ActionID
	entry.Operation = "verify"
	entry.Outcome = "succeeded"
	entry.Message = "read-only verification succeeded"
	if err := s.FinishTrafficResetJob(job, "dispatched", entry); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	logs, err := s.ListTrafficResetLogs(100)
	if err != nil || len(logs) != 2 {
		t.Fatalf("audit did not survive restart: %#v %v", logs, err)
	}
	if logs[0].Operation != "verify" || logs[1].Operation != "reset" || logs[1].Outcome != "unverified" || logs[1].Message != "reset acknowledged, verification failed" || logs[1].ClientName != "Alice" || logs[1].RecordedAt == "" {
		t.Fatalf("audit overwritten: %#v", logs)
	}
	if _, err := s.db.Exec(`DELETE FROM agents WHERE agent_id='audit-agent'`); err != nil {
		t.Fatal(err)
	}
	logs, err = s.ListTrafficResetLogs(100)
	if err != nil || len(logs) != 2 {
		t.Fatal("deleting client removed audit history")
	}
}

func TestTrafficResetFinalStateAndAuditCommitAtomically(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "audit-rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "audit-agent"}); err != nil {
		t.Fatal(err)
	}
	job := model.TrafficResetJob{ID: TrafficResetID("rollback"), AgentID: "audit-agent", Status: "prepared"}
	if err := s.PrepareTrafficResetJob(job); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_audit BEFORE INSERT ON traffic_reset_logs BEGIN SELECT RAISE(ABORT,'test disk write failure'); END;`); err != nil {
		t.Fatal(err)
	}
	job.Status = "skipped"
	if err := s.FinishTrafficResetJob(job, "prepared", model.TrafficResetLog{JobID: job.ID, Operation: "skip", Outcome: "skipped"}); err == nil {
		t.Fatal("expected failed audit write")
	}
	stored, err := s.GetTrafficResetJob(job.ID)
	if err != nil || stored.Status != "prepared" {
		t.Fatalf("job closed without audit log: %#v %v", stored, err)
	}
}
