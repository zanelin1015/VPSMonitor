package panels

import (
	"context"
	"net/http"
	"testing"
	"time"

	"bridge-core/internal/config"
)

func TestNezhaCollect(t *testing.T) {
	client := NewNezhaClient(config.NezhaConfig{
		Enabled:    true,
		BaseURL:    "https://nezha.local",
		Username:   "admin",
		Password:   "pass",
		ServerUUID: "uuid-b",
	}, 5*time.Second)

	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/v1/login":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"data": map[string]any{
						"token": "jwt-token",
					},
				}), nil
			case "/api/v1/server":
				if req.Header.Get("Authorization") != "Bearer jwt-token" {
					t.Fatalf("missing authorization header")
				}
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"data": []map[string]any{
						{"id": 7, "name": "sg-01", "uuid": "uuid-a"},
						{
							"id":   9,
							"name": "hk-01",
							"uuid": "uuid-b",
							"geoip": map[string]any{
								"ip": map[string]any{
									"ipv4_addr": "2.2.2.2",
								},
							},
						},
					},
				}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" {
		t.Fatalf("Collect returned error: %s", snapshot.Error)
	}
	if snapshot.ServerID != 9 {
		t.Fatalf("expected server id 9, got %d", snapshot.ServerID)
	}
	if snapshot.ServerName != "hk-01" {
		t.Fatalf("unexpected server name: %s", snapshot.ServerName)
	}
}
