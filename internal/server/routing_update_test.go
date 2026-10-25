package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestRoutingUpdateHTTPPermissionsAndVersionGate(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	root, ok, err := s.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatal("root auth failed")
	}
	rootToken, _, err := s.CreateAdminSession(root, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testRegisterAgent(s, model.AgentRegisterRequest{AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-1", Password: "password123", Enabled: &enabled, AgentIDs: []string{"agent-1"}}); err != nil {
		t.Fatal(err)
	}
	area, ok, err := s.AuthenticateAdmin("area-1", "password123")
	if err != nil || !ok {
		t.Fatal("area auth failed")
	}
	areaToken, _, err := s.CreateAdminSession(area, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s, realtime: newRealtimeHub()}
	payload := map[string]any{"rule_index": 2, "rules_fingerprint": strings.Repeat("a", 64), "rule_fingerprint": strings.Repeat("b", 64), "rule": map[string]any{"user": []string{"alice"}, "outboundTag": "direct"}}
	for _, tc := range []struct {
		name, token, version string
		template             bool
		payload              map[string]any
		status               int
	}{
		{"invalid", rootToken, "0.3.50", true, map[string]any{}, http.StatusBadRequest},
		{"old client", rootToken, "0.3.49", true, payload, http.StatusConflict},
		{"unknown client", rootToken, "unknown", true, payload, http.StatusConflict},
		{"runtime rules only", rootToken, "0.3.50", false, payload, http.StatusConflict},
		{"area forbidden", areaToken, "0.3.50", true, payload, http.StatusForbidden},
		{"root allowed", rootToken, "0.3.50", true, payload, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xui := &model.XUISnapshot{}
			if tc.template {
				xui.RoutingTemplate = &model.XUIRoutingTemplate{Rules: []map[string]any{}}
			}
			if err := s.SaveSnapshot(model.AgentSnapshot{AgentID: "agent-1", Version: tc.version, ReportedAt: time.Now(), XUI: xui}); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(model.XUIActionRequest{Kind: model.XUIActionUpdateRoutingRule, Payload: tc.payload})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/xui/actions", bytes.NewReader(body))
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: tc.token})
			response := httptest.NewRecorder()
			app.handleXUIActions(response, req, "agent-1", nil)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	actions, err := s.ListXUIActions("agent-1", 100)
	if err != nil || len(actions) != 1 {
		t.Fatal("invalid or forbidden updates entered the queue")
	}
}
