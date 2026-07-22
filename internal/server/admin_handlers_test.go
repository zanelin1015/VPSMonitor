package server

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestNormalizeVersionSupportsComponentTags(t *testing.T) {
	tests := map[string]string{
		"v0.1.5":                  "0.1.5",
		"server-v0.1.6":           "0.1.6",
		"client-0.2.0":            "0.2.0",
		"refs/tags/client-v1.2.3": "1.2.3",
	}
	for input, want := range tests {
		if got := normalizeVersion(input); got != want {
			t.Fatalf("normalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHasClientUpdateAsset(t *testing.T) {
	assets := []string{
		"VPSMonitor-server-linux-amd64.tar.gz",
		"VPSMonitor-client-linux-amd64.tar.gz",
	}
	if !hasClientUpdateAsset("VPSMonitor", assets) {
		t.Fatalf("expected client asset to be detected")
	}
	if hasClientUpdateAsset("Other", assets) {
		t.Fatalf("did not expect mismatched package prefix to be detected")
	}
}

func TestFilterRootOnlyXUIActionsHidesRemoteCommands(t *testing.T) {
	actions := []model.XUIAction{
		{ID: 1, Kind: model.XUIActionUpsertRoutingRule},
		{ID: 2, Kind: model.XUIActionExecuteCommand},
		{ID: 3, Kind: model.XUIActionRestartXUI},
	}
	filtered := filterRootOnlyXUIActions(actions)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 visible actions, got %#v", filtered)
	}
	for _, action := range filtered {
		if action.Kind == model.XUIActionExecuteCommand {
			t.Fatalf("remote command action leaked to non-root admin: %#v", filtered)
		}
	}
}

func TestAreaManagerXUIActionAllowedIncludesAddClient(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1", AgentName: "Agent 1"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-1",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"agent-1"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:     "agent-1",
		InboundID:   7,
		InboundTag:  "node-7",
		ClientEmail: "",
		Enabled:     &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment: %v", err)
	}

	app := &App{store: sqliteStore}
	user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, AgentIDs: []string{"agent-1"}}
	if !app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"inbound_id":  7,
			"inbound_tag": "node-7",
			"client": map[string]any{
				"email": "alice@example.com",
			},
		},
	}) {
		t.Fatal("expected area manager to be allowed to add a client under a node")
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"client": map[string]any{
				"email": "alice@example.com",
			},
		},
	}) {
		t.Fatal("expected add_client without inbound scope to be rejected")
	}
	if !app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionDeleteClient,
		Payload: map[string]any{
			"inbound_id":  7,
			"inbound_tag": "node-7",
			"email":       "alice@example.com",
		},
	}) {
		t.Fatal("expected area manager to be allowed to delete a client under a node")
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:     "agent-1",
		InboundID:   8,
		InboundTag:  "node-8",
		ClientEmail: "seed@example.com",
		Enabled:     &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment exact client: %v", err)
	}
	if !app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"inbound_id":  8,
			"inbound_tag": "node-8",
			"client": map[string]any{
				"email": "new@example.com",
			},
		},
	}) {
		t.Fatal("expected exact client authorization to allow adding a client under the same node")
	}
	if !app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionSetClientEnabled,
		Payload: map[string]any{
			"inbound_id":  7,
			"inbound_tag": "node-7",
			"email":       "alice@example.com",
			"enabled":     false,
		},
	}) {
		t.Fatal("expected area manager to be allowed to enable/disable a client under a node")
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionDeleteClient,
		Payload: map[string]any{
			"inbound_id": 8,
			"email":      "alice@example.com",
		},
	}) {
		t.Fatal("expected delete_client outside assigned node to be rejected")
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionExecuteCommand,
	}) {
		t.Fatal("expected remote command to remain root-only")
	}
}

func cloneTestAnyMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func TestAreaManagerXUIActionEnforcesOutboundScope(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1", AgentName: "Agent 1"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "source-1", AgentName: "Source 1"}); err != nil {
		t.Fatalf("RegisterAgent source: %v", err)
	}
	if _, err := sqliteStore.UpdateAgentConfig("source-1", model.ManagedAgentConfig{
		AgentID: "source-1",
		Entry:   model.AgentEntryConfig{ImportDomain: "source.example.com"},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig source: %v", err)
	}
	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username:              "outbound-area",
		Password:              "password123",
		Enabled:               &enabled,
		AgentIDs:              []string{"agent-1", "source-1"},
		OutboundCreateEnabled: &enabled,
		OutboundGrants: []model.AreaManagerOutboundGrantRequest{
			{AgentID: "agent-1", OutboundTag: "allowed-out"},
		},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:     "source-1",
		InboundID:   9,
		InboundTag:  "source-node",
		ClientEmail: "source@example.com",
		Enabled:     &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment source: %v", err)
	}
	app := &App{store: sqliteStore}
	now := time.Now().UTC()
	if err := sqliteStore.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "agent-1",
		ReportedAt: now,
		XUI: &model.XUISnapshot{
			Outbounds: []map[string]any{{"tag": "hidden-out", "protocol": "freedom"}},
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := sqliteStore.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "source-1",
		ReportedAt: now,
		Summary:    model.VPSSummary{PublicIPv4: "203.0.113.9"},
		XUI: &model.XUISnapshot{
			CollectedAt: now,
			Inbounds: []map[string]any{{
				"id":       9,
				"tag":      "source-node",
				"remark":   "Source VLESS",
				"protocol": "vless",
				"port":     443,
				"enable":   true,
				"settings": `{"clients":[{"email":"source@example.com","enable":true,"id":"11111111-1111-1111-1111-111111111111"}]}`,
			}},
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot source: %v", err)
	}
	user := model.AdminUser{
		ID:                    manager.ID,
		Role:                  model.AdminRoleAreaManager,
		AgentIDs:              []string{"agent-1", "source-1"},
		OutboundCreateEnabled: true,
	}

	if !app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{"type": "field", "outboundTag": "allowed-out"},
		},
	}) {
		t.Fatal("expected granted outbound to be usable")
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{"type": "field", "outboundTag": "hidden-out"},
		},
	}) {
		t.Fatal("expected ungranted outbound to be rejected")
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"rule":     map[string]any{"type": "field", "outboundTag": "hidden-out"},
			"outbound": map[string]any{"tag": "hidden-out", "protocol": "freedom"},
		},
	}) {
		t.Fatal("expected create permission not to overwrite an existing ungranted outbound")
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"previous_outbound_tag": "hidden-out",
			"rule":                  map[string]any{"type": "field", "outboundTag": "new-out"},
			"outbound":              map[string]any{"tag": "new-out", "protocol": "freedom"},
		},
	}) {
		t.Fatal("expected an ungranted previous outbound tag to be rejected")
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{"type": "field", "balancerTag": "hidden-balancer"},
		},
	}) {
		t.Fatal("expected unscoped balancer to be rejected")
	}
	createPayload := map[string]any{
		"rule": map[string]any{"type": "field", "outboundTag": "new-out"},
		"outbound": map[string]any{
			"tag":      "new-out",
			"protocol": "vless",
			"settings": map[string]any{
				"address":    "source.example.com",
				"port":       443,
				"id":         "11111111-1111-1111-1111-111111111111",
				"encryption": "none",
			},
		},
		"outbound_source": map[string]any{
			"type":         "authorized_client_node",
			"agent_id":     "source-1",
			"inbound_id":   9,
			"inbound_tag":  "source-node",
			"client_email": "source@example.com",
		},
	}
	if !app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{Kind: model.XUIActionUpsertRoutingRule, Payload: createPayload}) {
		t.Fatal("expected an authorized Client node to be usable as a new outbound")
	}
	missingSource := map[string]any{"rule": createPayload["rule"], "outbound": createPayload["outbound"]}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{Kind: model.XUIActionUpsertRoutingRule, Payload: missingSource}) {
		t.Fatal("expected outbound creation without authorized source metadata to be rejected")
	}
	unauthorizedSource := cloneTestAnyMap(createPayload)
	unauthorizedSource["outbound_source"] = map[string]any{
		"type":         "authorized_client_node",
		"agent_id":     "source-1",
		"inbound_id":   9,
		"inbound_tag":  "source-node",
		"client_email": "other@example.com",
	}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{Kind: model.XUIActionUpsertRoutingRule, Payload: unauthorizedSource}) {
		t.Fatal("expected an unauthorized source client to be rejected")
	}
	spoofedEndpoint := cloneTestAnyMap(createPayload)
	spoofedEndpoint["outbound"] = map[string]any{
		"tag":      "spoofed-out",
		"protocol": "vless",
		"settings": map[string]any{"address": "other.example.com", "port": 443, "id": "11111111-1111-1111-1111-111111111111"},
	}
	spoofedEndpoint["rule"] = map[string]any{"type": "field", "outboundTag": "spoofed-out"}
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{Kind: model.XUIActionUpsertRoutingRule, Payload: spoofedEndpoint}) {
		t.Fatal("expected source metadata with a spoofed endpoint to be rejected")
	}
	user.OutboundCreateEnabled = false
	if app.areaManagerXUIActionAllowed(user, "agent-1", model.XUIActionRequest{Kind: model.XUIActionUpsertRoutingRule, Payload: createPayload}) {
		t.Fatal("expected outbound creation to be rejected when disabled")
	}
}

func TestAreaManagerXUIActionAllowedUsesAssignmentScopeWhenAgentListIsStale(t *testing.T) {
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
		Username: "area-stale",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"gz"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:     "hk",
		InboundID:   1001,
		InboundTag:  "HK:20001",
		ClientEmail: "",
		Enabled:     &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment: %v", err)
	}

	app := &App{store: sqliteStore}
	// Simulate a session/user payload that was created before hk was added to area_manager_agents.
	user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, AgentIDs: []string{"gz"}}
	if !app.adminCanAccessAgent(user, "hk") {
		t.Fatal("expected x-ui assignment to grant target agent access even when the session agent list is stale")
	}
	if !app.areaManagerXUIActionAllowed(user, "hk", model.XUIActionRequest{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"inbound_id":  1001,
			"inbound_tag": "HK:20001",
			"client": map[string]any{
				"email": "alice@example.com",
			},
		},
	}) {
		t.Fatal("expected area manager to add client on the assigned HK node")
	}
}

func TestAreaManagerDashboardFiltersUseAssignmentScopeWhenAgentListIsStale(t *testing.T) {
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
		Username: "area-filter",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"gz"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if _, err := sqliteStore.CreateAreaManagerAssignment(manager.ID, model.AreaManagerAssignmentRequest{
		AgentID:    "hk",
		InboundID:  1001,
		InboundTag: "HK:20001",
		Enabled:    &enabled,
	}); err != nil {
		t.Fatalf("CreateAreaManagerAssignment: %v", err)
	}

	app := &App{store: sqliteStore}
	user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, AgentIDs: []string{"gz"}}
	agents := app.filterAgentRecordsForAdmin(user, []model.AgentRecord{
		{AgentID: "gz"},
		{AgentID: "hk"},
		{AgentID: "hidden"},
	})
	if len(agents) != 2 || agents[0].AgentID != "gz" || agents[1].AgentID != "hk" {
		t.Fatalf("expected dashboard agent filter to include assignment-scoped agent, got %#v", agents)
	}
	snapshots := app.filterSnapshotsForAdmin(user, []model.AgentSnapshot{
		{AgentID: "gz"},
		{AgentID: "hk"},
		{AgentID: "hidden"},
	})
	if len(snapshots) != 2 || snapshots[0].AgentID != "gz" || snapshots[1].AgentID != "hk" {
		t.Fatalf("expected dashboard snapshot filter to include assignment-scoped agent, got %#v", snapshots)
	}
}
