package store

import (
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestXUIActionClaimAndCompletionAreIdempotent(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent"}); err != nil {
		t.Fatal(err)
	}
	action, err := s.CreateXUIAction("agent", model.XUIActionRequest{Kind: model.XUIActionRestartXUI})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := s.ClaimXUIAction("agent", action.ID)
	if err != nil || !ok || claimed.Status != model.XUIActionStatusRunning {
		t.Fatalf("first claim: action=%+v claimed=%v err=%v", claimed, ok, err)
	}
	if _, ok, err := s.ClaimXUIAction("agent", action.ID); err != nil || ok {
		t.Fatalf("second claim must be ignored: claimed=%v err=%v", ok, err)
	}
	if _, err := s.CompleteXUIAction("agent", action.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteXUIAction("agent", action.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded}); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("duplicate completion should be rejected, got %v", err)
	}
}
