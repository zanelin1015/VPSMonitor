package dashboard

import "bridge-core/internal/model"

func routingDeletionView(snapshot *model.XUISnapshot, runtimeRules []routeRule) (*[]model.XUIRoutingRuleView, string) {
	if snapshot.RoutingTemplate == nil {
		// Older Client snapshots cannot attest that these are saved template
		// rules. Keep display compatibility, but do not enable destructive edits.
		return nil, ""
	}
	template := snapshot.RoutingTemplate
	saved := normalizeRouteRules(template.Rules)
	apiConfig := map[string]any{"api": map[string]any{"tag": template.APITag}}
	anchors := make(map[string]int, len(template.Rules))
	for index, raw := range template.Rules {
		saved[index].view.Protected = model.IsProtectedRoutingRule(raw, apiConfig)
		if enabled, ok := raw["enabled"].(bool); ok && !enabled {
			continue
		}
		key := routingMatchFingerprint(raw)
		if _, exists := anchors[key]; !exists {
			anchors[key] = index + 1
		}
	}
	// Keep live topology matching on runtime rules, but point its anchors at the
	// corresponding saved rule. Generated runtime-only rules have no anchor.
	for index, raw := range snapshot.RoutingRules {
		runtimeRules[index].view.Index = anchors[routingMatchFingerprint(raw)]
	}
	views := unwrapRules(saved)
	return &views, model.RoutingFingerprint(template.Rules)
}

func routingMatchFingerprint(raw map[string]any) string {
	rule := make(map[string]any, len(raw)+1)
	for key, value := range raw {
		// These are panel metadata, not routing criteria. All Xray criteria and
		// custom fields remain in the key to avoid guessing an incorrect anchor.
		if key != "enabled" && key != "comment" {
			rule[key] = value
		}
	}
	if stringValue(rule["type"]) == "" {
		rule["type"] = "field"
	}
	return model.RoutingFingerprint(rule)
}
