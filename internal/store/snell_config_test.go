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

func TestSnellConfigListRoundTripsWithAgentConfig(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()
	if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "snell-list-agent", AgentName: "Snell List Agent"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	_, err = store.UpdateAgentConfig("snell-list-agent", model.ManagedAgentConfig{
		AgentID:   "snell-list-agent",
		AgentName: "Snell List Agent",
		Snells: []model.SnellConfig{
			{Enabled: true, Name: "CM-HK-COX", Remark: "香港主节点", Server: "hk.example.com", Port: 27412, PSK: "secret-1", Version: 5},
			{Enabled: true, Name: "CM-US-COX", Remark: "美国备用", Server: "us.example.com", Port: 27413, PSK: "secret-2", Version: 5},
		},
	})
	if err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}
	cfg, found, err := store.GetAgentConfig("snell-list-agent")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig: found=%v err=%v", found, err)
	}
	if len(cfg.Snells) != 2 || cfg.Snells[0].Name != "CM-HK-COX" || cfg.Snells[1].Remark != "美国备用" || !cfg.Snells[0].UDP {
		t.Fatalf("Snell config list did not round-trip: %#v", cfg.Snells)
	}
}
