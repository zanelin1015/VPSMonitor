package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClientConfigUsesSanitizedAgentIDEnv(t *testing.T) {
	t.Setenv("VPSMONITOR_AGENT_ID", " HK Node #1 ")
	path := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(path, []byte(`{"registration_token":"token","poll_interval":"30s"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, _, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig() error = %v", err)
	}
	if cfg.AgentID != "hk-node-1" {
		t.Fatalf("AgentID = %q, want hk-node-1", cfg.AgentID)
	}
	if cfg.AgentIDGenerated {
		t.Fatalf("AgentIDGenerated = true, want false for env override")
	}
}

func TestPersistClientAgentIDIfMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(path, []byte(`{"registration_token":"token","agent_token":""}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := PersistClientAgentIDIfMissing(path, " GZ Node 01 "); err != nil {
		t.Fatalf("PersistClientAgentIDIfMissing() error = %v", err)
	}

	cfg, _, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig() error = %v", err)
	}
	if cfg.AgentID != "gz-node-01" {
		t.Fatalf("AgentID = %q, want gz-node-01", cfg.AgentID)
	}
}

func TestSanitizeClientAgentID(t *testing.T) {
	got := sanitizeClientAgentID("  My VPS@HK__01  ")
	if got != "my-vps-hk-01" {
		t.Fatalf("sanitizeClientAgentID() = %q, want my-vps-hk-01", got)
	}
}

func TestDefaultXUIDBPathForOS(t *testing.T) {
	if got := DefaultXUIDBPathForOS("linux"); got != DefaultXUIDBPath {
		t.Fatalf("DefaultXUIDBPathForOS(linux) = %q, want %q", got, DefaultXUIDBPath)
	}
	if got := DefaultXUIDBPathForOS("windows"); got != "" {
		t.Fatalf("DefaultXUIDBPathForOS(windows) = %q, want empty", got)
	}
}
