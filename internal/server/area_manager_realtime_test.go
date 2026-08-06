package server

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestAreaManagerRealtimeMetricsUseOnlyAuthorizedClientTraffic(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "hk", AgentName: "HK"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-realtime", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID: "hk", InboundID: 7, InboundTag: "node-7", ClientEmail: "allowed@example.com", Enabled: &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment: %v", err)
	}

	now := time.Now().UTC()
	if err := sqliteStore.SaveSnapshot(realtimeTrafficSnapshot("hk", "node-7", now.Add(-time.Minute))); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	app := &App{store: sqliteStore, areaTrafficSamples: make(map[string]areaManagerTrafficSample)}
	user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager}

	firstAt := now.Add(-2 * time.Second)
	first := app.areaManagerRealtimeMetrics(user, []model.AgentRealtimeMetrics{
		realtimeTrafficMetric("hk", firstAt, 100, 200, 50_000, 60_000),
	})
	firstHK := realtimeMetricByAgent(t, first, "hk")
	if firstHK.Summary.NetTrafficSent != 100 || firstHK.Summary.NetTrafficRecv != 200 || firstHK.Summary.NetIOUp != 0 || firstHK.Summary.NetIODown != 0 {
		t.Fatalf("unexpected first scoped metric: %#v", firstHK.Summary)
	}

	second := app.areaManagerRealtimeMetrics(user, []model.AgentRealtimeMetrics{
		realtimeTrafficMetric("hk", now, 140, 260, 900_000, 1_000_000),
	})
	secondHK := realtimeMetricByAgent(t, second, "hk")
	if secondHK.Summary.NetTrafficSent != 140 || secondHK.Summary.NetTrafficRecv != 260 || secondHK.Summary.NetTrafficTotal != 400 {
		t.Fatalf("unauthorized traffic affected scoped totals: %#v", secondHK.Summary)
	}
	if secondHK.Summary.NetIOUp != 20 || secondHK.Summary.NetIODown != 30 {
		t.Fatalf("expected two-second scoped rates 20/30, got %#v", secondHK.Summary)
	}
}

func TestAreaManagerRealtimeMetricsFollowRealmForwarding(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	for _, agent := range []model.AgentRegisterRequest{
		{AgentID: "gz", AgentName: "GZ", PublicIPv4: "192.0.2.10"},
		{AgentID: "hk", AgentName: "HK", PublicIPv4: "192.0.2.20"},
	} {
		if _, err := sqliteStore.RegisterAgent(agent); err != nil {
			t.Fatalf("RegisterAgent %s: %v", agent.AgentID, err)
		}
	}
	gzConfig, found, err := sqliteStore.GetAgentConfig("gz")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig gz: found=%v err=%v", found, err)
	}
	gzConfig.Entry.ImportDomain = "gz.example.com"
	gzConfig.Entry.PortForwarding = model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
		Enabled: true, ListenPort: 20001, TargetAgentID: "hk", TargetAddress: "192.0.2.20", TargetPort: 20001, Network: "tcp",
	}}}
	if _, err := sqliteStore.UpdateAgentConfig("gz", gzConfig); err != nil {
		t.Fatalf("UpdateAgentConfig gz: %v", err)
	}

	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-forward-realtime", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	for _, assignment := range []model.AreaManagerAssignmentRequest{
		{AgentID: "gz", InboundID: 20001, InboundTag: "realm:20001", Enabled: &enabled},
		{AgentID: "hk", InboundID: 7, InboundTag: "node-7", ClientEmail: "allowed@example.com", Enabled: &enabled},
	} {
		if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, assignment); err != nil {
			t.Fatalf("CreateAreaManagerAssignment: %v", err)
		}
	}

	now := time.Now().UTC()
	if err := sqliteStore.SaveSnapshot(model.AgentSnapshot{AgentID: "gz", AgentName: "GZ", ReportedAt: now.Add(-time.Minute)}); err != nil {
		t.Fatalf("SaveSnapshot gz: %v", err)
	}
	if err := sqliteStore.SaveSnapshot(realtimeTrafficSnapshot("hk", "node-7", now.Add(-time.Minute))); err != nil {
		t.Fatalf("SaveSnapshot hk: %v", err)
	}
	app := &App{store: sqliteStore, areaTrafficSamples: make(map[string]areaManagerTrafficSample)}
	user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager}

	firstAt := now.Add(-2 * time.Second)
	_ = app.areaManagerRealtimeMetrics(user, []model.AgentRealtimeMetrics{
		realtimeTrafficMetric("hk", firstAt, 100, 200, 50_000, 60_000),
	})
	second := app.areaManagerRealtimeMetrics(user, []model.AgentRealtimeMetrics{
		realtimeTrafficMetric("hk", now, 160, 280, 900_000, 1_000_000),
	})
	for _, agentID := range []string{"gz", "hk"} {
		metric := realtimeMetricByAgent(t, second, agentID)
		if metric.Summary.NetTrafficSent != 160 || metric.Summary.NetTrafficRecv != 280 || metric.Summary.NetIOUp != 30 || metric.Summary.NetIODown != 40 {
			t.Fatalf("%s did not follow authorized final-client traffic: %#v", agentID, metric.Summary)
		}
	}
}

func TestRealtimeBrowserMetricsNeverExposeRawXUITraffic(t *testing.T) {
	app := &App{}
	raw := []model.AgentRealtimeMetrics{{
		AgentID: "agent-1",
		Summary: model.VPSSummary{NetIOUp: 123},
		HAProxy: &model.HAProxySnapshot{Rules: []model.HAProxyRuleRuntimeStatus{{RuleID: "entry", Status: "primary"}}},
		XUITraffic: &model.XUIRealtimeTraffic{SampleID: 1, Clients: []model.XUIRealtimeClientTraffic{{
			InboundID: 1, Email: "secret@example.com", Up: 10, Down: 20,
		}}},
	}}
	filtered := app.filterRealtimeMetricsForAdmin(model.AdminUser{ID: 1, Role: model.AdminRoleRoot}, raw)
	if len(filtered) != 1 || filtered[0].XUITraffic != nil {
		t.Fatalf("root browser received raw X-ui traffic: %#v", filtered)
	}
	if filtered[0].Summary.NetIOUp != 123 {
		t.Fatalf("root system metric was unexpectedly changed: %#v", filtered[0].Summary)
	}
	if filtered[0].HAProxy == nil || filtered[0].HAProxy.Rules[0].Status != "primary" {
		t.Fatalf("root browser lost HAProxy runtime status: %#v", filtered[0].HAProxy)
	}
}

func TestRealtimeHubMarksOnlyNewXUITrafficSamples(t *testing.T) {
	hub := newRealtimeHub()
	updates, _, cancel := hub.subscribe()
	defer cancel()
	metric := model.AgentRealtimeMetrics{
		AgentID: "agent-1",
		XUITraffic: &model.XUIRealtimeTraffic{SampleID: 77, Clients: []model.XUIRealtimeClientTraffic{{
			InboundID: 1, Email: "client@example.com", Up: 10,
		}}},
	}
	hub.update(metric)
	first := <-updates
	if first.XUITraffic == nil || first.XUITraffic.CollectedAt.IsZero() {
		t.Fatalf("new X-ui traffic sample was not timestamped: %#v", first.XUITraffic)
	}
	collectedAt := first.XUITraffic.CollectedAt

	hub.update(metric)
	second := <-updates
	if second.XUITraffic != nil {
		t.Fatalf("duplicate X-ui traffic sample should not trigger recalculation: %#v", second.XUITraffic)
	}
	stored := hub.snapshot()
	if len(stored) != 1 || stored[0].XUITraffic == nil || !stored[0].XUITraffic.CollectedAt.Equal(collectedAt) {
		t.Fatalf("stored sample timestamp changed for duplicate payload: %#v", stored)
	}
}

func TestDispatchXUITrafficCollectionRealtimeAttachesCurrentToken(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-token", AgentName: "Token Agent"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	cfg, found, err := sqliteStore.GetAgentConfig("agent-token")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig: found=%v err=%v", found, err)
	}
	cfg.XUI = config.XUIConfig{Enabled: true, APIToken: "latest-token"}
	if _, err := sqliteStore.UpdateAgentConfig("agent-token", cfg); err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}

	hub := newRealtimeHub()
	session := hub.registerAgentControl("agent-token")
	defer hub.unregisterAgentControl("agent-token", session)
	app := &App{store: sqliteStore, realtime: hub}
	app.dispatchXUITrafficCollectionRealtime("agent-token")
	select {
	case message := <-session.ch:
		if message.Type != model.AgentControlCollectXUI || message.XUIAuth == nil || message.XUIAuth.APIToken != "latest-token" {
			t.Fatalf("unexpected collection control: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for X-ui traffic collection control")
	}
}

func realtimeTrafficSnapshot(agentID, inboundTag string, reportedAt time.Time) model.AgentSnapshot {
	return model.AgentSnapshot{
		AgentID: agentID, AgentName: agentID, ReportedAt: reportedAt,
		Summary: model.VPSSummary{PublicIPv4: "192.0.2.20"},
		XUI: &model.XUISnapshot{CollectedAt: reportedAt, Inbounds: []map[string]any{{
			"id": 7, "tag": inboundTag, "remark": "VLESS", "protocol": "vless", "port": 20001, "enable": true,
			"settings":       `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"allowed@example.com","enable":true},{"id":"22222222-2222-2222-2222-222222222222","email":"hidden@example.com","enable":true}]}`,
			"streamSettings": map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"serverName": "hk.example.com"}},
			"clientStats": []map[string]any{
				{"email": "allowed@example.com", "enable": true, "up": 1, "down": 2},
				{"email": "hidden@example.com", "enable": true, "up": 3, "down": 4},
			},
		}}},
	}
}

func realtimeTrafficMetric(agentID string, collectedAt time.Time, allowedUp, allowedDown, hiddenUp, hiddenDown int64) model.AgentRealtimeMetrics {
	return model.AgentRealtimeMetrics{
		AgentID: agentID,
		XUITraffic: &model.XUIRealtimeTraffic{SampleID: collectedAt.UnixNano(), CollectedAt: collectedAt, Clients: []model.XUIRealtimeClientTraffic{
			{InboundID: 7, InboundTag: "node-7", Email: "allowed@example.com", Up: allowedUp, Down: allowedDown},
			{InboundID: 7, InboundTag: "node-7", Email: "hidden@example.com", Up: hiddenUp, Down: hiddenDown},
		}},
	}
}

func realtimeMetricByAgent(t *testing.T, metrics []model.AgentRealtimeMetrics, agentID string) model.AgentRealtimeMetrics {
	t.Helper()
	for _, metric := range metrics {
		if metric.AgentID == agentID {
			return metric
		}
	}
	t.Fatalf("metric for agent %s not found in %#v", agentID, metrics)
	return model.AgentRealtimeMetrics{}
}
