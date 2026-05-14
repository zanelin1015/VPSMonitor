package server

import (
	"testing"
	"time"

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

func TestAreaManagerRealtimeMetricsAreSanitized(t *testing.T) {
	app := &App{}
	user := model.AdminUser{
		ID:       10,
		Role:     model.AdminRoleAreaManager,
		AgentIDs: []string{"allowed"},
	}
	metrics := []model.AgentRealtimeMetrics{
		{
			AgentID:       "allowed",
			AgentName:     "internal-name",
			ClientVersion: "0.2.11",
			ClientOS:      "linux",
			ClientArch:    "amd64",
			SystemVersion: "Debian 12",
			ReportedAt:    time.Now().UTC(),
			Summary: model.VPSSummary{
				Hostname:        "secret-host",
				ObservedIP:      "203.0.113.10",
				PublicIPv4:      "203.0.113.11",
				CPU:             42,
				MemUsed:         512,
				MemTotal:        1024,
				NetTrafficSent:  100,
				NetTrafficRecv:  200,
				NetTrafficTotal: 300,
				NetIOUp:         10,
				NetIODown:       20,
			},
		},
		{
			AgentID: "hidden",
			Summary: model.VPSSummary{
				NetTrafficSent: 999,
			},
		},
	}

	filtered := app.filterRealtimeMetricsForAdmin(user, metrics)
	if len(filtered) != 1 {
		t.Fatalf("expected only one authorized metric, got %d", len(filtered))
	}
	got := filtered[0]
	if got.AgentName != "" || got.ClientVersion != "" || got.ClientOS != "" || got.ClientArch != "" || got.SystemVersion != "" {
		t.Fatalf("expected client identity/runtime fields to be stripped, got %#v", got)
	}
	if got.Summary.Hostname != "" || got.Summary.ObservedIP != "" || got.Summary.PublicIPv4 != "" || got.Summary.CPU != 0 || got.Summary.MemTotal != 0 {
		t.Fatalf("expected host/system metrics to be stripped, got %#v", got.Summary)
	}
	if got.Summary.NetTrafficSent != 100 || got.Summary.NetTrafficRecv != 200 || got.Summary.NetTrafficTotal != 300 || got.Summary.NetIOUp != 10 || got.Summary.NetIODown != 20 {
		t.Fatalf("expected traffic metrics to remain, got %#v", got.Summary)
	}
}
