package panels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

type routingDeletePanel struct {
	config                           map[string]any
	updates, restarts, reads, logins int
	apiPathOnly                      bool
	updateError, restartError        string
	outboundTestURL                  *string
}

func (p *routingDeletePanel) client(t *testing.T) *XUIClient {
	t.Helper()
	client, err := NewXUIClient(config.XUIConfig{Enabled: true, BaseURL: "https://xui.local", APIToken: "test-token"}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/login":
			p.logins++
			return jsonResponse(t, req, map[string]any{"success": true}), nil
		case "/panel/xray/", "/panel/api/xray/":
			if p.apiPathOnly && req.URL.Path == "/panel/xray/" {
				return nil, xuiHTTPError{StatusCode: http.StatusNotFound}
			}
			p.reads++
			wrapper := map[string]any{"xraySetting": p.config}
			if p.outboundTestURL != nil {
				wrapper["outboundTestUrl"] = *p.outboundTestURL
			}
			return jsonResponse(t, req, map[string]any{"success": true, "obj": wrapper}), nil
		case "/panel/xray/update", "/panel/api/xray/update":
			if p.apiPathOnly && req.URL.Path == "/panel/xray/update" {
				return nil, xuiHTTPError{StatusCode: http.StatusNotFound}
			}
			p.updates++
			if p.updateError != "" {
				return nil, fmt.Errorf("%s", p.updateError)
			}
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if p.outboundTestURL != nil && req.Form.Get("outboundTestUrl") != *p.outboundTestURL {
				t.Fatalf("unrelated panel test URL changed: %q", req.Form.Get("outboundTestUrl"))
			}
			if err := json.Unmarshal([]byte(req.Form.Get("xraySetting")), &p.config); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(t, req, map[string]any{"success": true}), nil
		case "/panel/api/server/restartXrayService":
			p.restarts++
			if p.restartError != "" {
				return nil, fmt.Errorf("%s", p.restartError)
			}
			return jsonResponse(t, req, map[string]any{"success": true}), nil
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil, nil
		}
	})}
	return client
}

func deletionTestConfig() map[string]any {
	return map[string]any{
		"api":       map[string]any{"tag": "api"},
		"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}},
		"log":       map[string]any{"loglevel": "warning"},
		"routing": map[string]any{"domainStrategy": "IPIfNonMatch", "rules": []map[string]any{
			{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"},
			{"type": "field", "user": []string{"alice"}, "outboundTag": "direct", "enabled": false},
			{"type": "field", "user": []string{"bob"}, "outboundTag": "direct"},
			{"type": "field", "protocol": []string{"bittorrent"}, "outboundTag": "blocked"},
			{"type": "field", "ip": []string{"geoip:private"}, "outboundTag": "blocked"},
		}},
	}
}

func deletionTestPayload(config map[string]any, indexes ...int) map[string]any {
	rules := objectSlice(objectMap(config["routing"])["rules"])
	targets := make([]model.RoutingRuleDeleteTarget, 0, len(indexes))
	for _, index := range indexes {
		targets = append(targets, model.RoutingRuleDeleteTarget{Index: index, Fingerprint: model.RoutingFingerprint(rules[index-1])})
	}
	return map[string]any{"rules_fingerprint": model.RoutingFingerprint(rules), "rules": targets}
}

func TestDeleteRoutingRulesSingleAndBatch(t *testing.T) {
	for _, test := range []struct {
		name               string
		indexes, remaining []int
		apiOnly            bool
	}{
		{"single", []int{2}, []int{1, 3, 4, 5}, false},
		{"unordered batch", []int{2, 5, 3}, []int{1, 4}, false},
		{"consecutive", []int{2, 3, 4}, []int{1, 5}, false},
		{"all ordinary", []int{2, 3, 4, 5}, []int{1}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			initial := deletionTestConfig()
			before := objectSlice(objectMap(initial["routing"])["rules"])
			panel := &routingDeletePanel{config: initial, apiPathOnly: test.apiOnly}
			result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: deletionTestPayload(initial, test.indexes...)})
			if err != nil {
				t.Fatal(err)
			}
			if panel.updates != 1 || panel.restarts != 1 || result["deleted_count"] != len(test.indexes) || result["restarted"] != true {
				t.Fatalf("expected one save/restart: %#v, %#v", panel, result)
			}
			after := objectSlice(objectMap(panel.config["routing"])["rules"])
			if len(after) != len(test.remaining) {
				t.Fatalf("remaining rules: %#v", after)
			}
			for index, original := range test.remaining {
				if model.RoutingFingerprint(after[index]) != model.RoutingFingerprint(before[original-1]) {
					t.Fatalf("wrong rule deleted or reordered at %d: %#v", index, after)
				}
			}
			for _, key := range []string{"outbounds", "api", "log"} {
				if model.RoutingFingerprint(panel.config[key]) != model.RoutingFingerprint(initial[key]) {
					t.Fatalf("unrelated %s modified", key)
				}
			}
			if objectMap(panel.config["routing"])["domainStrategy"] != "IPIfNonMatch" {
				t.Fatal("routing metadata modified")
			}
		})
	}
}

func TestDeleteRoutingRulesPreservesPanelTestURL(t *testing.T) {
	for _, value := range []string{"https://example.org/custom-test", ""} {
		t.Run(value, func(t *testing.T) {
			initial := deletionTestConfig()
			panel := &routingDeletePanel{config: initial, outboundTestURL: &value}
			if _, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: deletionTestPayload(initial, 2)}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeleteRoutingRulesRejectsWholeBatchBeforeSaving(t *testing.T) {
	for _, name := range []string{"stale list", "stale target", "out of range", "system API", "custom API", "malformed rule", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			initial := deletionTestConfig()
			payload := deletionTestPayload(initial, 2, 4)
			switch name {
			case "stale list":
				payload["rules_fingerprint"] = strings.Repeat("a", 64)
			case "stale target":
				payload["rules"].([]model.RoutingRuleDeleteTarget)[1].Fingerprint = strings.Repeat("a", 64)
			case "out of range":
				payload["rules"].([]model.RoutingRuleDeleteTarget)[1].Index = 99
			case "system API":
				payload = deletionTestPayload(initial, 2, 1)
			case "custom API":
				objectMap(initial["api"])["tag"] = "panel-api"
				objectSlice(objectMap(initial["routing"])["rules"])[0]["outboundTag"] = "panel-api"
				payload = deletionTestPayload(initial, 2, 1)
			case "malformed rule":
				objectMap(initial["routing"])["rules"] = []any{map[string]any{"outboundTag": "direct"}, "bad"}
			case "duplicate":
				payload = deletionTestPayload(initial, 2, 2)
			}
			panel := &routingDeletePanel{config: initial}
			before := model.RoutingFingerprint(initial)
			result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: payload})
			if err == nil || result != nil || panel.updates != 0 || panel.restarts != 0 {
				t.Fatalf("unsafe batch was executed: %#v, %v", result, err)
			}
			if model.RoutingFingerprint(panel.config) != before {
				t.Fatal("rejected batch modified config")
			}
		})
	}
}

func TestDeleteRoutingRulesReplayDoesNotDeleteIdenticalAdjacentRules(t *testing.T) {
	initial := deletionTestConfig()
	rules := objectSlice(objectMap(initial["routing"])["rules"])
	rules[2] = rules[1]
	payload := deletionTestPayload(initial, 2)
	panel := &routingDeletePanel{config: initial}
	client := panel.client(t)
	action := model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: payload}
	if _, err := client.ExecuteAction(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	before := model.RoutingFingerprint(panel.config)
	if _, err := client.ExecuteAction(context.Background(), action); err == nil {
		t.Fatal("replayed task must fail its list guard")
	}
	if panel.updates != 1 || panel.restarts != 1 || model.RoutingFingerprint(panel.config) != before {
		t.Fatal("replay deleted another identical rule")
	}
}

func TestDeleteRoutingRulesSaveAndRestartFailures(t *testing.T) {
	t.Run("save fails", func(t *testing.T) {
		initial := deletionTestConfig()
		panel := &routingDeletePanel{config: initial, updateError: "save failed"}
		result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: deletionTestPayload(initial, 2)})
		if err == nil || result != nil || panel.restarts != 0 {
			t.Fatalf("unexpected result: %#v, %v", result, err)
		}
	})
	t.Run("restart loses auth after saving", func(t *testing.T) {
		initial := deletionTestConfig()
		panel := &routingDeletePanel{config: initial, restartError: "unauthorized"}
		result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: deletionTestPayload(initial, 2, 4)})
		if err == nil || !strings.Contains(err.Error(), "已保存") || result["saved"] != true || result["restarted"] != false {
			t.Fatalf("partial save not reported: %#v, %v", result, err)
		}
		if panel.reads != 1 || panel.updates != 1 || panel.restarts != 1 || panel.logins != 0 {
			t.Fatalf("destructive action was retried: %#v", panel)
		}
		if !reflect.DeepEqual(result["deleted_rule_indexes"], []int{4, 2}) {
			t.Fatalf("bad deleted index result: %#v", result)
		}
	})
}

func TestDeleteLastRoutingRuleSavesEmptyArray(t *testing.T) {
	initial := map[string]any{"routing": map[string]any{"rules": []map[string]any{{"outboundTag": "direct"}}}}
	panel := &routingDeletePanel{config: initial}
	if _, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: deletionTestPayload(initial, 1)}); err != nil {
		t.Fatal(err)
	}
	raw := objectMap(panel.config["routing"])["rules"]
	if rules, ok := raw.([]any); !ok || len(rules) != 0 {
		t.Fatalf("expected [], not null: %#v", raw)
	}
}

func TestRoutingDeletionReadsSavedTemplateNotRuntimeRules(t *testing.T) {
	saved := deletionTestConfig()
	runtimeRules := []map[string]any{{"type": "field", "user": []string{"bob"}, "outboundTag": "direct"}}
	client, err := NewXUIClient(config.XUIConfig{Enabled: true, BaseURL: "https://xui.local", APIToken: "test-token"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	logins := 0
	client.client.Transport = xuiCollectTransport(t, &logins, func(req *http.Request) *http.Response {
		switch req.URL.Path {
		case "/panel/api/server/getConfigJson":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{
				"outbounds": saved["outbounds"], "routing": map[string]any{"rules": runtimeRules},
			}})
		case "/panel/xray/":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"xraySetting": saved}})
		}
		return nil
	})
	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" || snapshot.RoutingTemplate == nil || len(snapshot.RoutingTemplate.Rules) != 5 || len(snapshot.RoutingRules) != 1 {
		t.Fatalf("saved template must coexist with runtime routes: %#v", snapshot)
	}
	overview := dashboard.BuildXUIOverview(model.AgentSnapshot{AgentID: "agent-1", XUI: snapshot})
	if overview.RoutingRules[0].Index != 3 || overview.EditableRoutingRules == nil {
		t.Fatalf("runtime rule should anchor to panel R3: %#v", overview)
	}
	views := *overview.EditableRoutingRules
	if views[1].Enabled == nil || *views[1].Enabled {
		t.Fatal("disabled saved rule must remain deletable")
	}
	panel := &routingDeletePanel{config: saved}
	payload := map[string]any{
		"rules_fingerprint": overview.RoutingRulesFingerprint,
		"rules":             []model.RoutingRuleDeleteTarget{{Index: views[1].Index, Fingerprint: views[1].Fingerprint}, {Index: views[3].Index, Fingerprint: views[3].Fingerprint}},
	}
	if result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteRoutingRules, Payload: payload}); err != nil || result["deleted_count"] != 2 {
		t.Fatalf("snapshot-to-deletion integration failed: %#v, %v", result, err)
	}
}
