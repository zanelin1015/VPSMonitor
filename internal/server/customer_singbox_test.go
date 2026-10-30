package server

import (
	"encoding/json"
	"testing"

	"bridge-core/internal/model"
)

func TestBuildSingBoxCustomerLinkAndSubscription(t *testing.T) {
	snapshots := []model.AgentSnapshot{{
		AgentID: "edge-1",
		Summary: model.VPSSummary{PublicIPv4: "198.51.100.10"},
		SingBox: &model.SingBoxSnapshot{Clients: []map[string]any{{
			"type":        "snell",
			"tag":         "snell-in",
			"listen":      "::",
			"listen_port": float64(27411),
			"version":     float64(5),
			"psk":         "secret",
			"obfs_mode":   "none",
			"users":       []any{map[string]any{"name": "alice", "userkey": "alice-key"}},
		}}},
	}}
	link := buildSingBoxCustomerLink(model.CustomerAssignment{ID: 7, AgentID: "edge-1", SourceType: "singbox", SingBoxInboundTag: "snell-in", SingBoxClientKey: "userkey:alice-key", PublicClientName: "Alice"}, snapshots, []model.AgentRecord{{AgentID: "edge-1", PublicIPv4: "198.51.100.10"}})
	if !link.Resolved || link.SingBoxConfig == nil {
		t.Fatalf("expected resolved sing-box link, got %#v", link)
	}
	content, err := buildSingBoxSubscription([]model.CustomerLinkView{link})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatal(err)
	}
	if len(config["outbounds"].([]any)) != 1 {
		t.Fatalf("expected one subscription outbound, got %#v", config)
	}
	outbound := config["outbounds"].([]any)[0].(map[string]any)
	if outbound["server"] != "198.51.100.10" || outbound["server_port"] != float64(27411) || outbound["userkey"] != "alice-key" {
		t.Fatalf("unexpected exported outbound: %#v", outbound)
	}
}

func TestBuildSingBoxCustomerLinkForNodeLevelSnell(t *testing.T) {
	snapshots := []model.AgentSnapshot{{
		AgentID: "edge-1",
		Summary: model.VPSSummary{PublicIPv4: "198.51.100.10"},
		SingBox: &model.SingBoxSnapshot{Clients: []map[string]any{{
			"type":        "snell",
			"tag":         "snell-single",
			"listen":      "::",
			"listen_port": float64(27411),
			"version":     float64(5),
			"psk":         "single-user-psk",
			"obfs_mode":   "none",
		}}},
	}}
	link := buildSingBoxCustomerLink(model.CustomerAssignment{
		ID:                8,
		AgentID:           "edge-1",
		SourceType:        "singbox",
		SingBoxInboundTag: "snell-single",
		PublicClientName:  "Clash Snell",
	}, snapshots, []model.AgentRecord{{AgentID: "edge-1", PublicIPv4: "198.51.100.10"}})
	if !link.Resolved || link.SingBoxConfig == nil {
		t.Fatalf("expected node-level Snell link to resolve, got %#v", link)
	}
	outbound := link.SingBoxConfig["outbounds"].([]any)[0].(map[string]any)
	if outbound["psk"] != "single-user-psk" || outbound["server"] != "198.51.100.10" {
		t.Fatalf("unexpected node-level Snell outbound: %#v", outbound)
	}
	if _, exists := outbound["userkey"]; exists {
		t.Fatalf("node-level Snell outbound must not require userkey: %#v", outbound)
	}
}
