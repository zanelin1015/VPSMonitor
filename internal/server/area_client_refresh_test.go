package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestAreaManagerCanRefreshOnlyAssignedAgent(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"allowed", "other"} {
		if _, err := testRegisterAgent(db, model.AgentRegisterRequest{AgentID: id}); err != nil {
			t.Fatal(err)
		}
	}
	enabled := true
	if _, err := db.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-refresh", Password: "password123", Enabled: &enabled, AgentIDs: []string{"allowed"}}); err != nil {
		t.Fatal(err)
	}
	user, ok, err := db.AuthenticateAdmin("area-refresh", "password123")
	if err != nil || !ok {
		t.Fatalf("authenticate: %v %v", ok, err)
	}
	token, _, err := db.CreateAdminSession(user, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: db, realtime: newRealtimeHub()}
	session := app.realtime.registerAgentControl("allowed")
	for _, test := range []struct {
		id   string
		code int
	}{{"allowed", http.StatusOK}, {"other", http.StatusForbidden}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+test.id+"/refresh", nil)
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		app.handleAgentRefresh(rec, req, test.id)
		if rec.Code != test.code {
			t.Fatalf("%s: got %d: %s", test.id, rec.Code, rec.Body.String())
		}
	}
	select {
	case msg := <-session.ch:
		if msg.Type != model.AgentControlCollectNow {
			t.Fatalf("unexpected control: %#v", msg)
		}
	default:
		t.Fatal("expected immediate collection")
	}
	app.realtime.unregisterAgentControl("allowed", session)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/allowed/refresh", nil)
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	app.handleAgentRefresh(rec, req, "allowed")
	if rec.Code != http.StatusConflict {
		t.Fatalf("offline refresh: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAddClientSuccessRequestsSnapshot(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	agent, err := testRegisterAgent(db, model.AgentRegisterRequest{AgentID: "refresh"})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: db, realtime: newRealtimeHub()}
	session := app.realtime.registerAgentControl("refresh")
	defer app.realtime.unregisterAgentControl("refresh", session)
	for _, status := range []string{model.XUIActionStatusSucceeded, model.XUIActionStatusFailed} {
		action, err := testCreateXUIAction(db, "refresh", model.XUIActionRequest{Kind: model.XUIActionAddClient, Payload: map[string]any{"inbound_id": 1, "client": map[string]any{"email": "COMMON"}}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"status":"`+status+`"}`))
		req.Header.Set("X-Agent-Token", agent.AgentToken)
		rec := httptest.NewRecorder()
		app.handleXUIActions(rec, req, "refresh", []string{strconv.FormatInt(action.ID, 10), "result"})
		if rec.Code != http.StatusOK {
			t.Fatalf("complete: %d %s", rec.Code, rec.Body.String())
		}
		select {
		case msg := <-session.ch:
			if status != model.XUIActionStatusSucceeded || msg.Type != model.AgentControlCollectNow {
				t.Fatalf("unexpected refresh: %s %#v", status, msg)
			}
		default:
			if status == model.XUIActionStatusSucceeded {
				t.Fatal("successful add must request a fresh snapshot")
			}
		}
	}
}

func TestAreaManagerOwnCreatedClientsDefaultVisible(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, id := range []string{"att", "other"} {
		if _, err := testRegisterAgent(db, model.AgentRegisterRequest{AgentID: id}); err != nil {
			t.Fatal(err)
		}
	}
	enabled := true
	manager, err := db.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-own-clients", Password: "password123", Enabled: &enabled, AgentIDs: []string{"att"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testCreateAreaManagerAssignment(db, manager.ID, model.AreaManagerAssignmentRequest{AgentID: "att", InboundID: 4, InboundTag: "in-25319", ClientEmail: "Test", Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, AgentIDs: []string{"att"}}
	for _, email := range []string{"t1", "COMMON"} {
		action, err := db.CreateXUIActionWithActor("att", model.XUIActionRequest{Kind: model.XUIActionAddClient, Payload: map[string]any{"inbound_id": 4, "inbound_tag": "in-25319", "client": map[string]any{"email": email}}}, model.XUIActionActor{Role: user.Role, AccountID: user.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.CompleteXUIAction("att", action.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded}); err != nil {
			t.Fatal(err)
		}
	}
	// This represents a fresh Server reading successful actions that predate the fix.
	app := &App{store: db}
	makeOverview := func(agentID string) *model.XUIOverview {
		return &model.XUIOverview{AgentID: agentID, Nodes: []model.XUINodeView{{ID: 4, Tag: "in-25319"}, {ID: 5, Tag: "in-other"}}, Clients: []model.XUIClientView{
			{InboundID: 4, InboundTag: "in-25319", Email: "Test"},
			{InboundID: 4, InboundTag: "in-25319", Email: "t1"},
			{InboundID: 4, InboundTag: "in-25319", Email: "COMMON"},
			{InboundID: 4, InboundTag: "in-25319", Email: "someone-else"},
			{InboundID: 5, InboundTag: "in-other", Email: "COMMON"},
		}}
	}
	overview := makeOverview("att")
	app.sanitizeXUIOverviewForAdmin(user, overview)
	if len(overview.Clients) != 3 || overview.ClientCount != 3 || len(overview.Nodes) != 1 || overview.Nodes[0].ClientCount != 3 {
		t.Fatalf("expected Test plus self-created t1/COMMON with matching counters, got %#v", overview)
	}
	if overview.Clients[1].Email != "t1" || overview.Clients[2].Email != "COMMON" {
		t.Fatalf("missing own clients: %#v", overview.Clients)
	}
	picker := makeOverview("att")
	app.sanitizeXUIOverviewForAreaAssignment(user, picker)
	if len(picker.Clients) != 3 {
		t.Fatalf("created clients must also appear in authorization picker: %#v", picker.Clients)
	}
	other := makeOverview("other")
	app.sanitizeXUIOverviewForAdmin(user, other)
	if len(other.Clients) != 0 {
		t.Fatalf("same email on another machine must not inherit access: %#v", other.Clients)
	}
}

func TestAreaManagerNodeOnlyCanCreateWithoutSeeingExistingClients(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := testRegisterAgent(db, model.AgentRegisterRequest{AgentID: "node-only"}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	manager, err := db.CreateAreaManager(model.AreaManagerAccountRequest{Username: "node-only", Password: "password123", Enabled: &enabled, AgentIDs: []string{"node-only"}})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := testCreateAreaManagerAssignment(db, manager.ID, model.AreaManagerAssignmentRequest{AgentID: "node-only", InboundID: 4, InboundTag: "in-4", NodeOnly: true, Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if !grant.NodeOnly {
		t.Fatal("node-only authorization was not persisted")
	}
	app := &App{store: db}
	user := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, AgentIDs: []string{"node-only"}}
	overview := func() *model.XUIOverview {
		return &model.XUIOverview{AgentID: "node-only", Nodes: []model.XUINodeView{{ID: 4, Tag: "in-4"}}, Clients: []model.XUIClientView{{InboundID: 4, InboundTag: "in-4", Email: "existing"}, {InboundID: 4, InboundTag: "in-4", Email: "own"}}}
	}
	dashboard := &model.GlobalDashboardView{Agents: []model.DashboardAgentView{{AgentID: "node-only"}}}
	applyAreaManagerClientCounts(dashboard, app.areaManagerClientScope(user))
	if dashboard.Agents[0].NodeCount != 1 || dashboard.Agents[0].ClientCount != 0 {
		t.Fatalf("expected one node and no clients: %#v", dashboard.Agents)
	}
	before := overview()
	app.sanitizeXUIOverviewForAdmin(user, before)
	if len(before.Clients) != 0 || len(before.Nodes) != 1 {
		t.Fatalf("only the node should be visible before creation: %#v", before)
	}
	empty := overview()
	empty.Clients = nil
	app.sanitizeXUIOverviewForAdmin(user, empty)
	if len(empty.Nodes) != 1 {
		t.Fatal("an empty node must remain available")
	}
	if app.areaManagerXUIActionAllowed(user, "node-only", model.XUIActionRequest{Kind: model.XUIActionDeleteClient, Payload: map[string]any{"inbound_id": 4, "inbound_tag": "in-4", "client_id": "existing-id"}}) {
		t.Fatal("node-only grants must not delete arbitrary clients by UUID")
	}
	payload := map[string]any{"inbound_id": 4, "inbound_tag": "in-4", "client": map[string]any{"email": "own"}}
	if !app.areaManagerXUIActionAllowed(user, "node-only", model.XUIActionRequest{Kind: model.XUIActionAddClient, Payload: payload}) {
		t.Fatal("node-only grant must allow adding the first client")
	}
	if app.areaManagerCustomerAssignmentAllowed(user, model.CustomerAssignmentRequest{AgentID: "node-only", InboundID: 4, InboundTag: "in-4"}) {
		t.Fatal("node-only grant must not delegate all existing clients")
	}
	action, err := db.CreateXUIActionWithActor("node-only", model.XUIActionRequest{Kind: model.XUIActionAddClient, Payload: payload}, model.XUIActionActor{Role: user.Role, AccountID: user.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CompleteXUIAction("node-only", action.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded}); err != nil {
		t.Fatal(err)
	}
	after := overview()
	app.sanitizeXUIOverviewForAdmin(user, after)
	if len(after.Clients) != 1 || after.Clients[0].Email != "own" {
		t.Fatalf("only the self-created client should become visible: %#v", after.Clients)
	}
	picker := overview()
	app.sanitizeXUIOverviewForAreaAssignment(user, picker)
	if len(picker.Nodes) != 1 || picker.Nodes[0].CanAssignAllClients == nil || *picker.Nodes[0].CanAssignAllClients || len(picker.Clients) != 1 {
		t.Fatalf("picker must allow only the created client: %#v", picker)
	}
}
