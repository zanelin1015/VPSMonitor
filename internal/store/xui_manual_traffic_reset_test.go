package store

import (
	"path/filepath"
	"sync"
	"testing"

	"bridge-core/internal/model"
)

func TestManualTrafficResetQueueAndVerification(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "reset.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.RegisterAgent(model.AgentRegisterRequest{AgentID: "manual-reset"}); err != nil {
		t.Fatal(err)
	}
	request := func(email string) model.XUIActionRequest {
		return model.XUIActionRequest{Kind: model.XUIActionManualResetClientTraffic, Payload: map[string]any{"inbound_id": 7, "email": email, "client_id": "id-" + email}}
	}
	var wg sync.WaitGroup
	results := make(chan model.XUIAction, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			action, err := s.CreateXUIAction("manual-reset", request("alice"))
			if err == nil {
				results <- action
			}
		}()
	}
	wg.Wait()
	close(results)
	if len(results) != 1 {
		t.Fatalf("duplicate reset queued: %d successes", len(results))
	}
	action := <-results
	if _, _, err = s.ClaimXUIAction("manual-reset", action.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateXUIAction("manual-reset", request(" alice ")); err == nil {
		t.Fatal("running reset allowed a whitespace duplicate")
	}
	if _, err = s.CreateXUIAction("manual-reset", request("bob")); err != nil {
		t.Fatalf("unrelated client blocked: %v", err)
	}
	completed, err := s.CompleteXUIAction("manual-reset", action.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: map[string]any{"reset_done": true}})
	if err != nil || completed.Status != model.XUIActionStatusFailed {
		t.Fatalf("unverified success accepted: %v %v", completed, err)
	}
	again, err := s.CreateXUIAction("manual-reset", request("alice"))
	if err != nil {
		t.Fatal(err)
	}
	completed, err = s.CompleteXUIAction("manual-reset", again.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: map[string]any{"reset_done": true, "verified": true}})
	if err != nil || completed.Status != model.XUIActionStatusSucceeded {
		t.Fatalf("verified reset rejected: %v %v", completed, err)
	}
	if _, err = s.CreateXUIAction("manual-reset", model.XUIActionRequest{Kind: model.XUIActionResetClientTraffic, Payload: request("alice").Payload}); err == nil {
		t.Fatal("legacy scheduled action was revived")
	}
}
