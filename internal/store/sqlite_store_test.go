package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func TestSQLiteStoreSaveAndReload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	snapshot := model.AgentSnapshot{
		AgentID:    "hk-01",
		AgentName:  "HK-01",
		ReportedAt: time.Now().UTC(),
		Summary: model.VPSSummary{
			PublicIPv4: "1.1.1.1",
			MemUsed:    256,
			MemTotal:   1024,
		},
	}

	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	got, ok := store.GetLatest("hk-01")
	if !ok {
		t.Fatalf("expected latest snapshot")
	}
	if got.Summary.PublicIPv4 != "1.1.1.1" {
		t.Fatalf("unexpected ipv4: %s", got.Summary.PublicIPv4)
	}

	history, err := store.ListHistory("hk-01", 10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(history))
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reload sqlite store: %v", err)
	}
	defer reloaded.Close()

	reloadedSnapshot, ok := reloaded.GetLatest("hk-01")
	if !ok {
		t.Fatalf("expected snapshot after reload")
	}
	if reloadedSnapshot.Summary.MemTotal != 1024 {
		t.Fatalf("unexpected mem total after reload: %d", reloadedSnapshot.Summary.MemTotal)
	}
}

func TestSQLiteStoreRegisterAndUpdateConfig(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	registerResp, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "jp-01",
		AgentName: "Japan 01",
		Hostname:  "jp-vps-01",
		SeedConfig: model.ManagedAgentConfig{
			AgentID:   "jp-01",
			AgentName: "Japan 01",
			Tags:      []string{"asia", "jp"},
			XUI: config.XUIConfig{
				Enabled:       true,
				BaseURL:       "https://127.0.0.1:2053",
				Username:      "admin",
				Password:      "bootstrap-pass",
				SkipTLSVerify: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if registerResp.AgentToken == "" {
		t.Fatalf("expected agent token")
	}
	if !store.ValidateAgentToken("jp-01", registerResp.AgentToken) {
		t.Fatalf("expected generated token to validate")
	}
	if registerResp.Config.XUI.BaseURL != "https://127.0.0.1:2053" {
		t.Fatalf("unexpected seeded x-ui base url: %s", registerResp.Config.XUI.BaseURL)
	}
	if len(registerResp.Config.Tags) != 2 {
		t.Fatalf("expected seeded tags, got %#v", registerResp.Config.Tags)
	}

	cfg, found, err := store.GetAgentConfig("jp-01")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found {
		t.Fatalf("expected registered agent config")
	}
	if !cfg.XUI.Enabled {
		t.Fatalf("expected seeded x-ui config to remain enabled")
	}

	updated, err := store.UpdateAgentConfig("jp-01", model.ManagedAgentConfig{
		AgentID:   "jp-01",
		AgentName: "Japan Relay 01",
		Tags:      []string{"relay", "asia", "relay"},
		XUI: config.XUIConfig{
			Enabled:       true,
			BaseURL:       "https://127.0.0.1:8443",
			Username:      "new-admin",
			Password:      "new-pass",
			SkipTLSVerify: true,
		},
	})
	if err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}
	if updated.AgentName != "Japan Relay 01" {
		t.Fatalf("unexpected updated agent name: %s", updated.AgentName)
	}

	reloadedCfg, found, err := store.GetAgentConfig("jp-01")
	if err != nil {
		t.Fatalf("GetAgentConfig reload: %v", err)
	}
	if !found {
		t.Fatalf("expected updated agent config")
	}
	if reloadedCfg.XUI.BaseURL != "https://127.0.0.1:8443" {
		t.Fatalf("unexpected updated x-ui base url: %s", reloadedCfg.XUI.BaseURL)
	}
	if got := len(reloadedCfg.Tags); got != 2 {
		t.Fatalf("expected normalized tags length 2, got %#v", reloadedCfg.Tags)
	}
}

func TestSQLiteStoreEncryptsXUIPasswordAtRest(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bridge.db")
	cipher, err := LoadOrCreateCredentialCipher(filepath.Join(dir, "credential.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateCredentialCipher: %v", err)
	}
	store, err := NewSQLiteStore(dbPath, WithCredentialCipher(cipher))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	if _, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID: "secret-01",
		SeedConfig: model.ManagedAgentConfig{
			XUI: config.XUIConfig{
				Enabled:  true,
				BaseURL:  "https://127.0.0.1:2053",
				Username: "admin",
				Password: "plain-xui-password",
			},
		},
	}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	var storedJSON string
	if err := store.db.QueryRow(`SELECT xui_config_json FROM agents WHERE agent_id = ?`, "secret-01").Scan(&storedJSON); err != nil {
		t.Fatalf("read raw xui config: %v", err)
	}
	if strings.Contains(storedJSON, "plain-xui-password") {
		t.Fatalf("expected raw x-ui config to be encrypted, got %s", storedJSON)
	}
	if !strings.Contains(storedJSON, encryptedValuePrefix) {
		t.Fatalf("expected encrypted value prefix in raw x-ui config, got %s", storedJSON)
	}

	cfg, found, err := store.GetAgentConfig("secret-01")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found {
		t.Fatalf("expected registered agent config")
	}
	if cfg.XUI.Password != "plain-xui-password" {
		t.Fatalf("expected decrypted password, got %q", cfg.XUI.Password)
	}
}

func TestSQLiteStoreMigratesPlaintextXUIPasswords(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bridge.db")
	legacyStore, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore legacy: %v", err)
	}
	if _, err := legacyStore.RegisterAgent(model.AgentRegisterRequest{
		AgentID: "legacy-01",
		SeedConfig: model.ManagedAgentConfig{
			XUI: config.XUIConfig{Enabled: true, Password: "legacy-plain-password"},
		},
	}); err != nil {
		t.Fatalf("RegisterAgent legacy: %v", err)
	}
	_ = legacyStore.Close()

	cipher, err := LoadOrCreateCredentialCipher(filepath.Join(dir, "credential.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateCredentialCipher: %v", err)
	}
	store, err := NewSQLiteStore(dbPath, WithCredentialCipher(cipher))
	if err != nil {
		t.Fatalf("NewSQLiteStore encrypted: %v", err)
	}
	defer store.Close()

	var storedJSON string
	if err := store.db.QueryRow(`SELECT xui_config_json FROM agents WHERE agent_id = ?`, "legacy-01").Scan(&storedJSON); err != nil {
		t.Fatalf("read migrated xui config: %v", err)
	}
	if strings.Contains(storedJSON, "legacy-plain-password") || !strings.Contains(storedJSON, encryptedValuePrefix) {
		t.Fatalf("expected migrated encrypted x-ui config, got %s", storedJSON)
	}
	cfg, found, err := store.GetAgentConfig("legacy-01")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found || cfg.XUI.Password != "legacy-plain-password" {
		t.Fatalf("expected decrypted migrated password, found=%v cfg=%#v", found, cfg.XUI)
	}
}

func TestSQLiteStoreSnapshotRetention(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath, WithSnapshotRetention(SnapshotRetentionPolicy{MaxPerAgent: 2}))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	base := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		if err := store.SaveSnapshot(model.AgentSnapshot{
			AgentID:    "hk-01",
			AgentName:  "HK-01",
			ReportedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("SaveSnapshot %d: %v", i, err)
		}
	}

	history, err := store.ListHistory("hk-01", 10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 retained snapshots, got %d", len(history))
	}
	if !history[0].ReportedAt.Equal(base.Add(3*time.Minute)) || !history[1].ReportedAt.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("unexpected retained history order: %#v", history)
	}
}

func TestSQLiteStoreXUIActionLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	registerResp, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "sg-01",
		AgentName: "Singapore 01",
	})
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	action, err := store.CreateXUIAction("sg-01", model.XUIActionRequest{
		Kind: model.XUIActionAddOutbound,
		Payload: map[string]any{
			"outbound": map[string]any{"tag": "relay-sg", "protocol": "freedom"},
			"restart":  true,
		},
	})
	if err != nil {
		t.Fatalf("CreateXUIAction: %v", err)
	}
	if action.Status != model.XUIActionStatusPending {
		t.Fatalf("expected pending action, got %s", action.Status)
	}

	claimed, err := store.ClaimPendingXUIActions("sg-01", 10)
	if err != nil {
		t.Fatalf("ClaimPendingXUIActions: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != action.ID {
		t.Fatalf("unexpected claimed actions: %#v", claimed)
	}
	if claimed[0].Status != model.XUIActionStatusRunning || claimed[0].ClaimedAt == nil {
		t.Fatalf("expected running claimed action, got %#v", claimed[0])
	}

	if again, err := store.ClaimPendingXUIActions("sg-01", 10); err != nil {
		t.Fatalf("ClaimPendingXUIActions second: %v", err)
	} else if len(again) != 0 {
		t.Fatalf("expected no pending action after claim, got %d", len(again))
	}

	completed, err := store.CompleteXUIAction("sg-01", action.ID, model.XUIActionResultRequest{
		Status: model.XUIActionStatusSucceeded,
		Result: map[string]any{"outbound_tag": "relay-sg"},
	})
	if err != nil {
		t.Fatalf("CompleteXUIAction: %v", err)
	}
	if completed.Status != model.XUIActionStatusSucceeded || completed.CompletedAt == nil {
		t.Fatalf("expected succeeded completed action, got %#v", completed)
	}
	if completed.Result["outbound_tag"] != "relay-sg" {
		t.Fatalf("unexpected action result: %#v", completed.Result)
	}
	if !store.ValidateAgentToken("sg-01", registerResp.AgentToken) {
		t.Fatalf("expected agent token to remain valid")
	}
}

func TestSQLiteStoreRenewalTrafficBaselineResetsByCycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 5, 9, 8, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -1)
	startDate := start.Format("2006-01-02")

	if _, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "traffic-01",
		AgentName: "Traffic 01",
	}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if err := store.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "traffic-01",
		AgentName:  "Traffic 01",
		ReportedAt: now,
		Summary: model.VPSSummary{
			NetTrafficSent:  200,
			NetTrafficRecv:  300,
			NetTrafficTotal: 500,
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	updated, err := store.UpdateAgentConfig("traffic-01", model.ManagedAgentConfig{
		AgentID:   "traffic-01",
		AgentName: "Traffic 01",
		Renewal: model.VPSRenewalConfig{
			Enabled:           true,
			StartDate:         startDate,
			Cycle:             "month",
			AutoRenew:         true,
			TrafficLimitBytes: 1024,
		},
	})
	if err != nil {
		t.Fatalf("UpdateAgentConfig: %v", err)
	}
	if updated.Config.Renewal.TrafficBaselinePeriodStart != startDate {
		t.Fatalf("expected baseline period %s, got %s", startDate, updated.Config.Renewal.TrafficBaselinePeriodStart)
	}
	if updated.Config.Renewal.TrafficBaselineBytes != 500 {
		t.Fatalf("expected total baseline 500, got %d", updated.Config.Renewal.TrafficBaselineBytes)
	}
	if updated.Config.Renewal.TrafficSentBaselineBytes != 200 || updated.Config.Renewal.TrafficRecvBaselineBytes != 300 {
		t.Fatalf("unexpected upload/download baselines: %#v", updated.Config.Renewal)
	}

	nextPeriod := start.AddDate(0, 1, 1)
	if err := store.SaveSnapshot(model.AgentSnapshot{
		AgentID:    "traffic-01",
		AgentName:  "Traffic 01",
		ReportedAt: nextPeriod,
		Summary: model.VPSSummary{
			NetTrafficSent:  350,
			NetTrafficRecv:  450,
			NetTrafficTotal: 800,
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot next period: %v", err)
	}
	reloaded, found, err := store.GetAgentConfig("traffic-01")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found {
		t.Fatalf("expected agent config")
	}
	expectedNextPeriod := start.AddDate(0, 1, 0).Format("2006-01-02")
	if reloaded.Renewal.TrafficBaselinePeriodStart != expectedNextPeriod {
		t.Fatalf("expected next baseline period %s, got %s", expectedNextPeriod, reloaded.Renewal.TrafficBaselinePeriodStart)
	}
	if reloaded.Renewal.TrafficBaselineBytes != 800 {
		t.Fatalf("expected next total baseline 800, got %d", reloaded.Renewal.TrafficBaselineBytes)
	}
}

func TestSQLiteStoreAdminAuthLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	if err := store.EnsureAdminAccount("admin", "first-password"); err != nil {
		t.Fatalf("EnsureAdminAccount: %v", err)
	}

	if _, ok, err := store.AuthenticateAdmin("admin", "wrong-password"); err != nil {
		t.Fatalf("AuthenticateAdmin wrong password: %v", err)
	} else if ok {
		t.Fatalf("expected wrong password to fail")
	}

	user, ok, err := store.AuthenticateAdmin("admin", "first-password")
	if err != nil {
		t.Fatalf("AuthenticateAdmin: %v", err)
	}
	if !ok || user.Username != "admin" {
		t.Fatalf("expected admin login, got ok=%v username=%q", ok, user.Username)
	}

	token, _, err := store.CreateAdminSession(user.Username, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession: %v", err)
	}
	if sessionUser, _, ok, err := store.ValidateAdminSession(token); err != nil {
		t.Fatalf("ValidateAdminSession: %v", err)
	} else if !ok || sessionUser.Username != "admin" {
		t.Fatalf("expected valid admin session, got ok=%v username=%q", ok, sessionUser.Username)
	}

	updated, err := store.UpdateAdminAccount(model.AdminAccountUpdateRequest{
		CurrentPassword: "first-password",
		NewUsername:     "owner",
		NewPassword:     "second-password",
	}, token)
	if err != nil {
		t.Fatalf("UpdateAdminAccount: %v", err)
	}
	if updated.Username != "owner" {
		t.Fatalf("unexpected updated username: %s", updated.Username)
	}

	if _, ok, err := store.AuthenticateAdmin("admin", "first-password"); err != nil {
		t.Fatalf("AuthenticateAdmin old credentials: %v", err)
	} else if ok {
		t.Fatalf("expected old credentials to fail")
	}
	if _, ok, err := store.AuthenticateAdmin("owner", "second-password"); err != nil {
		t.Fatalf("AuthenticateAdmin new credentials: %v", err)
	} else if !ok {
		t.Fatalf("expected new credentials to work")
	}
	if sessionUser, _, ok, err := store.ValidateAdminSession(token); err != nil {
		t.Fatalf("ValidateAdminSession after update: %v", err)
	} else if !ok || sessionUser.Username != "owner" {
		t.Fatalf("expected updated session user, got ok=%v username=%q", ok, sessionUser.Username)
	}
}
