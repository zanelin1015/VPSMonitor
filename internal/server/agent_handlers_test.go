package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestHandleRegisterSeedsDefaultXUIBootstrap(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.SaveClientInstallSettings(model.ClientInstallSettingsRequest{
		ServerURL:             "https://panel.example.com",
		InstallScriptURL:      "https://example.com/install.sh",
		PollInterval:          "30s",
		RequestTimeoutSeconds: 15,
		XUIAutoInstall:        true,
		XUIUsername:           "admin",
		XUIPassword:           "secret",
		XUIPanelPort:          2053,
		XUIWebPath:            "/xui/",
		XUIInstallScriptURL:   "https://example.com/3x-ui.sh",
	}); err != nil {
		t.Fatalf("SaveClientInstallSettings: %v", err)
	}

	app := &App{
		config: config.ServerConfig{RegistrationToken: "reg-token"},
		store:  sqliteStore,
	}
	body, err := json.Marshal(model.AgentRegisterRequest{AgentID: "agent-1", AgentName: "Agent 1"})
	if err != nil {
		t.Fatalf("Marshal register body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", bytes.NewReader(body))
	req.Header.Set("X-Registration-Token", "reg-token")
	rec := httptest.NewRecorder()

	app.handleRegister(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleRegister status=%d body=%s", rec.Code, rec.Body.String())
	}

	cfg, found, err := sqliteStore.GetAgentConfig("agent-1")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found {
		t.Fatalf("expected registered agent config")
	}
	if !cfg.XUI.Enabled || !cfg.XUI.AutoInstall {
		t.Fatalf("expected x-ui bootstrap to be enabled: %#v", cfg.XUI)
	}
	if cfg.XUI.BaseURL != "http://127.0.0.1:2053/xui/" || cfg.XUI.Username != "admin" || cfg.XUI.Password != "secret" {
		t.Fatalf("unexpected x-ui bootstrap config: %#v", cfg.XUI)
	}
	if cfg.XUI.DBPath != config.DefaultXUIDBPath {
		t.Fatalf("expected default x-ui db path to be saved in VPS management config, got %#v", cfg.XUI)
	}
	if cfg.XUI.InstallScriptURL != "https://example.com/3x-ui.sh" || cfg.XUI.PanelPort != 2053 || cfg.XUI.WebPath != "/xui/" {
		t.Fatalf("unexpected x-ui bootstrap install settings: %#v", cfg.XUI)
	}
}

func TestHandleRegisterDoesNotSeedLinuxXUIDBPathForWindows(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.SaveClientInstallSettings(model.ClientInstallSettingsRequest{
		ServerURL:             "https://panel.example.com",
		InstallScriptURL:      "https://example.com/install.sh",
		PollInterval:          "30s",
		RequestTimeoutSeconds: 15,
		XUIAutoInstall:        true,
		XUIUsername:           "admin",
		XUIPassword:           "secret",
		XUIPanelPort:          2053,
		XUIWebPath:            "/xui/",
	}); err != nil {
		t.Fatalf("SaveClientInstallSettings: %v", err)
	}

	app := &App{
		config: config.ServerConfig{RegistrationToken: "reg-token"},
		store:  sqliteStore,
	}
	body, err := json.Marshal(model.AgentRegisterRequest{AgentID: "win-agent-1", AgentName: "Win Agent", OS: "windows"})
	if err != nil {
		t.Fatalf("Marshal register body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", bytes.NewReader(body))
	req.Header.Set("X-Registration-Token", "reg-token")
	rec := httptest.NewRecorder()

	app.handleRegister(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleRegister status=%d body=%s", rec.Code, rec.Body.String())
	}

	cfg, found, err := sqliteStore.GetAgentConfig("win-agent-1")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found {
		t.Fatalf("expected registered agent config")
	}
	if cfg.XUI.DBPath != "" {
		t.Fatalf("expected windows x-ui db path to stay empty, got %#v", cfg.XUI)
	}
}

func TestHandleAgentConfigMergesCollectedRealmSnapshotForAdmin(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if err := sqliteStore.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatalf("EnsureAdminAccount: %v", err)
	}
	admin, ok, err := sqliteStore.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin ok=%v err=%v", ok, err)
	}
	token, _, err := sqliteStore.CreateAdminSession(admin, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1", AgentName: "Agent 1"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	now := time.Now().UTC()
	if err := sqliteStore.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "agent-1",
		AgentName:  "Agent 1",
		ReportedAt: now,
		Realm: &model.RealmSnapshot{
			ConfigPath:  "/root/custom-realm.toml",
			CollectedAt: now,
			Rules: []model.RealmForwardRule{{
				Enabled:       true,
				ListenAddress: "0.0.0.0",
				ListenPort:    20001,
				TargetAddress: "47.239.135.242",
				TargetPort:    20001,
				Network:       "tcp",
			}},
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	app := &App{store: sqliteStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/config", nil)
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	app.handleAgentConfig(rec, req, "agent-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("handleAgentConfig status=%d body=%s", rec.Code, rec.Body.String())
	}

	var cfg model.ManagedAgentConfig
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatalf("Decode config: %v", err)
	}
	if !cfg.Entry.PortForwarding.Enabled || len(cfg.Entry.PortForwarding.Rules) != 1 {
		t.Fatalf("expected collected realm rule to be merged into admin config: %#v", cfg.Entry.PortForwarding)
	}
	rule := cfg.Entry.PortForwarding.Rules[0]
	if rule.ListenPort != 20001 || rule.TargetAddress != "47.239.135.242" || rule.TargetPort != 20001 {
		t.Fatalf("unexpected merged realm rule: %#v", rule)
	}
}
