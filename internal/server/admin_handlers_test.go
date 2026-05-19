package server

import (
	"path/filepath"
	"testing"

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
