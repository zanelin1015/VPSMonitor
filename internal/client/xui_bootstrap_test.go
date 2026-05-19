package client

import (
	"testing"

	"bridge-core/internal/config"
)

func TestNormalizeXUIBootstrapPath(t *testing.T) {
	tests := map[string]string{
		"":          "",
		"/":         "",
		"xui":       "/xui/",
		"/xui":      "/xui/",
		"/xui/":     "/xui/",
		" /panel/ ": "/panel/",
	}
	for input, want := range tests {
		if got := normalizeXUIBootstrapPath(input); got != want {
			t.Fatalf("normalizeXUIBootstrapPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestXUIBootstrapSignature(t *testing.T) {
	cfg := config.XUIConfig{
		AutoInstall:      true,
		InstallScriptURL: "https://example.com/install.sh",
		Username:         "admin",
		Password:         "secret",
		PanelPort:        2053,
		WebPath:          "xui",
	}
	sig := xuiBootstrapSignature(cfg)
	if sig == "" {
		t.Fatal("expected signature for enabled bootstrap")
	}
	cfg.WebPath = "/xui/"
	if got := xuiBootstrapSignature(cfg); got != sig {
		t.Fatalf("expected equivalent normalized web path to keep signature, got %q want %q", got, sig)
	}
	cfg.Password = "other"
	if got := xuiBootstrapSignature(cfg); got == sig {
		t.Fatalf("expected changed password to change signature")
	}
	cfg.AutoInstall = false
	if got := xuiBootstrapSignature(cfg); got != "" {
		t.Fatalf("expected empty signature when disabled, got %q", got)
	}
}
