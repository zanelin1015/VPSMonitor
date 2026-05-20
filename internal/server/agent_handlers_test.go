package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
