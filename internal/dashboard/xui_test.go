package dashboard

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
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

func TestBuildXUIOverviewPrefersDirectAsDefaultOutbound(t *testing.T) {
	snapshot := model.AgentSnapshot{
		AgentID:    "agent-direct-default",
		ReportedAt: time.Now().UTC(),
		XUI: &model.XUISnapshot{
			CollectedAt: time.Now().UTC(),
			Inbounds: []map[string]any{
				{
					"id":       1,
					"tag":      "in-vless",
					"remark":   "HK-01",
					"protocol": "vless",
					"settings": `{"clients":[{"email":"user@example.com","enable":true}]}`,
				},
			},
			Outbounds: []map[string]any{
				{"tag": "blocked", "protocol": "blackhole"},
				{"tag": "direct", "protocol": "freedom"},
			},
			RoutingRules: []map[string]any{
				{"type": "field", "protocol": []string{"bittorrent"}, "outboundTag": "blocked"},
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
		t.Fatalf("expected unmatched client route to default direct, got %q", got)
	}
	directMarkedDefault := false
	for _, outbound := range overview.Outbounds {
		if outbound.Tag == "direct" && outbound.IsDefault {
			directMarkedDefault = true
			break
		}
	}
	if !directMarkedDefault {
		t.Fatalf("expected direct outbound to be marked as default: %#v", overview.Outbounds)
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

func TestBuildVMessImportURLMatchesXUIShape(t *testing.T) {
	link := buildVMessImportURL(inboundRecord{
		importHost: "hkvm.kynbbz.top",
		view: model.XUINodeView{
			Remark:   "临时",
			Port:     47770,
			Protocol: "vmess",
			Network:  "tcp",
			Security: "none",
		},
	}, clientConfig{
		email:    "vdxjk2ug",
		authUUID: "0b1f720d-fdff-4a55-8fcb-4e62cf555278",
		security: "auto",
	})
	if link == "" {
		t.Fatalf("expected vmess link")
	}
	const prefix = "vmess://"
	if len(link) <= len(prefix) || link[:len(prefix)] != prefix {
		t.Fatalf("unexpected vmess link %q", link)
	}
	body, err := base64.StdEncoding.DecodeString(link[len(prefix):])
	if err != nil {
		t.Fatalf("decode vmess payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal vmess payload: %v", err)
	}
	if got := payload["ps"]; got != "临时-vdxjk2ug" {
		t.Fatalf("unexpected ps %#v", got)
	}
	if got := payload["tls"]; got != "none" {
		t.Fatalf("unexpected tls %#v", got)
	}
	if got := payload["port"]; got != float64(47770) {
		t.Fatalf("unexpected port %#v", got)
	}
	if _, ok := payload["aid"]; ok {
		t.Fatalf("did not expect aid when alterId is zero")
	}
}

func TestBuildVLESSImportURLMatchesXUIShape(t *testing.T) {
	link := buildVLESSImportURL(inboundRecord{
		importHost:      "hkvm.kynbbz.top",
		vlessEncryption: "none",
		view:            testNode("vless"),
	}, clientConfig{
		email:    "alice",
		authUUID: "11111111-1111-1111-1111-111111111111",
	})
	parsed := mustParseURL(t, link)
	if parsed.Scheme != "vless" || parsed.User.Username() != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected vless authority %q", link)
	}
	if parsed.Host != "hkvm.kynbbz.top:47770" {
		t.Fatalf("unexpected host %q", parsed.Host)
	}
	query := parsed.Query()
	assertQuery(t, query, "type", "tcp")
	assertQuery(t, query, "encryption", "none")
	assertQuery(t, query, "security", "none")
	if got := parsed.Fragment; got != "临时-alice" {
		t.Fatalf("unexpected fragment %q", got)
	}
}

func TestBuildTrojanImportURLMatchesXUIShape(t *testing.T) {
	link := buildTrojanImportURL(inboundRecord{
		importHost: "hkvm.kynbbz.top",
		view:       testNode("trojan"),
	}, clientConfig{
		email:        "alice",
		authPassword: "trojan-pass",
	})
	parsed := mustParseURL(t, link)
	if parsed.Scheme != "trojan" || parsed.User.Username() != "trojan-pass" {
		t.Fatalf("unexpected trojan authority %q", link)
	}
	query := parsed.Query()
	assertQuery(t, query, "type", "tcp")
	assertQuery(t, query, "security", "none")
	if got := parsed.Fragment; got != "临时-alice" {
		t.Fatalf("unexpected fragment %q", got)
	}
}

func TestBuildShadowsocksImportURLMatchesXUIShape(t *testing.T) {
	link := buildShadowsocksImportURL(inboundRecord{
		importHost:        "hkvm.kynbbz.top",
		shadowsocksMethod: "aes-128-gcm",
		view:              testNode("shadowsocks"),
	}, clientConfig{
		email:        "alice",
		authPassword: "ss-pass",
	})
	parsed := mustParseURL(t, link)
	if parsed.Scheme != "ss" {
		t.Fatalf("unexpected shadowsocks scheme %q", link)
	}
	encoded := parsed.User.Username()
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode shadowsocks userinfo: %v", err)
	}
	if got := string(decoded); got != "aes-128-gcm:ss-pass" {
		t.Fatalf("unexpected shadowsocks auth %q", got)
	}
	assertQuery(t, parsed.Query(), "type", "tcp")
	if got := parsed.Fragment; got != "临时-alice" {
		t.Fatalf("unexpected fragment %q", got)
	}
}

func TestBuildHysteriaImportURLMatchesXUIShape(t *testing.T) {
	link := buildHysteriaImportURL(inboundRecord{
		importHost:      "hkvm.kynbbz.top",
		hysteriaVersion: 2,
		view:            testNode("hysteria2"),
	}, clientConfig{
		email: "alice",
		auth:  "hy-auth",
	})
	parsed := mustParseURL(t, link)
	if parsed.Scheme != "hysteria2" || parsed.User.Username() != "hy-auth" {
		t.Fatalf("unexpected hysteria authority %q", link)
	}
	assertQuery(t, parsed.Query(), "security", "tls")
	if got := parsed.Fragment; got != "临时-alice" {
		t.Fatalf("unexpected fragment %q", got)
	}
}

func testNode(protocol string) model.XUINodeView {
	return model.XUINodeView{
		Remark:   "临时",
		Port:     47770,
		Protocol: protocol,
		Network:  "tcp",
		Security: "none",
	}
}

func mustParseURL(t *testing.T, link string) *url.URL {
	t.Helper()
	if link == "" {
		t.Fatalf("expected non-empty link")
	}
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		t.Fatalf("expected host in %q", link)
	}
	return parsed
}

func assertQuery(t *testing.T, query url.Values, key string, want string) {
	t.Helper()
	if got := query.Get(key); got != want {
		t.Fatalf("unexpected query %s=%q, want %q", key, got, want)
	}
}
