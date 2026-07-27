package client

import "testing"

func TestOpenWrtInstallerURL(t *testing.T) {
	tests := map[string]string{
		"https://example.com/install.sh":          "https://example.com/install-openwrt.sh",
		"https://example.com/install.sh?v=1":      "https://example.com/install-openwrt.sh?v=1",
		"https://example.com/custom-installer.sh": "https://example.com/custom-installer.sh",
	}
	for input, want := range tests {
		if got := openWrtInstallerURL(input); got != want {
			t.Fatalf("openWrtInstallerURL(%q) = %q, want %q", input, got, want)
		}
	}
}
