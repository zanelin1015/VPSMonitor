package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestCustomerAssignmentBindingAcrossRealmHops(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	admin, ok, err := s.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatalf("authenticate: ok=%v err=%v", ok, err)
	}
	token, _, err := s.CreateAdminSession(admin, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	forwardEntry := func(host string, listenPort int, targetID string, targetPort int) model.AgentEntryConfig {
		return model.AgentEntryConfig{
			ImportDomain: host,
			PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
				ID: "rule", Name: "YT-CNIX-HK", Enabled: true, ListenPort: listenPort,
				TargetAgentID: targetID, TargetAddress: targetID + ".example.com", TargetPort: targetPort, Network: "tcp+udp",
			}}},
		}
	}
	entries := map[string]model.AgentEntryConfig{
		"gz":   forwardEntry("gz.example.com", 20002, "ix", 15601),
		"ix":   forwardEntry("ix.example.com", 15601, "dmit", 20002),
		"dmit": {ImportDomain: "dmit.example.com"},
	}
	now := time.Now().UTC()
	for id, entry := range entries {
		if _, err := testRegisterAgent(s, model.AgentRegisterRequest{AgentID: id, AgentName: id}); err != nil {
			t.Fatal(err)
		}
		if _, err := testUpdateAgentConfig(s, id, model.ManagedAgentConfig{AgentID: id, Entry: entry}); err != nil {
			t.Fatal(err)
		}
		snapshot := model.AgentSnapshot{AgentID: id, ReportedAt: now}
		if id == "dmit" {
			snapshot.XUI = &model.XUISnapshot{CollectedAt: now, Inbounds: []map[string]any{{
				"id": 2, "tag": "in-20002-tcp", "remark": "HK", "enable": true, "protocol": "vless", "port": 20002,
				"settings": `{"clients":[{"id":"stable-lei","email":"lei","enable":true}]}`,
			}}}
		}
		if err := s.SaveSnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	customer, err := testCreateCustomer(s, model.CustomerAccountRequest{Username: "lei", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	assignment, err := s.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{
		AgentID: "gz", InboundID: 20002, InboundTag: "YT-CNIX-HK", ClientID: "stable-lei", ClientEmail: "lei", Enabled: &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s}
	reconciliation, err := app.customerBillingReconciliationForAdmin(admin)
	if err != nil || reconciliation.Matched != 1 || len(reconciliation.Items) != 1 || reconciliation.Items[0].CanRebind {
		t.Fatalf("Realm assignment should match without repairing data: err=%v result=%#v", err, reconciliation)
	}
	agents, err := s.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	context := buildForwardedOverviewContext(agents, s.ListLatest())
	clients := customerAssignmentBindingClients("gz", context)
	if len(clients) != 1 || clients[0].InboundID != 20002 || clients[0].RealmTargetAgentID != "dmit" || clients[0].RealmTargetInboundID != 2 {
		t.Fatalf("expected entry listener with final DMIT identity: %#v", clients)
	}
	if len(context.targetOverviewByAgent["dmit"].Clients) != 1 || context.targetOverviewByAgent["dmit"].Clients[0].InboundID != 2 {
		t.Fatal("diagnostics mutated the target snapshot")
	}
	view := dashboard.BuildGlobalDashboardWithOptions(agents, s.ListLatest(), dashboard.GlobalDashboardOptions{})
	for _, agent := range view.Agents {
		if agent.AgentID == "gz" && len(agent.FinanceClients) != 0 {
			t.Fatal("diagnostics must not duplicate forwarded clients in finance totals")
		}
	}
	wrongListener := assignment
	wrongListener.InboundID = 20003
	if client, status, _ := reconcileCustomerAssignment(wrongListener, clients); client != nil || status != "missing" {
		t.Fatalf("same ID on another listener must not match: %s %#v", status, client)
	}
	// Exercise the actual rebind route: the selected client stays on the entry
	// machine and listener, rather than silently changing the grant to DMIT.
	request := httptest.NewRequest(http.MethodPost,
		"https://monitor.example/api/v1/admin/customers/"+strconv.FormatInt(customer.ID, 10)+"/assignments/"+strconv.FormatInt(assignment.ID, 10)+"/rebind",
		strings.NewReader(`{"client_id":"stable-lei"}`))
	request.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	var updated model.CustomerAssignment
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("rebind failed: %d %s err=%v", recorder.Code, recorder.Body.String(), err)
	}
	if updated.AgentID != "gz" || updated.InboundID != 20002 || updated.ClientID != "stable-lei" {
		t.Fatalf("rebind changed the entry namespace: %#v", updated)
	}
	if _, found, err := app.currentXUIClient("gz", "stable-lei", 20003); err != nil || found {
		t.Fatalf("rebind accepted another listener: found=%v err=%v", found, err)
	}
	// A break in the intermediate forwarding rule must stop resolving the grant.
	ix := entries["ix"]
	ix.PortForwarding.Rules[0].Enabled = false
	if _, err := testUpdateAgentConfig(s, "ix", model.ManagedAgentConfig{AgentID: "ix", Entry: ix}); err != nil {
		t.Fatal(err)
	}
	reconciliation, err = app.customerBillingReconciliationForAdmin(admin)
	if err != nil || reconciliation.Matched != 0 || reconciliation.Missing != 1 {
		t.Fatalf("broken intermediate hop should not match DMIT by ID alone: err=%v result=%#v", err, reconciliation)
	}
}
