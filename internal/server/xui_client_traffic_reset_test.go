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

func TestNativeClientResetAPIValidatesAndDispatches(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "reset.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "native-reset"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := r.Config
	cfg.XUI.Enabled, cfg.XUI.APIToken = true, "current-panel-token"
	if _, err := s.UpdateAgentConfig(r.AgentID, cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	root, ok, err := s.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatal("root login failed")
	}
	rootToken, _, err := s.CreateAdminSession(root, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area", Password: "password123", Enabled: &enabled, AgentIDs: []string{r.AgentID}}); err != nil {
		t.Fatal(err)
	}
	area, ok, err := s.AuthenticateAdmin("area", "password123")
	if err != nil || !ok {
		t.Fatal("area login failed")
	}
	areaToken, _, err := s.CreateAdminSession(area, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s, realtime: newRealtimeHub()}
	session := app.realtime.registerAgentControl(r.AgentID)
	defer app.realtime.unregisterAgentControl(r.AgentID, session)
	for _, test := range []struct {
		name, token, version string
		day                  any
		status               int
	}{
		{"unauthenticated", "", "0.3.46", 15, 401},
		{"restricted account", areaToken, "0.3.46", 15, 403},
		{"invalid day", rootToken, "0.3.46", 32, 400},
		{"old client", rootToken, "0.3.45", 15, 409},
		{"unknown client", rootToken, "", 15, 409},
		{"supported client", rootToken, "0.3.46", 15, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := s.SaveSnapshot(model.AgentSnapshot{AgentID: r.AgentID, Version: test.version, ReportedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(model.XUIActionRequest{Kind: model.XUIActionUpdateClientTrafficReset, Payload: map[string]any{
				"inbound_id": 7, "email": "alice", "traffic_reset": "monthly", "traffic_reset_day": test.day,
			}})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/native-reset/xui/actions", bytes.NewReader(body))
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: test.token})
			response := httptest.NewRecorder()
			app.handleXUIActions(response, req, r.AgentID, nil)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d: %s", response.Code, test.status, response.Body.String())
			}
			actions, err := s.ListXUIActions(r.AgentID, 100)
			if err != nil {
				t.Fatal(err)
			}
			if test.status != 200 {
				if len(actions) != 0 {
					t.Fatal("rejected request was queued")
				}
				return
			}
			if len(actions) != 1 || actions[0].Status != model.XUIActionStatusRunning || actions[0].XUIAuth != nil {
				t.Fatal("native reset action was not claimed or persisted an API token")
			}
			select {
			case control := <-session.ch:
				if control.Type != model.AgentControlExecuteXUI || control.Kind != model.XUIActionUpdateClientTrafficReset || control.XUIAuth == nil || control.XUIAuth.APIToken != "current-panel-token" {
					t.Fatal("native reset WS dispatch lost its action or panel authentication")
				}
			default:
				t.Fatal("native reset was not dispatched over WS")
			}
			pollActions := []model.XUIAction{{Kind: model.XUIActionUpdateClientTrafficReset}}
			app.attachXUIActionAuth(r.AgentID, pollActions)
			if pollActions[0].XUIAuth == nil || pollActions[0].XUIAuth.APIToken != "current-panel-token" {
				t.Fatal("native reset polling lost panel authentication")
			}
		})
	}
}
