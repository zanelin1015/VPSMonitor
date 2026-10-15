package panels

import (
	"context"
	"encoding/json"
	"testing"

	"bridge-core/internal/model"
)

func routingUpdateTestPayload(config map[string]any, index int) map[string]any {
	rules := objectSlice(objectMap(config["routing"])["rules"])
	body, _ := json.Marshal(rules[index-1])
	var rule map[string]any
	_ = json.Unmarshal(body, &rule)
	rule["comment"] = "updated remark"
	return map[string]any{"rules_fingerprint": model.RoutingFingerprint(rules), "rule_index": index, "rule_fingerprint": model.RoutingFingerprint(rules[index-1]), "rule": rule}
}

func TestUpdateRoutingRulePreservesPositionAndFullTemplate(t *testing.T) {
	for _, apiOnly := range []bool{false, true} {
		config := deletionTestConfig()
		rules := objectSlice(objectMap(config["routing"])["rules"])
		rules[1]["attrs"] = map[string]any{":method": "GET"}
		rules[1]["futureField"] = map[string]any{"nested": []string{"keep"}}
		rules[1]["ruleTag"] = "stable-rule-tag"
		payload := routingUpdateTestPayload(config, 2)
		payload["rule"].(map[string]any)["user"] = []string{"bob"}
		testURL := "https://test.example.invalid/preserve"
		panel := &routingDeletePanel{config: config, apiPathOnly: apiOnly, outboundTestURL: &testURL}
		result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateRoutingRule, Payload: payload})
		if err != nil || result["saved"] != true || result["verified"] != true || result["restarted"] != true || panel.updates != 1 || panel.restarts != 1 {
			t.Fatalf("update failed: result=%#v err=%v", result, err)
		}
		after := objectSlice(objectMap(panel.config["routing"])["rules"])
		if len(after) != len(rules) {
			t.Fatal("rule count changed")
		}
		for i := range rules {
			want := rules[i]
			if i == 1 {
				want = payload["rule"].(map[string]any)
			}
			if model.RoutingFingerprint(after[i]) != model.RoutingFingerprint(want) {
				t.Fatalf("rule R%d moved or lost fields", i+1)
			}
		}
		for _, key := range []string{"api", "outbounds", "log"} {
			if model.RoutingFingerprint(config[key]) != model.RoutingFingerprint(panel.config[key]) {
				t.Fatalf("unrelated %s changed", key)
			}
		}
		if objectMap(panel.config["routing"])["domainStrategy"] != "IPIfNonMatch" {
			t.Fatal("routing options changed")
		}
		if _, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateRoutingRule, Payload: payload}); err == nil || panel.updates != 1 {
			t.Fatal("stale replay was not rejected")
		}
	}
}

func TestUpdateRoutingRuleGuardsTargetsAndAPI(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"stale list", func(c, p map[string]any) { p["rules_fingerprint"] = model.RoutingFingerprint("stale") }},
		{"stale rule", func(c, p map[string]any) { p["rule_fingerprint"] = model.RoutingFingerprint("stale") }},
		{"bad index", func(c, p map[string]any) { p["rule_index"] = 999 }},
		{"protected original", func(c, p map[string]any) {
			original := routingUpdateTestPayload(c, 1)
			for key, value := range original {
				p[key] = value
			}
			p["rule"].(map[string]any)["outboundTag"] = "direct"
		}},
		{"protected new target", func(c, p map[string]any) { p["rule"].(map[string]any)["outboundTag"] = "api" }},
		{"custom API target", func(c, p map[string]any) {
			c["api"] = map[string]any{"tag": "custom-api"}
			p["rule"].(map[string]any)["outboundTag"] = "custom-api"
		}},
		{"missing outbound", func(c, p map[string]any) { p["rule"].(map[string]any)["outboundTag"] = "missing" }},
		{"missing balancer", func(c, p map[string]any) {
			rule := p["rule"].(map[string]any)
			delete(rule, "outboundTag")
			rule["balancerTag"] = "missing"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := deletionTestConfig()
			payload := routingUpdateTestPayload(config, 2)
			tc.edit(config, payload)
			before := model.RoutingFingerprint(config)
			panel := &routingDeletePanel{config: config}
			if _, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateRoutingRule, Payload: payload}); err == nil || panel.updates != 0 || model.RoutingFingerprint(panel.config) != before {
				t.Fatal("invalid update modified the panel")
			}
		})
	}
}

func TestUpdateRoutingRuleBalancerAndPartialFailures(t *testing.T) {
	t.Run("readback unavailable", func(t *testing.T) {
		config := deletionTestConfig()
		panel := &routingDeletePanel{config: config, readErrorAfter: 1}
		result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateRoutingRule, Payload: routingUpdateTestPayload(config, 2)})
		if err == nil || result["submitted"] != true || result["verified"] != false || panel.updates != 1 || panel.restarts != 0 {
			t.Fatalf("uncertain save was replayed or treated as success: %#v %v", result, err)
		}
	})
	t.Run("balancer", func(t *testing.T) {
		config := deletionTestConfig()
		objectMap(config["routing"])["balancers"] = []map[string]any{{"tag": "balance", "selector": []string{"direct"}}}
		payload := routingUpdateTestPayload(config, 2)
		rule := payload["rule"].(map[string]any)
		delete(rule, "outboundTag")
		rule["balancerTag"] = "balance"
		panel := &routingDeletePanel{config: config}
		if _, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateRoutingRule, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct {
		name                            string
		ignore                          bool
		saveErr, persistErr, restartErr string
		success, saved, verified        bool
		restarts                        int
	}{
		{"save failed", false, "save failed", "", "", false, false, false, 0},
		{"false panel success", true, "", "", "", false, true, false, 0},
		{"persisted then hot reload error", false, "", "hot apply failed", "", true, true, true, 1},
		{"restart failed", false, "", "", "restart failed", false, true, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := deletionTestConfig()
			panel := &routingDeletePanel{config: config, ignoreUpdate: tc.ignore, updateError: tc.saveErr, updatePersistError: tc.persistErr, restartError: tc.restartErr}
			result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateRoutingRule, Payload: routingUpdateTestPayload(config, 2)})
			if (err == nil) != tc.success || (result["saved"] == true) != tc.saved || (result["verified"] == true) != tc.verified || panel.restarts != tc.restarts {
				t.Fatalf("result=%#v err=%v restarts=%d", result, err, panel.restarts)
			}
		})
	}
}
