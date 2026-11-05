package store

import (
	"path/filepath"
	"testing"

	"bridge-core/internal/model"
)

func TestSnellConfigRoundTripsWithAgentConfig(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()
	if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "snell-agent", AgentName: "Snell Agent"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	_, err = store.UpdateAgentConfig("snell-agent", model.ManagedAgentConfig{
		AgentID:   "snell-agent",
		AgentName: "Snell Agent",
		Snell: model.SnellConfig{
			Enabled:  true,
			Name:     "HK Snell",
			Server:   "snell.example.com",
			Port:     443,
			PSK:      "secret-psk",
			Version:  5,
			ObfsMode: "http",
		},
	})
	if err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}
	cfg, found, err := store.GetAgentConfig("snell-agent")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig: found=%v err=%v", found, err)
	}
	if !cfg.Snell.Enabled || cfg.Snell.Server != "snell.example.com" || cfg.Snell.Port != 443 || cfg.Snell.PSK != "secret-psk" || cfg.Snell.ObfsMode != "http" {
		t.Fatalf("Snell config did not round-trip: %#v", cfg.Snell)
	}
}
