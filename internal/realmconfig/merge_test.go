package realmconfig

import (
	"testing"

	"bridge-core/internal/model"
)

func TestMergeSnapshotIntoForwardConfigPreservesCustomRuleName(t *testing.T) {
	cfg := model.RealmForwardConfig{
		Enabled: true,
		Backend: "realm",
		Rules: []model.RealmForwardRule{{
			ID:            "custom-gz-hk-20001",
			Name:          "GZ entry to HK 20001",
			Enabled:       true,
			ListenAddress: "0.0.0.0",
			ListenPort:    20001,
			TargetAgentID: "hk",
			TargetAddress: "hk.example.com",
			TargetPort:    20001,
			Network:       "both",
		}},
	}
	snapshot := &model.RealmSnapshot{
		ConfigPath:  "/etc/realm/config.toml",
		ServiceName: "realm",
		Rules: []model.RealmForwardRule{{
			ID:            "auto-realm-20001-20001-0",
			Name:          "realm 20001 -> hk.example.com:20001",
			Enabled:       true,
			ListenAddress: "0.0.0.0",
			ListenPort:    20001,
			TargetAddress: "hk.example.com",
			TargetPort:    20001,
			Network:       "both",
		}},
	}

	merged := MergeSnapshotIntoForwardConfig(cfg, snapshot)
	if len(merged.Rules) != 1 {
		t.Fatalf("expected one merged rule, got %#v", merged.Rules)
	}
	rule := merged.Rules[0]
	if rule.Name != "GZ entry to HK 20001" {
		t.Fatalf("expected custom name to be preserved, got %q", rule.Name)
	}
	if rule.ID != "custom-gz-hk-20001" {
		t.Fatalf("expected custom ID to be preserved, got %q", rule.ID)
	}
	if rule.TargetAgentID != "hk" {
		t.Fatalf("expected target agent metadata to be preserved, got %q", rule.TargetAgentID)
	}
}
