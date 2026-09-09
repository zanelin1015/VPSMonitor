package store

import (
	"errors"
	"path/filepath"
	"testing"

	"bridge-core/internal/model"
)

func TestRegisterAgentWithTokenProtectsExistingIdentity(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial, err := s.RegisterAgentWithToken(model.AgentRegisterRequest{AgentID: "protected", Hostname: "original-host"}, "")
	if err != nil || initial.AgentToken == "" {
		t.Fatalf("initial registration: err=%v", err)
	}
	req := model.AgentRegisterRequest{AgentID: "protected", Hostname: "changed-host"}
	for _, token := range []string{"", "incorrect-token"} {
		response, err := s.RegisterAgentWithToken(req, token)
		if !errors.Is(err, ErrAgentRegistrationUnauthorized) || response.AgentToken != "" {
			t.Fatalf("unauthorized refresh must not disclose credentials: err=%v", err)
		}
		record, found, err := s.GetAgent(req.AgentID)
		if err != nil || !found || record.Hostname != "original-host" {
			t.Fatalf("unauthorized refresh changed the existing agent: found=%v err=%v", found, err)
		}
	}
	response, err := s.RegisterAgentWithToken(req, initial.AgentToken)
	if err != nil || response.AgentToken != initial.AgentToken {
		t.Fatalf("authenticated refresh: err=%v", err)
	}
	record, found, err := s.GetAgent(req.AgentID)
	if err != nil || !found || record.Hostname != "changed-host" {
		t.Fatalf("authenticated refresh did not update the agent: found=%v err=%v", found, err)
	}
}

func TestRegisterAgentWithLegacyBootstrapRefreshesExistingIdentity(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "legacy", Hostname: "original-host"})
	if err != nil || initial.AgentToken == "" {
		t.Fatalf("initial registration: err=%v", err)
	}
	response, err := s.RegisterAgentWithLegacyBootstrap(model.AgentRegisterRequest{AgentID: "legacy", Hostname: "legacy-host"})
	if err != nil || response.AgentToken != initial.AgentToken {
		t.Fatalf("legacy refresh: err=%v token_changed=%t", err, response.AgentToken != initial.AgentToken)
	}
	record, found, err := s.GetAgent("legacy")
	if err != nil || !found || record.Hostname != "legacy-host" {
		t.Fatalf("legacy refresh did not update the agent: found=%v err=%v", found, err)
	}
}
