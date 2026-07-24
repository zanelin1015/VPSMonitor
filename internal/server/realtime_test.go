package server

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestRealtimeHubAgentControlLifecycle(t *testing.T) {
	hub := newRealtimeHub()
	if hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should fail when agent has no realtime control session")
	}

	session := hub.registerAgentControl("agent-1")
	if !hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should succeed for an active realtime control session")
	}
	select {
	case message := <-session.ch:
		if message.Type != model.AgentControlCollectNow {
			t.Fatalf("unexpected control message: %q", message.Type)
		}
	default:
		t.Fatal("control message was not queued")
	}

	hub.unregisterAgentControl("agent-1", session)
	if hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should fail after unregistering realtime control session")
	}
}

func TestDispatchXUIActionRealtimeAttachesCurrentAPITokenWithoutPersistingIt(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-token", AgentName: "Agent Token"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	cfg, found, err := sqliteStore.GetAgentConfig("agent-token")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig found=%v err=%v", found, err)
	}
	cfg.XUI = config.XUIConfig{Enabled: true, BaseURL: "https://xui.example", APIToken: "latest-server-token"}
	if _, err := sqliteStore.UpdateAgentConfig("agent-token", cfg); err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}
	action, err := sqliteStore.CreateXUIAction("agent-token", model.XUIActionRequest{
		Kind: model.XUIActionUpdateClientTraffic,
		Payload: map[string]any{
			"inbound_id":  1,
			"email":       "client@example.com",
			"total_bytes": int64(50 * 1024 * 1024 * 1024),
		},
	})
	if err != nil {
		t.Fatalf("CreateXUIAction: %v", err)
	}

	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	session := app.realtime.registerAgentControl("agent-token")
	defer app.realtime.unregisterAgentControl("agent-token", session)
	if _, ok := app.dispatchXUIActionRealtime("agent-token", action); !ok {
		t.Fatal("expected action to dispatch over realtime websocket")
	}
	select {
	case message := <-session.ch:
		if message.XUIAuth == nil || message.XUIAuth.APIToken != "latest-server-token" {
			t.Fatalf("expected current API token in one-use control message, got %#v", message.XUIAuth)
		}
	default:
		t.Fatal("expected websocket control message")
	}

	stored, found, err := sqliteStore.GetXUIAction("agent-token", action.ID)
	if err != nil || !found {
		t.Fatalf("GetXUIAction found=%v err=%v", found, err)
	}
	if stored.XUIAuth != nil {
		t.Fatalf("API token must not be persisted with an action: %#v", stored.XUIAuth)
	}
}

func TestAttachXUIActionAuthOnlyTargetsPanelActions(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-poll", AgentName: "Agent Poll"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	cfg, found, err := sqliteStore.GetAgentConfig("agent-poll")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig found=%v err=%v", found, err)
	}
	cfg.XUI = config.XUIConfig{Enabled: true, APIToken: "poll-token"}
	if _, err := sqliteStore.UpdateAgentConfig("agent-poll", cfg); err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}
	actions := []model.XUIAction{
		{Kind: model.XUIActionSetClientEnabled},
		{Kind: model.XUIActionUpdateClientTraffic},
		{Kind: model.XUIActionUpdateClient},
		{Kind: model.XUIActionExecuteCommand},
	}

	app := &App{store: sqliteStore}
	app.attachXUIActionAuth("agent-poll", actions)
	if actions[0].XUIAuth == nil || actions[0].XUIAuth.APIToken != "poll-token" {
		t.Fatalf("expected panel action auth, got %#v", actions[0].XUIAuth)
	}
	if actions[1].XUIAuth == nil || actions[1].XUIAuth.APIToken != "poll-token" {
		t.Fatalf("expected traffic action auth, got %#v", actions[1].XUIAuth)
	}
	if actions[2].XUIAuth != nil || actions[3].XUIAuth != nil {
		t.Fatalf("non-panel actions must not carry API tokens: %#v", actions)
	}
}

func TestRealtimeHubUsesServerReceiveTime(t *testing.T) {
	hub := newRealtimeHub()
	clientTime := time.Now().UTC().Add(-30 * time.Minute)
	before := time.Now().UTC().Add(-time.Second)

	hub.update(model.AgentRealtimeMetrics{
		AgentID:    "drifted-agent",
		ReportedAt: clientTime,
	})
	after := time.Now().UTC().Add(time.Second)

	snapshot := hub.snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("expected one realtime metric, got %d", len(snapshot))
	}
	reportedAt := snapshot[0].ReportedAt
	if reportedAt.Before(before) || reportedAt.After(after) {
		t.Fatalf("expected server receive time between %s and %s, got %s", before, after, reportedAt)
	}
	if reportedAt.Equal(clientTime) {
		t.Fatalf("expected client reported time to be ignored")
	}
}

func TestDispatchXUIActionRealtimeSendsUpdateClient(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1", AgentName: "Agent 1"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	action, err := sqliteStore.CreateXUIAction("agent-1", model.XUIActionRequest{
		Kind:    model.XUIActionUpdateClient,
		Payload: map[string]any{"version": "0.2.38"},
	})
	if err != nil {
		t.Fatalf("CreateXUIAction: %v", err)
	}
	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	session := app.realtime.registerAgentControl("agent-1")
	defer app.realtime.unregisterAgentControl("agent-1", session)

	dispatched, ok := app.dispatchXUIActionRealtime("agent-1", action)
	if !ok {
		t.Fatal("expected update_client to dispatch over realtime websocket")
	}
	if dispatched.Status != model.XUIActionStatusRunning || dispatched.ClaimedAt == nil {
		t.Fatalf("expected dispatched action to be marked running, got %#v", dispatched)
	}
	select {
	case message := <-session.ch:
		if message.Type != model.AgentControlExecuteXUI || message.Kind != model.XUIActionUpdateClient || message.ActionID != action.ID {
			t.Fatalf("unexpected control message: %#v", message)
		}
	default:
		t.Fatal("expected websocket control message")
	}

	stored, found, err := sqliteStore.GetXUIAction("agent-1", action.ID)
	if err != nil || !found {
		t.Fatalf("GetXUIAction found=%v err=%v", found, err)
	}
	if stored.Status != model.XUIActionStatusRunning || stored.ClaimedAt == nil {
		t.Fatalf("expected stored action running, got %#v", stored)
	}
}

func TestDispatchPendingXUIActionsRealtimeSendsExistingPendingUpdateClient(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1", AgentName: "Agent 1"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	action, err := sqliteStore.CreateXUIAction("agent-1", model.XUIActionRequest{
		Kind:    model.XUIActionUpdateClient,
		Payload: map[string]any{"version": "0.2.38"},
	})
	if err != nil {
		t.Fatalf("CreateXUIAction: %v", err)
	}
	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	session := app.realtime.registerAgentControl("agent-1")
	defer app.realtime.unregisterAgentControl("agent-1", session)

	app.dispatchPendingXUIActionsRealtime("agent-1")

	select {
	case message := <-session.ch:
		if message.Type != model.AgentControlExecuteXUI || message.Kind != model.XUIActionUpdateClient || message.ActionID != action.ID {
			t.Fatalf("unexpected control message: %#v", message)
		}
	default:
		t.Fatal("expected pending update_client to be pushed over websocket")
	}
	stored, found, err := sqliteStore.GetXUIAction("agent-1", action.ID)
	if err != nil || !found {
		t.Fatalf("GetXUIAction found=%v err=%v", found, err)
	}
	if stored.Status != model.XUIActionStatusRunning || stored.ClaimedAt == nil {
		t.Fatalf("expected pending action marked running, got %#v", stored)
	}
}

func TestIsUsableObservedIPRejectsLocalAddresses(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "::1", "10.0.0.1", "192.168.1.2", "0.0.0.0", ""} {
		if isUsableObservedIP(value) {
			t.Fatalf("expected %q to be unusable", value)
		}
	}
	if !isUsableObservedIP("47.239.135.242") {
		t.Fatal("expected public observed IP to be usable")
	}
}

func TestRealtimeHubAgentControlReplacesPreviousSession(t *testing.T) {
	hub := newRealtimeHub()
	first := hub.registerAgentControl("agent-1")
	second := hub.registerAgentControl("agent-1")

	select {
	case <-first.done:
	default:
		t.Fatal("previous session should be closed when a new session registers")
	}

	if !hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should target the replacement session")
	}
	select {
	case <-first.ch:
		t.Fatal("first session should not receive messages after replacement")
	default:
	}
	select {
	case message := <-second.ch:
		if message.Type != model.AgentControlCollectNow {
			t.Fatalf("unexpected control message: %q", message.Type)
		}
	default:
		t.Fatal("replacement session did not receive control message")
	}
}

func TestRealtimeHubTerminalRelayValidatesAgent(t *testing.T) {
	hub := newRealtimeHub()
	session := hub.registerTerminal("agent-1", "tty-1")
	defer hub.unregisterTerminal("tty-1", session)

	if hub.relayTerminalMessage(model.TerminalMessage{Type: model.TerminalMessageOutput, SessionID: "tty-1", AgentID: "other", Data: "leak"}) {
		t.Fatal("relay should reject messages from a different agent")
	}
	if !hub.relayTerminalMessage(model.TerminalMessage{Type: model.TerminalMessageOutput, SessionID: "tty-1", AgentID: "agent-1", Data: "ok"}) {
		t.Fatal("relay should accept matching agent messages")
	}
	select {
	case message := <-session.ch:
		if message.Data != "ok" {
			t.Fatalf("unexpected terminal relay message: %#v", message)
		}
	default:
		t.Fatal("terminal message was not relayed")
	}
}

func TestSanitizeGeoForAreaManagerKeepsOnlyCountry(t *testing.T) {
	geo := sanitizeGeoForAreaManager(&model.IPGeoView{
		IP:          "203.0.113.10",
		CountryCode: "VN",
		CountryName: "Vietnam",
		RegionName:  "Ho Chi Minh",
		City:        "Ho Chi Minh City",
	})
	if geo == nil || geo.CountryCode != "VN" || geo.CountryName != "Vietnam" {
		t.Fatalf("expected country to remain, got %#v", geo)
	}
	if geo.IP != "" || geo.RegionName != "" || geo.City != "" {
		t.Fatalf("expected precise geo fields to be redacted, got %#v", geo)
	}
}

func TestSanitizeDashboardAgentForAreaManagerRemovesFinanceClients(t *testing.T) {
	agent := sanitizeDashboardAgentForAreaManager(model.DashboardAgentView{
		AgentID:             "agent-1",
		FinanceClientsReady: true,
		FinanceClients: []model.FinanceClientView{{
			InboundID: 1,
			Email:     "private@example.com",
			Enabled:   true,
		}},
	}, map[string][]string{"agent-1": {"public"}})
	if agent.FinanceClientsReady || len(agent.FinanceClients) != 0 {
		t.Fatalf("expected finance client state to be removed for area manager, got %#v", agent.FinanceClients)
	}
}

func TestAreaManagerRealtimeMetricsAreSanitized(t *testing.T) {
	app := &App{}
	user := model.AdminUser{
		ID:       10,
		Role:     model.AdminRoleAreaManager,
		AgentIDs: []string{"allowed"},
	}
	metrics := []model.AgentRealtimeMetrics{
		{
			AgentID:       "allowed",
			AgentName:     "internal-name",
			ClientVersion: "0.2.11",
			ClientOS:      "linux",
			ClientArch:    "amd64",
			SystemVersion: "Debian 12",
			ReportedAt:    time.Now().UTC(),
			Summary: model.VPSSummary{
				Hostname:        "secret-host",
				ObservedIP:      "203.0.113.10",
				ServerSeenIP:    "198.51.100.20",
				PublicIPv4:      "203.0.113.11",
				CPU:             42,
				MemUsed:         512,
				MemTotal:        1024,
				NetTrafficSent:  100,
				NetTrafficRecv:  200,
				NetTrafficTotal: 300,
				NetIOUp:         10,
				NetIODown:       20,
			},
		},
		{
			AgentID: "hidden",
			Summary: model.VPSSummary{
				NetTrafficSent: 999,
			},
		},
	}

	filtered := app.filterRealtimeMetricsForAdmin(user, metrics)
	if len(filtered) != 1 {
		t.Fatalf("expected only one authorized metric, got %d", len(filtered))
	}
	got := filtered[0]
	if got.AgentName != "" || got.ClientVersion != "" || got.ClientOS != "" || got.ClientArch != "" || got.SystemVersion != "" {
		t.Fatalf("expected client identity/runtime fields to be stripped, got %#v", got)
	}
	if got.Summary.Hostname != "" || got.Summary.ObservedIP != "" || got.Summary.ServerSeenIP != "" || got.Summary.PublicIPv4 != "" || got.Summary.CPU != 0 || got.Summary.MemTotal != 0 {
		t.Fatalf("expected host/system metrics to be stripped, got %#v", got.Summary)
	}
	if got.Summary.NetTrafficSent != 100 || got.Summary.NetTrafficRecv != 200 || got.Summary.NetTrafficTotal != 300 || got.Summary.NetIOUp != 10 || got.Summary.NetIODown != 20 {
		t.Fatalf("expected traffic metrics to remain, got %#v", got.Summary)
	}
}

func TestAreaManagerClientScopeFiltersUnassignedTopologyClients(t *testing.T) {
	scope := areaManagerClientScope{
		exactClients: map[string]struct{}{
			areaClientExactKey("hk", 1001, "HK:20001", "assigned@example.com"): {},
		},
		inbounds: map[string]struct{}{},
		agents: map[string]struct{}{
			"hk": {},
		},
	}
	chains := []model.ClientChainView{
		{
			Key:             "hk::1001::assigned@example.com",
			RootAgentID:     "hk",
			RootAgentName:   "HK Internal",
			RootInboundTag:  "HK:20001",
			RootClientEmail: "assigned@example.com",
			Steps: []model.ClientChainStep{
				{StepType: "client", AgentID: "hk", Label: "assigned@example.com"},
				{StepType: "outbound", AgentID: "hk", OutboundTag: "direct", Label: "direct"},
			},
		},
		{
			Key:             "hk::1001::hidden@example.com",
			RootAgentID:     "hk",
			RootAgentName:   "HK Internal",
			RootInboundTag:  "HK:20001",
			RootClientEmail: "hidden@example.com",
			Steps: []model.ClientChainStep{
				{StepType: "client", AgentID: "hk", Label: "hidden@example.com"},
			},
		},
	}

	filtered := sanitizeClientChainsForAreaManager(chains, map[string]struct{}{"hk": {}}, nil, map[string]string{"hk": "HK Public"}, scope)
	if len(filtered) != 1 {
		t.Fatalf("expected only assigned topology client, got %d", len(filtered))
	}
	if filtered[0].RootClientEmail != "assigned@example.com" || filtered[0].RootAgentName != "HK Public" {
		t.Fatalf("unexpected filtered chain: %#v", filtered[0])
	}

	view := &model.GlobalDashboardView{
		Agents:       []model.DashboardAgentView{{AgentID: "hk", ClientCount: 22, NodeCount: 10, OnlineClientCount: 5}},
		ClientChains: filtered,
	}
	applyAreaManagerClientCounts(view, scope)
	if view.Agents[0].ClientCount != 1 || view.Agents[0].NodeCount != 1 || view.Agents[0].OnlineClientCount != 1 {
		t.Fatalf("expected counts to match assigned clients only, got %#v", view.Agents[0])
	}
}

func TestAreaManagerVisibleLinksKeepGrantedRealmEntry(t *testing.T) {
	scope := areaManagerClientScope{
		inbounds: map[string]struct{}{
			areaClientInboundKey("hk", 1001, "HK:20001"): {},
		},
		realmPorts: map[string]struct{}{
			areaRealmPortKey("gz", 20001): {},
		},
		agents: map[string]struct{}{
			"gz": {},
			"hk": {},
		},
	}
	links := []model.TopologyLinkView{
		{
			Source: model.TopologyOutboundRef{AgentID: "gz", OutboundTag: "realm:gz-20001", Protocol: "realm", ListenPort: 20001},
			Target: model.TopologyInboundRef{AgentID: "hk", InboundID: 1001, InboundTag: "HK:20001"},
		},
		{
			Source: model.TopologyOutboundRef{AgentID: "gz", OutboundTag: "realm:gz-20002", Protocol: "realm", ListenPort: 20002},
			Target: model.TopologyInboundRef{AgentID: "hk", InboundID: 1002, InboundTag: "HK:20002"},
		},
	}

	filtered := filterTopologyLinksVisibleToAreaManager(links, nil, scope)
	if len(filtered) != 1 || filtered[0].Source.ListenPort != 20001 {
		t.Fatalf("expected only the doubly granted Realm link to remain, got %#v", filtered)
	}
}

func TestAreaManagerXUIOverviewFiltersUnassignedClients(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "hk",
		AgentName: "HK Internal",
	}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-hk",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"hk"},
		OutboundGrants: []model.AreaManagerOutboundGrantRequest{
			{AgentID: "hk", OutboundTag: "assigned-out"},
			{AgentID: "hk", OutboundTag: "mixed-out"},
		},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:          "hk",
		InboundID:        1001,
		InboundTag:       "HK:20001",
		ClientEmail:      "",
		PublicClientName: "Accidental whole node",
		Enabled:          &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment: %v", err)
	}
	customer, err := sqliteStore.CreateCustomerForOwner(model.CustomerAccountRequest{
		Username: "customer-hk",
		Password: "password123",
	}, model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}
	if _, err := sqliteStore.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{
		AgentID:          "hk",
		InboundID:        1001,
		InboundTag:       "HK:20001",
		ClientEmail:      "assigned@example.com",
		PublicClientName: "Assigned",
		Enabled:          &enabled,
	}); err != nil {
		t.Fatalf("CreateCustomerAssignment: %v", err)
	}

	app := &App{store: sqliteStore}
	overview := &model.XUIOverview{
		AgentID:           "hk",
		AgentName:         "HK Internal",
		BaseURL:           "https://x-ui.example",
		ClientCount:       3,
		OnlineClientCount: 2,
		NodeCount:         2,
		Nodes: []model.XUINodeView{
			{ID: 1001, Tag: "HK:20001", ClientCount: 2, OnlineCount: 1, Up: 10, Down: 20, Total: 30, AllTime: 40},
			{ID: 1002, Tag: "HK:20002", ClientCount: 1},
		},
		Clients: []model.XUIClientView{
			{InboundID: 1001, InboundTag: "HK:20001", Email: "assigned@example.com", TotalGB: 100, ExpiryTime: 200, LastOnline: 300},
			{InboundID: 1001, InboundTag: "HK:20001", Email: "hidden@example.com"},
			{InboundID: 1002, InboundTag: "HK:20002", Email: "other@example.com"},
		},
		Outbounds: []model.XUIOutboundView{
			{Tag: "assigned-out", Address: "203.0.113.10"},
			{Tag: "mixed-out", Address: "203.0.113.11"},
			{Tag: "hidden-out", Address: "203.0.113.12"},
		},
		RoutingRules: []model.XUIRoutingRuleView{
			{Index: 1, Users: []string{"assigned@example.com"}, OutboundTag: "assigned-out", Summary: "user=assigned@example.com | outbound=assigned-out"},
			{Index: 2, Users: []string{"hidden@example.com"}, OutboundTag: "hidden-out", Summary: "user=hidden@example.com | outbound=hidden-out"},
			{Index: 3, Users: []string{"assigned@example.com", "hidden@example.com"}, OutboundTag: "mixed-out", Summary: "user=assigned@example.com,hidden@example.com | outbound=mixed-out"},
			{Index: 4, InboundTags: []string{"HK:20002"}, OutboundTag: "other-inbound", Summary: "inbound=HK:20002 | outbound=other-inbound"},
		},
	}

	app.sanitizeXUIOverviewForAdmin(model.AdminUser{
		ID:       manager.ID,
		Role:     model.AdminRoleAreaManager,
		AgentIDs: []string{"hk"},
	}, overview)

	if len(overview.Clients) != 1 || overview.Clients[0].Email != "assigned@example.com" {
		t.Fatalf("expected only assigned x-ui client, got %#v", overview.Clients)
	}
	if overview.ClientCount != 1 || overview.OnlineClientCount != 0 {
		t.Fatalf("expected sanitized client counters, got count=%d online=%d", overview.ClientCount, overview.OnlineClientCount)
	}
	if overview.Clients[0].TotalGB != 0 || overview.Clients[0].ExpiryTime != 0 || overview.Clients[0].LastOnline != 0 {
		t.Fatalf("expected sensitive client limits/timestamps stripped, got %#v", overview.Clients[0])
	}
	if len(overview.Nodes) != 1 || overview.Nodes[0].ID != 1001 {
		t.Fatalf("expected only assigned client inbound node, got %#v", overview.Nodes)
	}
	if overview.Nodes[0].ClientCount != 0 || overview.Nodes[0].Up != 0 || overview.BaseURL != "" {
		t.Fatalf("expected node metrics and base URL sanitized, got node=%#v base=%q", overview.Nodes[0], overview.BaseURL)
	}
	if len(overview.RoutingRules) != 2 {
		t.Fatalf("expected only assigned routing rules, got %#v", overview.RoutingRules)
	}
	if overview.RoutingRules[0].Index != 1 || overview.RoutingRules[0].Users[0] != "assigned@example.com" {
		t.Fatalf("expected assigned user rule to remain, got %#v", overview.RoutingRules[0])
	}
	if overview.RoutingRules[1].Index != 3 || len(overview.RoutingRules[1].Users) != 1 || overview.RoutingRules[1].Users[0] != "assigned@example.com" || overview.RoutingRules[1].Summary != "" {
		t.Fatalf("expected mixed user rule to be sanitized, got %#v", overview.RoutingRules[1])
	}
	if len(overview.Outbounds) != 2 || overview.Outbounds[0].Tag != "assigned-out" || overview.Outbounds[1].Tag != "mixed-out" {
		t.Fatalf("expected only granted outbounds, got %#v", overview.Outbounds)
	}
}

func TestAreaManagerXUIOverviewAllowsRealmForwardedClientsFromAssignedEntry(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz", AgentName: "GZ Entry"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "hk", AgentName: "HK Exit"}); err != nil {
		t.Fatalf("RegisterAgent hk: %v", err)
	}
	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-gz",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"gz", "hk"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:     "gz",
		InboundID:   20001,
		InboundTag:  "realm:gz-20001",
		ClientEmail: "",
		Enabled:     &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment realm: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:     "hk",
		InboundID:   1001,
		InboundTag:  "HK:20001",
		ClientEmail: "",
		Enabled:     &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment xui: %v", err)
	}

	app := &App{store: sqliteStore}
	overview := &model.XUIOverview{
		AgentID:           "gz",
		AgentName:         "GZ Entry",
		BaseURL:           "https://x-ui.example",
		ClientCount:       2,
		OnlineClientCount: 1,
		Clients: []model.XUIClientView{
			{
				InboundID:             1001,
				InboundTag:            "Realm 20001 -> HK VLESS",
				Email:                 "alice@example.com",
				ImportURL:             "vless://uuid@gz.example.com:20001?security=reality#HK",
				TotalGB:               100,
				ExpiryTime:            200,
				LastOnline:            300,
				RealmSourceAgentID:    "gz",
				RealmTargetAgentID:    "hk",
				RealmTargetInboundID:  1001,
				RealmTargetInboundTag: "HK:20001",
				RealmListenPort:       20001,
			},
			{InboundID: 1002, InboundTag: "local-hidden", Email: "hidden@example.com"},
		},
	}

	app.sanitizeXUIOverviewForAdmin(model.AdminUser{
		ID:       manager.ID,
		Role:     model.AdminRoleAreaManager,
		AgentIDs: []string{"gz", "hk"},
	}, overview)

	if len(overview.Clients) != 1 || overview.Clients[0].Email != "alice@example.com" {
		t.Fatalf("expected area manager to keep Realm-exported client from assigned entry, got %#v", overview.Clients)
	}
	if overview.Clients[0].ImportURL == "" {
		t.Fatalf("expected Realm-exported import URL to stay visible")
	}
	if overview.Clients[0].TotalGB != 0 || overview.Clients[0].ExpiryTime != 0 || overview.Clients[0].LastOnline != 0 {
		t.Fatalf("expected sensitive client metrics stripped, got %#v", overview.Clients[0])
	}
}

func TestAreaManagerXUIOverviewRejectsRealmForwardedClientsWithoutTargetGrant(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz", AgentName: "GZ Entry"}); err != nil {
		t.Fatalf("RegisterAgent gz: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "hk", AgentName: "HK Exit"}); err != nil {
		t.Fatalf("RegisterAgent hk: %v", err)
	}
	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-gz",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"gz", "hk"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:     "gz",
		InboundID:   20001,
		InboundTag:  "realm:gz-20001",
		ClientEmail: "",
		Enabled:     &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment realm: %v", err)
	}

	app := &App{store: sqliteStore}
	overview := &model.XUIOverview{
		AgentID: "gz",
		Clients: []model.XUIClientView{{
			InboundID:             20001,
			InboundTag:            "Realm 20001 -> HK VLESS",
			Email:                 "alice@example.com",
			ImportURL:             "vless://uuid@gz.example.com:20001?security=reality#HK",
			RealmSourceAgentID:    "gz",
			RealmTargetAgentID:    "hk",
			RealmTargetInboundID:  1001,
			RealmTargetInboundTag: "HK:20001",
			RealmListenPort:       20001,
		}},
	}

	app.sanitizeXUIOverviewForAdmin(model.AdminUser{
		ID:       manager.ID,
		Role:     model.AdminRoleAreaManager,
		AgentIDs: []string{"gz", "hk"},
	}, overview)

	if len(overview.Clients) != 0 {
		t.Fatalf("expected Realm-exported client without HK x-ui grant to be hidden, got %#v", overview.Clients)
	}
}

func TestAreaManagerXUIOverviewRejectsRealmForwardedClientsWithoutPortGrant(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	app := &App{store: sqliteStore}
	overview := &model.XUIOverview{
		AgentID: "gz",
		Clients: []model.XUIClientView{{
			InboundID:          20001,
			InboundTag:         "Realm 20001 -> HK VLESS",
			Email:              "alice@example.com",
			ImportURL:          "vless://uuid@gz.example.com:20001?security=reality#HK",
			RealmSourceAgentID: "gz",
			RealmTargetAgentID: "hk",
			RealmListenPort:    20001,
		}},
	}

	app.sanitizeXUIOverviewForAdmin(model.AdminUser{
		ID:       7,
		Role:     model.AdminRoleAreaManager,
		AgentIDs: []string{"gz"},
	}, overview)

	if len(overview.Clients) != 0 {
		t.Fatalf("expected whole-client access without port grant to hide Realm-exported clients, got %#v", overview.Clients)
	}
}

func TestAreaManagerAssignmentPickerKeepsAllowedAgentClients(t *testing.T) {
	app := &App{}
	overview := &model.XUIOverview{
		AgentID:           "hk",
		AgentName:         "HK Internal",
		BaseURL:           "https://x-ui.example",
		ClientCount:       2,
		OnlineClientCount: 1,
		NodeCount:         1,
		Nodes: []model.XUINodeView{
			{ID: 1001, Tag: "HK:20001", ClientCount: 2, OnlineCount: 1, Up: 10, Down: 20, Total: 30, AllTime: 40},
		},
		Clients: []model.XUIClientView{
			{InboundID: 1001, InboundTag: "HK:20001", Email: "assigned@example.com", TotalGB: 100, ExpiryTime: 200, LastOnline: 300},
			{InboundID: 1001, InboundTag: "HK:20001", Email: "available@example.com", TotalGB: 100, ExpiryTime: 200, LastOnline: 300},
		},
	}

	app.sanitizeXUIOverviewForAreaAssignment(model.AdminUser{
		ID:       10,
		Role:     model.AdminRoleAreaManager,
		AgentIDs: []string{"hk"},
	}, overview)

	if len(overview.Clients) != 2 {
		t.Fatalf("expected assignment picker to keep all clients on an allowed agent, got %#v", overview.Clients)
	}
	if overview.Clients[0].TotalGB != 0 || overview.Clients[1].LastOnline != 0 {
		t.Fatalf("expected sensitive client limits/timestamps stripped, got %#v", overview.Clients)
	}
	if len(overview.Nodes) != 1 || overview.Nodes[0].ClientCount != 0 || overview.Nodes[0].Total != 0 {
		t.Fatalf("expected node picker metadata without metrics, got %#v", overview.Nodes)
	}
	if overview.BaseURL != "" || overview.OnlineClientCount != 0 {
		t.Fatalf("expected x-ui URL and online count hidden, got base=%q online=%d", overview.BaseURL, overview.OnlineClientCount)
	}
}
