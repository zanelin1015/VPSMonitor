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

func TestBuildClientExpirySyncActionUsesStartTimeAsPeriodStart(t *testing.T) {
	start := time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC)            // 2026-04-22 00:00:00 Asia/Shanghai.
	expectedExpiry := time.Date(2026, 7, 21, 15, 59, 59, 0, time.UTC) // 2026-07-21 23:59:59 Asia/Shanghai.
	action, ok := buildClientExpirySyncAction("agent-1", model.XUIClientBillingConfig{
		InboundID:       20001,
		InboundTag:      "inbound-20001",
		Email:           "alice@example.com",
		RevenueCycle:    "quarter",
		StartTime:       start.UnixMilli(),
		ExpireCycle:     "quarter",
		ExpireAutoRenew: false,
	}, []model.XUIClientView{{
		InboundID:  20001,
		InboundTag: "inbound-20001",
		Email:      "alice@example.com",
	}}, time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), "test")
	if !ok {
		t.Fatal("expected sync action")
	}
	if got := action.Payload["expiry_time"]; got != expectedExpiry.UnixMilli() {
		t.Fatalf("expected expiry %d, got %#v", expectedExpiry.UnixMilli(), got)
	}
	if got := action.Payload["start_time"]; got != start.UnixMilli() {
		t.Fatalf("expected start_time to remain unchanged, got %#v", got)
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
