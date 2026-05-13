package server

import (
	"path/filepath"
	"testing"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

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
	if err := app.syncCustomerAssignmentRevenue(model.CustomerAssignmentRequest{
		AgentID:         "hk-01",
		InboundID:       7,
		InboundTag:      "entry-hk",
		ClientEmail:     "alice@example.com",
		RevenueAmount:   &amount,
		RevenueCurrency: "usdt",
		RevenueCycle:    "quarter",
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
	if billing.RevenueAmount != amount || billing.RevenueCurrency != "USDT" || billing.RevenueCycle != "quarter" {
		t.Fatalf("unexpected billing revenue: %#v", billing)
	}
	if billing.ExpireCycle != "month" {
		t.Fatalf("expected default expire cycle to be preserved, got %#v", billing)
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
						InboundID:       9,
						InboundTag:      "",
						Email:           "bob@example.com",
						RevenueAmount:   30,
						RevenueCurrency: "CNY",
						RevenueCycle:    "month",
						ExpireTime:      1760000000000,
						ExpireCycle:     "year",
						ExpireAutoRenew: true,
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
}
