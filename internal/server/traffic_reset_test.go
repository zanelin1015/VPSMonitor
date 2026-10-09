package server

import (
	"bridge-core/internal/model"
	"bridge-core/internal/store"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func resetFixture(t *testing.T, now time.Time) (*App, model.TrafficResetPolicy, model.AgentSnapshot, string) {
	t.Helper()
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "reset.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	r, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "reset-agent", AgentName: "Reset Agent"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := r.Config
	cfg.XUI.Enabled = true
	cfg.Renewal.ClientBillings = []model.XUIClientBillingConfig{{ClientID: "uuid", InboundID: 1, Email: "alice", StartTime: time.Date(2026, 9, 2, 0, 0, 0, 0, model.TrafficResetLocation).UnixMilli(), RevenueCycle: "month", ExpireTime: now.AddDate(0, 1, 0).UnixMilli(), TrafficMultiplier: 2, RevenueAmount: 99}}
	if _, err := s.UpdateAgentConfigWithActor(r.AgentID, cfg, "admin"); err != nil {
		t.Fatal(err)
	}
	settings, _, err := s.GetScheduledTaskSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.TrafficReset.Enabled = true
	if _, err := s.SaveScheduledTaskSettings(settings); err != nil {
		t.Fatal(err)
	}
	snapshot := model.AgentSnapshot{AgentID: r.AgentID, ReportedAt: now, Capabilities: model.AgentCapabilities{TrafficReset: true}, XUI: &model.XUISnapshot{CollectedAt: now, Inbounds: []map[string]any{{"id": 1, "enable": true, "protocol": "vless", "settings": fmt.Sprintf(`{"clients":[{"id":"uuid","email":"alice","enable":true,"totalGB":200,"expiryTime":%d}]}`, cfg.Renewal.ClientBillings[0].ExpireTime), "clientStats": []map[string]any{{"email": "alice", "enable": false, "up": 100, "down": 100, "total": 200, "expiryTime": cfg.Renewal.ClientBillings[0].ExpireTime, "allTime": 900}}}}}}
	if err := s.SaveSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	p, err := s.SaveTrafficResetPolicy(model.TrafficResetPolicy{AgentID: r.AgentID, InboundID: 1, ClientID: "uuid", Email: "alice", Enabled: true, FollowBilling: true, StartTime: cfg.Renewal.ClientBillings[0].StartTime, Cycle: "month"})
	if err != nil {
		t.Fatal(err)
	}
	return &App{store: s}, p, snapshot, r.AgentToken
}

func TestTrafficResetPreparationDispatchAndDedup(t *testing.T) {
	boundary := time.Date(2026, 10, 2, 0, 0, 0, 0, model.TrafficResetLocation)
	a, _, snap, _ := resetFixture(t, boundary.Add(-5*time.Minute))
	before, _, _ := a.store.GetAgentConfig(snap.AgentID)
	if err := a.runTrafficResetSweep(boundary.Add(-6 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	jobs, _ := a.store.ListTrafficResetJobs(100)
	if len(jobs) != 0 {
		t.Fatal("prepared too early")
	}
	if err := a.runTrafficResetSweep(boundary.Add(-5 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	jobs, _ = a.store.ListTrafficResetJobs(100)
	if len(jobs) != 1 || jobs[0].Status != "prepared" {
		t.Fatalf("bad prepared jobs: %#v", jobs)
	}
	if err := a.runTrafficResetSweep(boundary); err != nil {
		t.Fatal(err)
	}
	actions, _ := a.store.ListXUIActions(snap.AgentID, 100)
	if len(actions) != 0 {
		t.Fatal("dispatched before 00:01")
	}
	snap.ReportedAt = boundary.Add(time.Minute)
	snap.XUI.CollectedAt = snap.ReportedAt
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := a.runTrafficResetSweep(boundary.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	actions, _ = a.store.ListXUIActions(snap.AgentID, 100)
	if len(actions) != 1 || actions[0].Kind != model.XUIActionResetClientTraffic {
		t.Fatalf("unexpected actions: %#v", actions)
	}
	if _, err := a.store.ClaimPendingXUIActions(snap.AgentID, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.CompleteXUIAction(snap.AgentID, actions[0].ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: map[string]any{"reset_done": true, "message": "reset verified"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.runTrafficResetSweep(boundary.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	jobs, _ = a.store.ListTrafficResetJobs(100)
	if len(jobs) != 1 || jobs[0].Status != "succeeded" {
		t.Fatalf("receipt not reconciled: %#v", jobs)
	}
	after, _, _ := a.store.GetAgentConfig(snap.AgentID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("scheduler changed billing/expiry/quota configuration")
	}
}

func TestTrafficResetDoesNotDispatchUnsafeCandidates(t *testing.T) {
	boundary := time.Date(2026, 10, 2, 0, 0, 0, 0, model.TrafficResetLocation)
	for _, scenario := range []string{"old-client", "offline", "global-disabled", "policy-disabled", "expired-window", "changed-config"} {
		t.Run(scenario, func(t *testing.T) {
			a, p, snap, _ := resetFixture(t, boundary.Add(-5*time.Minute))
			if err := a.runTrafficResetSweep(boundary.Add(-5 * time.Minute)); err != nil {
				t.Fatal(err)
			}
			now := boundary.Add(time.Minute)
			if scenario != "offline" {
				snap.ReportedAt = now
			}
			if scenario == "old-client" {
				snap.Capabilities.TrafficReset = false
			}
			if err := a.store.SaveSnapshot(snap); err != nil {
				t.Fatal(err)
			}
			if scenario == "global-disabled" {
				s, _, _ := a.store.GetScheduledTaskSettings()
				s.TrafficReset.Enabled = false
				if _, err := a.store.SaveScheduledTaskSettings(s); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "policy-disabled" {
				p.Enabled = false
				if _, err := a.store.SaveTrafficResetPolicy(p); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "expired-window" {
				now = boundary.Add(8 * time.Hour)
			}
			if scenario == "changed-config" {
				cfg, _, _ := a.store.GetAgentConfig(p.AgentID)
				cfg.Renewal.ClientBillings[0].StartTime += 24 * 3600000
				if _, err := a.store.UpdateAgentConfigWithActor(p.AgentID, cfg, "admin"); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.runTrafficResetSweep(now); err != nil {
				t.Fatal(err)
			}
			actions, _ := a.store.ListXUIActions(p.AgentID, 100)
			if len(actions) != 0 {
				t.Fatalf("unsafe dispatch: %#v", actions)
			}
		})
	}
}

func TestTrafficResetValidationRechecksGlobalSwitch(t *testing.T) {
	now := time.Now()
	a, p, snap, token := resetFixture(t, now)
	candidates, err := a.trafficResetCandidates(now)
	if err != nil {
		t.Fatal(err)
	}
	resolved, c, err := resolveTrafficResetPolicy(p, candidates)
	if err != nil {
		t.Fatal(err)
	}
	job := model.TrafficResetJob{ID: store.TrafficResetID("validation"), AgentID: p.AgentID, PolicyID: p.ID, Status: "prepared", RunAt: now.Add(-time.Minute).UnixMilli(), Deadline: now.Add(time.Hour).UnixMilli(), Signature: trafficResetSignature(resolved, c)}
	if err := a.store.PrepareTrafficResetJob(job); err != nil {
		t.Fatal(err)
	}
	if err := a.store.QueueTrafficResetJob(job, map[string]any{"job_id": job.ID}); err != nil {
		t.Fatal(err)
	}
	actions, err := a.store.ClaimPendingXUIActions(snap.AgentID, 10)
	if err != nil || len(actions) != 1 {
		t.Fatalf("claim: %v %v", actions, err)
	}
	check := func(want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/agents/%s/traffic-reset-validation?action_id=%d", p.AgentID, actions[0].ID), nil)
		req.Header.Set("X-Agent-Token", token)
		w := httptest.NewRecorder()
		a.handleTrafficResetValidation(w, req, p.AgentID)
		if w.Code != want {
			t.Fatalf("validation status %d: %s", w.Code, w.Body.String())
		}
	}
	check(200)
	s, _, _ := a.store.GetScheduledTaskSettings()
	s.TrafficReset.Enabled = false
	if _, err := a.store.SaveScheduledTaskSettings(s); err != nil {
		t.Fatal(err)
	}
	check(409)
}

func TestResetTrafficNeverFallsBackToHistoricalAllTime(t *testing.T) {
	if got := customerClientTrafficUsed(model.XUIClientView{Up: 0, Down: 0, TrafficTotal: 0, AllTime: 999}); got != 0 {
		t.Fatalf("historical usage resurrected: %d", got)
	}
}

func TestTrafficResetAdminPermissionsAndNoDirectActionBypass(t *testing.T) {
	a, _, _, _ := resetFixture(t, time.Now())
	if err := a.store.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	root, ok, err := a.store.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatal("admin authentication failed")
	}
	rootToken, _, err := a.store.CreateAdminSession(root, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := a.store.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area", Password: "password123", Enabled: &enabled, AgentIDs: []string{"reset-agent"}}); err != nil {
		t.Fatal(err)
	}
	area, ok, err := a.store.AuthenticateAdmin("area", "password123")
	if err != nil || !ok {
		t.Fatal("area authentication failed")
	}
	areaToken, _, err := a.store.CreateAdminSession(area, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {areaToken, 403}, {rootToken, 200}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/traffic-reset", nil)
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: tc.token})
		w := httptest.NewRecorder()
		a.handleAdminTrafficReset(w, req)
		if w.Code != tc.status {
			t.Fatalf("reset permission %d: %s", w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/reset-agent/xui/actions", bytes.NewBufferString(`{"kind":"reset_client_traffic","payload":{}}`))
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: rootToken})
	w := httptest.NewRecorder()
	a.handleXUIActions(w, req, "reset-agent", nil)
	if w.Code != 403 {
		t.Fatalf("direct reset action bypassed scheduler: %d %s", w.Code, w.Body.String())
	}
}

func TestTrafficResetActionLogUsesActualTimeAndExplicitOutcome(t *testing.T) {
	started := "2026-10-02T00:01:09+08:00"
	finished := "2026-10-02T00:01:10+08:00"
	job := model.TrafficResetJob{ID: "job", AgentID: "agent", AgentName: "US-DMIT", ClientName: "Alice", Email: "alice@example.com", RunAt: 1, ActionID: 5}
	for _, tc := range []struct {
		name, status, outcome string
		result                map[string]any
		message               string
		verify                bool
	}{
		{"success", model.XUIActionStatusSucceeded, "succeeded", nil, "", false},
		{"known failure", model.XUIActionStatusFailed, "failed", nil, "manually disabled", false},
		{"unknown", model.XUIActionStatusFailed, "uncertain", map[string]any{"uncertain": true}, "connection lost", false},
		{"reset but verification failed", model.XUIActionStatusFailed, "unverified", map[string]any{"reset_done": true}, "readback failed", false},
		{"read-only verification", model.XUIActionStatusSucceeded, "succeeded", nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.result
			if result == nil {
				result = map[string]any{}
			}
			result["execution_started_at"] = started
			result["execution_finished_at"] = finished
			action := model.XUIAction{Status: tc.status, Error: tc.message, Result: result, Payload: map[string]any{"resume_only": tc.verify}}
			entry := trafficResetActionLog(job, action)
			if entry.Outcome != tc.outcome || entry.StartedAt != "2026-10-01T16:01:09Z" || entry.CompletedAt != "2026-10-01T16:01:10Z" || entry.AgentName != "US-DMIT" || entry.ClientName != "Alice" || entry.TimeSource != "client" || entry.Message == "" {
				t.Fatalf("bad execution log: %#v", entry)
			}
			if (entry.Operation == "verify") != tc.verify {
				t.Fatalf("read-only verification misreported as reset: %#v", entry)
			}
		})
	}
	claimed := time.Now().UTC()
	completed := claimed.Add(time.Minute)
	entry := trafficResetActionLog(job, model.XUIAction{Status: model.XUIActionStatusFailed, Error: "execution lease expired; retry manually if appropriate", ClaimedAt: &claimed, CompletedAt: &completed})
	if entry.Outcome != "uncertain" || entry.TimeSource != "server_claim" || entry.CompletedTimeSource != "server" {
		t.Fatalf("unknown outcome reported as actual success/failure: %#v", entry)
	}
}

func TestTrafficResetSweepKeepsSuccessAndSkipAuditLogs(t *testing.T) {
	boundary := time.Date(2026, 10, 2, 0, 0, 0, 0, model.TrafficResetLocation)
	a, _, snap, _ := resetFixture(t, boundary.Add(time.Minute))
	if err := a.runTrafficResetSweep(boundary.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	actions, err := a.store.ClaimPendingXUIActions(snap.AgentID, 10)
	if err != nil || len(actions) != 1 {
		t.Fatalf("claim: %v %v", actions, err)
	}
	result := model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: map[string]any{"execution_started_at": "2026-10-02T00:01:12+08:00", "execution_finished_at": "2026-10-02T00:01:13+08:00", "reset_done": true}}
	if _, err := a.store.CompleteXUIAction(snap.AgentID, actions[0].ID, result); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := a.runTrafficResetSweep(boundary.Add(2 * time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := a.store.ListTrafficResetLogs(100)
	if err != nil || len(logs) != 1 || logs[0].Outcome != "succeeded" || logs[0].StartedAt != "2026-10-01T16:01:12Z" {
		t.Fatalf("success receipt not logged exactly once: %#v %v", logs, err)
	}
	b, _, _, _ := resetFixture(t, boundary.Add(-5*time.Minute))
	if err := b.runTrafficResetSweep(boundary.Add(-5 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	settings, _, err := b.store.GetScheduledTaskSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.TrafficReset.Enabled = false
	if _, err := b.store.SaveScheduledTaskSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := b.runTrafficResetSweep(boundary); err != nil {
		t.Fatal(err)
	}
	logs, err = b.store.ListTrafficResetLogs(100)
	if err != nil || len(logs) != 1 || logs[0].Outcome != "skipped" || logs[0].StartedAt != "" || logs[0].TimeSource != "not_executed" {
		t.Fatalf("skip mistaken for actual reset: %#v %v", logs, err)
	}
}
