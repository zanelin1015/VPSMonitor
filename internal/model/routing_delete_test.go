package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRoutingFingerprintIncludesAllFieldsAndOrder(t *testing.T) {
	rule := map[string]any{"user": []string{"alice"}, "outboundTag": "direct", "enabled": false}
	var decoded map[string]any
	body, _ := json.Marshal(rule)
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if RoutingFingerprint(rule) != RoutingFingerprint(decoded) {
		t.Fatal("fingerprint must survive the snapshot/HTTP JSON round trip")
	}
	decoded["enabled"] = true
	if RoutingFingerprint(rule) == RoutingFingerprint(decoded) {
		t.Fatal("hidden panel-specific fields must be included")
	}
	if RoutingFingerprint([]map[string]any{rule, decoded}) == RoutingFingerprint([]map[string]any{decoded, rule}) {
		t.Fatal("rule ordering must be included")
	}
}

func TestParseRoutingRulesDeletePayload(t *testing.T) {
	fingerprint := strings.Repeat("a", 64)
	valid := func() map[string]any {
		return map[string]any{"rules_fingerprint": fingerprint, "rules": []any{map[string]any{"index": 2, "fingerprint": fingerprint}}}
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing list guard", func(p map[string]any) { delete(p, "rules_fingerprint") }},
		{"invalid list guard", func(p map[string]any) { p["rules_fingerprint"] = "bad" }},
		{"empty", func(p map[string]any) { p["rules"] = []any{} }},
		{"missing rule guard", func(p map[string]any) { p["rules"].([]any)[0].(map[string]any)["fingerprint"] = "" }},
		{"zero index", func(p map[string]any) { p["rules"].([]any)[0].(map[string]any)["index"] = 0 }},
		{"negative index", func(p map[string]any) { p["rules"].([]any)[0].(map[string]any)["index"] = -2 }},
		{"fractional index", func(p map[string]any) { p["rules"].([]any)[0].(map[string]any)["index"] = 2.5 }},
		{"string index", func(p map[string]any) { p["rules"].([]any)[0].(map[string]any)["index"] = "2" }},
		{"overflow index", func(p map[string]any) { p["rules"].([]any)[0].(map[string]any)["index"] = 1e30 }},
		{"duplicate", func(p map[string]any) { p["rules"] = []any{p["rules"].([]any)[0], p["rules"].([]any)[0]} }},
		{"too many", func(p map[string]any) { p["rules"] = make([]any, 1001) }},
	}
	if result, err := ParseRoutingRulesDeletePayload(valid()); err != nil || result.Rules[0].Index != 2 {
		t.Fatalf("valid request: %#v %v", result, err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := valid()
			test.mutate(payload)
			if _, err := ParseRoutingRulesDeletePayload(payload); err == nil {
				t.Fatal("expected invalid payload to be rejected")
			}
		})
	}
}

func TestProtectedRoutingRuleUsesConfiguredAPITag(t *testing.T) {
	config := map[string]any{"api": map[string]any{"tag": "panel-api"}}
	if !IsProtectedRoutingRule(map[string]any{"outboundTag": "panel-api"}, config) {
		t.Fatal("custom API route must be protected")
	}
	if IsProtectedRoutingRule(map[string]any{"outboundTag": "direct"}, config) {
		t.Fatal("ordinary routes must remain deletable")
	}
	if !IsProtectedRoutingRule(map[string]any{"outboundTag": "api"}, nil) {
		t.Fatal("default API route must be protected")
	}
}
