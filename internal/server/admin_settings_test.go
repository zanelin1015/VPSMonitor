package server

import (
	"testing"

	"bridge-core/internal/model"
)

func TestValidateClientInstallSettingsRealm(t *testing.T) {
	settings, err := validateClientInstallSettings(model.ClientInstallSettingsRequest{
		ServerURL:             "https://panel.example.com",
		InstallScriptURL:      "https://example.com/install.sh",
		PollInterval:          "30s",
		RequestTimeoutSeconds: 15,
		RealmAutoInstall:      true,
		RealmVersion:          " v2.9.4 ",
		RealmDownloadBaseURL:  " https://mirror.example.com/realm/v2.9.4/ ",
	})
	if err != nil {
		t.Fatalf("validateClientInstallSettings: %v", err)
	}
	if !settings.RealmAutoInstall || settings.RealmVersion != "v2.9.4" {
		t.Fatalf("unexpected realm settings: %#v", settings)
	}
	if settings.HAProxyAutoInstall {
		t.Fatal("expected HAProxy auto install to remain disabled")
	}
	if settings.RealmDownloadBaseURL != "https://mirror.example.com/realm/v2.9.4" {
		t.Fatalf("unexpected realm download base url: %q", settings.RealmDownloadBaseURL)
	}
}

func TestValidateClientInstallSettingsRejectsRealmAndHAProxy(t *testing.T) {
	_, err := validateClientInstallSettings(model.ClientInstallSettingsRequest{
		ServerURL:          "https://panel.example.com",
		InstallScriptURL:   "https://example.com/install.sh",
		PollInterval:       "30s",
		RealmAutoInstall:   true,
		HAProxyAutoInstall: true,
	})
	if err == nil {
		t.Fatal("expected Realm and HAProxy auto install to be mutually exclusive")
	}
}

func TestValidateClientInstallSettingsRejectsInvalidRealmValues(t *testing.T) {
	base := model.ClientInstallSettingsRequest{
		ServerURL:        "https://panel.example.com",
		InstallScriptURL: "https://example.com/install.sh",
		PollInterval:     "30s",
	}
	base.RealmVersion = "v2.9.4; reboot"
	if _, err := validateClientInstallSettings(base); err == nil {
		t.Fatal("expected invalid realm version to be rejected")
	}
	base.RealmVersion = "v2.9.4"
	base.RealmDownloadBaseURL = "file:///tmp/realm"
	if _, err := validateClientInstallSettings(base); err == nil {
		t.Fatal("expected non-http realm download base url to be rejected")
	}
}

func TestValidateFrontendSettingsAnnouncements(t *testing.T) {
	err := validateFrontendSettings(model.FrontendSettings{
		Announcements: []model.CustomerAnnouncement{
			{
				Enabled:  true,
				Title:    "Telegram 已更换",
				LinkURL:  "https://t.me/example",
				StartsAt: "2026-08-04T12:00:00Z",
				EndsAt:   "2026-08-05T12:00:00Z",
			},
		},
	})
	if err != nil {
		t.Fatalf("validateFrontendSettings: %v", err)
	}
}

func TestValidateFrontendSettingsRejectsInvalidAnnouncement(t *testing.T) {
	tests := []model.CustomerAnnouncement{
		{Enabled: true, LinkURL: "https://t.me/example"},
		{Enabled: true, Title: "Invalid URL", LinkURL: "javascript:alert(1)"},
		{Enabled: true, Title: "Invalid window", StartsAt: "2026-08-05T12:00:00Z", EndsAt: "2026-08-04T12:00:00Z"},
	}
	for _, announcement := range tests {
		if err := validateFrontendSettings(model.FrontendSettings{Announcements: []model.CustomerAnnouncement{announcement}}); err == nil {
			t.Fatalf("expected announcement to be rejected: %#v", announcement)
		}
	}
}
