package server

import (
	"path/filepath"
	"testing"
	"time"

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
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
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
