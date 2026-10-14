package store

import (
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestOutboundDeletionQueueAndResultVerification(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	payload := map[string]any{"config_fingerprint": hash, "outbounds": []model.OutboundDeleteTarget{{Tag: "old-a", Fingerprint: hash}, {Tag: "old-b", Fingerprint: hash}}}
	for _, tc := range []struct {
		name    string
		result  map[string]any
		success bool
	}{
		{"verified batch", map[string]any{"saved": true, "verified": true, "restarted": true}, true},
		{"false panel success", map[string]any{"saved": true, "restarted": true}, false},
		{"reload missing", map[string]any{"saved": true, "verified": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			action, err := s.CreateXUIAction("agent-1", model.XUIActionRequest{Kind: model.XUIActionDeleteOutbounds, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := model.ParseOutboundsDeletePayload(action.Payload)
			if err != nil || len(parsed.Outbounds) != 2 {
				t.Fatal("queued selection lost its guard")
			}
			completed, err := s.CompleteXUIAction("agent-1", action.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: tc.result})
			if err != nil || (completed.Status == model.XUIActionStatusSucceeded) != tc.success {
				t.Fatalf("result=%#v err=%v", completed, err)
			}
		})
	}
	if _, err := s.CreateXUIAction("agent-1", model.XUIActionRequest{Kind: model.XUIActionDeleteOutbounds}); err == nil {
		t.Fatal("unguarded deletion queued")
	}
}
