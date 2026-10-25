package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestCustomerAssignmentPricingDoesNotMutateSharedClientBilling(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	user, ok, err := s.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatalf("authenticate: ok=%v err=%v", ok, err)
	}
	token, _, err := s.CreateAdminSession(user, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testRegisterAgent(s, model.AgentRegisterRequest{AgentID: "shared-agent", AgentName: "Shared"}); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := s.GetAgentConfig("shared-agent")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Renewal.ClientBillings = []model.XUIClientBillingConfig{{InboundID: 7, Email: "shared@example.com", TrafficMultiplier: 2, RevenueAmount: 99, RevenueCurrency: "CNY", RevenueCycle: "month"}}
	if _, err := testUpdateAgentConfig(s, "shared-agent", cfg); err != nil {
		t.Fatal(err)
	}
	before, _, err := s.GetAgentConfig("shared-agent")
	if err != nil {
		t.Fatal(err)
	}
	customer, err := testCreateCustomer(s, model.CustomerAccountRequest{Username: "buyer", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s}
	baseURL := "https://monitor.example/api/v1/admin/customers/" + strconv.FormatInt(customer.ID, 10) + "/assignments"
	request := func(method, url, body string) model.CustomerAssignment {
		t.Helper()
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s assignment: %d %s", method, rec.Code, rec.Body.String())
		}
		var assignment model.CustomerAssignment
		if err := json.Unmarshal(rec.Body.Bytes(), &assignment); err != nil {
			t.Fatal(err)
		}
		after, _, err := s.GetAgentConfig("shared-agent")
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("customer assignment changed shared client config: err=%v before=%#v after=%#v", err, before, after)
		}
		return assignment
	}
	// Old callers may still send pricing and multiplier without an explicit price mode.
	legacyBody := `{"agent_id":"shared-agent","inbound_id":7,"client_email":"shared@example.com","revenue_amount":188,"traffic_multiplier":8}`
	assignment := request(http.MethodPost, baseURL, legacyBody)
	if assignment.PriceMode != "inherit" || assignment.RevenueAmount != nil {
		t.Fatalf("legacy assignment should inherit pricing: %#v", assignment)
	}
	url := baseURL + "/" + strconv.FormatInt(assignment.ID, 10)
	request(http.MethodPut, url, legacyBody)
	override := request(http.MethodPut, url, `{"agent_id":"shared-agent","inbound_id":7,"client_email":"shared@example.com","price_mode":"override","revenue_amount":188,"revenue_currency":"USDT","revenue_cycle":"quarter"}`)
	if override.PriceMode != "override" || override.RevenueAmount == nil || *override.RevenueAmount != 188 || override.RevenueCurrency != "USDT" {
		t.Fatalf("customer-specific price should remain supported: %#v", override)
	}
}

func TestAdminCustomerSubscriptionURLIsScopedToVisibleCustomers(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	if err := sqliteStore.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatalf("EnsureAdminAccount: %v", err)
	}
	enabled := true
	manager, err := sqliteStore.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-subscription",
		Password: "password123",
		Enabled:  &enabled,
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	rootCustomer, err := testCreateCustomer(sqliteStore, model.CustomerAccountRequest{Username: "root-subscription", Password: "password123"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	areaCustomer, err := sqliteStore.CreateCustomerForOwner(model.CustomerAccountRequest{Username: "area-subscription-customer", Password: "password123"}, model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}
	rootUser, ok, err := sqliteStore.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin root: ok=%v err=%v", ok, err)
	}
	rootToken, _, err := sqliteStore.CreateAdminSession(rootUser, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession root: %v", err)
	}
	areaUser, ok, err := sqliteStore.AuthenticateAdmin("area-subscription", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin area: ok=%v err=%v", ok, err)
	}
	areaToken, _, err := sqliteStore.CreateAdminSession(areaUser, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession area: %v", err)
	}
	app := &App{store: sqliteStore}
	request := func(customerID int64, sessionToken string) (int, model.CustomerSubscriptionURLResponse) {
		req := httptest.NewRequest(http.MethodGet, "https://monitor.example/api/v1/admin/customers/"+strconv.FormatInt(customerID, 10)+"/subscription", nil)
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: sessionToken})
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, req)
		var response model.CustomerSubscriptionURLResponse
		_ = json.NewDecoder(recorder.Body).Decode(&response)
		return recorder.Code, response
	}

	status, rootURLs := request(rootCustomer.ID, rootToken)
	if status != http.StatusOK || !strings.HasPrefix(rootURLs.ClashSubscriptionURL, "https://monitor.example/api/v1/customer/subscription/") {
		t.Fatalf("root subscription URL response: status=%d body=%#v", status, rootURLs)
	}
	status, areaURLs := request(areaCustomer.ID, areaToken)
	if status != http.StatusOK || !strings.HasPrefix(areaURLs.ClashSubscriptionURL, "https://monitor.example/api/v1/customer/subscription/") {
		t.Fatalf("area subscription URL response: status=%d body=%#v", status, areaURLs)
	}
	status, _ = request(rootCustomer.ID, areaToken)
	if status != http.StatusNotFound {
		t.Fatalf("expected area manager to be denied access to root customer, got status=%d", status)
	}
}

func TestReconcileCustomerAssignmentUsesStableClientIDBeforeLegacyEmail(t *testing.T) {
	clients := []model.XUIClientView{{
		ClientID:   "client-uuid-current",
		InboundID:  7,
		InboundTag: "entry-hk",
		Email:      "actual@example.com",
		Comment:    "Claudia",
	}}

	client, status, _ := reconcileCustomerAssignment(model.CustomerAssignment{
		InboundID:   7,
		InboundTag:  "entry-hk",
		ClientID:    "client-uuid-current",
		ClientEmail: "stale@example.com",
	}, clients)
	if status != "matched" || client == nil || client.Email != "actual@example.com" {
		t.Fatalf("stable id should win over stale email: status=%s client=%#v", status, client)
	}

	client, status, _ = reconcileCustomerAssignment(model.CustomerAssignment{
		InboundID:   7,
		InboundTag:  "entry-hk",
		ClientEmail: "actual@example.com",
	}, clients)
	if status != "legacy" || client == nil {
		t.Fatalf("legacy email-only assignment should be reported as upgradeable: status=%s client=%#v", status, client)
	}

	client, status, _ = reconcileCustomerAssignment(model.CustomerAssignment{
		InboundID:        7,
		InboundTag:       "entry-hk",
		ClientEmail:      "Claudia",
		PublicClientName: "HK Claudia",
	}, clients)
	if status != "mismatch" || client == nil {
		t.Fatalf("comment-based assignment should be flagged for repair: status=%s client=%#v", status, client)
	}
}
