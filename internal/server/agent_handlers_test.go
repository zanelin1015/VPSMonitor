package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestValidateRealmForwardTargetsRejectsUnresolvableDomain(t *testing.T) {
	originalLookup := realmForwardLookupHost
	realmForwardLookupHost = func(ctx context.Context, host string) ([]string, error) {
		if host == "hkq1.zanelin.top" {
			return []string{"8.217.202.247"}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host}
	}
	t.Cleanup(func() {
		realmForwardLookupHost = originalLookup
	})

	err := validateRealmForwardTargets(context.Background(), model.RealmForwardConfig{
		Enabled: true,
		Backend: "realm",
		Rules: []model.RealmForwardRule{{
			Enabled:       true,
			Name:          "HK typo",
			ListenPort:    20002,
			TargetAddress: "hkq1.zanellin.top",
			TargetPort:    20002,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "无法解析") {
		t.Fatalf("expected DNS validation error, got %v", err)
	}

	err = validateRealmForwardTargets(context.Background(), model.RealmForwardConfig{
		Enabled: true,
		Backend: "realm",
		Rules: []model.RealmForwardRule{{
			Enabled:       true,
			Name:          "HK",
			ListenPort:    20002,
			TargetAddress: "hkq1.zanelin.top",
			TargetPort:    20002,
		}},
	})
	if err != nil {
		t.Fatalf("expected valid target to pass, got %v", err)
	}
}

func TestHandleRegisterDoesNotSeedDefaultXUIBootstrap(t *testing.T) {
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
	if cfg.XUI.Enabled || cfg.XUI.AutoInstall || cfg.XUI.BaseURL != "" || cfg.XUI.Username != "" || cfg.XUI.Password != "" {
		t.Fatalf("expected x-ui bootstrap seed to stay disabled, got %#v", cfg.XUI)
	}
	if cfg.XUI.DBPath != "" || cfg.XUI.InstallScriptURL != "" || cfg.XUI.PanelPort != 0 || cfg.XUI.WebPath != "" {
		t.Fatalf("expected x-ui bootstrap install settings to stay empty, got %#v", cfg.XUI)
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

func TestHandleHeartbeatUsesServerReceiveTime(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	registerResp, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "drifted-agent", AgentName: "Drifted Agent"})
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	app := &App{store: sqliteStore}

	clientTime := time.Now().UTC().Add(-30 * time.Minute)
	body, err := json.Marshal(model.AgentSnapshot{
		AgentID:    "drifted-agent",
		AgentName:  "Drifted Agent",
		ReportedAt: clientTime,
		Summary: model.VPSSummary{
			Hostname: "drifted-host",
		},
	})
	if err != nil {
		t.Fatalf("Marshal heartbeat body: %v", err)
	}
	before := time.Now().UTC().Add(-time.Second)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/drifted-agent/heartbeat", bytes.NewReader(body))
	req.Header.Set("X-Agent-Token", registerResp.AgentToken)
	rec := httptest.NewRecorder()

	app.handleHeartbeat(rec, req, "drifted-agent")
	if rec.Code != http.StatusOK {
		t.Fatalf("handleHeartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}
	after := time.Now().UTC().Add(time.Second)

	latest, found := sqliteStore.GetLatest("drifted-agent")
	if !found {
		t.Fatalf("expected latest snapshot")
	}
	if latest.ReportedAt.Before(before) || latest.ReportedAt.After(after) {
		t.Fatalf("expected server receive time between %s and %s, got %s", before, after, latest.ReportedAt)
	}
	if latest.ReportedAt.Equal(clientTime) {
		t.Fatalf("expected client reported time to be ignored")
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
	if !cfg.Features.Realm {
		t.Fatalf("expected legacy realm snapshot to enable realm feature switch: %#v", cfg.Features)
	}
	rule := cfg.Entry.PortForwarding.Rules[0]
	if rule.ListenPort != 20001 || rule.TargetAddress != "47.239.135.242" || rule.TargetPort != 20001 {
		t.Fatalf("unexpected merged realm rule: %#v", rule)
	}
}

func TestHandleAgentConfigPreservesExplicitFeatureSwitches(t *testing.T) {
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

	body, err := json.Marshal(model.ManagedAgentConfig{
		AgentID: "agent-1",
		Features: model.AgentFeatureConfig{
			XUI:        false,
			Realm:      false,
			NAT:        false,
			PortPolicy: false,
			Configured: true,
		},
	})
	if err != nil {
		t.Fatalf("Marshal config: %v", err)
	}
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/agent-1/config", bytes.NewReader(body))
	putReq.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	putRec := httptest.NewRecorder()
	app := &App{store: sqliteStore}
	app.handleAgentConfig(putRec, putReq, "agent-1")
	if putRec.Code != http.StatusOK {
		t.Fatalf("handleAgentConfig put status=%d body=%s", putRec.Code, putRec.Body.String())
	}
	storedConfig, found, err := sqliteStore.GetAgentConfig("agent-1")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig after put: found=%v err=%v", found, err)
	}
	if !storedConfig.Features.RealmExplicitlyConfigured {
		t.Fatal("expected the admin Realm switch choice to be persisted")
	}
	registerResponse, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{
		AgentID:      "agent-1",
		AgentName:    "Agent 1",
		Capabilities: model.AgentCapabilities{Realm: true},
	})
	if err != nil {
		t.Fatalf("RegisterAgent capability after admin disable: %v", err)
	}
	if registerResponse.Config.Features.Realm {
		t.Fatal("expected Realm capability discovery to preserve the admin disable")
	}

	now := time.Now().UTC()
	if err := sqliteStore.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "agent-1",
		AgentName:  "Agent 1",
		ReportedAt: now,
		Summary:    model.VPSSummary{InboundCount: 1, OutboundCount: 1},
		XUI: &model.XUISnapshot{
			BaseURL:     "http://127.0.0.1:2053",
			CollectedAt: now,
			Inbounds:    []map[string]any{{"id": 1, "remark": "node"}},
		},
		Realm: &model.RealmSnapshot{
			ConfigPath:  "/etc/realm/config.toml",
			CollectedAt: now,
			Rules:       []model.RealmForwardRule{{Enabled: true, ListenPort: 20001, TargetAddress: "1.1.1.1", TargetPort: 20001}},
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/config", nil)
	getReq.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	getRec := httptest.NewRecorder()
	app.handleAgentConfig(getRec, getReq, "agent-1")
	if getRec.Code != http.StatusOK {
		t.Fatalf("handleAgentConfig get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var cfg model.ManagedAgentConfig
	if err := json.NewDecoder(getRec.Body).Decode(&cfg); err != nil {
		t.Fatalf("Decode config: %v", err)
	}
	if cfg.Features.XUI || cfg.Features.Realm || cfg.Features.NAT || cfg.Features.PortPolicy {
		t.Fatalf("expected explicit disabled feature switches to be preserved, got %#v", cfg.Features)
	}
}

func TestHandleXUIActionsScopesAreaManagerLogsToCurrentAccount(t *testing.T) {
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
	adminToken, _, err := sqliteStore.CreateAdminSession(admin, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession admin: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "shared-agent", AgentName: "Shared Agent"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	enabled := true
	managerOne, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-one", Password: "password123", Enabled: &enabled, AgentIDs: []string{"shared-agent"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager one: %v", err)
	}
	managerTwo, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-two", Password: "password123", Enabled: &enabled, AgentIDs: []string{"shared-agent"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager two: %v", err)
	}
	for _, managerID := range []int64{managerOne.ID, managerTwo.ID} {
		if _, err := sqliteStore.CreateAreaManagerAssignment(managerID, model.AreaManagerAssignmentRequest{
			AgentID: "shared-agent", InboundID: 7, InboundTag: "node-7", Enabled: &enabled,
		}); err != nil {
			t.Fatalf("CreateAreaManagerAssignment manager=%d: %v", managerID, err)
		}
	}
	oneUser, ok, err := sqliteStore.AuthenticateAdmin("area-one", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin area-one ok=%v err=%v", ok, err)
	}
	twoUser, ok, err := sqliteStore.AuthenticateAdmin("area-two", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin area-two ok=%v err=%v", ok, err)
	}
	oneToken, _, err := sqliteStore.CreateAdminSession(oneUser, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession area-one: %v", err)
	}
	twoToken, _, err := sqliteStore.CreateAdminSession(twoUser, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession area-two: %v", err)
	}

	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	createAction := func(token, email string) {
		t.Helper()
		body, marshalErr := json.Marshal(model.XUIActionRequest{
			Kind: model.XUIActionAddClient,
			Payload: map[string]any{
				"inbound_id": 7, "inbound_tag": "node-7", "client": map[string]any{"email": email},
			},
		})
		if marshalErr != nil {
			t.Fatalf("Marshal action: %v", marshalErr)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/shared-agent/xui/actions", bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		app.handleXUIActions(rec, req, "shared-agent", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("create action status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	createAction(oneToken, "one@example.com")
	createAction(twoToken, "two@example.com")
	if _, err := sqliteStore.CreateXUIAction("shared-agent", model.XUIActionRequest{
		Kind: model.XUIActionRestartXUI, Payload: map[string]any{},
	}); err != nil {
		t.Fatalf("CreateXUIAction legacy: %v", err)
	}

	listActions := func(token string) []model.XUIAction {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/shared-agent/xui/actions", nil)
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		app.handleXUIActions(rec, req, "shared-agent", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("list actions status=%d body=%s", rec.Code, rec.Body.String())
		}
		var actions []model.XUIAction
		if err := json.NewDecoder(rec.Body).Decode(&actions); err != nil {
			t.Fatalf("Decode actions: %v", err)
		}
		return actions
	}
	oneActions := listActions(oneToken)
	if len(oneActions) != 1 || oneActions[0].CreatedByAccountID != managerOne.ID || oneActions[0].CreatedByUsername != "area-one" {
		t.Fatalf("area-one must only see its own action, got %#v", oneActions)
	}
	twoActions := listActions(twoToken)
	if len(twoActions) != 1 || twoActions[0].CreatedByAccountID != managerTwo.ID || twoActions[0].CreatedByUsername != "area-two" {
		t.Fatalf("area-two must only see its own action, got %#v", twoActions)
	}
	adminActions := listActions(adminToken)
	if len(adminActions) != 3 {
		t.Fatalf("admin must see all actions, got %#v", adminActions)
	}
}

func TestHandleAgentConfigUpdateRequestsImmediateClientApply(t *testing.T) {
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
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz", AgentName: "Guangzhou"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	controlSession := app.realtime.registerAgentControl("gz")
	defer app.realtime.unregisterAgentControl("gz", controlSession)

	body, err := json.Marshal(model.ManagedAgentConfig{
		AgentID:   "gz",
		AgentName: "Guangzhou",
		XUI: config.XUIConfig{
			Enabled:          true,
			AutoInstall:      true,
			InstallScriptURL: "https://example.com/3x-ui.sh",
			PanelPort:        2053,
			WebPath:          "/xui/",
		},
		Entry: model.AgentEntryConfig{
			PortForwarding: model.RealmForwardConfig{
				Enabled:     true,
				Backend:     "realm",
				ConfigPath:  "/etc/realm/config.toml",
				ServiceName: "realm",
				Rules: []model.RealmForwardRule{{
					Enabled:       true,
					ListenPort:    20003,
					TargetAddress: "47.239.135.242",
					TargetPort:    20003,
					Network:       "both",
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal config: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/gz/config", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	rec := httptest.NewRecorder()

	app.handleAgentConfig(rec, req, "gz")
	if rec.Code != http.StatusOK {
		t.Fatalf("handleAgentConfig status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved model.ManagedAgentConfig
	if err := json.NewDecoder(rec.Body).Decode(&saved); err != nil {
		t.Fatalf("Decode config: %v", err)
	}
	if saved.XUI.AutoInstall || saved.XUI.InstallScriptURL != "" || saved.XUI.PanelPort != 0 || saved.XUI.WebPath != "" {
		t.Fatalf("expected x-ui auto install fields to be disabled on save, got %#v", saved.XUI)
	}
	select {
	case message := <-controlSession.ch:
		if message.Type != model.AgentControlApplyConfig || message.Config == nil {
			t.Fatalf("expected apply_config control message with config, got %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("expected config update to request immediate client apply")
	}
}

func TestHandleRealmConfigCopyCopiesAndAppliesToTarget(t *testing.T) {
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
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz", AgentName: "Guangzhou"}); err != nil {
		t.Fatalf("RegisterAgent source: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "backup", AgentName: "Backup"}); err != nil {
		t.Fatalf("RegisterAgent target: %v", err)
	}
	if _, err := sqliteStore.UpdateAgentConfig("backup", model.ManagedAgentConfig{
		AgentID:  "backup",
		Features: model.AgentFeatureConfig{HAProxy: true, Configured: true},
		Entry:    model.AgentEntryConfig{HAProxy: model.HAProxyConfig{Enabled: true}},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig target HAProxy: %v", err)
	}
	if _, err := sqliteStore.UpdateAgentConfig("gz", model.ManagedAgentConfig{
		AgentID:   "gz",
		AgentName: "Guangzhou",
		Entry: model.AgentEntryConfig{
			PortForwarding: model.RealmForwardConfig{
				Enabled:     true,
				Backend:     "realm",
				ConfigPath:  "/etc/realm/config.toml",
				ServiceName: "realm",
				Rules: []model.RealmForwardRule{{
					Enabled:       true,
					ListenAddress: "0.0.0.0",
					ListenPort:    20001,
					TargetAddress: "47.239.135.242",
					TargetPort:    20001,
					Network:       "both",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig source: %v", err)
	}

	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	controlSession := app.realtime.registerAgentControl("backup")
	defer app.realtime.unregisterAgentControl("backup", controlSession)

	body, err := json.Marshal(map[string]string{"target_agent_id": "backup"})
	if err != nil {
		t.Fatalf("Marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/gz/realm/copy", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	app.handleAgentByID(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleRealmConfigCopy status=%d body=%s", rec.Code, rec.Body.String())
	}
	targetCfg, found, err := sqliteStore.GetAgentConfig("backup")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig target found=%v err=%v", found, err)
	}
	forwarding := targetCfg.Entry.PortForwarding
	if !forwarding.Enabled || forwarding.ConfigPath != "/etc/realm/config.toml" || forwarding.ServiceName != "realm" || len(forwarding.Rules) != 1 {
		t.Fatalf("expected copied realm forwarding config, got %#v", forwarding)
	}
	if !targetCfg.Features.Realm {
		t.Fatalf("expected copied config to enable realm feature, got %#v", targetCfg.Features)
	}
	if targetCfg.Features.HAProxy || targetCfg.Entry.HAProxy.Enabled {
		t.Fatalf("copying Realm must disable HAProxy on the target, got features=%#v entry=%#v", targetCfg.Features, targetCfg.Entry.HAProxy)
	}
	select {
	case message := <-controlSession.ch:
		if message.Type != model.AgentControlApplyConfig || message.Config == nil {
			t.Fatalf("expected apply_config control message with config, got %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("expected realm copy to request immediate client apply")
	}
}

func TestSyncRealmConfigFromSnapshotPersistsMachineRealmConfig(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz", AgentName: "Guangzhou"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	now := time.Now().UTC()
	app := &App{store: sqliteStore}
	app.syncRealmConfigFromSnapshot("gz", &model.RealmSnapshot{
		ConfigPath:  "/etc/realm/config.toml",
		ServiceName: "realm",
		CollectedAt: now,
		Rules: []model.RealmForwardRule{{
			Enabled:       true,
			ListenAddress: "0.0.0.0",
			ListenPort:    20001,
			TargetAddress: "47.239.135.242",
			TargetPort:    20001,
			Network:       "tcp",
		}},
	})

	cfg, found, err := sqliteStore.GetAgentConfig("gz")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig found=%v err=%v", found, err)
	}
	forwarding := cfg.Entry.PortForwarding
	if !forwarding.Enabled || forwarding.ConfigPath != "/etc/realm/config.toml" || forwarding.ServiceName != "realm" || len(forwarding.Rules) != 1 {
		t.Fatalf("expected collected realm config to be persisted, got %#v", forwarding)
	}
	rule := forwarding.Rules[0]
	if rule.ListenPort != 20001 || rule.TargetAddress != "47.239.135.242" || rule.TargetPort != 20001 {
		t.Fatalf("unexpected persisted realm rule: %#v", rule)
	}
}

func TestSyncRealmConfigFromSnapshotIgnoresHAProxyClient(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz", AgentName: "Guangzhou"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	cfg, found, err := sqliteStore.GetAgentConfig("gz")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig found=%v err=%v", found, err)
	}
	cfg.Features.HAProxy = true
	cfg.Features.Configured = true
	cfg.Entry.HAProxy.Enabled = true
	cfg.Entry.PortForwarding.Backend = "none"
	if _, err := sqliteStore.UpdateAgentConfig("gz", cfg); err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}

	app := &App{store: sqliteStore}
	app.syncRealmConfigFromSnapshot("gz", &model.RealmSnapshot{Rules: []model.RealmForwardRule{{
		Enabled: true, ListenPort: 20001, TargetAddress: "192.0.2.20", TargetPort: 20001,
	}}})

	saved, found, err := sqliteStore.GetAgentConfig("gz")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig after sync found=%v err=%v", found, err)
	}
	if saved.Entry.PortForwarding.Enabled || saved.Entry.PortForwarding.Backend != "none" || len(saved.Entry.PortForwarding.Rules) != 0 {
		t.Fatalf("local Realm discovery must not re-enable Realm while HAProxy is selected: %#v", saved.Entry.PortForwarding)
	}
}

func TestAppendRealmForwardedImportURLsScopesAdminExportToEntryAgent(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz", AgentName: "Guangzhou"}); err != nil {
		t.Fatalf("RegisterAgent gz: %v", err)
	}
	if _, err := sqliteStore.UpdateAgentConfig("gz", model.ManagedAgentConfig{
		AgentID:   "gz",
		AgentName: "Guangzhou",
		Entry: model.AgentEntryConfig{
			ImportDomain: "gz.example.com",
			PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{{
				Enabled:       true,
				ListenAddress: "0.0.0.0",
				ListenPort:    20001,
				TargetAgentID: "hk",
				TargetAddress: "47.239.135.242",
				TargetPort:    20001,
				Network:       "tcp",
			}}},
		},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig gz: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "hk", AgentName: "Hong Kong"}); err != nil {
		t.Fatalf("RegisterAgent hk: %v", err)
	}
	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "sz", AgentName: "Shenzhen"}); err != nil {
		t.Fatalf("RegisterAgent sz: %v", err)
	}
	if _, err := sqliteStore.UpdateAgentConfig("sz", model.ManagedAgentConfig{
		AgentID:   "sz",
		AgentName: "Shenzhen",
		Entry: model.AgentEntryConfig{
			ImportDomain: "sz.example.com",
			PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{{
				Enabled:       true,
				ListenAddress: "0.0.0.0",
				ListenPort:    30001,
				TargetAgentID: "hk",
				TargetAddress: "47.239.135.242",
				TargetPort:    20001,
				Network:       "tcp",
			}}},
		},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig sz: %v", err)
	}

	now := time.Now().UTC()
	if err := sqliteStore.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "gz",
		AgentName:  "Guangzhou",
		ReportedAt: now,
		Summary:    model.VPSSummary{PublicIPv4: "1.1.1.1", ObservedIP: "1.1.1.1"},
	}); err != nil {
		t.Fatalf("SaveSnapshot gz: %v", err)
	}
	if err := sqliteStore.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "hk",
		AgentName:  "Hong Kong",
		ReportedAt: now,
		Summary:    model.VPSSummary{PublicIPv4: "47.239.135.242"},
		XUI: &model.XUISnapshot{
			CollectedAt: now,
			Inbounds: []map[string]any{{
				"id":       1001,
				"tag":      "inbound-20001",
				"remark":   "HK VLESS",
				"protocol": "vless",
				"port":     20001,
				"settings": `{"clients":[{"email":"alice@example.com","enable":true,"id":"11111111-1111-1111-1111-111111111111"}]}`,
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "tls",
					"tlsSettings": map[string]any{
						"serverName": "hk.example.com",
					},
				},
			}},
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot hk: %v", err)
	}

	app := &App{store: sqliteStore}
	overview := emptyAgentXUIOverview(model.AgentSnapshot{
		AgentID:    "gz",
		AgentName:  "Guangzhou",
		ReportedAt: now,
		Summary:    model.VPSSummary{PublicIPv4: "1.1.1.1", ObservedIP: "1.1.1.1"},
	}, model.ManagedAgentConfig{AgentID: "gz", AgentName: "Guangzhou"})
	app.appendForwardedImportURLs("gz", overview)

	parsed, err := url.Parse(overview.Clients[0].ImportURL)
	if err != nil {
		t.Fatalf("parse import url: %v", err)
	}
	if parsed.Host != "gz.example.com:20001" {
		t.Fatalf("expected Guangzhou realm entry host, got %q from %q", parsed.Host, overview.Clients[0].ImportURL)
	}
	if parsed.Query().Get("sni") != "hk.example.com" {
		t.Fatalf("expected HK stream parameters to stay, got %q", overview.Clients[0].ImportURL)
	}

	hkOverview := dashboard.BuildXUIOverviewWithOptions(model.AgentSnapshot{
		AgentID:    "hk",
		AgentName:  "Hong Kong",
		ReportedAt: now,
		Summary:    model.VPSSummary{PublicIPv4: "47.239.135.242"},
		XUI: &model.XUISnapshot{
			CollectedAt: now,
			Inbounds: []map[string]any{{
				"id":       1001,
				"tag":      "inbound-20001",
				"remark":   "HK VLESS",
				"protocol": "vless",
				"port":     20001,
				"settings": `{"clients":[{"email":"alice@example.com","enable":true,"id":"11111111-1111-1111-1111-111111111111"}]}`,
			}},
		},
	}, dashboard.XUIOverviewOptions{})
	if hkOverview == nil || len(hkOverview.Clients) != 1 {
		t.Fatalf("expected hk overview client")
	}
	app.appendForwardedImportURLs("hk", hkOverview)
	parsedHK, err := url.Parse(hkOverview.Clients[0].ImportURL)
	if err != nil {
		t.Fatalf("parse hk import url: %v", err)
	}
	if parsedHK.Host == "gz.example.com:20001" || parsedHK.Host == "sz.example.com:30001" {
		t.Fatalf("HK page should not arbitrarily export through GZ or SZ, got %q", hkOverview.Clients[0].ImportURL)
	}
}

func TestPendingTopologyLookupValuesIncludesUncachedOutboundIP(t *testing.T) {
	now := time.Now().UTC()
	cache := model.TopologyLookupCache{
		Geos: map[string]model.TopologyGeoCacheEntry{
			"47.239.135.242": {
				Geo:       model.IPGeoView{IP: "47.239.135.242", CountryCode: "HK", CountryName: "Hong Kong"},
				UpdatedAt: now,
				ExpiresAt: now.Add(time.Hour),
			},
		},
	}
	values := pendingTopologyLookupValues(cache, []string{
		"47.239.135.242",
		"123.30.235.76:54401",
		"https://123.30.235.76:54401/path",
	})
	if len(values) != 1 || values[0] != "123.30.235.76" {
		t.Fatalf("expected only uncached VN outbound IP to be refreshed, got %#v", values)
	}
}
