package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestUpgradeRetiresPendingLegacyResetWithoutChangingNativeTasksOrHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s != nil {
			_ = s.Close()
		}
	})
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent"}); err != nil {
		t.Fatal(err)
	}
	legacy := make(map[string]model.XUIAction)
	for _, status := range []string{model.XUIActionStatusPending, model.XUIActionStatusRunning, model.XUIActionStatusSucceeded, model.XUIActionStatusFailed} {
		action, err := s.CreateXUIAction("agent", model.XUIActionRequest{Kind: model.XUIActionRestartXUI})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE xui_actions SET kind=?, status=?, error='historical result', result_json='{"reset_done":true}' WHERE id=?`, model.XUIActionResetClientTraffic, status, action.ID); err != nil {
			t.Fatal(err)
		}
		legacy[status], _, err = s.GetXUIAction("agent", action.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	native, err := s.CreateXUIAction("agent", model.XUIActionRequest{Kind: model.XUIActionUpdateClientTrafficReset, Payload: map[string]any{"traffic_reset": "monthly", "traffic_reset_day": 15}})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate existing historical tables; new databases no longer create them.
	if _, err := s.db.Exec(`CREATE TABLE traffic_reset_logs(id INTEGER PRIMARY KEY, data_json TEXT); INSERT INTO traffic_reset_logs VALUES(1, '{"outcome":"succeeded"}')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for status, before := range legacy {
		after, found, err := s.GetXUIAction("agent", before.ID)
		if err != nil || !found {
			t.Fatalf("legacy receipt lost: found=%v err=%v", found, err)
		}
		if status == model.XUIActionStatusPending {
			if after.Status != model.XUIActionStatusFailed || after.Error != model.LegacyTrafficResetRemovedMessage || after.CompletedAt == nil || after.ClaimedAt != nil {
				t.Fatalf("pending reset was not retired: %+v", after)
			}
			continue
		}
		if after.Status != before.Status || after.Error != before.Error || !after.UpdatedAt.Equal(before.UpdatedAt) || after.Result["reset_done"] != true {
			t.Fatalf("existing execution/history changed: %+v", after)
		}
	}
	var log string
	if err := s.db.QueryRow(`SELECT data_json FROM traffic_reset_logs WHERE id=1`).Scan(&log); err != nil || log != `{"outcome":"succeeded"}` {
		t.Fatalf("historical audit log changed: %s %v", log, err)
	}
	claimed, err := s.ClaimPendingXUIActions("agent", 20)
	if err != nil || len(claimed) != 1 || claimed[0].ID != native.ID || claimed[0].Payload["traffic_reset"] != "monthly" {
		t.Fatalf("upgrade failed to preserve native reset settings task: %+v %v", claimed, err)
	}
}

func TestLegacyResetCannotBeCreatedOrClaimed(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "claims.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateXUIAction("agent", model.XUIActionRequest{Kind: model.XUIActionResetClientTraffic}); err == nil {
		t.Fatal("removed reset action was accepted")
	}
	legacy, err := s.CreateXUIAction("agent", model.XUIActionRequest{Kind: model.XUIActionRestartXUI})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE xui_actions SET kind=? WHERE id=?`, model.XUIActionResetClientTraffic, legacy.ID); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := s.ClaimXUIAction("agent", legacy.ID); err != nil || claimed {
		t.Fatalf("realtime claimed removed reset: %v %v", claimed, err)
	}
	if action, found, err := s.GetXUIAction("agent", legacy.ID); err != nil || !found || action.Status != model.XUIActionStatusPending {
		t.Fatalf("removed reset was marked running: %+v %v", action, err)
	}
	if actions, err := s.ClaimPendingXUIActions("agent", 20); err != nil || len(actions) != 0 {
		t.Fatalf("polling claimed removed reset: %+v %v", actions, err)
	}
	var tables int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name LIKE 'traffic_reset_%'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("fresh database still creates legacy reset tables: %d %v", tables, err)
	}
}

func TestScheduledTaskSettingsIgnoreLegacyResetAndPreserveOtherTasks(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	legacy := `{"traffic_reset":{"enabled":true,"catch_up_minutes":360},"alert_sweep":{"enabled":false,"interval_minutes":10},"daily_traffic_report":{"enabled":true,"time_of_day":"09:00","interval_days":2}}`
	if _, err := s.db.Exec(`INSERT INTO app_settings(key,value_json,updated_at) VALUES(?,?,'')`, scheduledTasksKey, legacy); err != nil {
		t.Fatal(err)
	}
	settings, found, err := s.GetScheduledTaskSettings()
	if err != nil || !found || settings.AlertSweep.Enabled || settings.AlertSweep.IntervalMinutes != 10 || !settings.DailyTrafficReport.Enabled || settings.DailyTrafficReport.IntervalDays != 2 {
		t.Fatalf("other scheduled task settings lost: %+v %v", settings, err)
	}
	if _, err := s.SaveScheduledTaskSettings(settings); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key=?`, scheduledTasksKey).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || len(fields) != 2 || strings.Contains(raw, "traffic_reset") {
		t.Fatalf("legacy reset settings re-saved: %s %v", raw, err)
	}
}
