package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/panels"
)

func TestRoutingDeletionTimeoutAndPartialResult(t *testing.T) {
	rules := []map[string]any{{"outboundTag": "direct", "user": []string{"alice"}}}
	updates := 0
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/panel/xray/":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": map[string]any{"xraySetting": map[string]any{"routing": map[string]any{"rules": rules}}}})
		case "/panel/xray/update":
			updates++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case "/panel/api/server/restartXrayService":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "msg": "restart failed"})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer panel.Close()
	cfg := config.XUIConfig{Enabled: true, BaseURL: panel.URL, APIToken: "test-token"}
	xui, err := panels.NewXUIClient(cfg, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{requestTimeout: time.Second}
	if timeout := app.xuiActionTimeout(model.XUIActionDeleteRoutingRules); timeout < 90*time.Second {
		t.Fatalf("short timeout: %s", timeout)
	}
	result := app.executeXUIAction(context.Background(), model.ManagedAgentConfig{XUI: cfg}, xui, nil, model.XUIAction{
		Kind: model.XUIActionDeleteRoutingRules,
		Payload: map[string]any{
			"rules_fingerprint": model.RoutingFingerprint(rules),
			"rules":             []model.RoutingRuleDeleteTarget{{Index: 1, Fingerprint: model.RoutingFingerprint(rules[0])}},
		},
	})
	if result.Status != model.XUIActionStatusFailed || result.Result["saved"] != true || result.Result["restarted"] != false || !strings.Contains(result.Error, "已保存") {
		t.Fatalf("partial save must reach the Server operation log: %#v", result)
	}
	if updates != 1 {
		t.Fatalf("destructive update retried %d times", updates)
	}
}
