package panels

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"bridge-core/internal/model"
)

func TestUnifiedXUIClientConfigSingleWriteVerificationAndRetry(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "partial-and-retry"}[partial], func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			writes := 0
			ignoreExpiry := partial
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/panel/api/inbounds/list":
					return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
				case "/panel/api/clients/get/" + stringValue(record["email"]):
					return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7, 9}}}), nil
				case "/panel/api/clients/update/" + stringValue(record["email"]):
					writes++
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					for key, expected := range map[string]string{"id": "stable-uuid", "password": "stable-password", "subId": "stable-sub-id", "secret": "stable-secret", "flow": "xtls-rprx-vision"} {
						if stringValue(body[key]) != expected {
							t.Fatalf("configuration update changed %s", key)
						}
					}
					for _, field := range []string{"enable", "totalGB", "expiryTime", "trafficReset", "trafficResetDay"} {
						if field == "expiryTime" && ignoreExpiry {
							continue
						}
						record[field] = body[field]
					}
					return jsonResponse(t, req, map[string]any{"success": true, "msg": "updated"}), nil
				default:
					t.Fatalf("unexpected request %s", req.URL.Path)
					return nil, nil
				}
			})
			payload := map[string]any{"inbound_id": 7, "email": record["email"], "client_id": "stable-uuid", "changes": map[string]any{"enabled": false, "total_bytes": int64(25 * 1024 * 1024 * 1024), "expiry_time": int64(1896048000000), "traffic_reset": "monthly", "traffic_reset_day": 31}, "expected": map[string]any{"enabled": true, "total_bytes": record["totalGB"], "expiry_time": record["expiryTime"], "traffic_reset": "never", "traffic_reset_day": 1}}
			result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientConfig, Payload: payload})
			if writes != 1 || result["saved"] != true {
				t.Fatal("all changes must be written in one panel request")
			}
			if !partial {
				if err != nil || result["verified"] != true {
					t.Fatalf("merged update failed: %v", err)
				}
				return
			}
			if err == nil || result["verified"] != false || result["partial"] != true {
				t.Fatal("a panel success without expiry persistence was accepted")
			}
			fields := result["field_results"].(map[string]any)
			if fields["expiryTime"].(map[string]any)["verified"] != false || fields["totalGB"].(map[string]any)["verified"] != true {
				t.Fatal("verification did not retain per-field results")
			}
			ignoreExpiry = false
			payload["changes"] = map[string]any{"expiry_time": int64(1896048000000)}
			result, err = client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientConfig, Payload: payload})
			if err != nil || result["verified"] != true || writes != 2 || record["enable"] != false || int64Value(record["totalGB"]) != 25*1024*1024*1024 || record["trafficReset"] != "monthly" {
				t.Fatalf("failed-field retry lost earlier successful fields: %v", err)
			}
		})
	}
}

func TestUnifiedXUIClientConfigRejectsConcurrentPanelEdits(t *testing.T) {
	record := fullXUIV3UpdateRecord()
	writes := 0
	client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/panel/api/inbounds/list":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
		case "/panel/api/clients/get/" + stringValue(record["email"]):
			return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7}}}), nil
		default:
			writes++
			return jsonResponse(t, req, map[string]any{"success": true}), nil
		}
	})
	_, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientConfig, Payload: map[string]any{"inbound_id": 7, "email": record["email"], "client_id": "stable-uuid", "changes": map[string]any{"total_bytes": 0}, "expected": map[string]any{"total_bytes": int64(200 * 1024 * 1024 * 1024)}}})
	if err == nil || writes != 0 {
		t.Fatal("a newer external panel edit was overwritten")
	}
}

func TestUnifiedXUIClientConfigLegacyPanelSingleWrite(t *testing.T) {
	record := fullXUIV3UpdateRecord()
	record["id"] = "stable-uuid"
	other := fullXUIV3UpdateRecord()
	other["id"], other["uuid"], other["email"] = "other-uuid", "other-uuid", "bob"
	settings, _ := json.Marshal(map[string]any{"clients": []map[string]any{record, other}})
	inbound := map[string]any{"id": 7, "tag": "legacy", "protocol": "vless", "settings": string(settings)}
	writes := 0
	client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/panel/api/inbounds/list":
			return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{inbound}}), nil
		case "/panel/api/clients/get/" + stringValue(record["email"]):
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
		case "/panel/api/inbounds/update/7":
			writes++
			if err := json.NewDecoder(req.Body).Decode(&inbound); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(t, req, map[string]any{"success": true}), nil
		default:
			t.Fatalf("unexpected legacy request %s", req.URL.Path)
			return nil, nil
		}
	})
	result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientConfig, Payload: map[string]any{"inbound_id": 7, "email": record["email"], "client_id": "stable-uuid", "changes": map[string]any{"enabled": false, "total_bytes": 0, "expiry_time": 0}, "expected": map[string]any{"enabled": true, "total_bytes": record["totalGB"], "expiry_time": record["expiryTime"]}}})
	if err != nil || writes != 1 || result["verified"] != true {
		t.Fatalf("legacy merged update failed: %v", err)
	}
	updated, _, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		t.Fatal(err)
	}
	clients := objectSlice(updated["clients"])
	if len(clients) != 2 || stringValue(clients[0]["id"]) != "stable-uuid" || stringValue(clients[1]["id"]) != "other-uuid" || int64Value(clients[1]["expiryTime"]) != int64Value(other["expiryTime"]) {
		t.Fatal("legacy update changed credentials or another client")
	}
}
