package panels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"bridge-core/internal/model"
)

func TestManualClientTrafficReset(t *testing.T) {
	for _, tc := range []struct {
		name                                                                                                                                                                         string
		legacy, disabled, falseSuccess, readFailure, resetFailure, restoreFailure, configChanged, liveTraffic, wrongIdentity, wrongInbound, wrongTraffic, partialReset, emptyTraffic bool
	}{
		{name: "modern"}, {name: "empty counters", emptyTraffic: true}, {name: "one counter unchanged", partialReset: true}, {name: "legacy", legacy: true}, {name: "disabled", disabled: true},
		{name: "legacy disabled", legacy: true, disabled: true}, {name: "live traffic", liveTraffic: true},
		{name: "false success", falseSuccess: true}, {name: "legacy false success", legacy: true, falseSuccess: true},
		{name: "readback failure", readFailure: true}, {name: "ambiguous reset failure", resetFailure: true, disabled: true},
		{name: "restore failure", disabled: true, restoreFailure: true}, {name: "config changed", configChanged: true},
		{name: "wrong identity", wrongIdentity: true}, {name: "wrong inbound", wrongInbound: true}, {name: "wrong traffic record", wrongTraffic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := fullXUIV3UpdateRecord()
			record["enable"] = !tc.disabled
			record["trafficReset"], record["trafficResetDay"] = "monthly", 31
			email := stringValue(record["email"])
			original := normalizeClientUpdateRecord(record)
			up, down := int64(1000), int64(2000)
			if tc.emptyTraffic {
				up, down = 0, 0
			}
			resets, restores := 0, 0
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				respond := func(obj any) (*http.Response, error) {
					return jsonResponse(t, req, map[string]any{"success": true, "obj": obj}), nil
				}
				switch req.URL.Path {
				case "/panel/api/inbounds/list":
					id := 7
					if tc.wrongInbound {
						id = 8
					}
					return respond([]map[string]any{{"id": id, "tag": "node-7", "protocol": "vless", "settings": map[string]any{"clients": []map[string]any{normalizeClientUpdateRecord(record)}}}})
				case "/panel/api/clients/get/" + email:
					if tc.legacy {
						r := jsonResponse(t, req, map[string]any{"success": false})
						r.StatusCode = 404
						return r, nil
					}
					ids := []int{7, 9}
					if tc.wrongInbound {
						ids = []int{8}
					}
					return respond(map[string]any{"client": record, "inboundIds": ids})
				case "/panel/api/clients/traffic/" + email, "/panel/api/inbounds/getClientTraffics/" + email:
					if (tc.legacy && req.URL.Path != "/panel/api/inbounds/getClientTraffics/"+email) || (!tc.legacy && req.URL.Path != "/panel/api/clients/traffic/"+email) {
						t.Fatal("wrong traffic API family")
					}
					if tc.readFailure && resets > 0 {
						r := jsonResponse(t, req, map[string]any{"success": false})
						r.StatusCode = 401
						return r, nil
					}
					gotEmail := email
					if tc.wrongTraffic {
						gotEmail = "another-client"
					}
					return respond(map[string]any{"email": gotEmail, "up": up, "down": down})
				case "/panel/api/clients/resetTraffic/" + email, "/panel/api/inbounds/7/resetClientTraffic/" + email:
					resets++
					want := "/panel/api/clients/resetTraffic/" + url.PathEscape(email)
					if tc.legacy {
						want = "/panel/api/inbounds/7/resetClientTraffic/" + url.PathEscape(email)
					}
					if req.Method != http.MethodPost || req.URL.EscapedPath() != want {
						t.Fatalf("wrong reset endpoint: %s", req.URL)
					}
					var payload map[string]any
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || len(payload) != 0 {
						t.Fatal("reset must not send a config update")
					}
					if !tc.falseSuccess {
						up, down = 0, 0
					}
					if tc.partialReset {
						down = 2000
					}
					if tc.liveTraffic {
						up, down = 10, 25
					}
					if !tc.legacy {
						record["enable"] = true
					}
					if tc.configChanged {
						record["totalGB"] = int64(1)
					}
					if tc.resetFailure {
						return nil, fmt.Errorf("connection lost after reset")
					}
					return respond(nil)
				case "/panel/api/clients/update/" + email:
					restores++
					if !tc.disabled || tc.legacy {
						t.Fatal("unexpected config write")
					}
					if tc.restoreFailure {
						r := jsonResponse(t, req, map[string]any{"success": false})
						r.StatusCode = 500
						return r, nil
					}
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["enable"] != false || body["id"] != "stable-uuid" || body["trafficReset"] != "monthly" || intValue(body["trafficResetDay"]) != 31 || int64Value(body["totalGB"]) != int64Value(original["totalGB"]) || int64Value(body["expiryTime"]) != int64Value(original["expiryTime"]) {
						t.Fatal("restoration altered client config")
					}
					record["enable"] = false
					return respond(nil)
				default:
					t.Fatalf("unexpected request (must never reset all clients or fall back after POST): %s", req.URL)
					return nil, nil
				}
			})
			clientID := "stable-uuid"
			if tc.wrongIdentity {
				clientID = "stale-uuid"
			}
			result, err := client.ExecuteAction(context.Background(), model.XUIAction{Kind: model.XUIActionManualResetClientTraffic, Payload: map[string]any{"inbound_id": 7, "inbound_tag": "node-7", "email": email, "client_id": clientID}})
			rejected := tc.wrongIdentity || tc.wrongInbound || tc.wrongTraffic
			failure := rejected || tc.partialReset || tc.falseSuccess || tc.readFailure || tc.resetFailure || tc.restoreFailure || tc.configChanged
			if (err != nil) != failure {
				t.Fatalf("err=%v failure=%v result=%v", err, failure, result)
			}
			if rejected {
				if resets != 0 {
					t.Fatal("invalid target was reset")
				}
				return
			}
			if resets != 1 || result == nil {
				t.Fatalf("must attempt reset only once and return a receipt: resets=%d result=%v", resets, result)
			}
			if failure && result["verified"] == true {
				t.Fatal("unverified reset marked successful")
			}
			if !failure && (result["verified"] != true || result["reset_done"] != true) {
				t.Fatalf("missing verified result: %v", result)
			}
			if !tc.restoreFailure && record["enable"] != !tc.disabled {
				t.Fatal("enable state not preserved")
			}
			if tc.disabled && !tc.legacy && restores != 1 {
				t.Fatalf("restore count=%d", restores)
			}
			if !tc.configChanged && !tc.restoreFailure && !reflect.DeepEqual(original, normalizeClientUpdateRecord(record)) {
				t.Fatal("unrelated config changed")
			}
		})
	}
}
