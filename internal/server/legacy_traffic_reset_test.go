package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestLegacyTrafficResetEndpointsCannotResumeOrQueueReset(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "retired.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	agent, err := testRegisterAgent(s, model.AgentRegisterRequest{AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	root, ok, err := s.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatal("root login failed")
	}
	token, _, err := s.CreateAdminSession(root, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{store: s, realtime: newRealtimeHub()}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPost} {
		req := httptest.NewRequest(method, "/api/v1/admin/traffic-reset", bytes.NewBufferString(`{"enabled":true}`))
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		w := httptest.NewRecorder()
		a.handleAdmin(w, req)
		if w.Code != http.StatusGone || !strings.Contains(w.Body.String(), model.LegacyTrafficResetRemovedMessage) {
			t.Fatalf("retired admin endpoint still active: %d %s", w.Code, w.Body.String())
		}
	}
	for _, test := range []struct {
		token string
		code  int
	}{{"", http.StatusUnauthorized}, {agent.AgentToken, http.StatusGone}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent/traffic-reset-validation?action_id=1", nil)
		req.Header.Set("X-Agent-Token", test.token)
		w := httptest.NewRecorder()
		a.handleAgentByID(w, req)
		if w.Code != test.code {
			t.Fatalf("legacy Client validation status: %d want %d: %s", w.Code, test.code, w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent/xui/actions", bytes.NewBufferString(`{"kind":"reset_client_traffic","payload":{}}`))
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	w := httptest.NewRecorder()
	a.handleXUIActions(w, req, agent.AgentID, nil)
	if w.Code != http.StatusGone {
		t.Fatalf("removed reset could be queued directly: %d %s", w.Code, w.Body.String())
	}
	if actions, err := s.ListXUIActions(agent.AgentID, 100); err != nil || len(actions) != 0 {
		t.Fatalf("removed endpoint queued an action: %+v %v", actions, err)
	}
}
