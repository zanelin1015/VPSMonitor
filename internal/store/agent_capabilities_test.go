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

func TestRegisterAgentHAProxyCapabilityEnablesFeature(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	response, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:      "haproxy-capable",
		AgentName:    "HAProxy Capable",
		Capabilities: model.AgentCapabilities{HAProxy: true},
	})
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if !response.Config.Features.HAProxy {
		t.Fatalf("expected HAProxy capability to enable the feature: %#v", response.Config.Features)
	}
	if response.Config.Entry.HAProxy.Enabled || len(response.Config.Entry.HAProxy.Rules) != 0 {
		t.Fatalf("capability discovery must not create HAProxy rules: %#v", response.Config.Entry.HAProxy)
	}
}

func TestRegisterAgentHAProxyCapabilityRespectsPersistedAdminDisable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "existing-haproxy", Capabilities: model.AgentCapabilities{HAProxy: true}}); err != nil {
		t.Fatalf("RegisterAgent initial: %v", err)
	}
	cfg, found, err := store.GetAgentConfig("existing-haproxy")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig: found=%v err=%v", found, err)
	}
	cfg.Features.HAProxy = false
	cfg.Features.Configured = true
	cfg.Features.HAProxyExplicitlyConfigured = true
	if _, err := store.UpdateAgentConfigWithActor("existing-haproxy", cfg, "admin"); err != nil {
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
	response, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "existing-haproxy", Capabilities: model.AgentCapabilities{HAProxy: true}})
	if err != nil {
		t.Fatalf("RegisterAgent after disable: %v", err)
	}
	if response.Config.Features.HAProxy {
		t.Fatal("expected an explicit admin disable to survive later HAProxy capability reports")
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
