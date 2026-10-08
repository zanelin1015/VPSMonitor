package dashboard

import (
	"testing"

	"bridge-core/internal/model"
)

func TestRoutingOverviewProvidesDeletionGuards(t *testing.T) {
	rules := []map[string]any{
		{"outboundTag": "panel-api", "inboundTag": []string{"api"}},
		{"outboundTag": "direct", "user": []string{"alice"}, "enabled": false},
	}
	overview := BuildXUIOverview(model.AgentSnapshot{AgentID: "agent-1", XUI: &model.XUISnapshot{
		RawConfig:       map[string]any{"api": map[string]any{"tag": "panel-api"}},
		RoutingRules:    rules,
		RoutingTemplate: &model.XUIRoutingTemplate{Rules: rules, APITag: "panel-api"},
	}})
	if overview.RoutingRulesFingerprint != model.RoutingFingerprint(rules) || overview.EditableRoutingRules == nil || len(*overview.EditableRoutingRules) != 2 {
		t.Fatalf("bad routing list guard: %#v", overview)
	}
	for index, rule := range *overview.EditableRoutingRules {
		if rule.Index != index+1 || rule.Fingerprint != model.RoutingFingerprint(rules[index]) {
			t.Fatalf("bad rule guard: %#v", rule)
		}
	}
	if !(*overview.EditableRoutingRules)[0].Protected || (*overview.EditableRoutingRules)[1].Protected {
		t.Fatalf("bad API rule protection: %#v", overview.RoutingRules)
	}
}

func TestEditableRoutingRulesPreserveRuntimeMatchingAndMapAnchors(t *testing.T) {
	saved := []map[string]any{
		{"outboundTag": "disabled", "user": []string{"alice"}, "enabled": false},
		{"outboundTag": "active", "user": []string{"alice"}, "enabled": true, "comment": "panel metadata"},
	}
	runtime := []map[string]any{{"outboundTag": "active", "user": []string{"alice"}, "type": "field"}}
	snapshot := &model.XUISnapshot{RoutingRules: runtime, RoutingTemplate: &model.XUIRoutingTemplate{Rules: saved}}
	rules := normalizeRouteRules(runtime)
	editable, fingerprint := routingDeletionView(snapshot, rules)
	if len(*editable) != 2 || fingerprint != model.RoutingFingerprint(saved) || (*editable)[0].Enabled == nil || *(*editable)[0].Enabled {
		t.Fatalf("saved disabled rule must remain editable: %#v", editable)
	}
	trace := resolveRoute("alice", "in-a", rules, "direct", nil)
	if trace.OutboundTag != "active" || trace.RuleIndex != 2 {
		t.Fatalf("runtime trace must point at saved R2: %#v", trace)
	}
	empty := &model.XUISnapshot{RoutingRules: runtime, RoutingTemplate: &model.XUIRoutingTemplate{Rules: []map[string]any{}}}
	editable, _ = routingDeletionView(empty, normalizeRouteRules(runtime))
	if editable == nil || len(*editable) != 0 {
		t.Fatal("empty saved list must not fall back to stale runtime rules")
	}
	legacy := BuildXUIOverview(model.AgentSnapshot{XUI: &model.XUISnapshot{RoutingRules: runtime}})
	if legacy.EditableRoutingRules != nil || legacy.RoutingRulesFingerprint != "" || len(legacy.RoutingRules) != 1 {
		t.Fatal("old Client snapshots must remain read-only")
	}
}
