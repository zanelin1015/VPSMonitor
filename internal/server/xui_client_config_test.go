package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestUnifiedClientConfigAPIValidatesPermissionsVersionAndDispatches(t *testing.T) {
	for _, tc := range []struct {
		name, role, version string
		day                 int
		want                int
	}{{"anonymous", "", "0.3.49", 31, 401}, {"area", "area", "0.3.49", 31, 403}, {"old-client", "root", "0.3.48", 31, 409}, {"invalid-reset", "root", "0.3.49", 32, 400}, {"save", "root", "0.3.49", 31, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "save.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			agent, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "config-agent"})
			if err != nil {
				t.Fatal(err)
			}
			cfg := agent.Config
			cfg.XUI.Enabled = true
			cfg.XUI.APIToken = "ephemeral-panel-token"
			if _, err := s.UpdateAgentConfig(agent.AgentID, cfg); err != nil {
				t.Fatal(err)
			}
			if err := s.SaveSnapshot(model.AgentSnapshot{AgentID: agent.AgentID, Version: tc.version, ReportedAt: time.Now(), XUI: &model.XUISnapshot{Inbounds: []map[string]any{{"id": 7, "tag": "inbound-7", "protocol": "vless", "enable": true, "port": 20001, "settings": `{"clients":[{"id":"stable-uuid","email":"alice","enable":true,"totalGB":0,"expiryTime":0,"trafficReset":"never","trafficResetDay":1}]}`}}}}); err != nil {
				t.Fatal(err)
			}
			if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area", Password: "password123", AgentIDs: []string{agent.AgentID}}); err != nil {
				t.Fatal(err)
			}
			token := ""
			if tc.role != "" {
				username := "admin"
				if tc.role == "area" {
					username = "area"
				}
				user, ok, err := s.AuthenticateAdmin(username, "password123")
				if err != nil || !ok {
					t.Fatal("login failed")
				}
				token, _, err = s.CreateAdminSession(user, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
			}
			zero := int64(0)
			limit := int64(10 * 1024 * 1024 * 1024)
			never, monthly := "never", "monthly"
			day := tc.day
			originalDay := 1
			request := model.XUIClientConfigRequest{RequestID: "unified-test-request", ClientID: "stable-uuid", InboundID: 7, InboundTag: "inbound-7", Email: "alice", Billing: model.XUIClientBillingConfig{RevenueAmount: 63, RevenueCurrency: "CNY", RevenueCycle: "month", TrafficMultiplier: 1}, Changes: model.XUIClientConfigChanges{TotalBytes: &limit, TrafficReset: &monthly, TrafficResetDay: &day}, Expected: model.XUIClientConfigChanges{TotalBytes: &zero, TrafficReset: &never, TrafficResetDay: &originalDay}}
			body, _ := json.Marshal(request)
			app := &App{store: s, realtime: newRealtimeHub()}
			control := app.realtime.registerAgentControl(agent.AgentID)
			defer app.realtime.unregisterAgentControl(agent.AgentID, control)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/config-agent/xui/client-config", bytes.NewReader(body))
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
			w := httptest.NewRecorder()
			app.Handler().ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
			stored, _, _ := s.GetAgentConfig(agent.AgentID)
			actions, _ := s.ListXUIActions(agent.AgentID, 100)
			if tc.want != 200 {
				if len(actions) != 0 || len(stored.Renewal.ClientBillings) != 0 {
					t.Fatal("rejected request changed configuration")
				}
				return
			}
			var save model.XUIClientConfigSave
			if err := json.Unmarshal(w.Body.Bytes(), &save); err != nil {
				t.Fatal(err)
			}
			if save.Action == nil || save.Action.Status != "running" || len(stored.Renewal.ClientBillings) != 1 {
				t.Fatal("local save and queue were not committed together")
			}
			select {
			case message := <-control.ch:
				if message.Kind != model.XUIActionUpdateClientConfig || message.XUIAuth == nil || message.XUIAuth.APIToken != "ephemeral-panel-token" {
					t.Fatal("merged WS operation lost its panel auth")
				}
			default:
				t.Fatal("no WS dispatch")
			}
			req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/config-agent/xui/client-config", bytes.NewReader(body))
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
			w = httptest.NewRecorder()
			app.Handler().ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("duplicate submission status=%d body=%s", w.Code, w.Body.String())
			}
			actions, _ = s.ListXUIActions(agent.AgentID, 100)
			if len(actions) != 1 || actions[0].XUIAuth != nil {
				t.Fatal("duplicate actions or panel token persisted")
			}
			if _, err := s.CompleteXUIAction(agent.AgentID, save.Action.ID, model.XUIActionResultRequest{Status: "failed", Error: "expiry did not persist", Result: map[string]any{"partial": true}}); err != nil {
				t.Fatal(err)
			}
			req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/config-agent/xui/client-config/"+save.RequestID, nil)
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
			w = httptest.NewRecorder()
			app.Handler().ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatal("could not reload saved result")
			}
			if err := json.Unmarshal(w.Body.Bytes(), &save); err != nil || save.Action.Status != "failed" || !save.BillingSaved {
				t.Fatal("partial failure receipt was lost")
			}
		})
	}
}
