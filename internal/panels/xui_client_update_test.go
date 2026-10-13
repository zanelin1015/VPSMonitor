package panels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func TestXUIV3ClientUpdatesPreserveFullRecord(t *testing.T) {
	for _, test := range []struct {
		name, kind, field, payloadField string
		value                           any
	}{
		{"expiry", model.XUIActionUpdateClientExpiry, "expiryTime", "expiry_time", int64(1896048000000)},
		{"unlimited expiry", model.XUIActionUpdateClientExpiry, "expiryTime", "expiry_time", int64(0)},
		{"traffic limit", model.XUIActionUpdateClientTraffic, "totalGB", "total_bytes", int64(50 * 1024 * 1024 * 1024)},
		{"unlimited traffic", model.XUIActionUpdateClientTraffic, "totalGB", "total_bytes", int64(0)},
		{"disable", model.XUIActionSetClientEnabled, "enable", "enabled", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			original := normalizeClientUpdateRecord(record)
			reads, writes := 0, 0
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/panel/api/inbounds/list":
					return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "protocol": "vless", "settings": `{"clients":[]}`}}}), nil
				case "/panel/api/clients/get/" + stringValue(record["email"]):
					reads++
					return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7, 9}, "externalLinks": []int{11}}}), nil
				case "/panel/api/clients/update/" + stringValue(record["email"]):
					writes++
					if req.Method != http.MethodPost || req.Header.Get("Content-Type") != "application/json" {
						t.Fatal("client updates must use JSON POST")
					}
					if req.URL.EscapedPath() != "/panel/api/clients/update/"+url.PathEscape(stringValue(record["email"])) {
						t.Fatal("client email was not escaped as a path segment")
					}
					var apiClient struct {
						ID         string   `json:"id"`
						AllowedIPs []string `json:"allowedIPs"`
					}
					data, err := readPanelResponse(req.Body)
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(data, &apiClient); err != nil {
						t.Fatalf("request does not match the panel's Client API schema: %v", err)
					}
					var body map[string]any
					if err := json.Unmarshal(data, &body); err != nil {
						t.Fatalf("decode update: %v", err)
					}
					if stringValue(body["id"]) != "stable-uuid" {
						t.Fatal("the request id must be the UUID, not the numeric database id")
					}
					for key, value := range original {
						if key == test.field {
							continue
						}
						switch key {
						case "uuid":
							if _, exists := body[key]; exists {
								t.Error("database-only uuid must not be included in the Client payload")
							}
							continue
						case "allowedIPs":
							value = []string{}
						case "createdAt":
							key = "created_at"
						case "updatedAt":
							key = "updated_at"
						}
						want, _ := json.Marshal(value)
						got, _ := json.Marshal(body[key])
						if string(want) != string(got) {
							t.Errorf("unrelated client field %s changed", key)
						}
					}
					if got, want := body[test.field], test.value; test.field == "enable" {
						if got != want {
							t.Fatalf("unexpected enabled flag: %v", got)
						}
					} else if int64Value(got) != int64Value(want) {
						t.Fatalf("unexpected %s: %v", test.field, got)
					}
					// Apply only the requested field, as a real server persists it.
					record[test.field] = body[test.field]
					return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
				default:
					t.Fatalf("unexpected API call; v3 updates must not save a whole inbound: %s", req.URL.Path)
					return nil, nil
				}
			})
			result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: test.kind, Payload: map[string]any{
				"inbound_id": 7, "email": record["email"], test.payloadField: test.value,
			}})
			if err != nil {
				t.Fatalf("update client: %v", err)
			}
			if result["verified"] != true || result["update_method"] != "client_api" || reads != 2 || writes != 1 {
				t.Fatalf("expected one verified client update; result=%v reads=%d writes=%d", result, reads, writes)
			}
		})
	}
}

func TestXUIClientUpdateDetectsFalseSuccess(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, test := range []struct {
			kind, payloadField string
			value              any
		}{
			{model.XUIActionUpdateClientExpiry, "expiry_time", int64(1896048000000)},
			{model.XUIActionUpdateClientTraffic, "total_bytes", int64(0)},
			{model.XUIActionSetClientEnabled, "enabled", false},
		} {
			name := test.kind
			if legacy {
				name += "_legacy"
			}
			t.Run(name, func(t *testing.T) {
				record := fullXUIV3UpdateRecord()
				writes := 0
				client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/panel/api/inbounds/list":
						return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": map[string]any{"clients": []map[string]any{normalizeClientUpdateRecord(record)}}}}}), nil
					case "/panel/api/clients/get/" + stringValue(record["email"]):
						if legacy {
							response := jsonResponse(t, req, map[string]any{"success": false})
							response.StatusCode = http.StatusNotFound
							return response, nil
						}
						return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7}}}), nil
					case "/panel/api/clients/update/" + stringValue(record["email"]), "/panel/api/inbounds/update/7":
						writes++
						return jsonResponse(t, req, map[string]any{"success": true, "msg": "Inbound has been successfully updated."}), nil
					default:
						t.Fatalf("unexpected request: %s", req.URL.Path)
						return nil, nil
					}
				})
				result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: test.kind, Payload: map[string]any{
					"inbound_id": 7, "email": record["email"], test.payloadField: test.value,
				}})
				if err == nil || !strings.Contains(err.Error(), "did not persist") || result["verified"] != false || writes != 1 {
					t.Fatalf("false success must be reported as a failed update: result=%v err=%v writes=%d", result, err, writes)
				}
			})
		}
	}
}

func TestXUIV3ClientUpdateRejectsUnsafeReads(t *testing.T) {
	for _, scenario := range []string{"forbidden", "malformed", "wrong inbound", "wrong email", "wrong credential"} {
		t.Run(scenario, func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			payload := map[string]any{"inbound_id": 7, "email": record["email"], "expiry_time": int64(1896048000000)}
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/login" {
					return jsonResponse(t, req, map[string]any{"success": false, "msg": "forbidden"}), nil
				}
				if req.Method != http.MethodGet {
					t.Fatal("unsafe reads must not trigger a write or fallback")
				}
				if req.URL.Path == "/panel/api/inbounds/list" {
					return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
				}
				ids := []int{7}
				switch scenario {
				case "forbidden":
					response := jsonResponse(t, req, map[string]any{"success": false})
					response.StatusCode = http.StatusForbidden
					return response, nil
				case "malformed":
					return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{}}), nil
				case "wrong inbound":
					ids = []int{9}
				case "wrong email":
					record["email"] = "someone-else"
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": ids}}), nil
			})
			if scenario == "wrong credential" {
				payload["client_id"] = "someone-else-uuid"
			}
			if _, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientExpiry, Payload: payload}); err == nil {
				t.Fatal("expected unsafe client read to fail")
			}
		})
	}
}

func TestXUIV3ClientUpdateVerificationFailureDoesNotReplay(t *testing.T) {
	for _, scenario := range []string{"auth expired", "UUID changed", "missing expiry", "invalid expiry"} {
		t.Run(scenario, func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			reads, writes := 0, 0
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/panel/api/inbounds/list":
					return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
				case "/panel/api/clients/get/" + stringValue(record["email"]):
					reads++
					if reads == 2 && scenario == "auth expired" {
						response := jsonResponse(t, req, map[string]any{"success": false})
						response.StatusCode = http.StatusUnauthorized
						return response, nil
					}
					return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7}}}), nil
				case "/panel/api/clients/update/" + stringValue(record["email"]):
					writes++
					record["expiryTime"] = int64(1896048000000)
					if scenario == "UUID changed" {
						record["uuid"] = "unexpected-new-uuid"
					}
					if scenario == "missing expiry" {
						delete(record, "expiryTime")
					}
					if scenario == "invalid expiry" {
						record["expiryTime"] = "invalid"
					}
					return jsonResponse(t, req, map[string]any{"success": true}), nil
				default:
					t.Fatalf("verification errors must not trigger replay/login: %s", req.URL.Path)
					return nil, nil
				}
			})
			result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientExpiry, Payload: map[string]any{
				"inbound_id": 7, "email": record["email"], "expiry_time": int64(1896048000000),
			}})
			if err == nil || result["verified"] != false || writes != 1 || reads != 2 {
				t.Fatalf("expected verification failure without replay: result=%v err=%v reads=%d writes=%d", result, err, reads, writes)
			}
		})
	}
}

func TestXUIUpdateClientExpiryRejectsInvalidValuesBeforeWriting(t *testing.T) {
	for _, value := range []any{nil, false, "invalid", float64(1.5), int64(-1)} {
		client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
			t.Fatalf("invalid expiry must not reach x-ui: %s", req.URL.Path)
			return nil, nil
		})
		if _, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientExpiry, Payload: map[string]any{
			"inbound_id": 7, "email": "alice", "expiry_time": value,
		}}); err == nil {
			t.Fatalf("invalid expiry was accepted: %v", value)
		}
	}
}

func TestNormalizeClientUpdateRecordDoesNotUseNumericIDAsCredential(t *testing.T) {
	raw := map[string]any{"id": 12, "email": "trojan-client", "password": "stable-password"}
	client := normalizeClientUpdateRecord(raw)
	if _, exists := client["id"]; exists || client["password"] != raw["password"] || !reflect.DeepEqual(raw, map[string]any{"id": 12, "email": "trojan-client", "password": "stable-password"}) {
		t.Fatal("normalization must preserve passwords and leave the source record unchanged")
	}
}

func TestClientUpdatePayloadMatchesClientSchema(t *testing.T) {
	for _, test := range []struct {
		name string
		ips  any
		want []string
	}{
		{"empty database text", "", []string{}},
		{"database text", " 10.0.0.2/32, fd00::2/128 \n", []string{"10.0.0.2/32", "fd00::2/128"}},
		{"encoded array", `["10.0.0.2/32","fd00::2/128"]`, []string{"10.0.0.2/32", "fd00::2/128"}},
		{"JSON array", []any{"10.0.0.2/32", "fd00::2/128"}, []string{"10.0.0.2/32", "fd00::2/128"}},
		{"string array", []string{"10.0.0.2/32"}, []string{"10.0.0.2/32"}},
		{"null", nil, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			record["allowedIPs"] = test.ips
			record["reverse"] = `{"tag":"keep-reverse"}`
			record["allowedIPsByInbound"] = map[string]any{"7": "10.0.0.2/32", "9": "10.8.0.2/32, fd08::2/128"}
			before, _ := json.Marshal(record)
			payload, err := clientUpdateAPIPayload(normalizeClientUpdateRecord(record))
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(payload)
			var request struct {
				ID                  string                `json:"id"`
				AllowedIPs          []string              `json:"allowedIPs"`
				AllowedIPsByInbound map[int][]string      `json:"allowedIPsByInbound"`
				Reverse             *struct{ Tag string } `json:"reverse"`
				CreatedAt           int64                 `json:"created_at"`
				UpdatedAt           int64                 `json:"updated_at"`
			}
			if err := json.Unmarshal(encoded, &request); err != nil {
				t.Fatalf("payload does not match the documented Client schema: %v", err)
			}
			if request.ID != "stable-uuid" || !reflect.DeepEqual(request.AllowedIPs, test.want) ||
				!reflect.DeepEqual(request.AllowedIPsByInbound, map[int][]string{7: {"10.0.0.2/32"}, 9: {"10.8.0.2/32", "fd08::2/128"}}) ||
				request.Reverse == nil || request.Reverse.Tag != "keep-reverse" ||
				request.CreatedAt != 1735689600000 || request.UpdatedAt != 1735776000000 {
				t.Fatal("client configuration was not preserved in the API payload")
			}
			for _, key := range []string{"uuid", "createdAt", "updatedAt"} {
				if _, exists := payload[key]; exists {
					t.Errorf("database field %s leaked into API payload", key)
				}
			}
			after, _ := json.Marshal(record)
			if string(before) != string(after) {
				t.Fatal("payload conversion changed the source client record")
			}
		})
	}
}

func TestXUIClientUpdateRejectsInvalidConfigurationBeforeWriting(t *testing.T) {
	for _, test := range []struct {
		field string
		value any
	}{
		{"allowedIPs", false},
		{"allowedIPs", []any{"10.0.0.2/32", 2}},
		{"allowedIPs", `["10.0.0.2/32",2]`},
		{"allowedIPsByInbound", "not-an-object"},
		{"allowedIPsByInbound", map[string]any{"7": false}},
		{"reverse", `{"tag":`},
	} {
		t.Run(test.field+"/"+stringValue(test.value), func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			record[test.field] = test.value
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatal("invalid configuration must not reach a write endpoint")
				}
				if req.URL.Path == "/panel/api/inbounds/list" {
					return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
				}
				return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7}}}), nil
			})
			if _, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientTraffic, Payload: map[string]any{
				"inbound_id": 7, "email": record["email"], "total_bytes": int64(50 * 1024 * 1024 * 1024),
			}}); err == nil {
				t.Fatal("invalid stored configuration must fail without writing")
			}
		})
	}
}

func TestXUIClientUpdatePreservesPerInboundTunnelAddresses(t *testing.T) {
	record := fullXUIV3UpdateRecord()
	client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/panel/api/inbounds/list":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
		case "/panel/api/clients/get/" + stringValue(record["email"]):
			return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{
				"client": record, "inboundIds": []int{7, 9}, "tunnelAllowedIPs": map[string]any{"7": "10.0.0.2/32", "9": "10.8.0.2/32"},
			}}), nil
		default:
			var request struct {
				AllowedIPsByInbound map[int][]string `json:"allowedIPsByInbound"`
			}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(request.AllowedIPsByInbound, map[int][]string{7: {"10.0.0.2/32"}, 9: {"10.8.0.2/32"}}) {
				t.Fatal("distinct per-inbound tunnel addresses were lost")
			}
			record["totalGB"] = int64(50 * 1024 * 1024 * 1024)
			return jsonResponse(t, req, map[string]any{"success": true}), nil
		}
	})
	result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientTraffic, Payload: map[string]any{
		"inbound_id": 7, "email": record["email"], "total_bytes": int64(50 * 1024 * 1024 * 1024),
	}})
	if err != nil || result["verified"] != true {
		t.Fatalf("update failed: %v", err)
	}
}

func fullXUIV3UpdateRecord() map[string]any {
	return map[string]any{
		"id": 42, "uuid": "stable-uuid", "email": "alice+main@example.com", "enable": true,
		"expiryTime": int64(1893456000000), "totalGB": int64(107374182400), "subId": "stable-sub-id",
		"password": "stable-password", "auth": "stable-auth", "flow": "xtls-rprx-vision", "security": "auto",
		"limitIp": 2, "limitHwid": 3, "tgId": int64(123), "group": "group-a", "comment": "keep-me",
		"reset": 30, "resetDay": 0, "resetWeekday": 0, "resetMax": 4, "resetCount": 2,
		"trafficReset": "never", "trafficResetDay": 1, "reverse": map[string]any{"tag": "keep-reverse"},
		// The GET endpoint returns a database ClientRecord, not the POST Client
		// schema: allowedIPs is stored as text even for non-tunnel clients.
		"allowedIPs": "",
		"privateKey": "stable-private-key", "publicKey": "stable-public-key", "preSharedKey": "stable-pre-shared-key",
		"keepAlive": 0, "forwardedPorts": "80,443", "secret": "stable-secret", "adTag": "stable-ad-tag",
		"createdAt": int64(1735689600000), "updatedAt": int64(1735776000000),
	}
}

func xuiClientUpdateTestClient(t *testing.T, transport roundTripFunc) *XUIClient {
	t.Helper()
	client, err := NewXUIClient(config.XUIConfig{Enabled: true, BaseURL: "https://xui.local", APIToken: "test-token"}, time.Second)
	if err != nil {
		t.Fatalf("create x-ui client: %v", err)
	}
	client.client = &http.Client{Transport: transport, Timeout: time.Second}
	return client
}
