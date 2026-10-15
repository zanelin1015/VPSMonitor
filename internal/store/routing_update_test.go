package store

import (
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestRoutingUpdateQueueAndVerifiedResult(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"rule_index": 2, "rules_fingerprint": strings.Repeat("a", 64), "rule_fingerprint": strings.Repeat("b", 64), "rule": map[string]any{"user": []string{"alice"}, "outboundTag": "direct"}}
	for _, result := range []map[string]any{{"saved": true, "verified": true, "restarted": true}, {"saved": true, "verified": true}, {"saved": true, "restarted": true}, {"verified": true, "restarted": true}} {
		action, err := s.CreateXUIAction("agent-1", model.XUIActionRequest{Kind: model.XUIActionUpdateRoutingRule, Payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		if parsed, err := model.ParseRoutingRuleUpdatePayload(action.Payload); err != nil || parsed.RuleIndex != 2 {
			t.Fatal("update guards were not queued")
		}
		completed, err := s.CompleteXUIAction("agent-1", action.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: result})
		wantSuccess := result["saved"] == true && result["verified"] == true && result["restarted"] == true
		if err != nil || (completed.Status == model.XUIActionStatusSucceeded) != wantSuccess {
			t.Fatalf("result=%#v err=%v", completed, err)
		}
	}
	if _, err := s.CreateXUIAction("agent-1", model.XUIActionRequest{Kind: model.XUIActionUpdateRoutingRule}); err == nil {
		t.Fatal("unguarded update was queued")
	}
}
