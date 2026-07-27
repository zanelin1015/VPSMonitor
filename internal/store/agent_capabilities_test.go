package store

import (
	"path/filepath"
	"testing"

	"bridge-core/internal/model"
)

func TestRegisterAgentRealmCapabilityEnablesFeatureWithoutCreatingRules(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	response, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:      "realm-capable",
		AgentName:    "Realm Capable",
		Capabilities: model.AgentCapabilities{Realm: true},
	})
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if !response.Config.Features.Realm {
		t.Fatalf("expected Realm capability to enable the feature: %#v", response.Config.Features)
	}
	forwarding := response.Config.Entry.PortForwarding
	if forwarding.Enabled || len(forwarding.Rules) != 0 {
		t.Fatalf("capability discovery must not create or start Realm forwarding: %#v", forwarding)
	}
}

func TestRegisterAgentRealmCapabilityRespectsPersistedAdminDisable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "existing-realm", AgentName: "Existing Realm"}); err != nil {
		t.Fatalf("RegisterAgent initial: %v", err)
	}
	response, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:      "existing-realm",
		AgentName:    "Existing Realm",
		Capabilities: model.AgentCapabilities{Realm: true},
	})
	if err != nil {
		t.Fatalf("RegisterAgent capability: %v", err)
	}
	if !response.Config.Features.Realm {
		t.Fatal("expected an existing Client to gain Realm after its first capability report")
	}

	cfg, found, err := store.GetAgentConfig("existing-realm")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig: found=%v err=%v", found, err)
	}
	cfg.Features.Realm = false
	cfg.Features.Configured = true
	cfg.Features.RealmExplicitlyConfigured = true
	if _, err := store.UpdateAgentConfigWithActor("existing-realm", cfg, "admin"); err != nil {
		t.Fatalf("UpdateAgentConfigWithActor: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store, err = NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen NewSQLiteStore: %v", err)
	}
	defer store.Close()
	response, err = store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:      "existing-realm",
		AgentName:    "Existing Realm",
		Capabilities: model.AgentCapabilities{Realm: true},
	})
	if err != nil {
		t.Fatalf("RegisterAgent after disable: %v", err)
	}
	if response.Config.Features.Realm {
		t.Fatal("expected an explicit admin disable to survive later capability reports")
	}
}
