package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistClientRegistrationPreservesIdentityAndSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(path, []byte(`{"agent_id":"HK_Node.01","registration_token":"bootstrap","poll_interval":"45s","custom":{"enabled":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PersistClientRegistration(path, "HK_Node.01", "issued-token"); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadClientConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentID != "HK_Node.01" || cfg.AgentToken != "issued-token" || cfg.RegistrationToken != "" || cfg.PollInterval != "45s" || cfg.ConfigPath != path {
		t.Fatalf("persisted config lost identity or settings: %+v", cfg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	var custom struct{ Enabled bool }
	if err := json.Unmarshal(payload["custom"], &custom); err != nil || !custom.Enabled {
		t.Fatalf("custom settings were not preserved: err=%v", err)
	}
}

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

func TestLoadClientConfigRejectsUnsafeAgentID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(path, []byte(`{"agent_id":"node/one","registration_token":"token"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := LoadClientConfig(path); err == nil || !strings.Contains(err.Error(), "agent_id must") {
		t.Fatalf("expected invalid agent_id error, got %v", err)
	}
}

func TestLoadServerConfigRequiresTLSCertificateAndKeyTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{"tls_cert_file":"/etc/ssl/server.pem"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadServerConfig(path); err == nil || !strings.Contains(err.Error(), "tls_cert_file") {
		t.Fatalf("expected TLS pair validation error, got %v", err)
	}
}

func TestLoadServerConfigRequiresExplicitSecureTransportChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadServerConfig(path); err == nil || !strings.Contains(err.Error(), "TLS is required") {
		t.Fatalf("expected TLS requirement, got %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"allow_insecure_http":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadServerConfig(path); err != nil {
		t.Fatalf("explicit local HTTP opt-in rejected: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"trusted_proxy_cidrs":["127.0.0.1/32"]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadServerConfig(path); err != nil {
		t.Fatalf("trusted reverse proxy configuration rejected: %v", err)
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
