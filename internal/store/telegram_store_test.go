package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestTelegramMigrationOwnershipAndEncryption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bridge.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE telegram_bots (
        id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, bot_token TEXT NOT NULL DEFAULT '',
        chat_id TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
        INSERT INTO telegram_bots (name, bot_token, chat_id, created_at, updated_at) VALUES ('old admin','old-secret','admin-chat','','');`)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	cipher, err := LoadOrCreateCredentialCipher(filepath.Join(dir, "credential.key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLiteStore(path, WithCredentialCipher(cipher))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bots, err := s.ListTelegramBots(0)
	if err != nil || len(bots) != 1 || !bots[0].NotifyExpiry || !bots[0].NotifySupport {
		t.Fatalf("legacy bot migration: %#v %v", bots, err)
	}
	secret, found, err := s.GetTelegramBotSecret(bots[0].ID, 0)
	if err != nil || !found || secret.BotToken != "old-secret" {
		t.Fatalf("legacy token not preserved: %v", err)
	}
	var stored string
	if err := s.db.QueryRow(`SELECT bot_token FROM telegram_bots WHERE id = ?`, bots[0].ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, encryptedValuePrefix) {
		t.Fatal("legacy token must be encrypted during migration")
	}
	encoded, _ := json.Marshal(bots)
	if strings.Contains(string(encoded), "old-secret") || strings.Contains(string(encoded), `"bot_token"`) {
		t.Fatal("public bot views disclose a token")
	}

	enabled := true
	one, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "one", Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "two", Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	areaBot, err := s.CreateTelegramBot(model.TelegramBotRequest{Name: "one", BotToken: "area-one-secret", ChatID: "area-one-chat", Enabled: true}, one.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTelegramBot(model.TelegramBotRequest{Name: "two", BotToken: "area-two-secret", ChatID: "area-two-chat", Enabled: true}, two.ID); err != nil {
		t.Fatal(err)
	}
	for _, ownerID := range []int64{0, one.ID, two.ID} {
		items, err := s.ListTelegramBots(ownerID)
		if err != nil || len(items) != 1 {
			t.Fatalf("owner %d: %#v %v", ownerID, items, err)
		}
	}
	for _, other := range []int64{0, two.ID} {
		if _, found, err := s.GetTelegramBotSecret(areaBot.ID, other); err != nil || found {
			t.Fatalf("cross-owner secret access: %v", err)
		}
		if _, err := s.UpdateTelegramBot(areaBot.ID, model.TelegramBotRequest{Name: "hijack"}, other); err == nil {
			t.Fatal("cross-owner update allowed")
		}
		if err := s.DeleteTelegramBot(areaBot.ID, other); err == nil {
			t.Fatal("cross-owner deletion allowed")
		}
	}
	no := false
	if _, err := s.UpdateTelegramBot(areaBot.ID, model.TelegramBotRequest{Name: "renamed", Enabled: true, NotifySupport: &no}, one.ID); err != nil {
		t.Fatal(err)
	}
	updated, found, err := s.GetTelegramBotSecret(areaBot.ID, one.ID)
	if err != nil || !found || updated.BotToken != "area-one-secret" || updated.NotifySupport || !updated.NotifyExpiry {
		t.Fatalf("blank token/flag preservation failed: %v", err)
	}
	if _, err := s.UpdateTelegramBot(areaBot.ID, model.TelegramBotRequest{Enabled: true, NotifyExpiry: &no}, one.ID); err != nil {
		t.Fatal(err)
	}
	if owners, err := s.ListTelegramExpiryOwnerIDs(); err != nil || len(owners) != 1 || owners[0] != two.ID {
		t.Fatalf("muted expiry owner selected: %v %v", owners, err)
	}
	if _, err := s.db.Exec(`UPDATE area_manager_accounts SET enabled = 0 WHERE id = ?`, one.ID); err != nil {
		t.Fatal(err)
	}
	if active, err := s.ListEnabledTelegramBotSecrets(one.ID); err != nil || len(active) != 0 {
		t.Fatalf("disabled owner receives notifications: %v", err)
	}
	owners, err := s.ListTelegramExpiryOwnerIDs()
	if err != nil || len(owners) != 1 || owners[0] != two.ID {
		t.Fatalf("expiry owner selection: %v %v", owners, err)
	}
	if err := s.DeleteAreaManager(one.ID); err != nil {
		t.Fatal(err)
	}
	if items, err := s.ListTelegramBots(one.ID); err != nil || len(items) != 0 {
		t.Fatalf("deleted owner's bot credentials remain: %v", err)
	}
	if items, err := s.ListTelegramBots(0); err != nil || len(items) != 1 {
		t.Fatalf("admin bot changed: %v", err)
	}
}
