package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func TestXUIClientForDoesNotCacheAPIToken(t *testing.T) {
	app := &App{requestTimeout: time.Second}
	base := config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.example/panel",
		Username: "admin",
		Password: "password",
	}
	first, err := app.xuiClientFor(base)
	if err != nil {
		t.Fatalf("create cached x-ui client: %v", err)
	}
	second, err := app.xuiClientFor(base)
	if err != nil {
		t.Fatalf("reuse cached x-ui client: %v", err)
	}
	if first != second {
		t.Fatal("x-ui client without an API token should be reused")
	}

	withToken := base
	withToken.APIToken = "server-token"
	tokenFirst, err := app.xuiClientFor(withToken)
	if err != nil {
		t.Fatalf("create first token x-ui client: %v", err)
	}
	tokenSecond, err := app.xuiClientFor(withToken)
	if err != nil {
		t.Fatalf("create second token x-ui client: %v", err)
	}
	if tokenFirst == tokenSecond {
		t.Fatal("x-ui clients carrying API tokens must be one-use instances")
	}
}

func TestXUIClientForActionUsesEphemeralAuth(t *testing.T) {
	app := &App{requestTimeout: time.Second}
	cfg := config.XUIConfig{Enabled: true, BaseURL: "https://xui.example", APIToken: "stale-token"}
	auth := &model.XUIActionAuth{APIToken: "latest-token"}

	first, err := app.xuiClientForAction(cfg, auth)
	if err != nil {
		t.Fatalf("create first action x-ui client: %v", err)
	}
	second, err := app.xuiClientForAction(cfg, auth)
	if err != nil {
		t.Fatalf("create second action x-ui client: %v", err)
	}
	if first == second {
		t.Fatal("each action must receive a one-use x-ui client")
	}
}

func TestXUIClientForActionSendsLatestEphemeralToken(t *testing.T) {
	const latestToken = "latest-action-token"
	requestCount := 0
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("Authorization"); got != "Bearer "+latestToken {
			t.Errorf("unexpected Authorization header: %q", got)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/panel/api/inbounds/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"obj": []map[string]any{{
					"id":       1,
					"tag":      "inbound-1",
					"protocol": "vless",
					"settings": `{"clients":[{"id":"stable-uuid","email":"client@example.com","enable":true,"expiryTime":0}]}`,
				}},
			})
		case "/panel/api/inbounds/update/1":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "msg": "updated"})
		default:
			t.Errorf("unexpected x-ui request path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer panel.Close()

	app := &App{requestTimeout: time.Second}
	cfg := config.XUIConfig{Enabled: true, BaseURL: panel.URL, APIToken: "stale-config-token"}
	xuiClient, err := app.xuiClientForAction(cfg, &model.XUIActionAuth{APIToken: latestToken})
	if err != nil {
		t.Fatalf("create action x-ui client: %v", err)
	}
	_, err = xuiClient.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionUpdateClientExpiry,
		Payload: map[string]any{
			"inbound_id":  1,
			"email":       "client@example.com",
			"expiry_time": int64(1896048000000),
		},
	})
	if err != nil {
		t.Fatalf("execute x-ui action: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected list and update requests, got %d", requestCount)
	}
}
