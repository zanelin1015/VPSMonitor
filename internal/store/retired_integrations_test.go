package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestRetiredClientInstallCredentialsAreIgnored(t *testing.T) {
	s, _ := customerSessionFixture(t)
	const legacy = `{"server_url":"https://monitor.example","poll_interval":"45s","realm_version":"v2.9.4","xui_auto_install":true,"xui_username":"old-admin","xui_password":"obsolete-credential","xui_panel_port":2053,"xui_web_path":"/old/","xui_install_script_url":"https://old.example/install.sh"}`
	if _, err := s.db.Exec(`INSERT INTO app_settings(key,value_json,updated_at) VALUES(?,?,?)`, clientInstallSettingsKey, legacy, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	settings, found, err := s.GetClientInstallSettings()
	if err != nil || !found || settings.ServerURL != "https://monitor.example" || settings.XUIAutoInstall {
		t.Fatalf("legacy settings did not load safely: %#v %v", settings, err)
	}
	if err := s.init(); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key=?`, clientInstallSettingsKey).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{"xui_username", "xui_password", "xui_panel_port", "xui_web_path", "xui_install_script_url", "obsolete-credential"} {
		if strings.Contains(raw, retired) {
			t.Fatalf("retired setting was persisted: %s", retired)
		}
	}
	if _, err := s.SaveClientInstallSettings(settings); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredNezhaColumnsRemainUntouched(t *testing.T) {
	s, _ := customerSessionFixture(t)
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "legacy-agent", AgentName: "Legacy"}); err != nil {
		t.Fatal(err)
	}
	const legacyConfig = `{"username":"historical-value"}`
	if _, err := s.db.Exec(`UPDATE agents SET nezha_config_json=? WHERE agent_id=?`, legacyConfig, "legacy-agent"); err != nil {
		t.Fatal(err)
	}
	cfg, found, err := s.GetAgentConfig("legacy-agent")
	if err != nil || !found {
		t.Fatalf("load config: found=%v err=%v", found, err)
	}
	cfg.Tags = []string{"active"}
	if _, err := s.UpdateAgentConfig("legacy-agent", cfg); err != nil {
		t.Fatal(err)
	}
	var retained string
	if err := s.db.QueryRow(`SELECT nezha_config_json FROM agents WHERE agent_id=?`, "legacy-agent").Scan(&retained); err != nil || retained != legacyConfig {
		t.Fatalf("retired config column changed: %q %v", retained, err)
	}
	var snapshot model.AgentSnapshot
	if err := json.Unmarshal([]byte(`{"agent_id":"legacy-agent","nezha":{"server_name":"old-name"},"summary":{"cpu":12,"nezha_server_id":7,"nezha_server_name":"old-name"}}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	history, err := s.ListHistory("legacy-agent", 1)
	if err != nil || len(history) != 1 || history[0].Summary.CPU != 12 {
		t.Fatalf("active metrics were not preserved: %#v %v", history, err)
	}
	encoded, err := json.Marshal(history)
	if err != nil || strings.Contains(string(encoded), "nezha") {
		t.Fatalf("retired integration leaked into API data: %s %v", encoded, err)
	}
	if !sqliteColumnExists(t, s.db, "snapshots", "nezha_server_id") {
		t.Fatal("historical database columns must remain available")
	}
}
