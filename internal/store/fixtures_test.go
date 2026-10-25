package store

import (
	"fmt"

	"bridge-core/internal/model"
)

// Fixture shortcuts are compiled only for tests. Runtime callers use the
// authenticated registration and actor/owner-aware store APIs.
func (s *SQLiteStore) RegisterAgent(req model.AgentRegisterRequest) (model.AgentRegisterResponse, error) {
	return s.RegisterAgentWithTokenOrLegacyBootstrap(req, "", true)
}

func (s *SQLiteStore) RegisterAgentWithToken(req model.AgentRegisterRequest, presentedAgentToken string) (model.AgentRegisterResponse, error) {
	return s.RegisterAgentWithTokenOrLegacyBootstrap(req, presentedAgentToken, false)
}

func (s *SQLiteStore) RegisterAgentWithLegacyBootstrap(req model.AgentRegisterRequest) (model.AgentRegisterResponse, error) {
	return s.RegisterAgentWithTokenOrLegacyBootstrap(req, "", true)
}

func (s *SQLiteStore) UpdateAgentConfig(agentID string, cfg model.ManagedAgentConfig) (model.AgentRecord, error) {
	return s.UpdateAgentConfigWithActor(agentID, cfg, "system")
}

func (s *SQLiteStore) CreateCustomer(req model.CustomerAccountRequest) (model.CustomerAdminView, error) {
	return s.CreateCustomerForOwner(req, model.AdminRoleRoot, adminAccountID)
}

func (s *SQLiteStore) CreateXUIAction(agentID string, req model.XUIActionRequest) (model.XUIAction, error) {
	return s.CreateXUIActionWithActor(agentID, req, model.XUIActionActor{})
}

func (s *SQLiteStore) CreateAreaManagerAssignment(managerID int64, req model.AreaManagerAssignmentRequest) (model.AreaManagerAssignment, error) {
	items, err := s.CreateAreaManagerAssignments(managerID, []model.AreaManagerAssignmentRequest{req})
	if err != nil {
		return model.AreaManagerAssignment{}, err
	}
	if len(items) == 0 {
		return model.AreaManagerAssignment{}, fmt.Errorf("created area manager assignment not found")
	}
	return items[0], nil
}
