package panels

import (
	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestTrafficResetOnlyTouchesPeriodCounters(t *testing.T) {
	for _, scenario := range []string{"enabled", "quota-disabled", "shadowsocks", "manual-disabled", "expired", "changed-quota", "duplicate-email", "journal-failed", "lost-response", "historical-counter-reset", "reset-noop"} {
		t.Run(scenario, func(t *testing.T) {
			expiry := time.Now().Add(time.Hour).UnixMilli()
			if scenario == "expired" {
				expiry = time.Now().Add(-time.Hour).UnixMilli()
			}
			clientConfig := map[string]any{"id": "uuid", "email": "alice", "enable": scenario != "manual-disabled", "totalGB": int64(200), "expiryTime": expiry, "subId": "untouched", "flow": "xtls-rprx-vision"}
			protocol, identity := "vless", "uuid"
			if scenario == "shadowsocks" {
				protocol = "shadowsocks"
				delete(clientConfig, "id")
				clientConfig["password"] = "secret-password"
				identity = model.TrafficResetClientIdentity(protocol, "", "secret-password")
			}
			beforeConfig := fmt.Sprint(clientConfig)
			stats := map[string]any{"email": "alice", "enable": scenario != "quota-disabled", "up": int64(90), "down": int64(110), "total": int64(200), "expiryTime": expiry, "allTime": int64(900)}
			inbound := map[string]any{"id": 1, "enable": true, "protocol": protocol, "settings": map[string]any{"clients": []map[string]any{clientConfig}}, "clientStats": []map[string]any{stats}}
			inbounds := []map[string]any{inbound}
			if scenario == "duplicate-email" {
				inbounds = append(inbounds, inbound)
			}
			c, err := NewXUIClient(config.XUIConfig{Enabled: true, BaseURL: "https://xui.local", APIToken: "token"}, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			posts := 0
			journals := 0
			c.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/panel/api/inbounds/list" {
					return jsonResponse(t, req, map[string]any{"success": true, "obj": inbounds}), nil
				}
				if req.URL.Path == "/panel/api/inbounds/1/resetClientTraffic/alice" {
					posts++
					if journals != 1 {
						t.Fatal("mutated before journal persisted")
					}
					stats["up"] = int64(0)
					stats["down"] = int64(0)
					stats["enable"] = true
					if scenario == "reset-noop" {
						stats["up"] = int64(90)
						stats["down"] = int64(110)
					}
					if scenario == "historical-counter-reset" {
						stats["allTime"] = int64(0)
					}
					if scenario == "lost-response" {
						return nil, fmt.Errorf("connection lost after reset")
					}
					return jsonResponse(t, req, map[string]any{"success": true}), nil
				}
				t.Fatalf("unexpected request: %s", req.URL.Path)
				return nil, nil
			})}
			total := int64(200)
			if scenario == "changed-quota" {
				total = 201
			}
			result, err := c.ResetClientPeriodTraffic(context.Background(), map[string]any{"inbound_id": 1, "client_id": identity, "email": "alice", "expected_protocol": protocol, "expected_total": total, "expected_expiry": expiry, "not_before": time.Now().Add(-time.Minute).UnixMilli(), "deadline": time.Now().Add(time.Minute).UnixMilli()}, func() error {
				journals++
				if scenario == "journal-failed" {
					return fmt.Errorf("disk full")
				}
				return nil
			})
			success := scenario == "enabled" || scenario == "quota-disabled" || scenario == "shadowsocks"
			mutated := success || scenario == "lost-response" || scenario == "historical-counter-reset" || scenario == "reset-noop"
			if (err == nil) != success {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if (posts == 1) != mutated {
				t.Fatalf("unexpected mutations: %d %v", posts, err)
			}
			if beforeConfig != fmt.Sprint(clientConfig) {
				t.Fatal("client settings changed")
			}
			if success && (!reflect.DeepEqual(stats["allTime"], int64(900)) || stats["up"] != int64(0) || stats["down"] != int64(0) || stats["enable"] != true) {
				t.Fatalf("bad counters: %#v", stats)
			}
			if scenario == "lost-response" && result["uncertain"] != true {
				t.Fatal("lost acknowledgement treated as safe retry")
			}
		})
	}
}
