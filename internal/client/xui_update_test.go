package client

import "testing"

func TestUpdate3XUIRejectsUnverifiedUpdater(t *testing.T) {
	result, err := update3XUI(t.Context(), map[string]any{"target_version": "2.0.0"})
	if err == nil {
		t.Fatal("expected unverified updater to be rejected")
	}
	if result["status"] != "rejected" {
		t.Fatalf("expected rejected status, got %#v", result)
	}
}
