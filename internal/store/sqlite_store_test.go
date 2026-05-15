package store

import (
	"database/sql"
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

func TestSQLiteStoreMigratesLegacyCustomerOwnerColumnsBeforeIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE customer_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL COLLATE NOCASE UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO customer_accounts (username, password_hash, display_name, enabled, created_at, updated_at)
		VALUES ('legacy', 'hash', 'Legacy User', 1, '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z');
		CREATE TABLE admin_sessions (
			token_hash TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		);
	`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("create legacy db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore should migrate legacy db: %v", err)
	}
	defer store.Close()

	for _, column := range []string{"style_code", "owner_type", "owner_id"} {
		if !sqliteColumnExists(t, store.db, "customer_accounts", column) {
			t.Fatalf("expected migrated customer_accounts.%s column", column)
		}
	}
	for _, column := range []string{"role", "account_id"} {
		if !sqliteColumnExists(t, store.db, "admin_sessions", column) {
			t.Fatalf("expected migrated admin_sessions.%s column", column)
		}
	}
	var ownerType string
	var ownerID int64
	if err := store.db.QueryRow(`SELECT owner_type, owner_id FROM customer_accounts WHERE username = ?`, "legacy").Scan(&ownerType, &ownerID); err != nil {
		t.Fatalf("read migrated owner defaults: %v", err)
	}
	if ownerType != model.AdminRoleRoot || ownerID != adminAccountID {
		t.Fatalf("unexpected owner defaults: %s/%d", ownerType, ownerID)
	}
	var indexName string
	if err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, "idx_customer_accounts_owner").Scan(&indexName); err != nil {
		t.Fatalf("expected owner index after column migration: %v", err)
	}
}

func TestSQLiteStoreAreaManagersIncludeOwnedCustomersAndAssignments(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	if _, err := store.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "sg-01",
		AgentName: "Singapore 01",
	}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	enabled := true
	manager, err := store.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-sg",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"sg-01"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	customer, err := store.CreateCustomerForOwner(model.CustomerAccountRequest{
		Username: "customer-a",
		Password: "password123",
	}, model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}
	if _, err := store.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{
		AgentID:          "sg-01",
		InboundID:        101,
		InboundTag:       "entry",
		ClientEmail:      "a@example.com",
		PublicClientName: "SG Entry A",
		Enabled:          &enabled,
	}); err != nil {
		t.Fatalf("CreateCustomerAssignment: %v", err)
	}

	managers, err := store.ListAreaManagers()
	if err != nil {
		t.Fatalf("ListAreaManagers: %v", err)
	}
	if len(managers) != 1 {
		t.Fatalf("expected 1 manager, got %d", len(managers))
	}
	if len(managers[0].Customers) != 1 {
		t.Fatalf("expected manager customer to be included, got %#v", managers[0].Customers)
	}
	gotCustomer := managers[0].Customers[0]
	if gotCustomer.ID != customer.ID || gotCustomer.OwnerType != model.AdminRoleAreaManager || gotCustomer.OwnerID != manager.ID {
		t.Fatalf("unexpected owned customer: %#v", gotCustomer.CustomerUser)
	}
	if len(gotCustomer.Assignments) != 1 || gotCustomer.Assignments[0].AgentID != "sg-01" {
		t.Fatalf("expected owned customer assignments, got %#v", gotCustomer.Assignments)
	}
	directAssignments, err := store.CreateAreaManagerAssignments(manager.ID, []model.AreaManagerAssignmentRequest{
		{
			AgentID:          "sg-01",
			InboundID:        102,
			InboundTag:       "entry-b",
			ClientEmail:      "b@example.com",
			PublicClientName: "SG Entry B",
			Enabled:          &enabled,
		},
	})
	if err != nil {
		t.Fatalf("CreateAreaManagerAssignments: %v", err)
	}
	if len(directAssignments) != 1 || directAssignments[0].ClientEmail != "b@example.com" {
		t.Fatalf("unexpected direct assignments: %#v", directAssignments)
	}
	updatedManager, found, err := store.GetAreaManager(manager.ID)
	if err != nil || !found {
		t.Fatalf("GetAreaManager found=%v err=%v", found, err)
	}
	if len(updatedManager.Assignments) != 1 || updatedManager.Assignments[0].AgentID != "sg-01" {
		t.Fatalf("expected direct area manager assignment, got %#v", updatedManager.Assignments)
	}
}

func sqliteColumnExists(t *testing.T, db *sql.DB, tableName string, columnName string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		t.Fatalf("query table info for %s: %v", tableName, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			t.Fatalf("scan table info for %s: %v", tableName, err)
		}
		if name == columnName {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info for %s: %v", tableName, err)
	}
	return false
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
		AgentID:             "jp-01",
		AgentName:           "Japan Relay 01",
		CustomerDisplayName: "Customer JP Entry",
		Tags:                []string{"relay", "asia", "relay"},
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
	if updated.CustomerDisplayName != "Customer JP Entry" {
		t.Fatalf("unexpected customer display name: %s", updated.CustomerDisplayName)
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
	if reloadedCfg.CustomerDisplayName != "Customer JP Entry" {
		t.Fatalf("expected reloaded customer display name, got %q", reloadedCfg.CustomerDisplayName)
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
	restartAction, err := store.CreateXUIAction("sg-01", model.XUIActionRequest{
		Kind:    model.XUIActionRestartXUI,
		Payload: map[string]any{"service_name": "x-ui"},
	})
	if err != nil {
		t.Fatalf("CreateXUIAction restart: %v", err)
	}
	if restartAction.Kind != model.XUIActionRestartXUI || restartAction.Status != model.XUIActionStatusPending {
		t.Fatalf("unexpected restart action: %#v", restartAction)
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

	token, _, err := store.CreateAdminSession(user, time.Hour)
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
		AvatarURL:       stringPtr("data:image/png;base64,avatar"),
	}, token)
	if err != nil {
		t.Fatalf("UpdateAdminAccount: %v", err)
	}
	if updated.Username != "owner" || updated.AvatarURL == "" {
		t.Fatalf("unexpected updated user: %#v", updated)
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
	} else if !ok || sessionUser.Username != "owner" || sessionUser.AvatarURL == "" {
		t.Fatalf("expected updated session user, got ok=%v user=%#v", ok, sessionUser)
	}
}

func TestSQLiteStoreAreaManagerScope(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	if err := store.EnsureAdminAccount("admin", "admin-password"); err != nil {
		t.Fatalf("EnsureAdminAccount: %v", err)
	}
	if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "hk-01", AgentName: "HK Entry"}); err != nil {
		t.Fatalf("RegisterAgent hk: %v", err)
	}
	if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "us-01", AgentName: "US Entry"}); err != nil {
		t.Fatalf("RegisterAgent us: %v", err)
	}

	enabled := true
	manager, err := store.CreateAreaManager(model.AreaManagerAccountRequest{
		Username:    "east",
		Password:    "manager-pass",
		DisplayName: "East Region",
		Enabled:     &enabled,
		AgentIDs:    []string{"hk-01"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	if len(manager.AgentIDs) != 1 || manager.AgentIDs[0] != "hk-01" {
		t.Fatalf("unexpected manager agents: %#v", manager)
	}

	user, ok, err := store.AuthenticateAdmin("east", "manager-pass")
	if err != nil {
		t.Fatalf("AuthenticateAdmin manager: %v", err)
	}
	if !ok || user.Role != model.AdminRoleAreaManager || user.ID != manager.ID || len(user.AgentIDs) != 1 || user.AgentIDs[0] != "hk-01" {
		t.Fatalf("expected area manager login, ok=%v user=%#v", ok, user)
	}
	token, _, err := store.CreateAdminSession(user, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession manager: %v", err)
	}
	sessionUser, _, ok, err := store.ValidateAdminSession(token)
	if err != nil {
		t.Fatalf("ValidateAdminSession manager: %v", err)
	}
	if !ok || sessionUser.Role != model.AdminRoleAreaManager || sessionUser.Username != "east" {
		t.Fatalf("unexpected manager session user: ok=%v user=%#v", ok, sessionUser)
	}

	adminCustomer, err := store.CreateCustomer(model.CustomerAccountRequest{
		Username: "admin-customer",
		Password: "customer-pass",
		Enabled:  &enabled,
	})
	if err != nil {
		t.Fatalf("CreateCustomer admin: %v", err)
	}
	managerCustomer, err := store.CreateCustomerForOwner(model.CustomerAccountRequest{
		Username: "east-customer",
		Password: "customer-pass",
		Enabled:  &enabled,
	}, model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("CreateCustomer manager: %v", err)
	}

	scopedCustomers, err := store.ListCustomersForOwner(model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("ListCustomersForOwner: %v", err)
	}
	if len(scopedCustomers) != 1 || scopedCustomers[0].ID != managerCustomer.ID {
		t.Fatalf("expected only manager-owned customer, got %#v", scopedCustomers)
	}
	if ok, err := store.CustomerOwnedBy(adminCustomer.ID, model.AdminRoleAreaManager, manager.ID); err != nil || ok {
		t.Fatalf("expected admin customer hidden from manager, ok=%v err=%v", ok, err)
	}
	if ok, err := store.AreaManagerCanAccessAgent(manager.ID, "us-01"); err != nil || ok {
		t.Fatalf("expected unassigned agent access to be denied, ok=%v err=%v", ok, err)
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestSQLiteStoreClientInstallSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	if _, found, err := store.GetClientInstallSettings(); err != nil {
		t.Fatalf("GetClientInstallSettings empty: %v", err)
	} else if found {
		t.Fatalf("expected no saved client install settings")
	}

	saved, err := store.SaveClientInstallSettings(model.ClientInstallSettingsRequest{
		ServerURL:             " https://panel.example.com ",
		InstallScriptURL:      " https://example.com/install.sh ",
		PollInterval:          "45s",
		RequestTimeoutSeconds: 20,
		ServerSkipTLSVerify:   true,
	})
	if err != nil {
		t.Fatalf("SaveClientInstallSettings: %v", err)
	}
	if saved.ServerURL != "https://panel.example.com" || saved.InstallScriptURL != "https://example.com/install.sh" {
		t.Fatalf("settings were not normalized: %#v", saved)
	}

	loaded, found, err := store.GetClientInstallSettings()
	if err != nil {
		t.Fatalf("GetClientInstallSettings loaded: %v", err)
	}
	if !found {
		t.Fatalf("expected saved client install settings")
	}
	if loaded != saved {
		t.Fatalf("unexpected loaded settings: got %#v want %#v", loaded, saved)
	}
}

func TestSQLiteStoreCustomerAccountsAssignmentsAndSessions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: "hk-01", AgentName: "HK Entry"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	enabled := true
	customer, err := store.CreateCustomer(model.CustomerAccountRequest{
		Username:    "alice",
		Password:    "customer-pass",
		DisplayName: "Alice",
		Enabled:     &enabled,
	})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if customer.ID == 0 || !customer.Enabled {
		t.Fatalf("unexpected customer: %#v", customer)
	}

	user, ok, err := store.AuthenticateCustomer("ALICE", "customer-pass")
	if err != nil {
		t.Fatalf("AuthenticateCustomer: %v", err)
	}
	if !ok || user.ID != customer.ID {
		t.Fatalf("expected customer authentication, ok=%v user=%#v", ok, user)
	}
	token, session, err := store.CreateCustomerSession(customer.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateCustomerSession: %v", err)
	}
	if token == "" || session.CustomerID != customer.ID {
		t.Fatalf("unexpected session: token=%q session=%#v", token, session)
	}
	if _, _, ok, err := store.ValidateCustomerSession(token); err != nil || !ok {
		t.Fatalf("ValidateCustomerSession ok=%v err=%v", ok, err)
	}
	otherToken, _, err := store.CreateCustomerSession(customer.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateCustomerSession other: %v", err)
	}
	if _, err := store.UpdateCustomerPassword(customer.ID, model.CustomerPasswordUpdateRequest{
		CurrentPassword: "wrong-password",
		NewPassword:     "customer-pass-2",
	}, token); err == nil {
		t.Fatalf("expected wrong current password to fail")
	}
	if _, err := store.UpdateCustomerPassword(customer.ID, model.CustomerPasswordUpdateRequest{
		CurrentPassword: "customer-pass",
		NewPassword:     "short",
	}, token); err == nil {
		t.Fatalf("expected short new password to fail")
	}
	updatedUser, err := store.UpdateCustomerPassword(customer.ID, model.CustomerPasswordUpdateRequest{
		CurrentPassword: "customer-pass",
		NewPassword:     "customer-pass-2",
	}, token)
	if err != nil {
		t.Fatalf("UpdateCustomerPassword: %v", err)
	}
	if updatedUser.ID != customer.ID || !updatedUser.UpdatedAt.After(customer.UpdatedAt) {
		t.Fatalf("unexpected updated customer user: %#v", updatedUser)
	}
	if _, ok, err := store.AuthenticateCustomer("alice", "customer-pass"); err != nil {
		t.Fatalf("AuthenticateCustomer old password: %v", err)
	} else if ok {
		t.Fatalf("expected old customer password to fail")
	}
	if _, ok, err := store.AuthenticateCustomer("alice", "customer-pass-2"); err != nil {
		t.Fatalf("AuthenticateCustomer new password: %v", err)
	} else if !ok {
		t.Fatalf("expected new customer password to work")
	}
	if _, _, ok, err := store.ValidateCustomerSession(token); err != nil || !ok {
		t.Fatalf("expected current customer session to remain valid, ok=%v err=%v", ok, err)
	}
	if _, _, ok, err := store.ValidateCustomerSession(otherToken); err != nil || ok {
		t.Fatalf("expected other customer session to be invalidated, ok=%v err=%v", ok, err)
	}

	assignment, err := store.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{
		AgentID:          "hk-01",
		InboundID:        7,
		InboundTag:       "entry-hk",
		ClientEmail:      "alice@example.com",
		PublicClientName: "香港入口 A",
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreateCustomerAssignment: %v", err)
	}
	if assignment.PublicClientName != "香港入口 A" {
		t.Fatalf("unexpected assignment: %#v", assignment)
	}
	if _, err := store.UpdateCustomerAssignmentRemark(customer.ID, assignment.ID, "主业务链路"); err != nil {
		t.Fatalf("UpdateCustomerAssignmentRemark: %v", err)
	}
	assignments, err := store.ListEnabledCustomerAssignments(customer.ID)
	if err != nil {
		t.Fatalf("ListEnabledCustomerAssignments: %v", err)
	}
	if len(assignments) != 1 || assignments[0].Remark != "主业务链路" {
		t.Fatalf("unexpected assignments: %#v", assignments)
	}

	disabled := false
	if _, err := store.UpdateCustomer(customer.ID, model.CustomerAccountRequest{Username: "alice", DisplayName: "Alice", Enabled: &disabled}); err != nil {
		t.Fatalf("UpdateCustomer disable: %v", err)
	}
	if _, _, ok, err := store.ValidateCustomerSession(token); err != nil || ok {
		t.Fatalf("expected disabled customer session to be invalid, ok=%v err=%v", ok, err)
	}
}
