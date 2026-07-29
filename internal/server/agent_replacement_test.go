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

func TestHandleAgentReplacementRequiresRootAndDispatchesUpdatedConfigs(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if err := sqliteStore.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatalf("EnsureAdminAccount: %v", err)
	}
	admin, ok, err := sqliteStore.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin admin ok=%v err=%v", ok, err)
	}
	adminToken, _, err := sqliteStore.CreateAdminSession(admin, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession admin: %v", err)
	}
	for _, agentID := range []string{"old", "new", "entry"} {
		if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: agentID}); err != nil {
			t.Fatalf("RegisterAgent %s: %v", agentID, err)
		}
	}
	if _, err := sqliteStore.UpdateAgentConfig("new", model.ManagedAgentConfig{
		AgentID: "new",
		Entry:   model.AgentEntryConfig{ImportDomain: "new.example.com"},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig new: %v", err)
	}
	if _, err := sqliteStore.UpdateAgentConfig("entry", model.ManagedAgentConfig{
		AgentID: "entry",
		Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{
			Enabled: true,
			Backend: "realm",
			Rules: []model.RealmForwardRule{{
				ID:            "entry-20001",
				Enabled:       true,
				ListenPort:    20001,
				TargetAgentID: "old",
				TargetAddress: "old.example.com",
				TargetPort:    20001,
			}},
		}},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig entry: %v", err)
	}
	enabled := true
	if _, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"old"},
	}); err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	area, ok, err := sqliteStore.AuthenticateAdmin("area", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin area ok=%v err=%v", ok, err)
	}
	areaToken, _, err := sqliteStore.CreateAdminSession(area, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession area: %v", err)
	}

	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	body, _ := json.Marshal(model.AgentReplacementRequest{ReplacementAgentID: "new"})
	areaReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/old/replace", bytes.NewReader(body))
	areaReq.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: areaToken})
	areaRec := httptest.NewRecorder()
	app.handleAgentByID(areaRec, areaReq)
	if areaRec.Code != http.StatusForbidden {
		t.Fatalf("area manager replacement status=%d body=%s", areaRec.Code, areaRec.Body.String())
	}

	controlSession := app.realtime.registerAgentControl("entry")
	defer app.realtime.unregisterAgentControl("entry", controlSession)
	adminReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/old/replace", bytes.NewReader(body))
	adminReq.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: adminToken})
	adminRec := httptest.NewRecorder()
	app.handleAgentByID(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin replacement status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
	var result model.AgentReplacementResult
	if err := json.NewDecoder(adminRec.Body).Decode(&result); err != nil {
		t.Fatalf("decode replacement result: %v", err)
	}
	if result.RealmReferencesUpdated != 1 || len(result.UpdatedConfigAgentIDs) != 1 || result.UpdatedConfigAgentIDs[0] != "entry" {
		t.Fatalf("unexpected replacement result: %#v", result)
	}
	if len(result.ConfigApplySentAgentIDs) != 1 || result.ConfigApplySentAgentIDs[0] != "entry" {
		t.Fatalf("expected realtime apply result: %#v", result)
	}
	select {
	case message := <-controlSession.ch:
		if message.Type != model.AgentControlApplyConfig || message.Config == nil {
			t.Fatalf("expected apply_config control message, got %#v", message)
		}
		if got := message.Config.Entry.PortForwarding.Rules[0].TargetAgentID; got != "new" {
			t.Fatalf("expected dispatched config to target replacement agent, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected replacement to dispatch updated config")
	}
}
