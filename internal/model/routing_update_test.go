package model

import (
	"strings"
	"testing"
)

func TestParseRoutingRuleUpdatePayload(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{"rule_index": 2, "rules_fingerprint": strings.Repeat("a", 64), "rule_fingerprint": strings.Repeat("b", 64), "rule": map[string]any{"type": "field", "user": []string{"alice"}, "outboundTag": "direct", "enabled": false, "future": map[string]any{"preserve": true}}}
	}
	parsed, err := ParseRoutingRuleUpdatePayload(valid())
	if err != nil || parsed.Rule["enabled"] != false || parsed.Rule["future"] == nil {
		t.Fatalf("valid full rule lost fields: %#v, %v", parsed, err)
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"missing guard", func(p, r map[string]any) { delete(p, "rules_fingerprint") }},
		{"invalid rule guard", func(p, r map[string]any) { p["rule_fingerprint"] = "bad" }},
		{"zero index", func(p, r map[string]any) { p["rule_index"] = 0 }},
		{"fractional index", func(p, r map[string]any) { p["rule_index"] = 1.5 }},
		{"missing rule", func(p, r map[string]any) { delete(p, "rule") }},
		{"two targets", func(p, r map[string]any) { r["balancerTag"] = "balance" }},
		{"blank target", func(p, r map[string]any) { r["outboundTag"] = " " }},
		{"padded target", func(p, r map[string]any) { r["outboundTag"] = " direct " }},
		{"bad type", func(p, r map[string]any) { r["type"] = "unknown" }},
		{"bad enabled", func(p, r map[string]any) { r["enabled"] = "false" }},
		{"scalar users", func(p, r map[string]any) { r["user"] = "alice" }},
		{"bad users", func(p, r map[string]any) { r["user"] = []any{1} }},
		{"bad network", func(p, r map[string]any) { r["network"] = "tcp,other" }},
		{"port overflow", func(p, r map[string]any) { r["port"] = "65536" }},
		{"reversed range", func(p, r map[string]any) { r["sourcePort"] = "2000-1000" }},
		{"empty conditions", func(p, r map[string]any) { delete(r, "user") }},
		{"unchanged", func(p, r map[string]any) { p["rule_fingerprint"] = RoutingFingerprint(r) }},
		{"oversized", func(p, r map[string]any) { r["comment"] = strings.Repeat("x", 256*1024) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := valid()
			tc.edit(payload, payload["rule"].(map[string]any))
			if _, err := ParseRoutingRuleUpdatePayload(payload); err == nil {
				t.Fatal("invalid update accepted")
			}
		})
	}
	for _, network := range []any{"tcp,udp", []string{"tcp", "udp"}} {
		payload := valid()
		rule := payload["rule"].(map[string]any)
		rule["network"], rule["port"] = network, 443
		if _, err := ParseRoutingRuleUpdatePayload(payload); err != nil {
			t.Fatal(err)
		}
	}
}
