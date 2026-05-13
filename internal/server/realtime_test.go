package server

import (
	"testing"

	"bridge-core/internal/model"
)

func TestRealtimeHubAgentControlLifecycle(t *testing.T) {
	hub := newRealtimeHub()
	if hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should fail when agent has no realtime control session")
	}

	session := hub.registerAgentControl("agent-1")
	if !hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should succeed for an active realtime control session")
	}
	select {
	case message := <-session.ch:
		if message.Type != model.AgentControlCollectNow {
			t.Fatalf("unexpected control message: %q", message.Type)
		}
	default:
		t.Fatal("control message was not queued")
	}

	hub.unregisterAgentControl("agent-1", session)
	if hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should fail after unregistering realtime control session")
	}
}

func TestRealtimeHubAgentControlReplacesPreviousSession(t *testing.T) {
	hub := newRealtimeHub()
	first := hub.registerAgentControl("agent-1")
	second := hub.registerAgentControl("agent-1")

	select {
	case <-first.done:
	default:
		t.Fatal("previous session should be closed when a new session registers")
	}

	if !hub.sendAgentControl("agent-1", model.AgentControlMessage{Type: model.AgentControlCollectNow}) {
		t.Fatal("send should target the replacement session")
	}
	select {
	case <-first.ch:
		t.Fatal("first session should not receive messages after replacement")
	default:
	}
	select {
	case message := <-second.ch:
		if message.Type != model.AgentControlCollectNow {
			t.Fatalf("unexpected control message: %q", message.Type)
		}
	default:
		t.Fatal("replacement session did not receive control message")
	}
}
