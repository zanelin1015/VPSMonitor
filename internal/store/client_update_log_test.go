package store

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestClientUpdateLogAtomicDispatchDurabilityAndTaskLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent", AgentName: "Test Client"}); err != nil {
		t.Fatal(err)
	}
	base := model.ClientUpdateLog{BatchID: "batch1", AgentID: "agent", AgentName: "Test Client", Version: "0.3.35", TargetVersion: "0.3.36", Decision: "dispatched", Reason: "已下发"}
	entry, action, err := s.RecordClientUpdate(base, map[string]any{"version": "v0.3.36"})
	if err != nil || entry.ActionID == 0 || action.ID != entry.ActionID {
		t.Fatalf("dispatch: %#v %v", entry, err)
	}
	duplicate, action2, err := s.RecordClientUpdate(base, map[string]any{"version": "v0.3.36"})
	if err != nil || duplicate.ReasonCode != "already_pending" || duplicate.Decision != "skipped" || action2.ID != 0 {
		t.Fatalf("duplicate: %#v %v", duplicate, err)
	}
	actions, _ := s.ListXUIActions("agent", 100)
	if len(actions) != 1 {
		t.Fatal("duplicate task")
	}
	if _, _, err := s.ClaimXUIAction("agent", action.ID); err != nil {
		t.Fatal(err)
	}
	logs, err := s.ListClientUpdateLogs(200)
	if err != nil || logs[1].TaskStatus != "running" || logs[1].ClaimedAt == "" {
		t.Fatalf("claim not logged: %#v %v", logs, err)
	}
	if _, err := s.CompleteXUIAction("agent", action.ID, model.XUIActionResultRequest{Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	logs, _ = s.ListClientUpdateLogs(200)
	if logs[1].TaskStatus != "succeeded" || logs[1].ConfirmedAt != "" || logs[1].CompletedAt == "" {
		t.Fatal("receipt must not confirm installed version")
	}
	if err := s.ConfirmClientUpdateLog(entry.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	base.BatchID, base.Decision, base.ReasonCode = "batch2", "skipped", "manual_upgrade_required"
	base.Reason = "需要手动升级一次"
	if _, _, err := s.RecordClientUpdate(base, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM agents WHERE agent_id='agent'`); err != nil {
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
	logs, err = s.ListClientUpdateLogs(200)
	if err != nil || len(logs) != 3 || logs[2].ConfirmedAt == "" || logs[2].TaskStatus != "succeeded" || logs[2].AgentName != "Test Client" || logs[0].Reason != "需要手动升级一次" {
		t.Fatalf("lost historical log: %#v %v", logs, err)
	}
}

func TestClientUpdateAuditFailureRollsBackExecutableTask(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "updates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_update_log BEFORE INSERT ON client_update_logs BEGIN SELECT RAISE(FAIL,'log unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RecordClientUpdate(model.ClientUpdateLog{AgentID: "agent", Decision: "dispatched"}, map[string]any{"version": "v0.3.36"}); err == nil {
		t.Fatal("audit failure ignored")
	}
	actions, err := s.ListXUIActions("agent", 100)
	if err != nil || len(actions) > 0 {
		t.Fatal("task committed without log")
	}
}

func TestClientUpdateLogRecordsExecutionFailureAndLeaseTimeout(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "updates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent"}); err != nil {
		t.Fatal(err)
	}
	_, action, err := s.RecordClientUpdate(model.ClientUpdateLog{AgentID: "agent", Decision: "dispatched"}, map[string]any{"version": "v0.3.36"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteXUIAction("agent", action.ID, model.XUIActionResultRequest{Status: "failed", Error: "package SHA-256 mismatch"}); err != nil {
		t.Fatal(err)
	}
	_, action, err = s.RecordClientUpdate(model.ClientUpdateLog{AgentID: "agent", Decision: "dispatched"}, map[string]any{"version": "v0.3.36"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkXUIActionRunning("agent", action.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE xui_actions SET claimed_at=? WHERE id=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), action.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExpireStaleXUIActions("agent", 0); err != nil {
		t.Fatal(err)
	}
	logs, err := s.ListClientUpdateLogs(200)
	if err != nil || len(logs) != 2 || logs[0].TaskError == "" || logs[0].TaskStatus != "failed" || logs[1].TaskError != "package SHA-256 mismatch" {
		t.Fatalf("missing task failure: %#v %v", logs, err)
	}
}
