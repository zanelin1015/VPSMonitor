package panels

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestXUIClientTrafficResetUpdatesOnlySchedule(t *testing.T) {
	for _, period := range []string{"never", "hourly", "daily", "weekly", "monthly"} {
		t.Run(period, func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			writes := 0
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/panel/api/inbounds/list":
					return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
				case "/panel/api/clients/get/" + stringValue(record["email"]):
					return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7}}}), nil
				case "/panel/api/clients/update/" + stringValue(record["email"]):
					writes++
					var request struct {
						TrafficReset string   `json:"trafficReset"`
						Day          int      `json:"trafficResetDay"`
						Expiry       int64    `json:"expiryTime"`
						Total        int64    `json:"totalGB"`
						ID           string   `json:"id"`
						Enable       bool     `json:"enable"`
						AllowedIPs   []string `json:"allowedIPs"`
					}
					if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
						t.Fatal(err)
					}
					if request.TrafficReset != period || request.Day != 31 || request.Expiry != int64Value(record["expiryTime"]) || request.Total != int64Value(record["totalGB"]) || request.ID != "stable-uuid" || !request.Enable {
						t.Fatal("native schedule payload changed the expiry, quota, identity or enabled state")
					}
					record["trafficReset"], record["trafficResetDay"] = request.TrafficReset, request.Day
					return jsonResponse(t, req, map[string]any{"success": true}), nil
				default:
					t.Fatalf("schedule settings must not reset counters or change an inbound: %s", req.URL.Path)
					return nil, nil
				}
			})
			result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientTrafficReset, Payload: map[string]any{
				"inbound_id": 7, "email": record["email"], "traffic_reset": period, "traffic_reset_day": 31,
			}})
			if err != nil || result["verified"] != true || result["traffic_reset"] != period || result["traffic_reset_day"] != 31 || writes != 1 {
				t.Fatalf("expected verified native schedule: result=%v err=%v writes=%d", result, err, writes)
			}
		})
	}
}

func TestXUIClientTrafficResetVerifiesPeriodAndDay(t *testing.T) {
	for _, failedField := range []string{"trafficReset", "trafficResetDay"} {
		t.Run(failedField, func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/panel/api/inbounds/list":
					return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": `{"clients":[]}`}}}), nil
				case "/panel/api/clients/get/" + stringValue(record["email"]):
					return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7}}}), nil
				default:
					if failedField != "trafficReset" {
						record["trafficReset"] = "monthly"
					}
					if failedField != "trafficResetDay" {
						record["trafficResetDay"] = 15
					}
					return jsonResponse(t, req, map[string]any{"success": true}), nil
				}
			})
			result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientTrafficReset, Payload: map[string]any{
				"inbound_id": 7, "email": record["email"], "traffic_reset": "monthly", "traffic_reset_day": 15,
			}})
			if err == nil || !strings.Contains(err.Error(), failedField) || result["verified"] != false {
				t.Fatalf("false schedule success was not detected: result=%v err=%v", result, err)
			}
		})
	}
}

func TestXUIClientTrafficResetRejectsUnsupportedPanels(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		record := fullXUIV3UpdateRecord()
		delete(record, "trafficReset")
		client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatal("unsupported panels must not receive any write")
			}
			if req.URL.Path == "/panel/api/inbounds/list" {
				return jsonResponse(t, req, map[string]any{"success": true, "obj": []map[string]any{{"id": 7, "settings": map[string]any{"clients": []map[string]any{normalizeClientUpdateRecord(record)}}}}}), nil
			}
			if legacy {
				response := jsonResponse(t, req, map[string]any{"success": false})
				response.StatusCode = http.StatusNotFound
				return response, nil
			}
			return jsonResponse(t, req, map[string]any{"success": true, "obj": map[string]any{"client": record, "inboundIds": []int{7}}}), nil
		})
		if _, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionUpdateClientTrafficReset, Payload: map[string]any{
			"inbound_id": 7, "email": record["email"], "traffic_reset": "monthly",
		}}); err == nil || !strings.Contains(err.Error(), "does not support") {
			t.Fatalf("expected an explicit unsupported-panel error, got %v", err)
		}
	}
}

func TestXUICollectEnrichesExistingClientsWithNativeResetSettings(t *testing.T) {
	inbounds := []map[string]any{{"id": 7, "settings": `{"clients":[{"email":"alice","id":"keep-uuid","totalGB":100,"expiryTime":123}]}`}}
	mergeXUIClientsIntoInbounds(inbounds, []map[string]any{{"email": "alice", "uuid": "other-source-uuid", "inboundIds": []int{7}, "trafficReset": "monthly", "trafficResetDay": 15}})
	settings, _, err := decodeInboundSettings(inbounds[0]["settings"])
	if err != nil {
		t.Fatal(err)
	}
	clients := objectSlice(settings["clients"])
	if len(clients) != 1 || clients[0]["id"] != "keep-uuid" || int64Value(clients[0]["totalGB"]) != 100 || int64Value(clients[0]["expiryTime"]) != 123 || clients[0]["trafficReset"] != "monthly" || intValue(clients[0]["trafficResetDay"]) != 15 {
		t.Fatal("collection did not enrich the existing client without replacing its identity and lifecycle fields")
	}
}
