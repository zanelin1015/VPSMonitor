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

func TestRoutingDeletionDispatchAndPermissionPolicy(t *testing.T) {
	kind := model.XUIActionDeleteRoutingRules
	if !realtimeXUIActionAllowed(kind) {
		t.Fatal("routing deletion must dispatch immediately via WS")
	}
	if !xuiActionUsesPanelAuth(kind) {
		t.Fatal("routing deletion requires panel authentication")
	}
	if !isRootOnlyXUIActionKind(kind) {
		t.Fatal("arbitrary route deletion must not be available to area accounts")
	}
	visible := filterRootOnlyXUIActions([]model.XUIAction{{Kind: kind}, {Kind: model.XUIActionAddClient}})
	if len(visible) != 1 || visible[0].Kind != model.XUIActionAddClient {
		t.Fatalf("root-only action leaked: %#v", visible)
	}
}

func TestRoutingDeletionHTTPValidationPermissionsAndRedaction(t *testing.T) {
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
		t.Fatalf("root auth: %v", err)
	}
	rootToken, _, err := s.CreateAdminSession(root, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	manager, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-1", Password: "password123", Enabled: &enabled, AgentIDs: []string{"agent-1"}})
	if err != nil {
		t.Fatal(err)
	}
	area, ok, err := s.AuthenticateAdmin("area-1", "password123")
	if err != nil || !ok {
		t.Fatalf("area auth: %v", err)
	}
	areaToken, _, err := s.CreateAdminSession(area, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s, realtime: newRealtimeHub()}
	fingerprint := strings.Repeat("a", 64)
	valid := map[string]any{"rules_fingerprint": fingerprint, "rules": []model.RoutingRuleDeleteTarget{{Index: 2, Fingerprint: fingerprint}}}
	for _, test := range []struct {
		name, token string
		payload     map[string]any
		status      int
	}{
		{"invalid", rootToken, map[string]any{}, http.StatusBadRequest},
		{"area denied", areaToken, valid, http.StatusForbidden},
		{"root allowed", rootToken, valid, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, _ := json.Marshal(model.XUIActionRequest{Kind: model.XUIActionDeleteRoutingRules, Payload: test.payload})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/xui/actions", bytes.NewReader(body))
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: test.token})
			rec := httptest.NewRecorder()
			app.handleXUIActions(rec, req, "agent-1", nil)
			if rec.Code != test.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	for _, sanitize := range []func(model.AdminUser, *model.XUIOverview){app.sanitizeXUIOverviewForAdmin, app.sanitizeXUIOverviewForAreaAssignment} {
		views := []model.XUIRoutingRuleView{{Index: 2, Users: []string{"other-user"}, Fingerprint: fingerprint}}
		overview := &model.XUIOverview{AgentID: "agent-1", EditableRoutingRules: &views, RoutingRulesFingerprint: fingerprint}
		sanitize(model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, AgentIDs: []string{"agent-1"}}, overview)
		if overview.EditableRoutingRules != nil || overview.RoutingRulesFingerprint != "" {
			t.Fatal("unscoped saved rules leaked to an area manager")
		}
	}
}
