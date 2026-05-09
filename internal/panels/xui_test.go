package panels

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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
			case "/panel/api/xray/":
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
			case "/panel/api/xray/update":
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
			case "/panel/api/xray/":
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
			case "/panel/api/xray/update":
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
		case "/panel/api/server/getConfigJson":
			return jsonResponse(t, req, map[string]any{
				"success": true,
				"obj": map[string]any{
					"outbounds": []map[string]any{},
					"routing":   map[string]any{"rules": []map[string]any{}},
				},
			}), nil
		case "/panel/xray/getOutboundsTraffic":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{}}), nil
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil, nil
		}
	})
}
