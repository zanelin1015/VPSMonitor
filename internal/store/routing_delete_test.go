package store

import (
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestRoutingDeletionActionQueuesWithGuards(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	fingerprint := strings.Repeat("a", 64)
	request := model.XUIActionRequest{Kind: model.XUIActionDeleteRoutingRules, Payload: map[string]any{
		"rules_fingerprint": fingerprint,
		"rules":             []model.RoutingRuleDeleteTarget{{Index: 2, Fingerprint: fingerprint}, {Index: 3, Fingerprint: fingerprint}},
	}}
	queued, err := s.CreateXUIAction("agent-1", request)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Kind != model.XUIActionDeleteRoutingRules || queued.Status != model.XUIActionStatusPending {
		t.Fatalf("bad action: %#v", queued)
	}
	claimed, err := s.ClaimPendingXUIActions("agent-1", 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %#v, %v", claimed, err)
	}
	if payload, err := model.ParseRoutingRulesDeletePayload(claimed[0].Payload); err != nil || len(payload.Rules) != 2 || payload.Rules[1].Index != 3 {
		t.Fatalf("queued payload lost guards: %#v, %v", payload, err)
	}
	request.Payload = map[string]any{}
	if _, err := s.CreateXUIAction("agent-1", request); err == nil {
		t.Fatal("unguarded delete should not be queued")
	}
	all, err := s.ListXUIActions("agent-1", 10)
	if err != nil || len(all) != 1 {
		t.Fatalf("invalid request created an action: %#v, %v", all, err)
	}
}
