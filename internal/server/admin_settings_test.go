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
	if settings.RealmDownloadBaseURL != "https://mirror.example.com/realm/v2.9.4" {
		t.Fatalf("unexpected realm download base url: %q", settings.RealmDownloadBaseURL)
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
