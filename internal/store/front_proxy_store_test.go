package store

import (
	"path/filepath"
	"testing"

	"bridge-core/internal/model"
)

func TestSQLiteStoreFrontProxyNodesGrantsAndAssignmentSelections(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()

	enabled := true
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "gz-01", AgentName: "GZ 01"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	manager, err := s.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area-front",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"gz-01"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	customer, err := s.CreateCustomerForOwner(model.CustomerAccountRequest{
		Username: "cust-front",
		Password: "password123",
		Enabled:  &enabled,
	}, model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}
	assignment, err := s.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{
		AgentID:          "gz-01",
		InboundID:        1,
		ClientEmail:      "cust@example.com",
		PublicClientName: "CS1",
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreateCustomerAssignment: %v", err)
	}

	hk, err := s.CreateFrontProxyNode(model.FrontProxyNodeRequest{
		Name:     "HK IEPL",
		ShareURL: "ss://YWVzLTI1Ni1nY206cGFzcw@example.com:8388#HK",
		Enabled:  &enabled,
	})
	if err != nil {
		t.Fatalf("CreateFrontProxyNode hk: %v", err)
	}
	jp, err := s.CreateFrontProxyNode(model.FrontProxyNodeRequest{
		Name:     "JP IEPL",
		ShareURL: "ss://YWVzLTI1Ni1nY206cGFzczI@example.net:12014#JP",
		Enabled:  &enabled,
	})
	if err != nil {
		t.Fatalf("CreateFrontProxyNode jp: %v", err)
	}

	if err := s.ReplaceFrontProxyGrants(model.FrontProxyGranteeAreaManager, manager.ID, []int64{hk.ID}); err != nil {
		t.Fatalf("ReplaceFrontProxyGrants: %v", err)
	}
	allowed, err := s.ListFrontProxyNodesForGrantee(model.FrontProxyGranteeAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("ListFrontProxyNodesForGrantee: %v", err)
	}
	if len(allowed) != 1 || allowed[0].ID != hk.ID {
		t.Fatalf("expected only HK proxy granted to manager, got %#v", allowed)
	}

	if err := s.ReplaceCustomerAssignmentFrontProxyNodes(assignment.ID, []int64{hk.ID}, model.FrontProxyGranteeAreaManager, manager.ID); err != nil {
		t.Fatalf("ReplaceCustomerAssignmentFrontProxyNodes allowed: %v", err)
	}
	if err := s.ReplaceCustomerAssignmentFrontProxyNodes(assignment.ID, []int64{jp.ID}, model.FrontProxyGranteeAreaManager, manager.ID); err == nil {
		t.Fatal("expected ungranted front proxy to be rejected for area manager")
	}

	byAssignment, err := s.ListFrontProxyNodesForAssignments([]int64{assignment.ID})
	if err != nil {
		t.Fatalf("ListFrontProxyNodesForAssignments: %v", err)
	}
	items := byAssignment[assignment.ID]
	if len(items) != 1 || items[0].Name != "HK IEPL" || items[0].ShareURL == "" {
		t.Fatalf("expected selected HK front proxy with share url, got %#v", items)
	}
}
