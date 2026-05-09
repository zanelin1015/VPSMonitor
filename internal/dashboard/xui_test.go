package dashboard

import (
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestBuildXUIOverview(t *testing.T) {
	reportedAt := time.Date(2026, 5, 3, 3, 0, 0, 0, time.UTC)
	lastOnline := reportedAt.Add(-2 * time.Minute).UnixMilli()

	snapshot := model.AgentSnapshot{
		AgentID:    "agent-1",
		AgentName:  "hk-vps",
		ReportedAt: reportedAt,
		Summary: model.VPSSummary{
			Hostname:      "vps-01",
			PublicIPv4:    "1.1.1.1",
			InboundCount:  1,
			OutboundCount: 2,
		},
		XUI: &model.XUISnapshot{
			CollectedAt: reportedAt,
			Inbounds: []map[string]any{
				{
					"id":       1,
					"tag":      "in-vless",
					"remark":   "HK-01",
					"protocol": "vless",
					"port":     443,
					"enable":   true,
					"settings": `{"clients":[{"email":"alice@example.com","enable":true,"comment":"Alice","subId":"sub-a"},{"email":"bob@example.com","enable":true}]}`,
					"clientStats": []map[string]any{
						{"email": "alice@example.com", "enable": true, "up": 100, "down": 200, "allTime": 300, "lastOnline": lastOnline},
						{"email": "bob@example.com", "enable": true, "up": 10, "down": 20, "allTime": 30, "lastOnline": 0},
					},
				},
			},
			Outbounds: []map[string]any{
				{"tag": "direct", "protocol": "freedom"},
				{"tag": "relay-hk", "protocol": "vless", "settings": map[string]any{
					"vnext": []map[string]any{
						{"address": "relay.example.com", "port": 8443},
					},
				}},
			},
			OutboundTraffic: []map[string]any{
				{"tag": "relay-hk", "up": 500, "down": 1000, "total": 1500},
			},
			RoutingRules: []map[string]any{
				{"type": "field", "user": []string{"alice@example.com"}, "outboundTag": "relay-hk"},
				{"type": "field", "inboundTag": []string{"in-vless"}, "outboundTag": "direct"},
				{"type": "field", "outboundTag": "blocked", "protocol": []string{"bittorrent"}},
			},
		},
	}

	overview := BuildXUIOverview(snapshot)
	if overview == nil {
		t.Fatalf("expected overview")
	}
	if overview.NodeCount != 1 {
		t.Fatalf("expected 1 node, got %d", overview.NodeCount)
	}
	if overview.ClientCount != 2 {
		t.Fatalf("expected 2 clients, got %d", overview.ClientCount)
	}
	if overview.OnlineClientCount != 1 {
		t.Fatalf("expected 1 online client, got %d", overview.OnlineClientCount)
	}
	if got := overview.Nodes[0].Route.OutboundTag; got != "direct" {
		t.Fatalf("expected inbound route direct, got %q", got)
	}
	if got := overview.Nodes[0].Route.RuleIndex; got != 2 {
		t.Fatalf("expected inbound route rule 2, got %d", got)
	}
	if got := overview.Outbounds[1].Target; got != "relay.example.com:8443" {
		t.Fatalf("unexpected outbound target %q", got)
	}

	clientByEmail := make(map[string]model.XUIClientView, len(overview.Clients))
	for _, client := range overview.Clients {
		clientByEmail[client.Email] = client
	}

	if got := clientByEmail["alice@example.com"].Route.OutboundTag; got != "relay-hk" {
		t.Fatalf("expected alice route relay-hk, got %q", got)
	}
	if got := clientByEmail["alice@example.com"].Route.MatchScope; got != "user" {
		t.Fatalf("expected alice scope user, got %q", got)
	}
	if got := clientByEmail["bob@example.com"].Route.OutboundTag; got != "direct" {
		t.Fatalf("expected bob route direct, got %q", got)
	}
	if got := len(clientByEmail["bob@example.com"].Route.GlobalRuleIndexes); got != 1 {
		t.Fatalf("expected bob to expose 1 global rule, got %d", got)
	}
}

func TestBuildXUIOverviewUsesDefaultOutbound(t *testing.T) {
	snapshot := model.AgentSnapshot{
		AgentID:    "agent-2",
		ReportedAt: time.Now().UTC(),
		XUI: &model.XUISnapshot{
			CollectedAt: time.Now().UTC(),
			Inbounds: []map[string]any{
				{
					"id":       1,
					"tag":      "in-trojan",
					"remark":   "JP-01",
					"protocol": "trojan",
					"settings": `{"clients":[{"email":"user@example.com","enable":true}]}`,
				},
			},
			Outbounds: []map[string]any{
				{"tag": "direct", "protocol": "freedom"},
			},
		},
	}

	overview := BuildXUIOverview(snapshot)
	if overview == nil {
		t.Fatalf("expected overview")
	}
	if got := overview.Clients[0].Route.MatchScope; got != "default" {
		t.Fatalf("expected default route scope, got %q", got)
	}
	if got := overview.Clients[0].Route.OutboundTag; got != "direct" {
		t.Fatalf("expected default outbound direct, got %q", got)
	}
}

func TestBuildXUIOverviewParsesFlatVLESSOutbound(t *testing.T) {
	now := time.Now().UTC()
	snapshot := model.AgentSnapshot{
		AgentID:    "agent-flat",
		ReportedAt: now,
		XUI: &model.XUISnapshot{
			CollectedAt: now,
			Outbounds: []map[string]any{
				{
					"tag":      "relay-flat",
					"protocol": "vless",
					"settings": map[string]any{
						"address": "relay.example.com",
						"port":    20108,
						"id":      "11111111-1111-1111-1111-111111111111",
					},
				},
			},
		},
	}

	overview := BuildXUIOverview(snapshot)
	if overview == nil {
		t.Fatalf("expected overview")
	}
	if got := overview.Outbounds[0].Target; got != "relay.example.com:20108" {
		t.Fatalf("unexpected outbound target %q", got)
	}
	if got := overview.Outbounds[0].Address; got != "relay.example.com" {
		t.Fatalf("unexpected outbound address %q", got)
	}
	if got := overview.Outbounds[0].Port; got != 20108 {
		t.Fatalf("unexpected outbound port %d", got)
	}
	if len(overview.Outbounds[0].AuthKeys) == 0 {
		t.Fatalf("expected outbound auth keys")
	}
}
