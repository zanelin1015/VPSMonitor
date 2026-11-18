package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

type customerOwnerFixture struct {
	t       *testing.T
	s       *store.SQLiteStore
	app     *App
	root    string
	area    string
	other   string
	manager model.AreaManagerAdminView
	peer    model.AreaManagerAdminView
}

func newCustomerOwnerFixture(t *testing.T) *customerOwnerFixture {
	t.Helper()
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	f := &customerOwnerFixture{t: t, s: s, app: &App{store: s}}
	for _, name := range []string{"area-b", "area-c"} {
		manager, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: name, Password: "password123"})
		if err != nil {
			t.Fatal(err)
		}
		if name == "area-b" {
			f.manager = manager
		} else {
			f.peer = manager
		}
	}
	f.root, f.area, f.other = f.login("admin"), f.login("area-b"), f.login("area-c")
	return f
}

func (f *customerOwnerFixture) login(name string) string {
	f.t.Helper()
	user, ok, err := f.s.AuthenticateAdmin(name, "password123")
	if err != nil || !ok {
		f.t.Fatalf("authenticate %s: ok=%v err=%v", name, ok, err)
	}
	token, _, err := f.s.CreateAdminSession(user, time.Hour)
	if err != nil {
		f.t.Fatal(err)
	}
	return token
}

func (f *customerOwnerFixture) request(token, method, path string, body any, status int) *httptest.ResponseRecorder {
	f.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		f.t.Fatal(err)
	}
	r := httptest.NewRequest(method, "https://monitor.example/api/v1/admin/customers"+path, bytes.NewReader(encoded))
	r.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, r)
	if w.Code != status {
		f.t.Fatalf("%s %s: expected %d, got %d %s", method, path, status, w.Code, w.Body.String())
	}
	return w
}

func (f *customerOwnerFixture) customer(token, name string, owner *int64) model.CustomerAdminView {
	f.t.Helper()
	w := f.request(token, http.MethodPost, "", model.CustomerAccountRequest{Username: name, Password: "password123", OwnerAreaManagerID: owner}, http.StatusOK)
	var customer model.CustomerAdminView
	if err := json.Unmarshal(w.Body.Bytes(), &customer); err != nil {
		f.t.Fatal(err)
	}
	return customer
}

func (f *customerOwnerFixture) grant(managerID int64, email string) {
	f.t.Helper()
	if _, found, _ := f.s.GetAgent("owner-node"); !found {
		if _, err := testRegisterAgent(f.s, model.AgentRegisterRequest{AgentID: "owner-node"}); err != nil {
			f.t.Fatal(err)
		}
	}
	if _, err := testCreateAreaManagerAssignment(f.s, managerID, model.AreaManagerAssignmentRequest{AgentID: "owner-node", InboundID: 7, InboundTag: "in-7", ClientEmail: email}); err != nil {
		f.t.Fatal(err)
	}
}

func TestCustomerOwnerCreationAndAccess(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	assigned := f.customer(f.root, "assigned-a", &f.manager.ID)
	if assigned.OwnerType != model.AdminRoleAreaManager || assigned.OwnerID != f.manager.ID {
		t.Fatalf("root-selected owner not saved: %#v", assigned.CustomerUser)
	}
	path := fmt.Sprintf("/%d/assignments", assigned.ID)
	f.request(f.area, http.MethodGet, path, nil, http.StatusOK)
	f.request(f.other, http.MethodGet, path, nil, http.StatusNotFound)
	defaultRoot := f.customer(f.root, "root-default", nil)
	if defaultRoot.OwnerType != model.AdminRoleRoot || defaultRoot.OwnerID != 1 {
		t.Fatalf("legacy root creation changed: %#v", defaultRoot.CustomerUser)
	}
	areaCustomer := f.customer(f.area, "area-default", nil)
	if areaCustomer.OwnerType != model.AdminRoleAreaManager || areaCustomer.OwnerID != f.manager.ID {
		t.Fatalf("area creation must retain its owner: %#v", areaCustomer.CustomerUser)
	}
	var visible []model.CustomerAdminView
	if err := json.Unmarshal(f.request(f.area, http.MethodGet, "", nil, http.StatusOK).Body.Bytes(), &visible); err != nil {
		t.Fatal(err)
	}
	if len(visible) != 2 {
		t.Fatalf("area should see assigned and self-created users only: %#v", visible)
	}
	if err := json.Unmarshal(f.request(f.root, http.MethodGet, "", nil, http.StatusOK).Body.Bytes(), &visible); err != nil || len(visible) != 3 {
		t.Fatalf("root should retain all users: %#v err=%v", visible, err)
	}
	for _, owner := range []int64{0, f.manager.ID, f.peer.ID} {
		f.request(f.area, http.MethodPut, fmt.Sprintf("/%d", areaCustomer.ID), map[string]any{"owner_area_manager_id": owner}, http.StatusForbidden)
		f.request(f.area, http.MethodPost, "", map[string]any{"username": "forged", "owner_area_manager_id": owner}, http.StatusForbidden)
	}
	if _, err := f.s.CreateOperator(model.OperatorAccountRequest{Username: "operator", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	operator := f.login("operator")
	f.request(operator, http.MethodPut, fmt.Sprintf("/%d", assigned.ID), map[string]any{"owner_area_manager_id": 0}, http.StatusForbidden)
}

func TestCustomerOwnerTransferPreservesAccountAndGrants(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	f.grant(f.manager.ID, "alice@example.com")
	f.grant(f.peer.ID, "alice@example.com")
	customer := f.customer(f.root, "alice", nil)
	amount := 18.5
	assignment, err := f.s.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{AgentID: "owner-node", InboundID: 7, InboundTag: "in-7", ClientID: "uuid-a", ClientEmail: "alice@example.com", PriceMode: "override", RevenueAmount: &amount, RevenueCurrency: "CNY", RevenueCycle: "month", PublicClientName: "Alice line"})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := f.s.EnsureCustomerSubscriptionToken(customer.ID)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := f.s.CreateCustomerSession(customer.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/%d", customer.ID)
	for _, owner := range []int64{f.manager.ID, f.peer.ID, 0} {
		f.request(f.root, http.MethodPut, path, map[string]any{"owner_area_manager_id": owner}, http.StatusOK)
		stored, found, err := f.s.GetCustomer(customer.ID)
		if err != nil || !found || len(stored.Assignments) != 1 || !reflect.DeepEqual(stored.Assignments[0], assignment) {
			t.Fatalf("transfer changed grants: %#v err=%v", stored, err)
		}
		if token, err := f.s.EnsureCustomerSubscriptionToken(customer.ID); err != nil || token != subscription {
			t.Fatalf("transfer rotated subscription: %q err=%v", token, err)
		}
		if _, ok, err := f.s.AuthenticateCustomer("alice", "password123"); err != nil || !ok {
			t.Fatalf("transfer changed password: ok=%v err=%v", ok, err)
		}
		if user, _, ok, err := f.s.ValidateCustomerSession(session); err != nil || !ok || user.OwnerType != stored.OwnerType || user.OwnerID != stored.OwnerID {
			t.Fatalf("existing customer session must survive and see its current owner: %#v ok=%v err=%v", user, ok, err)
		}
		for _, area := range []struct {
			token string
			id    int64
		}{{f.area, f.manager.ID}, {f.other, f.peer.ID}} {
			status := http.StatusNotFound
			if area.id == owner {
				status = http.StatusOK
			}
			f.request(area.token, http.MethodGet, path+"/assignments", nil, status)
		}
	}
}

func TestCustomerOwnerTransferRejectsUnauthorizedGrantsBeforeUpdating(t *testing.T) {
	for _, grant := range []string{"no-grant", "other-client", "whole-inbound", "disabled-assignment", "owned-customer-is-not-a-grant"} {
		t.Run(grant, func(t *testing.T) {
			f := newCustomerOwnerFixture(t)
			f.grant(f.manager.ID, "allowed@example.com")
			customer := f.customer(f.root, "transfer-a", nil)
			email := "outside@example.com"
			if grant == "whole-inbound" {
				email = ""
			}
			enabled := grant != "disabled-assignment"
			agentID := "owner-node"
			if grant == "no-grant" {
				agentID = "unassigned-node"
				if _, err := testRegisterAgent(f.s, model.AgentRegisterRequest{AgentID: agentID}); err != nil {
					t.Fatal(err)
				}
			}
			assignmentReq := model.CustomerAssignmentRequest{AgentID: agentID, InboundID: 7, InboundTag: "in-7", ClientEmail: email, Enabled: &enabled}
			if _, err := f.s.CreateCustomerAssignment(customer.ID, assignmentReq); err != nil {
				t.Fatal(err)
			}
			if grant == "owned-customer-is-not-a-grant" {
				owned := f.customer(f.root, "already-owned", &f.manager.ID)
				if _, err := f.s.CreateCustomerAssignment(owned.ID, assignmentReq); err != nil {
					t.Fatal(err)
				}
			}
			before, _, _ := f.s.GetCustomer(customer.ID)
			f.request(f.root, http.MethodPut, fmt.Sprintf("/%d", customer.ID), map[string]any{"owner_area_manager_id": f.manager.ID, "display_name": "must-not-save", "password": "must-not-change"}, http.StatusBadRequest)
			after, _, _ := f.s.GetCustomer(customer.ID)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected transfer partially updated customer: before=%#v after=%#v", before, after)
			}
			if _, ok, err := f.s.AuthenticateCustomer("transfer-a", "password123"); err != nil || !ok {
				t.Fatalf("rejected transfer changed credentials: ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestCustomerOwnerValidationAndLegacyPartialUpdates(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	customer := f.customer(f.root, "owner-validation", &f.manager.ID)
	off := false
	if _, err := f.s.UpdateAreaManager(f.peer.ID, model.AreaManagerAccountRequest{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []int64{-1, 99999, f.peer.ID} {
		f.request(f.root, http.MethodPost, "", map[string]any{"username": "invalid-owner", "owner_area_manager_id": owner}, http.StatusBadRequest)
		f.request(f.root, http.MethodPut, fmt.Sprintf("/%d", customer.ID), map[string]any{"owner_area_manager_id": owner}, http.StatusBadRequest)
	}
	if _, err := f.s.UpdateAreaManager(f.manager.ID, model.AreaManagerAccountRequest{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	// Editing a disabled owner's existing user and legacy status-only requests
	// must not reassign it to the root or run transfer validation.
	f.request(f.root, http.MethodPut, fmt.Sprintf("/%d", customer.ID), map[string]any{"owner_area_manager_id": f.manager.ID, "display_name": "Updated"}, http.StatusOK)
	f.request(f.root, http.MethodPut, fmt.Sprintf("/%d", customer.ID), map[string]any{"enabled": false}, http.StatusOK)
	stored, _, err := f.s.GetCustomer(customer.ID)
	if err != nil || stored.OwnerType != model.AdminRoleAreaManager || stored.OwnerID != f.manager.ID || stored.Enabled {
		t.Fatalf("partial update changed ownership: %#v err=%v", stored, err)
	}
}

func TestCustomerOwnerTransferValidatesAccountAndAssignmentFrontProxies(t *testing.T) {
	for _, assignmentProxy := range []bool{false, true} {
		for _, disabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("assignment=%v/disabled=%v", assignmentProxy, disabled), func(t *testing.T) {
				f := newCustomerOwnerFixture(t)
				f.grant(f.manager.ID, "allowed@example.com")
				customer := f.customer(f.root, "proxy-user", nil)
				proxy, err := f.s.CreateFrontProxyNode(model.FrontProxyNodeRequest{Name: "Front", ShareURL: "socks5://user:pass@proxy.example:1080"})
				if err != nil {
					t.Fatal(err)
				}
				if assignmentProxy {
					if _, err := f.s.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{AgentID: "owner-node", InboundID: 7, InboundTag: "in-7", ClientEmail: "allowed@example.com", FrontProxyNodeIDs: []int64{proxy.ID}}); err != nil {
						t.Fatal(err)
					}
				} else if err := f.s.ReplaceFrontProxyGrants(model.FrontProxyGranteeCustomer, customer.ID, []int64{proxy.ID}); err != nil {
					t.Fatal(err)
				}
				if disabled {
					off := false
					if _, err := f.s.UpdateFrontProxyNode(proxy.ID, model.FrontProxyNodeRequest{Name: proxy.Name, ShareURL: proxy.ShareURL, Enabled: &off}); err != nil {
						t.Fatal(err)
					}
				}
				path := fmt.Sprintf("/%d", customer.ID)
				f.request(f.root, http.MethodPut, path, map[string]any{"owner_area_manager_id": f.manager.ID}, http.StatusBadRequest)
				if assignmentProxy {
					// Removing account-level grants cannot hide an unauthorized link proxy.
					f.request(f.root, http.MethodPut, path, map[string]any{"owner_area_manager_id": f.manager.ID, "front_proxy_node_ids": []int64{}}, http.StatusBadRequest)
				}
				if err := f.s.ReplaceFrontProxyGrants(model.FrontProxyGranteeAreaManager, f.manager.ID, []int64{proxy.ID}); err != nil {
					t.Fatal(err)
				}
				before, _, _ := f.s.GetCustomer(customer.ID)
				f.request(f.root, http.MethodPut, path, map[string]any{"owner_area_manager_id": f.manager.ID}, http.StatusOK)
				after, _, _ := f.s.GetCustomer(customer.ID)
				if !reflect.DeepEqual(before.FrontProxies, after.FrontProxies) || !reflect.DeepEqual(before.Assignments, after.Assignments) {
					t.Fatal("authorized transfer changed existing front proxies")
				}
			})
		}
	}
}
