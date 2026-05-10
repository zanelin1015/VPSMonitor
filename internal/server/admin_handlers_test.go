package server

import "testing"

func TestNormalizeVersionSupportsComponentTags(t *testing.T) {
	tests := map[string]string{
		"v0.1.5":                  "0.1.5",
		"server-v0.1.6":           "0.1.6",
		"client-0.2.0":            "0.2.0",
		"refs/tags/client-v1.2.3": "1.2.3",
	}
	for input, want := range tests {
		if got := normalizeVersion(input); got != want {
			t.Fatalf("normalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHasClientUpdateAsset(t *testing.T) {
	assets := []string{
		"VPSMonitor-server-linux-amd64.tar.gz",
		"VPSMonitor-client-linux-amd64.tar.gz",
	}
	if !hasClientUpdateAsset("VPSMonitor", assets) {
		t.Fatalf("expected client asset to be detected")
	}
	if hasClientUpdateAsset("Other", assets) {
		t.Fatalf("did not expect mismatched package prefix to be detected")
	}
}
