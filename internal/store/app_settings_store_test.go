package store

import (
	"path/filepath"
	"testing"

	"bridge-core/internal/model"
)

func TestFrontendSettingsPersistCustomerAnnouncements(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	saved, err := store.SaveFrontendSettings(model.FrontendSettings{
		CustomCode: "<style>body { color: red; }</style>",
		Announcements: []model.CustomerAnnouncement{
			{
				Enabled:   true,
				Level:     "WARNING",
				Title:     " Telegram 已更换 ",
				Content:   " 请使用新账号 ",
				LinkLabel: " 联系我们 ",
				LinkURL:   " https://t.me/example ",
				StartsAt:  "2026-08-04T04:00:00+08:00",
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveFrontendSettings: %v", err)
	}
	if len(saved.Announcements) != 1 {
		t.Fatalf("expected one announcement, got %#v", saved.Announcements)
	}
	announcement := saved.Announcements[0]
	if announcement.ID == "" || announcement.Level != "warning" || announcement.Title != "Telegram 已更换" {
		t.Fatalf("unexpected normalized announcement: %#v", announcement)
	}
	if announcement.StartsAt != "2026-08-03T20:00:00Z" {
		t.Fatalf("unexpected normalized start time: %q", announcement.StartsAt)
	}

	loaded, found, err := store.GetFrontendSettings()
	if err != nil {
		t.Fatalf("GetFrontendSettings: %v", err)
	}
	if !found || len(loaded.Announcements) != 1 || loaded.Announcements[0].ID != announcement.ID {
		t.Fatalf("unexpected loaded frontend settings: %#v", loaded)
	}
	if len(loaded.AnnouncementHistory) == 0 {
		t.Fatalf("expected announcement history to be seeded for the current announcement")
	}
	legacySave, err := store.SaveFrontendSettings(model.FrontendSettings{CustomCode: "legacy client"})
	if err != nil {
		t.Fatalf("SaveFrontendSettings without announcements: %v", err)
	}
	if len(legacySave.Announcements) != 1 || legacySave.Announcements[0].ID != announcement.ID {
		t.Fatalf("omitted announcements should be preserved: %#v", legacySave.Announcements)
	}

	if _, err := store.SaveFrontendSettings(model.FrontendSettings{
		CustomCode:    "<style>body { color: red; }</style>",
		Announcements: []model.CustomerAnnouncement{},
	}); err != nil {
		t.Fatalf("SaveFrontendSettings after removal: %v", err)
	}
	removed, found, err := store.GetFrontendSettings()
	if err != nil {
		t.Fatalf("GetFrontendSettings after removal: %v", err)
	}
	if !found || len(removed.Announcements) != 0 {
		t.Fatalf("expected announcements to be removed, got %#v", removed.Announcements)
	}
	var hasRemoved bool
	for _, item := range removed.AnnouncementHistory {
		if item.AnnouncementID == announcement.ID && item.Action == "removed" {
			hasRemoved = true
			break
		}
	}
	if !hasRemoved {
		t.Fatalf("expected removed announcement to remain in history: %#v", removed.AnnouncementHistory)
	}
}
