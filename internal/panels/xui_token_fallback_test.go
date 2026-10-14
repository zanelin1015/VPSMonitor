package panels

import (
	"context"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func TestXUITokenRejectionDoesNotUseLegacyLogin(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client, err := NewXUIClient(config.XUIConfig{
				Enabled: true, BaseURL: "https://xui.local", APIToken: "expired-token",
				Username: "legacy-user", Password: "legacy-password", TwoFactorCode: "123456",
				DBPath: filepath.Join(t.TempDir(), "missing.db"),
			}, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			requests := 0
			client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.Path == "/login" {
					t.Fatal("rejected API token must not send stored username/password/OTP")
				}
				if req.Header.Get("Authorization") != "Bearer expired-token" {
					t.Fatal("local enrichment lost token authentication")
				}
				response := jsonResponse(t, req, map[string]any{"success": false, "msg": "access denied"})
				response.StatusCode = status
				return response, nil
			})}
			localConfig := map[string]any{"outbounds": []any{}, "routing": map[string]any{"rules": []any{}}}
			got := client.enrichLocalXrayConfig(context.Background(), localConfig)
			if requests == 0 || !reflect.DeepEqual(got, localConfig) {
				t.Fatalf("failed enrichment must preserve local fallback, requests=%d config=%#v", requests, got)
			}
			beforeAction := requests
			result, err := client.ExecuteAction(context.Background(), model.XUIAction{
				Kind:    model.XUIActionUpdateClientExpiry,
				Payload: map[string]any{"inbound_id": 7, "email": "alice", "expiry_time": int64(1896048000000)},
			})
			if requests == beforeAction || result != nil || !isXUIAuthError(err) {
				t.Fatalf("rejected token action must return its auth error without legacy login: result=%#v err=%v", result, err)
			}
		})
	}
}
