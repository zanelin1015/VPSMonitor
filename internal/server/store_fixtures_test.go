package server

import (
	"fmt"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

// Fixtures deliberately allow legacy re-registration; production handlers
// enforce the client's version and per-agent token before calling the store.
func testRegisterAgent(s *store.SQLiteStore, req model.AgentRegisterRequest) (model.AgentRegisterResponse, error) {
	return s.RegisterAgentWithTokenOrLegacyBootstrap(req, "", true)
}
func testUpdateAgentConfig(s *store.SQLiteStore, agentID string, cfg model.ManagedAgentConfig) (model.AgentRecord, error) {
	return s.UpdateAgentConfigWithActor(agentID, cfg, "system")
}
func testCreateCustomer(s *store.SQLiteStore, req model.CustomerAccountRequest) (model.CustomerAdminView, error) {
	return s.CreateCustomerForOwner(req, model.AdminRoleRoot, 1)
}
func testCreateXUIAction(s *store.SQLiteStore, agentID string, req model.XUIActionRequest) (model.XUIAction, error) {
	return s.CreateXUIActionWithActor(agentID, req, model.XUIActionActor{})
}
func testCreateAreaManagerAssignment(s *store.SQLiteStore, managerID int64, req model.AreaManagerAssignmentRequest) (model.AreaManagerAssignment, error) {
	items, err := s.CreateAreaManagerAssignments(managerID, []model.AreaManagerAssignmentRequest{req})
	if err != nil {
		return model.AreaManagerAssignment{}, err
	}
	if len(items) == 0 {
		return model.AreaManagerAssignment{}, fmt.Errorf("created area manager assignment not found")
	}
	return items[0], nil
}
