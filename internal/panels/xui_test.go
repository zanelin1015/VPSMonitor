package panels

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func TestXUICollect(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"msg":     "ok",
				}), nil
			case "/panel/api/server/status":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"cpu":    13.5,
						"uptime": 100,
						"public_ip": map[string]any{
							"ipv4": "1.1.1.1",
						},
						"mem": map[string]any{
							"current": 256,
							"total":   1024,
						},
						"xray": map[string]any{
							"state":   "running",
							"version": "1.8.0",
						},
					},
				}), nil
			case "/panel/api/inbounds/list":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": []map[string]any{
						{"id": 1, "remark": "hk-01"},
						{"id": 2, "remark": "jp-01"},
					},
				}), nil
			case "/panel/api/clients/list":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
			case "/panel/api/server/getConfigJson":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"outbounds": []map[string]any{
							{"tag": "direct", "protocol": "freedom"},
							{"tag": "relay-hk", "protocol": "vless"},
						},
						"routing": map[string]any{
							"rules": []map[string]any{
								{"outboundTag": "relay-hk"},
							},
						},
					},
				}), nil
			case "/panel/xray/getOutboundsTraffic":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": []map[string]any{
						{"tag": "relay-hk", "up": 10, "down": 20},
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
	if got := len(snapshot.Inbounds); got != 2 {
		t.Fatalf("expected 2 inbounds, got %d", got)
	}
	if got := len(snapshot.Outbounds); got != 2 {
		t.Fatalf("expected 2 outbounds, got %d", got)
	}
	if got := len(snapshot.RoutingRules); got != 1 {
		t.Fatalf("expected 1 route rule, got %d", got)
	}
	if snapshot.ServerStatus.PublicIP.IPv4 != "1.1.1.1" {
		t.Fatalf("unexpected ipv4: %s", snapshot.ServerStatus.PublicIP.IPv4)
	}
}

func TestXUIClientUsesBearerAPIToken(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		APIToken: "api-token",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/login" {
				t.Fatalf("api token auth should not call /login")
			}
			if got := req.Header.Get("Authorization"); got != "Bearer api-token" {
				t.Fatalf("unexpected Authorization header %q", got)
			}
			return jsonResponse(t, req, map[string]any{
				"success": true,
				"obj": map[string]any{
					"cpu":  1,
					"xray": map[string]any{"state": "running"},
				},
			}), nil
		}),
	}
	if err := client.ensureLogin(context.Background()); err != nil {
		t.Fatalf("ensureLogin with api token: %v", err)
	}
	if _, err := client.getStatus(context.Background()); err != nil {
		t.Fatalf("getStatus with api token: %v", err)
	}
}

func TestXUICollectMerges3XUIV3Clients(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local/HK/",
		APIToken: "api-token",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("Authorization"); got != "Bearer api-token" {
				t.Fatalf("unexpected Authorization header %q", got)
			}
			switch req.URL.Path {
			case "/HK/panel/api/server/status":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"xray": map[string]any{"state": "running"}}}), nil
			case "/HK/panel/api/inbounds/list":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": []map[string]any{{
						"id":       7,
						"tag":      "in-443",
						"settings": map[string]any{"clients": []any{}},
					}},
				}), nil
			case "/HK/panel/api/clients/list":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": []map[string]any{{
						"email":      "alice@example.com",
						"uuid":       "uuid-1",
						"inboundIds": []int{7},
						"enable":     true,
						"up":         11,
						"down":       22,
					}},
				}), nil
			case "/HK/panel/api/server/getConfigJson":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"outbounds": []map[string]any{}, "routing": map[string]any{"rules": []map[string]any{}}}}), nil
			case "/HK/panel/xray/":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": `{"xraySetting":"{\"outbounds\":[],\"routing\":{\"rules\":[]}}"}`}), nil
			case "/HK/panel/xray/getOutboundsTraffic":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
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
	settings, _, err := decodeInboundSettings(snapshot.Inbounds[0]["settings"])
	if err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	clients := objectSlice(settings["clients"])
	if len(clients) != 1 || clients[0]["email"] != "alice@example.com" || clients[0]["id"] != "uuid-1" {
		t.Fatalf("expected v3 client merged into inbound settings, got %#v", clients)
	}
	stats := objectSlice(snapshot.Inbounds[0]["clientStats"])
	if len(stats) != 1 || stats[0]["email"] != "alice@example.com" || intValue(stats[0]["up"]) != 11 {
		t.Fatalf("expected v3 client traffic merged, got %#v", stats)
	}
}

func TestXUICollectIgnoresInvalidOutboundTrafficResponse(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	loginCount := 0
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: xuiCollectTransport(t, &loginCount, func(req *http.Request) *http.Response {
			if req.URL.Path != "/panel/xray/getOutboundsTraffic" {
				return nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader("<html>not json</html>")),
				Request:    req,
			}
		}),
	}
	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" {
		t.Fatalf("Collect returned error: %s", snapshot.Error)
	}
	if got := len(snapshot.Inbounds); got != 0 {
		t.Fatalf("expected normal collection to continue, got %d inbounds", got)
	}
	if got := len(snapshot.OutboundTraffic); got != 0 {
		t.Fatalf("expected outbound traffic to be skipped, got %d", got)
	}
}

func TestXUICollectFallsBackToXrayTemplateWhenServerConfigIsEmpty(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	configBody, err := json.Marshal(map[string]any{
		"outbounds": []map[string]any{
			{"tag": "direct", "protocol": "freedom"},
			{"tag": "blocked", "protocol": "blackhole"},
		},
		"routing": map[string]any{
			"rules": []map[string]any{
				{"outboundTag": "direct"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal config body: %v", err)
	}
	wrapperBody, err := json.Marshal(map[string]any{"xraySetting": string(configBody)})
	if err != nil {
		t.Fatalf("marshal wrapper body: %v", err)
	}

	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/server/status":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"cpu":  1,
						"mem":  map[string]any{"current": 1, "total": 2},
						"xray": map[string]any{"state": "running"},
					},
				}), nil
			case "/panel/api/inbounds/list":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 1, "remark": "ph"}}}), nil
			case "/panel/api/clients/list":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
			case "/panel/api/server/getConfigJson":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj":     map[string]any{},
				}), nil
			case "/panel/xray/", "/panel/api/xray/":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj":     string(wrapperBody),
				}), nil
			case "/panel/xray/getOutboundsTraffic":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
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
	if got := len(snapshot.Outbounds); got != 2 {
		t.Fatalf("expected fallback to collect 2 outbounds, got %d", got)
	}
	if got := len(snapshot.RoutingRules); got != 1 {
		t.Fatalf("expected fallback to collect 1 rule, got %d", got)
	}
}

func TestXUICollectMergesXrayTemplateWhenServerConfigMissesRoutingRules(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	configBody, err := json.Marshal(map[string]any{
		"outbounds": []map[string]any{
			{"tag": "direct", "protocol": "freedom"},
			{"tag": "blocked", "protocol": "blackhole"},
		},
		"routing": map[string]any{
			"rules": []map[string]any{
				{"inboundTag": []string{"api"}, "outboundTag": "api"},
				{"ip": []string{"geoip:private"}, "outboundTag": "blocked"},
				{"protocol": []string{"bittorrent"}, "outboundTag": "blocked"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal config body: %v", err)
	}
	wrapperBody, err := json.Marshal(map[string]any{"xraySetting": string(configBody)})
	if err != nil {
		t.Fatalf("marshal wrapper body: %v", err)
	}

	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/server/status":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"cpu":  1,
						"mem":  map[string]any{"current": 1, "total": 2},
						"xray": map[string]any{"state": "running"},
					},
				}), nil
			case "/panel/api/inbounds/list":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 1, "remark": "ph"}}}), nil
			case "/panel/api/clients/list":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
			case "/panel/api/server/getConfigJson":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"outbounds": []map[string]any{
							{"tag": "direct", "protocol": "freedom"},
							{"tag": "blocked", "protocol": "blackhole"},
						},
						"routing": map[string]any{"rules": []map[string]any{}},
					},
				}), nil
			case "/panel/xray/":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj":     string(wrapperBody),
				}), nil
			case "/panel/xray/getOutboundsTraffic":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
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
	if got := len(snapshot.Outbounds); got != 2 {
		t.Fatalf("expected primary config to keep 2 outbounds, got %d", got)
	}
	if got := len(snapshot.RoutingRules); got != 3 {
		t.Fatalf("expected fallback to merge 3 routing rules, got %d", got)
	}
}

func TestXUICollectReusesSessionCookie(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	loginCount := 0
	client.client = &http.Client{
		Timeout:   5 * time.Second,
		Transport: xuiCollectTransport(t, &loginCount, nil),
	}

	for i := 0; i < 2; i++ {
		snapshot := client.Collect(context.Background())
		if snapshot.Error != "" {
			t.Fatalf("Collect #%d returned error: %s", i+1, snapshot.Error)
		}
	}
	if loginCount != 1 {
		t.Fatalf("expected one x-ui login for cached session, got %d", loginCount)
	}
}

func TestXUICollectNormalizesPanelBaseURLBeforeLogin(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local/secret/panel/inbounds",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	loginCount := 0
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/secret/login":
				loginCount++
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/secret/panel/api/server/status":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"cpu":  1,
						"mem":  map[string]any{"current": 1, "total": 2},
						"xray": map[string]any{"state": "running"},
					},
				}), nil
			case "/secret/panel/api/inbounds/list":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
			case "/secret/panel/api/clients/list":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
			case "/secret/panel/api/server/getConfigJson":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj":     map[string]any{"outbounds": []map[string]any{}, "routing": map[string]any{"rules": []map[string]any{}}},
				}), nil
			case "/secret/panel/xray/":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj":     `{"xraySetting":"{\"outbounds\":[],\"routing\":{\"rules\":[]}}"}`,
				}), nil
			case "/secret/panel/xray/getOutboundsTraffic":
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
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
	if loginCount != 1 {
		t.Fatalf("expected one login, got %d", loginCount)
	}
	if snapshot.BaseURL != "https://xui.local/secret" {
		t.Fatalf("unexpected normalized base URL: %s", snapshot.BaseURL)
	}
}

func TestXUICollectReloginsAfterAuthFailure(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	loginCount := 0
	statusCount := 0
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: xuiCollectTransport(t, &loginCount, func(req *http.Request) *http.Response {
			if req.URL.Path == "/panel/api/server/status" {
				statusCount++
				if statusCount == 1 {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader([]byte("session expired"))),
						Request:    req,
					}
				}
			}
			return nil
		}),
	}

	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" {
		t.Fatalf("Collect returned error: %s", snapshot.Error)
	}
	if loginCount != 2 {
		t.Fatalf("expected login then relogin, got %d", loginCount)
	}
}

func TestXUICollectReloginsAfterAPI404(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	loginCount := 0
	statusCount := 0
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: xuiCollectTransport(t, &loginCount, func(req *http.Request) *http.Response {
			if req.URL.Path == "/panel/api/server/status" {
				statusCount++
				if statusCount == 1 {
					return &http.Response{
						StatusCode: http.StatusNotFound,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader([]byte("404 page not found"))),
						Request:    req,
					}
				}
			}
			return nil
		}),
	}

	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" {
		t.Fatalf("Collect returned error: %s", snapshot.Error)
	}
	if loginCount != 2 {
		t.Fatalf("expected login then relogin after API 404, got %d", loginCount)
	}
}

func TestXUICollectUsesLocalDBWithoutLogin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	createLocalXUITestDB(t, dbPath)

	client, err := NewXUIClient(config.XUIConfig{
		Enabled: true,
		BaseURL: "https://xui.local/secret/",
		DBPath:  dbPath,
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	client.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("local DB collection should not perform HTTP request: %s", req.URL.Path)
			return nil, nil
		}),
	}

	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" {
		t.Fatalf("Collect returned error: %s", snapshot.Error)
	}
	if got := len(snapshot.Inbounds); got != 1 {
		t.Fatalf("expected 1 inbound, got %d", got)
	}
	if got := len(snapshot.Outbounds); got != 1 {
		t.Fatalf("expected 1 outbound, got %d", got)
	}
	if got := len(snapshot.RoutingRules); got != 1 {
		t.Fatalf("expected 1 route rule, got %d", got)
	}
	if got := len(snapshot.OutboundTraffic); got != 1 {
		t.Fatalf("expected 1 outbound traffic row, got %d", got)
	}
	if snapshot.Inbounds[0]["remark"] != "local-hk" {
		t.Fatalf("unexpected inbound: %#v", snapshot.Inbounds[0])
	}
	stats, ok := snapshot.Inbounds[0]["clientStats"].([]map[string]any)
	if !ok || len(stats) != 1 || stats[0]["email"] != "alice" {
		t.Fatalf("unexpected client stats: %#v", snapshot.Inbounds[0]["clientStats"])
	}
}

func TestXUICollectPrefersAPIBeforeLocalDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "broken-x-ui.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE inbounds (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create broken db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	loginCount := 0
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
		DBPath:   dbPath,
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	client.client = &http.Client{
		Timeout:   5 * time.Second,
		Transport: xuiCollectTransport(t, &loginCount, nil),
	}

	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" {
		t.Fatalf("Collect returned error: %s", snapshot.Error)
	}
	if loginCount != 1 {
		t.Fatalf("expected API login before local DB fallback, got %d", loginCount)
	}
	if snapshot.Inbounds == nil {
		t.Fatalf("expected API inbounds to be collected")
	}
}

func TestXUICollectLocalDBSupports3XUISchemaWithoutAllTime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui-v3.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE inbounds (
			id INTEGER PRIMARY KEY, user_id INTEGER, up INTEGER, down INTEGER, total INTEGER,
			remark TEXT, enable BOOLEAN, expiry_time INTEGER, traffic_reset TEXT, last_traffic_reset_time INTEGER,
			listen TEXT, port INTEGER, protocol TEXT, settings TEXT, stream_settings TEXT, tag TEXT, sniffing TEXT
		)`,
		`CREATE TABLE client_traffics (
			id INTEGER PRIMARY KEY, inbound_id INTEGER, enable BOOLEAN, email TEXT, up INTEGER, down INTEGER,
			expiry_time INTEGER, total INTEGER, reset INTEGER, last_online INTEGER
		)`,
		`CREATE TABLE outbound_traffics (id INTEGER PRIMARY KEY, tag TEXT, up INTEGER, down INTEGER, total INTEGER)`,
		`CREATE TABLE settings (id INTEGER PRIMARY KEY, key TEXT, value TEXT)`,
		`INSERT INTO inbounds
			(id, user_id, up, down, total, remark, enable, expiry_time, traffic_reset, last_traffic_reset_time,
			 listen, port, protocol, settings, stream_settings, tag, sniffing)
			VALUES
			(1, 1, 10, 20, 0, 'v3-hk', 1, 0, 'never', 0, '', 443, 'vless',
			 '{"clients":[{"id":"uuid-1","email":"alice","enable":true}]}',
			 '{"network":"tcp","security":"none"}', 'inbound-443', '{}')`,
		`INSERT INTO client_traffics
			(id, inbound_id, enable, email, up, down, expiry_time, total, reset, last_online)
			VALUES (1, 1, 1, 'alice', 1, 2, 0, 0, 0, 123456)`,
		`INSERT INTO outbound_traffics (id, tag, up, down, total) VALUES (1, 'direct', 5, 6, 11)`,
		`INSERT INTO settings (id, key, value) VALUES (1, 'xrayTemplateConfig', '{"outbounds":[],"routing":{"rules":[]}}')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec schema/data: %v\n%s", err, stmt)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	client, err := NewXUIClient(config.XUIConfig{Enabled: true, DBPath: dbPath}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	snapshot := client.Collect(context.Background())
	if snapshot.Error != "" {
		t.Fatalf("Collect returned error: %s", snapshot.Error)
	}
	if got := snapshot.Inbounds[0]["allTime"]; got != int64(0) {
		t.Fatalf("expected missing inbound all_time to default to 0, got %#v", got)
	}
	stats := snapshot.Inbounds[0]["clientStats"].([]map[string]any)
	if got := stats[0]["allTime"]; got != int64(0) {
		t.Fatalf("expected missing client all_time to default to 0, got %#v", got)
	}
}

func TestXUICollectLocalDBEnrichesMissingXrayTemplateFromAPI(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	createLocalXUITestDB(t, dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`UPDATE settings SET value = '{"outbounds":[],"routing":{"rules":[]}}' WHERE key = 'xrayTemplateConfig'`); err != nil {
		t.Fatalf("clear local xray template: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	configBody, err := json.Marshal(map[string]any{
		"outbounds": []map[string]any{
			{"tag": "direct", "protocol": "freedom"},
			{"tag": "blocked", "protocol": "blackhole"},
		},
		"routing": map[string]any{
			"rules": []map[string]any{
				{"outboundTag": "direct"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal config body: %v", err)
	}
	wrapperBody, err := json.Marshal(map[string]any{"xraySetting": string(configBody)})
	if err != nil {
		t.Fatalf("marshal wrapper body: %v", err)
	}

	loginCount := 0
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local/secret/",
		Username: "admin",
		Password: "pass",
		DBPath:   dbPath,
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/secret/login":
				loginCount++
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/secret/panel/api/server/status":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"cpu":  1,
						"mem":  map[string]any{"current": 1, "total": 2},
						"xray": map[string]any{"state": "running"},
					},
				}), nil
			case "/secret/panel/api/inbounds/list":
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("api unavailable")),
					Request:    req,
				}, nil
			case "/secret/panel/api/server/getConfigJson":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj":     map[string]any{"outbounds": []map[string]any{}, "routing": map[string]any{"rules": []map[string]any{}}},
				}), nil
			case "/secret/panel/xray/":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj":     string(wrapperBody),
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
	if loginCount != 1 {
		t.Fatalf("expected one login, got %d", loginCount)
	}
	if got := len(snapshot.Outbounds); got != 2 {
		t.Fatalf("expected enriched 2 outbounds, got %d", got)
	}
	if got := len(snapshot.RoutingRules); got != 1 {
		t.Fatalf("expected enriched 1 routing rule, got %d", got)
	}
}

func TestXUIExecuteAddOutbound(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	var updatedConfig map[string]any
	restarted := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"log":       map[string]any{"loglevel": "warning"},
						"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}},
						"routing":   map[string]any{"rules": []map[string]any{}},
					},
					"inboundTags":     []string{},
					"outboundTestUrl": "https://www.google.com/generate_204",
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				if err := req.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if err := json.Unmarshal([]byte(req.Form.Get("xraySetting")), &updatedConfig); err != nil {
					t.Fatalf("decode updated xraySetting: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				restarted = true
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddOutbound,
		Payload: map[string]any{
			"outbound": map[string]any{
				"tag":      "relay-hk",
				"protocol": "freedom",
				"settings": map[string]any{},
			},
			"restart": false,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["outbound_tag"] != "relay-hk" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !restarted {
		t.Fatalf("expected xray restart even when payload restart=false")
	}
	outbounds, ok := updatedConfig["outbounds"].([]any)
	if !ok || len(outbounds) != 2 {
		t.Fatalf("expected two outbounds in updated config: %#v", updatedConfig["outbounds"])
	}
	got, ok := outbounds[1].(map[string]any)
	if !ok || got["tag"] != "relay-hk" {
		t.Fatalf("expected appended relay-hk outbound, got %#v", outbounds[1])
	}
}

func createLocalXUITestDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE inbounds (
			id INTEGER PRIMARY KEY, user_id INTEGER, up INTEGER, down INTEGER, total INTEGER, all_time INTEGER,
			remark TEXT, enable BOOLEAN, expiry_time INTEGER, traffic_reset TEXT, last_traffic_reset_time INTEGER,
			listen TEXT, port INTEGER, protocol TEXT, settings TEXT, stream_settings TEXT, tag TEXT, sniffing TEXT
		)`,
		`CREATE TABLE client_traffics (
			id INTEGER PRIMARY KEY, inbound_id INTEGER, enable BOOLEAN, email TEXT, up INTEGER, down INTEGER,
			all_time INTEGER, expiry_time INTEGER, total INTEGER, reset INTEGER, last_online INTEGER
		)`,
		`CREATE TABLE outbound_traffics (id INTEGER PRIMARY KEY, tag TEXT, up INTEGER, down INTEGER, total INTEGER)`,
		`CREATE TABLE settings (id INTEGER PRIMARY KEY, key TEXT, value TEXT)`,
		`INSERT INTO inbounds
			(id, user_id, up, down, total, all_time, remark, enable, expiry_time, traffic_reset, last_traffic_reset_time,
			 listen, port, protocol, settings, stream_settings, tag, sniffing)
			VALUES
			(1, 1, 10, 20, 0, 30, 'local-hk', 1, 0, 'never', 0, '', 443, 'vless',
			 '{"clients":[{"id":"uuid-1","email":"alice","enable":true}]}',
			 '{"network":"tcp","security":"none"}', 'inbound-443', '{}')`,
		`INSERT INTO client_traffics
			(id, inbound_id, enable, email, up, down, all_time, expiry_time, total, reset, last_online)
			VALUES (1, 1, 1, 'alice', 1, 2, 3, 0, 0, 0, 123456)`,
		`INSERT INTO outbound_traffics (id, tag, up, down, total) VALUES (1, 'direct', 5, 6, 11)`,
	}
	configJSON := `{"outbounds":[{"tag":"direct","protocol":"freedom"}],"routing":{"rules":[{"outboundTag":"direct"}]}}`
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec schema/data: %v\n%s", err, stmt)
		}
	}
	if _, err := db.Exec(`INSERT INTO settings (id, key, value) VALUES (1, 'xrayTemplateConfig', ?)`, configJSON); err != nil {
		t.Fatalf("insert xray template: %v", err)
	}
}

func TestXUIExecuteAddRoutingRuleForcesRestart(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	restarted := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}},
						"routing":   map[string]any{"rules": []map[string]any{}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				restarted = true
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{
				"type":        "field",
				"inboundTag":  []string{"in-a"},
				"outboundTag": "direct",
			},
			"restart": false,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["restarted"] != true {
		t.Fatalf("expected restarted result, got %#v", result)
	}
	if !restarted {
		t.Fatalf("expected xray restart after adding routing rule")
	}
}

func TestXUIExecuteRoutingRuleChecksCookieAndReloginsBeforeUpdate(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	client.authenticated = true

	loginCount := 0
	statusCount := 0
	templateCount := 0
	updateCount := 0
	restartCount := 0
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/panel/api/server/status":
				statusCount++
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("404 page not found"))),
					Request:    req,
				}, nil
			case "/login":
				loginCount++
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				templateCount++
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}},
						"routing":   map[string]any{"rules": []map[string]any{}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				updateCount++
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				restartCount++
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{
				"type":        "field",
				"inboundTag":  []string{"in-a"},
				"outboundTag": "direct",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["rule_index"] != 1 || result["restarted"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
	if statusCount != 1 || loginCount != 1 || templateCount != 1 || updateCount != 1 || restartCount != 1 {
		t.Fatalf("expected status/login/template/update/restart once, got status=%d login=%d template=%d update=%d restart=%d", statusCount, loginCount, templateCount, updateCount, restartCount)
	}
}

func TestXUIExecuteFallsBackToLocalDBWhenTemplateAPI404(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	createLocalXUITestDB(t, dbPath)

	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		DBPath:   dbPath,
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	restarted := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/", "/panel/api/xray/":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("404 page not found"))),
					Request:    req,
				}, nil
			case "/panel/api/server/restartXrayService":
				restarted = true
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{
				"type":        "field",
				"inboundTag":  []string{"inbound-443"},
				"outboundTag": "direct",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["rule_index"] != 1 || result["restarted"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !restarted {
		t.Fatalf("expected xray restart after local db update")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	configJSON, err := readLocalXrayConfig(context.Background(), db)
	if err != nil {
		t.Fatalf("read local xray template: %v", err)
	}
	routing := configJSON["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("expected inserted local db rule, got %#v", rules)
	}
	first := rules[0].(map[string]any)
	if first["outboundTag"] != "direct" {
		t.Fatalf("expected new local db rule to be first, got %#v", first)
	}
}

func TestXUIExecuteFallsBackToLocalDBWhenLoginForbidden(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	createLocalXUITestDB(t, dbPath)

	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		DBPath:   dbPath,
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	restarted := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login", "/panel/xray/":
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("forbidden"))),
					Request:    req,
				}, nil
			case "/panel/api/server/restartXrayService":
				restarted = true
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{
				"type":        "field",
				"inboundTag":  []string{"inbound-443"},
				"outboundTag": "direct",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["rule_index"] != 1 || result["restarted"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !restarted {
		t.Fatalf("expected xray restart after local db update")
	}
}

func TestXUIExecuteUpsertRoutingRuleAddsOutboundAndRuleOnce(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	restartCount := 0
	updateCount := 0
	var updatedConfig map[string]any
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}},
						"routing":   map[string]any{"rules": []map[string]any{}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				updateCount++
				if err := req.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if err := json.Unmarshal([]byte(req.Form.Get("xraySetting")), &updatedConfig); err != nil {
					t.Fatalf("decode updated xraySetting: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				restartCount++
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"outbound": map[string]any{
				"tag":      "relay-my",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []map[string]any{
						{"address": "relay.example.com", "port": 443, "users": []map[string]any{{"id": "uuid"}}},
					},
				},
			},
			"rule": map[string]any{
				"type":        "field",
				"inboundTag":  []string{"in-a"},
				"outboundTag": "relay-my",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["rule_index"] != 1 || result["outbound_added"] != true || result["restarted"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
	if updateCount != 1 || restartCount != 1 {
		t.Fatalf("expected one template update and one restart, got update=%d restart=%d", updateCount, restartCount)
	}
	outbounds, ok := updatedConfig["outbounds"].([]any)
	if !ok || len(outbounds) != 2 {
		t.Fatalf("expected appended outbound: %#v", updatedConfig["outbounds"])
	}
	outbound, ok := outbounds[1].(map[string]any)
	if !ok {
		t.Fatalf("expected appended outbound object, got %#v", outbounds[1])
	}
	settings, ok := outbound["settings"].(map[string]any)
	if !ok {
		t.Fatalf("expected appended outbound settings object, got %#v", outbound["settings"])
	}
	if settings["address"] != "relay.example.com" || intValue(settings["port"]) != 443 {
		t.Fatalf("expected appended outbound address/port, got %#v", settings)
	}
	if settings["id"] != "uuid" {
		t.Fatalf("expected appended outbound id, got %#v", settings)
	}
	if _, exists := settings["vnext"]; exists {
		t.Fatalf("expected x-ui compatible vless settings without vnext, got %#v", settings)
	}
	routing := updatedConfig["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("expected appended rule: %#v", rules)
	}
}

func TestXUIExecuteUpsertRoutingRuleReusesEquivalentOutbound(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	var updatedConfig map[string]any
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{
							{"tag": "direct", "protocol": "freedom"},
							{
								"tag":      "relay-existing",
								"protocol": "vless",
								"settings": map[string]any{
									"address":    "relay.example.com",
									"port":       443,
									"id":         "uuid",
									"encryption": "none",
								},
							},
						},
						"routing": map[string]any{"rules": []map[string]any{}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				if err := req.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if err := json.Unmarshal([]byte(req.Form.Get("xraySetting")), &updatedConfig); err != nil {
					t.Fatalf("decode updated xraySetting: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"outbound": map[string]any{
				"tag":      "relay-new",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []map[string]any{
						{"address": "relay.example.com", "port": 443, "users": []map[string]any{{"id": "uuid"}}},
					},
				},
			},
			"rule": map[string]any{
				"type":        "field",
				"inboundTag":  []string{"in-a"},
				"outboundTag": "relay-new",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["outbound_reused"] != true || result["outbound_added"] != false || result["outbound_updated"] != false {
		t.Fatalf("expected equivalent outbound to be reused, got %#v", result)
	}
	outbounds := updatedConfig["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("expected no duplicate outbound, got %#v", outbounds)
	}
	routing := updatedConfig["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("expected one routing rule, got %#v", rules)
	}
	rule := rules[0].(map[string]any)
	if rule["outboundTag"] != "relay-existing" {
		t.Fatalf("expected rule to point existing outbound tag, got %#v", rule)
	}
}

func TestXUIValidateOutboundRejectsUndefinedEndpoint(t *testing.T) {
	err := validateOutboundConfig(map[string]any{
		"tag":      "relay-broken",
		"protocol": "vless",
		"settings": map[string]any{
			"vnext": []map[string]any{
				{"address": "undefined", "port": "undefined"},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected undefined endpoint to be rejected")
	}
}

func TestXUIValidateOutboundAcceptsFlatVLESSEndpoint(t *testing.T) {
	err := validateOutboundConfig(map[string]any{
		"tag":      "relay-flat",
		"protocol": "vless",
		"settings": map[string]any{
			"address":    "relay.example.com",
			"port":       443,
			"id":         "uuid",
			"encryption": "none",
		},
	})
	if err != nil {
		t.Fatalf("expected flat vless endpoint to be accepted, got %v", err)
	}
}

func TestXUIValidateOutboundRequiresRealitySettings(t *testing.T) {
	err := validateOutboundConfig(map[string]any{
		"tag":      "relay-reality",
		"protocol": "vless",
		"settings": map[string]any{
			"vnext": []map[string]any{
				{"address": "relay.example.com", "port": 443, "users": []map[string]any{{"id": "uuid"}}},
			},
		},
		"streamSettings": map[string]any{
			"network":  "tcp",
			"security": "reality",
		},
	})
	if err == nil {
		t.Fatalf("expected missing realitySettings to be rejected")
	}

	err = validateOutboundConfig(map[string]any{
		"tag":      "relay-reality",
		"protocol": "vless",
		"settings": map[string]any{
			"vnext": []map[string]any{
				{"address": "relay.example.com", "port": 443, "users": []map[string]any{{"id": "uuid"}}},
			},
		},
		"streamSettings": map[string]any{
			"network":  "tcp",
			"security": "reality",
			"realitySettings": map[string]any{
				"serverName":  "www.example.com",
				"fingerprint": "chrome",
				"publicKey":   "public-key",
				"shortId":     "abcd",
				"spiderX":     "/",
			},
		},
	})
	if err != nil {
		t.Fatalf("expected valid reality outbound, got %v", err)
	}
}

func TestXUIExecuteUpsertRoutingRuleUpdatesExistingRule(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	var updatedConfig map[string]any
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}, {"tag": "relay-us", "protocol": "freedom"}},
						"routing": map[string]any{"rules": []map[string]any{
							{"type": "field", "inboundTag": []string{"in-a"}, "outboundTag": "direct"},
							{"type": "field", "inboundTag": []string{"in-b"}, "outboundTag": "direct"},
						}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				if err := req.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if err := json.Unmarshal([]byte(req.Form.Get("xraySetting")), &updatedConfig); err != nil {
					t.Fatalf("decode updated xraySetting: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"rule_index": 2,
			"outbound": map[string]any{
				"tag":      "relay-us",
				"protocol": "freedom",
			},
			"rule": map[string]any{
				"type":        "field",
				"inboundTag":  []string{"in-b"},
				"outboundTag": "relay-us",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["updated"] != true || result["outbound_added"] != false || result["outbound_updated"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
	outbounds := updatedConfig["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("expected existing outbound to be updated in place, got %#v", outbounds)
	}
	updatedOutbound := outbounds[1].(map[string]any)
	if updatedOutbound["protocol"] != "freedom" {
		t.Fatalf("expected outbound relay-us to be replaced, got %#v", updatedOutbound)
	}
	routing := updatedConfig["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	got := rules[0].(map[string]any)
	if got["outboundTag"] != "relay-us" {
		t.Fatalf("expected updated rule to be moved to #1 and point relay-us, got %#v", got)
	}
	second := rules[1].(map[string]any)
	if second["outboundTag"] != "direct" {
		t.Fatalf("expected previous rule #1 to move down unchanged, got %#v", second)
	}
}

func TestXUIExecuteUpsertRoutingRuleDeduplicatesAndMovesExistingRuleFirst(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	var updatedConfig map[string]any
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{
							{"tag": "direct", "protocol": "freedom"},
							{"tag": "relay-us", "protocol": "freedom"},
						},
						"routing": map[string]any{"rules": []map[string]any{
							{"type": "field", "ip": []string{"geoip:private"}, "outboundTag": "blocked"},
							{"type": "field", "user": []string{"Akko"}, "outboundTag": "relay-us"},
						}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				if err := req.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if err := json.Unmarshal([]byte(req.Form.Get("xraySetting")), &updatedConfig); err != nil {
					t.Fatalf("decode updated xraySetting: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"rule": map[string]any{
				"type":        "field",
				"user":        []string{"Akko"},
				"outboundTag": "relay-us",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["updated"] != true || result["rule_index"] != 1 {
		t.Fatalf("expected existing rule updated at top, got %#v", result)
	}
	routing := updatedConfig["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("expected duplicate rule to be reused, got %#v", rules)
	}
	first := rules[0].(map[string]any)
	if first["outboundTag"] != "relay-us" {
		t.Fatalf("expected existing relay rule to move to #1, got %#v", first)
	}
	second := rules[1].(map[string]any)
	if second["outboundTag"] != "blocked" {
		t.Fatalf("expected previous first rule to move down, got %#v", second)
	}
}

func TestXUIExecuteUpsertRoutingRuleRenamesOutboundReferences(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	var updatedConfig map[string]any
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{
							{"tag": "relay-old", "protocol": "freedom"},
							{"tag": "direct", "protocol": "freedom"},
						},
						"routing": map[string]any{"rules": []map[string]any{
							{"type": "field", "user": []string{"Akko"}, "outboundTag": "relay-old"},
							{"type": "field", "inboundTag": []string{"in-a"}, "outboundTag": "relay-old"},
						}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				if err := req.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if err := json.Unmarshal([]byte(req.Form.Get("xraySetting")), &updatedConfig); err != nil {
					t.Fatalf("decode updated xraySetting: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "restarted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionUpsertRoutingRule,
		Payload: map[string]any{
			"previous_outbound_tag": "relay-old",
			"outbound": map[string]any{
				"tag":      "relay-new",
				"protocol": "freedom",
			},
			"rule": map[string]any{
				"type":        "field",
				"user":        []string{"Akko"},
				"outboundTag": "relay-new",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["routing_refs"] != 2 || result["outbound_removed"] != true {
		t.Fatalf("expected outbound references to be rewritten, got %#v", result)
	}
	outbounds := updatedConfig["outbounds"].([]any)
	for _, item := range outbounds {
		if item.(map[string]any)["tag"] == "relay-old" {
			t.Fatalf("expected old outbound to be removed, got %#v", outbounds)
		}
	}
	rules := updatedConfig["routing"].(map[string]any)["rules"].([]any)
	for _, item := range rules {
		if item.(map[string]any)["outboundTag"] != "relay-new" {
			t.Fatalf("expected routing rule to point to relay-new, got %#v", rules)
		}
	}
}

func TestXUIExecuteDeleteClientUsesDeleteAPI(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	deleteCalled := false
	routingUpdateCalled := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{{"tag": "relay", "protocol": "freedom"}},
						"routing": map[string]any{"rules": []map[string]any{
							{"type": "field", "user": []string{"alice@example.com", "bob@example.com"}, "outboundTag": "relay"},
						}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/xray/update":
				routingUpdateCalled = true
				form, _ := io.ReadAll(req.Body)
				values, _ := url.ParseQuery(string(form))
				var updated map[string]any
				if err := json.Unmarshal([]byte(values.Get("xraySetting")), &updated); err != nil {
					t.Fatalf("decode updated xray setting: %v", err)
				}
				rules := updated["routing"].(map[string]any)["rules"].([]any)
				users := rules[0].(map[string]any)["user"].([]any)
				if len(users) != 1 || users[0] != "bob@example.com" {
					t.Fatalf("expected alice to be removed from routing users, got %#v", users)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/clients/del/alice@example.com":
				deleteCalled = true
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "deleted"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionDeleteClient,
		Payload: map[string]any{
			"inbound_id": 7,
			"email":      "alice@example.com",
			"client_id":  "uuid-1",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["client_id"] != "uuid-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !deleteCalled || !routingUpdateCalled {
		t.Fatalf("expected v3 delete API and routing update to be called, delete=%v routing=%v", deleteCalled, routingUpdateCalled)
	}
}

func TestXUIExecuteSetClientEnabledUsesUpdateClientAPI(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	updateCalled := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/clients/get/alice@example.com":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": map[string]any{
						"id":         42,
						"email":      "alice@example.com",
						"uuid":       "uuid-1",
						"enable":     true,
						"expiryTime": int64(1893456000000),
						"inboundIds": []int{7},
					},
				}), nil
			case "/panel/api/clients/update/alice@example.com":
				updateCalled = true
				var client map[string]any
				if err := json.NewDecoder(req.Body).Decode(&client); err != nil {
					t.Fatalf("decode update body: %v", err)
				}
				if client["enable"] != false || int64Value(client["expiryTime"]) == 0 {
					t.Fatalf("expected enable=false while preserving client fields, got %#v", client)
				}
				if client["id"] != "uuid-1" {
					t.Fatalf("expected auth uuid to be preserved as id, got %#v", client["id"])
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionSetClientEnabled,
		Payload: map[string]any{
			"inbound_id": 7,
			"email":      "alice@example.com",
			"enabled":    false,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["enabled"] != false {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !updateCalled {
		t.Fatalf("expected v3 update client API to be called")
	}
}

func TestXUIExecuteAddClientUsesAddClientAPI(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	addCalled := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/inbounds/list":
				t.Fatalf("inbound_id add should not require listing inbounds")
				return nil, nil
			case "/panel/api/clients/add":
				addCalled = true
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatalf("decode add client body: %v", err)
				}
				inboundIDs := body["inboundIds"].([]any)
				if len(inboundIDs) != 1 || intValue(inboundIDs[0]) != 7 {
					t.Fatalf("expected inbound id 7, got %#v", body["inboundIds"])
				}
				added := body["client"].(map[string]any)
				if added["email"] != "bob@example.com" {
					t.Fatalf("unexpected added client: %#v", added)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "added"}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"inbound_id": 7,
			"client": map[string]any{
				"email":  "bob@example.com",
				"enable": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["email"] != "bob@example.com" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !addCalled {
		t.Fatalf("expected v3 add client API to be called")
	}
}

func TestXUIExecuteAddClientUsesInboundAddClientBeforeList(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	addClientCalled := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/clients/add":
				return jsonResponse(t, req, map[string]any{"success": false, "msg": "v3 add unavailable"}), nil
			case "/panel/api/inbounds/addClient":
				addClientCalled = true
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatalf("decode addClient body: %v", err)
				}
				if intValue(body["id"]) != 7 {
					t.Fatalf("expected inbound id 7, got %#v", body["id"])
				}
				settingsText, ok := body["settings"].(string)
				if !ok || !strings.Contains(settingsText, "bob@example.com") {
					t.Fatalf("unexpected settings: %#v", body["settings"])
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "added"}), nil
			case "/panel/api/inbounds/list":
				t.Fatalf("old addClient fallback should not require listing inbounds")
				return nil, nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"inbound_id": 7,
			"protocol":   "vless",
			"client": map[string]any{
				"email":  "bob@example.com",
				"enable": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["email"] != "bob@example.com" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !addClientCalled {
		t.Fatalf("expected inbound addClient API to be called")
	}
}

func TestXUIExecuteAddClientResolvesLocalInboundWhenList404(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	createLocalXUITestDB(t, dbPath)

	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
		DBPath:   dbPath,
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	resolvedAddCalled := false
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/clients/add":
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatalf("decode add body: %v", err)
				}
				inboundIDs := body["inboundIds"].([]any)
				if len(inboundIDs) == 1 && intValue(inboundIDs[0]) == 1 {
					resolvedAddCalled = true
					return jsonResponse(t, req, map[string]any{"success": true, "msg": "added"}), nil
				}
				return jsonResponse(t, req, map[string]any{"success": false, "msg": "bad inbound id"}), nil
			case "/panel/api/inbounds/addClient":
				return jsonResponse(t, req, map[string]any{"success": false, "msg": "bad inbound id"}), nil
			case "/panel/api/inbounds/list":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("404 page not found"))),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	result, err := client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"inbound_id":  443,
			"inbound_tag": "inbound-443",
			"protocol":    "vless",
			"client": map[string]any{
				"email":  "bob@example.com",
				"enable": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["inbound_id"] != 1 {
		t.Fatalf("expected resolved inbound id 1, got %#v", result)
	}
	if !resolvedAddCalled {
		t.Fatalf("expected local DB resolved add to be called")
	}
}

func TestXUIExecuteAddClientFallsBackToInboundUpdate(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	var updatedInbound map[string]any
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/inbounds/list":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": []map[string]any{{
						"id":       7,
						"tag":      "in-a",
						"protocol": "vless",
						"settings": `{"clients":[{"id":"uuid-1","email":"alice@example.com"}]}`,
					}},
				}), nil
			case "/panel/api/clients/add":
				return jsonResponse(t, req, map[string]any{"success": false, "msg": "v3 add unavailable"}), nil
			case "/panel/api/inbounds/addClient":
				return jsonResponse(t, req, map[string]any{"success": false, "msg": "add api unavailable"}), nil
			case "/panel/api/inbounds/update/7":
				if err := json.NewDecoder(req.Body).Decode(&updatedInbound); err != nil {
					t.Fatalf("decode update body: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				t.Fatalf("add_client must not restart xray")
				return nil, nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	_, err = client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionAddClient,
		Payload: map[string]any{
			"inbound_id": 7,
			"client": map[string]any{
				"email":  "bob@example.com",
				"enable": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	settingsText, ok := updatedInbound["settings"].(string)
	if !ok {
		t.Fatalf("expected settings string update, got %#v", updatedInbound["settings"])
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(settingsText), &settings); err != nil {
		t.Fatalf("decode updated settings: %v", err)
	}
	clients := settings["clients"].([]any)
	if len(clients) != 2 || clients[1].(map[string]any)["email"] != "bob@example.com" {
		t.Fatalf("expected bob to be appended, got %#v", clients)
	}
}

func TestXUIExecuteDeleteClientFallsBackToInboundUpdate(t *testing.T) {
	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  "https://xui.local",
		Username: "admin",
		Password: "pass",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}

	var updatedInbound map[string]any
	client.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/login":
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
			case "/panel/api/inbounds/list":
				return jsonResponse(t, req, map[string]any{
					"success": true,
					"obj": []map[string]any{{
						"id":       7,
						"tag":      "in-a",
						"settings": `{"clients":[{"id":"uuid-1","email":"alice@example.com"},{"id":"uuid-2","email":"bob@example.com"}]}`,
					}},
				}), nil
			case "/panel/xray/":
				wrapper, err := json.Marshal(map[string]any{
					"xraySetting": map[string]any{
						"outbounds": []map[string]any{},
						"routing":   map[string]any{"rules": []map[string]any{}},
					},
				})
				if err != nil {
					t.Fatalf("marshal wrapper: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": string(wrapper)}), nil
			case "/panel/api/clients/del/alice@example.com":
				return jsonResponse(t, req, map[string]any{"success": false, "msg": "v3 delete unavailable"}), nil
			case "/panel/api/inbounds/7/delClient/uuid-1":
				return jsonResponse(t, req, map[string]any{"success": false, "msg": "delete api unavailable"}), nil
			case "/panel/api/inbounds/update/7":
				if err := json.NewDecoder(req.Body).Decode(&updatedInbound); err != nil {
					t.Fatalf("decode update body: %v", err)
				}
				return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
			case "/panel/api/server/restartXrayService":
				t.Fatalf("delete_client must not restart xray")
				return nil, nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	_, err = client.ExecuteAction(context.Background(), model.XUIAction{
		Kind: model.XUIActionDeleteClient,
		Payload: map[string]any{
			"inbound_id": 7,
			"email":      "alice@example.com",
			"client_id":  "uuid-1",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	settingsText, ok := updatedInbound["settings"].(string)
	if !ok {
		t.Fatalf("expected settings string update, got %#v", updatedInbound["settings"])
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(settingsText), &settings); err != nil {
		t.Fatalf("decode updated settings: %v", err)
	}
	clients := settings["clients"].([]any)
	if len(clients) != 1 || clients[0].(map[string]any)["email"] != "bob@example.com" {
		t.Fatalf("expected only bob to remain, got %#v", clients)
	}
}

func TestLocalXUIRestartSuccessOutput(t *testing.T) {
	output := "The OS release is: debian \x1b[0;32m[INF] x-ui and xray Restarted successfully \x1b[0m"
	if !isLocalXUIRestartSuccessOutput(output) {
		t.Fatalf("expected success output to be recognized")
	}
	if isLocalXUIRestartSuccessOutput("x-ui restart failed") {
		t.Fatalf("did not expect failed output to be recognized")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(t *testing.T, req *http.Request, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}

func xuiCollectTransport(t *testing.T, loginCount *int, override func(*http.Request) *http.Response) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if override != nil {
			if resp := override(req); resp != nil {
				return resp, nil
			}
		}
		switch req.URL.Path {
		case "/login":
			(*loginCount)++
			return jsonResponse(t, req, map[string]any{"success": true, "msg": "ok"}), nil
		case "/panel/api/server/status":
			return jsonResponse(t, req, map[string]any{
				"success": true,
				"obj": map[string]any{
					"cpu":  1,
					"mem":  map[string]any{"current": 1, "total": 2},
					"xray": map[string]any{"state": "running"},
				},
			}), nil
		case "/panel/api/inbounds/list":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
		case "/panel/api/clients/list":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
		case "/panel/api/server/getConfigJson":
			return jsonResponse(t, req, map[string]any{
				"success": true,
				"obj": map[string]any{
					"outbounds": []map[string]any{},
					"routing":   map[string]any{"rules": []map[string]any{}},
				},
			}), nil
		case "/panel/xray/":
			return jsonResponse(t, req, map[string]any{
				"success": true,
				"obj":     `{"xraySetting":"{\"outbounds\":[],\"routing\":{\"rules\":[]}}"}`,
			}), nil
		case "/panel/xray/getOutboundsTraffic":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil, nil
		}
	})
}
