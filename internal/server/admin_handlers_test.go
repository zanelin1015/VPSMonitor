package server

import (
	"testing"

	"bridge-core/internal/model"
)

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

func TestFilterRootOnlyXUIActionsHidesRemoteCommands(t *testing.T) {
	actions := []model.XUIAction{
		{ID: 1, Kind: model.XUIActionUpsertRoutingRule},
		{ID: 2, Kind: model.XUIActionExecuteCommand},
		{ID: 3, Kind: model.XUIActionRestartXUI},
	}
	filtered := filterRootOnlyXUIActions(actions)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 visible actions, got %#v", filtered)
	}
	for _, action := range filtered {
		if action.Kind == model.XUIActionExecuteCommand {
			t.Fatalf("remote command action leaked to non-root admin: %#v", filtered)
		}
	}
}
