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

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

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
	rootCustomer, err := sqliteStore.CreateCustomer(model.CustomerAccountRequest{Username: "root-subscription", Password: "password123"})
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

func TestSyncCustomerAssignmentRevenueCreatesBilling(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "hk-01",
		AgentName: "HK 01",
	}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	app := &App{store: sqliteStore}
	amount := 188.5
	multiplier := 2.0
	if err := app.syncCustomerAssignmentRevenue(model.CustomerAssignmentRequest{
		AgentID:           "hk-01",
		InboundID:         7,
		InboundTag:        "entry-hk",
		ClientEmail:       "alice@example.com",
		TrafficMultiplier: &multiplier,
		RevenueAmount:     &amount,
		RevenueCurrency:   "usdt",
		RevenueCycle:      "semiannual",
	}, "admin"); err != nil {
		t.Fatalf("syncCustomerAssignmentRevenue: %v", err)
	}

	cfg, found, err := sqliteStore.GetAgentConfig("hk-01")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found {
		t.Fatalf("expected agent config")
	}
	if len(cfg.Renewal.ClientBillings) != 1 {
		t.Fatalf("expected 1 billing, got %#v", cfg.Renewal.ClientBillings)
	}
	billing := cfg.Renewal.ClientBillings[0]
	if billing.InboundID != 7 || billing.InboundTag != "entry-hk" || billing.Email != "alice@example.com" {
		t.Fatalf("unexpected billing target: %#v", billing)
	}
	if billing.TrafficMultiplier != 2 || billing.RevenueAmount != amount || billing.RevenueCurrency != "USDT" || billing.RevenueCycle != "semiannual" {
		t.Fatalf("unexpected billing revenue: %#v", billing)
	}
	if billing.ExpireCycle != "semiannual" {
		t.Fatalf("expected expire cycle to follow revenue cycle, got %#v", billing)
	}
}

func TestSyncCustomerAssignmentRevenueMatchesExistingBillingByEmail(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "us-01",
		AgentName: "US 01",
		SeedConfig: model.ManagedAgentConfig{
			Renewal: model.VPSRenewalConfig{
				ClientBillings: []model.XUIClientBillingConfig{
					{
						InboundID:         9,
						InboundTag:        "",
						Email:             "bob@example.com",
						TrafficMultiplier: 2,
						RevenueAmount:     30,
						RevenueCurrency:   "CNY",
						RevenueCycle:      "month",
						ExpireTime:        1760000000000,
						ExpireCycle:       "year",
						ExpireAutoRenew:   true,
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	app := &App{store: sqliteStore}
	amount := 45.5
	if err := app.syncCustomerAssignmentRevenue(model.CustomerAssignmentRequest{
		AgentID:         "us-01",
		InboundID:       9,
		InboundTag:      "us-west-entry",
		ClientEmail:     "bob@example.com",
		RevenueAmount:   &amount,
		RevenueCurrency: "cny",
		RevenueCycle:    "year",
	}, "admin"); err != nil {
		t.Fatalf("syncCustomerAssignmentRevenue: %v", err)
	}

	cfg, found, err := sqliteStore.GetAgentConfig("us-01")
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if !found {
		t.Fatalf("expected agent config")
	}
	if len(cfg.Renewal.ClientBillings) != 1 {
		t.Fatalf("expected existing billing to be updated in place, got %#v", cfg.Renewal.ClientBillings)
	}
	billing := cfg.Renewal.ClientBillings[0]
	if billing.InboundTag != "us-west-entry" {
		t.Fatalf("expected inbound tag to be refreshed, got %#v", billing)
	}
	if billing.RevenueAmount != amount || billing.RevenueCycle != "year" || billing.RevenueCurrency != "CNY" {
		t.Fatalf("unexpected refreshed revenue: %#v", billing)
	}
	if billing.ExpireTime != 1760000000000 || billing.ExpireCycle != "year" || !billing.ExpireAutoRenew {
		t.Fatalf("expected expiry fields to be preserved, got %#v", billing)
	}
	if billing.TrafficMultiplier != 2 {
		t.Fatalf("expected existing traffic multiplier to be preserved, got %#v", billing)
	}
}

func TestSyncCustomerAssignmentRevenueUpdatesOnlyTrafficMultiplier(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	if _, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{
		AgentID:   "vn-01",
		AgentName: "VN 01",
		SeedConfig: model.ManagedAgentConfig{
			Renewal: model.VPSRenewalConfig{
				ClientBillings: []model.XUIClientBillingConfig{{
					InboundID:       11,
					InboundTag:      "vn-entry",
					Email:           "alice@example.com",
					RevenueAmount:   90,
					RevenueCurrency: "USDT",
					RevenueCycle:    "quarter",
					StartTime:       1750000000000,
					ExpireTime:      1760000000000,
					ExpireCycle:     "quarter",
					ExpireAutoRenew: true,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	beforeCfg, found, err := sqliteStore.GetAgentConfig("vn-01")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig before update: found=%v err=%v", found, err)
	}
	before := beforeCfg.Renewal.ClientBillings[0]

	app := &App{store: sqliteStore}
	multiplier := 2.5
	if err := app.syncCustomerAssignmentRevenue(model.CustomerAssignmentRequest{
		AgentID:           "vn-01",
		InboundID:         11,
		InboundTag:        "vn-entry",
		ClientEmail:       "alice@example.com",
		TrafficMultiplier: &multiplier,
	}, "admin"); err != nil {
		t.Fatalf("syncCustomerAssignmentRevenue: %v", err)
	}

	cfg, found, err := sqliteStore.GetAgentConfig("vn-01")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig: found=%v err=%v", found, err)
	}
	billing := cfg.Renewal.ClientBillings[0]
	if billing.TrafficMultiplier != 2.5 {
		t.Fatalf("expected updated traffic multiplier, got %#v", billing)
	}
	if billing.RevenueAmount != before.RevenueAmount || billing.RevenueCurrency != before.RevenueCurrency || billing.RevenueCycle != before.RevenueCycle {
		t.Fatalf("expected revenue fields to stay unchanged, got %#v", billing)
	}
	if billing.StartTime != before.StartTime || billing.ExpireTime != before.ExpireTime || billing.ExpireCycle != before.ExpireCycle || billing.ExpireAutoRenew != before.ExpireAutoRenew {
		t.Fatalf("expected expiry fields to stay unchanged, got %#v", billing)
	}
}
