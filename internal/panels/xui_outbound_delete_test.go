package panels

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func outboundDeletionTestConfig() map[string]any {
	return map[string]any{
		"api": map[string]any{"tag": "api"}, "log": map[string]any{"loglevel": "warning"},
		"routing":   map[string]any{"rules": []map[string]any{{"inboundTag": []string{"api"}, "outboundTag": "api"}}},
		"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}, {"tag": "old-a", "protocol": "vless", "settings": map[string]any{"servers": []map[string]any{{"address": "example.invalid", "port": 443, "password": "keep-secret"}}}}, {"tag": "old-b", "protocol": "freedom"}, {"tag": "keep", "protocol": "freedom"}},
	}
}

func outboundDeletionTestPayload(config map[string]any, tags ...string) map[string]any {
	byTag := make(map[string]map[string]any)
	for _, outbound := range objectSlice(config["outbounds"]) {
		byTag[stringValue(outbound["tag"])] = outbound
	}
	targets := make([]model.OutboundDeleteTarget, 0, len(tags))
	for _, tag := range tags {
		targets = append(targets, model.OutboundDeleteTarget{Tag: tag, Fingerprint: model.RoutingFingerprint(byTag[tag])})
	}
	return map[string]any{"config_fingerprint": model.RoutingFingerprint(config), "outbounds": targets}
}

func TestOutboundDeletionBatchOneWriteAndReplayGuard(t *testing.T) {
	config := outboundDeletionTestConfig()
	testURL := "https://example.invalid/probe"
	panel := &routingDeletePanel{config: config, outboundTestURL: &testURL, apiPathOnly: true}
	client := panel.client(t)
	action := model.XUIAction{Kind: model.XUIActionDeleteOutbounds, Payload: outboundDeletionTestPayload(config, "old-a", "old-b")}
	before := model.RoutingFingerprint(config["routing"])
	result, err := client.ExecuteAction(context.Background(), action)
	if err != nil || result["deleted_count"] != 2 || result["verified"] != true || panel.updates != 1 || panel.restarts != 1 {
		t.Fatalf("result=%#v err=%v writes=%d", result, err, panel.updates)
	}
	if before != model.RoutingFingerprint(panel.config["routing"]) || !reflect.DeepEqual(objectSlice(panel.config["outbounds"])[1], map[string]any{"tag": "keep", "protocol": "freedom"}) {
		t.Fatal("unselected configuration changed")
	}
	if _, err := client.ExecuteAction(context.Background(), action); err == nil || panel.updates != 1 {
		t.Fatal("replay must not delete new or shifted outbounds")
	}
}

func TestOutboundDeletionRejectsEntireBatchBeforeWrite(t *testing.T) {
	for _, tc := range []struct {
		name, tag string
		mutate    func(map[string]any)
	}{
		{"default", "direct", nil},
		{"route", "old-a", func(c map[string]any) {
			objectMap(c["routing"])["rules"] = []map[string]any{{"user": []string{"removed"}, "outboundTag": "old-a", "enabled": false}}
		}},
		{"balancer prefix", "old-a", func(c map[string]any) {
			objectMap(c["routing"])["balancers"] = []map[string]any{{"tag": "pool", "selector": []string{"old-"}}}
		}},
		{"balancer fallback", "old-a", func(c map[string]any) {
			objectMap(c["routing"])["balancers"] = []map[string]any{{"tag": "pool", "fallbackTag": "old-a"}}
		}},
		{"proxy chain", "old-a", func(c map[string]any) {
			objectSlice(c["outbounds"])[3]["streamSettings"] = map[string]any{"sockopt": map[string]any{"dialerProxy": "old-a"}}
		}},
		{"legacy proxy chain", "old-a", func(c map[string]any) {
			objectSlice(c["outbounds"])[3]["proxySettings"] = map[string]any{"tag": "old-a"}
		}},
		{"observatory", "old-a", func(c map[string]any) { c["observatory"] = map[string]any{"subjectSelector": []string{"old-"}} }},
		{"missing", "gone", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := outboundDeletionTestConfig()
			if tc.mutate != nil {
				tc.mutate(config)
			}
			panel := &routingDeletePanel{config: config}
			before := model.RoutingFingerprint(config)
			if _, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteOutbounds, Payload: outboundDeletionTestPayload(config, "old-b", tc.tag)}); err == nil || panel.updates != 0 || model.RoutingFingerprint(panel.config) != before {
				t.Fatalf("unsafe batch: err=%v writes=%d", err, panel.updates)
			}
		})
	}
}

func TestOutboundDeletionChangedTemplateAndReloadFailure(t *testing.T) {
	config := outboundDeletionTestConfig()
	payload := outboundDeletionTestPayload(config, "old-a")
	objectMap(config["log"])["loglevel"] = "debug"
	panel := &routingDeletePanel{config: config}
	if _, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteOutbounds, Payload: payload}); err == nil || panel.updates != 0 {
		t.Fatal("stale template was saved")
	}
	panel.restartError = "restart unavailable"
	result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteOutbounds, Payload: outboundDeletionTestPayload(config, "old-a")})
	if err == nil || !strings.Contains(err.Error(), "不要重复删除") || result["saved"] != true || result["verified"] != true || result["restarted"] != false {
		t.Fatalf("partial result lost: %#v %v", result, err)
	}
}

func TestOutboundDeletionDoesNotTrustPanelSuccessEnvelope(t *testing.T) {
	config := outboundDeletionTestConfig()
	panel := &routingDeletePanel{config: config, ignoreUpdate: true}
	result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteOutbounds, Payload: outboundDeletionTestPayload(config, "old-a", "old-b")})
	if err == nil || result["verified"] != false || result["deleted_count"] != 0 || panel.updates != 1 || panel.restarts != 0 {
		t.Fatalf("false success: %#v err=%v", result, err)
	}
	panel.ignoreUpdate = false
	panel.updateError = "save unavailable"
	result, err = panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteOutbounds, Payload: outboundDeletionTestPayload(config, "old-a")})
	if err == nil || result != nil || panel.restarts != 0 {
		t.Fatal("failed save must not restart or claim completion")
	}
}

func TestOutboundDeletionResolvesErrorAfterPanelPersistWithoutReplay(t *testing.T) {
	config := outboundDeletionTestConfig()
	panel := &routingDeletePanel{config: config, updatePersistError: "panel hot apply failed after save"}
	result, err := panel.client(t).ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionDeleteOutbounds, Payload: outboundDeletionTestPayload(config, "old-a", "old-b")})
	if err != nil || result["saved"] != true || result["verified"] != true || result["restarted"] != true || panel.updates != 1 || panel.restarts != 1 {
		t.Fatalf("persisted result not reconciled: %#v %v", result, err)
	}
}
