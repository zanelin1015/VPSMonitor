package server

import (
	"strings"
	"testing"

	"bridge-core/internal/model"
)

const realityCompatibilityURL = "vless://11111111-1111-1111-1111-111111111111@entry.example.com:20001?encryption=none&security=reality&type=tcp&fp=ios&pbk=original-public-key&sid=abcd&sni=target.example.com&flow=xtls-rprx-vision"

func TestMihomoSubscriptionRealityHandshakeByServingCoreVersion(t *testing.T) {
	for _, tc := range []struct {
		version string
		mlkem   bool
	}{
		{"", false}, {"unknown", false}, {"26.9", false},
		{"3.10.0", false}, {"24.12.31", false}, {"26.6.27", false},
		{"26.7.11", false}, {"26.9.7", false}, {"26.9.8", true},
		{"26.10.10", true}, {"v26.10.10", true}, {"26.10.10-custom", true},
		{"Xray 26.10.10 (Xray, Penetrates Everything.)", true}, {"27.1.1", true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			content := buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{{
				Resolved: true, EntryClientName: "Cranes", ImportURL: realityCompatibilityURL, XrayVersion: tc.version,
			}})
			if got := strings.Contains(content, "support-x25519mlkem768: true"); got != tc.mlkem {
				t.Fatalf("core %q: ML-KEM=%v, want %v", tc.version, got, tc.mlkem)
			}
			fingerprint := "ios"
			if tc.mlkem {
				fingerprint = "chrome"
			}
			assertContains(t, content, `client-fingerprint: "`+fingerprint+`"`)
			if strings.Count(content, "client-fingerprint:") != 1 {
				t.Fatal("each node must have a single effective fingerprint")
			}
			for _, preserved := range []string{
				`server: "entry.example.com"`, "port: 20001", `uuid: "11111111-1111-1111-1111-111111111111"`,
				`public-key: "original-public-key"`, `short-id: "abcd"`, `servername: "target.example.com"`,
				`flow: "xtls-rprx-vision"`, "tls: true",
			} {
				assertContains(t, content, preserved)
			}
		})
	}
}

func TestMihomoRealityCompatibilityDoesNotAlterTLSOrUnknownFrontProxy(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{
		{Resolved: true, EntryClientName: "TLS", ImportURL: strings.Replace(realityCompatibilityURL, "security=reality", "security=tls", 1), XrayVersion: "26.10.10"},
		{Resolved: true, EntryClientName: "REALITY", ImportURL: realityCompatibilityURL, XrayVersion: "26.10.10", FrontProxies: []model.CustomerLinkFrontProxy{{
			ID: 1, Name: "External REALITY", ShareURL: realityCompatibilityURL,
		}}},
	})
	if got := strings.Count(content, "support-x25519mlkem768: true"); got != 1 {
		t.Fatalf("only the managed new REALITY server should enable ML-KEM, got %d", got)
	}
	if got := strings.Count(content, `client-fingerprint: "ios"`); got != 2 {
		t.Fatalf("TLS and unknown third-party REALITY fingerprints must be preserved, got %d", got)
	}
	assertContains(t, content, `dialer-proxy: "REALITY 前置代理"`)

	content = buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{{
		Resolved: true, EntryClientName: "No fingerprint", ImportURL: strings.Replace(realityCompatibilityURL, "&fp=ios", "", 1), XrayVersion: "26.10.10",
	}})
	assertContains(t, content, `client-fingerprint: "chrome"`)
	assertContains(t, content, "support-x25519mlkem768: true")
}

func TestCustomerClientMapUsesXrayCoreVersionForSubscription(t *testing.T) {
	const clientID = "11111111-1111-1111-1111-111111111111"
	snapshot := model.AgentSnapshot{
		AgentID: "backend", Version: "0.3.46", Summary: model.VPSSummary{PublicIPv4: "203.0.113.1"},
		XUI: &model.XUISnapshot{
			AppVersion: "3.10.0", ServerStatus: model.XUIServerStatus{Xray: model.XUIXrayStatus{Version: "26.10.10"}},
			Inbounds: []map[string]any{{
				"id": 1, "tag": "reality-in", "protocol": "vless", "port": 20001, "enable": true,
				"settings":       `{"decryption":"none","clients":[{"id":"` + clientID + `","email":"Cranes","enable":true}]}`,
				"streamSettings": `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["target.example.com"],"publicKey":"original-public-key","shortIds":["abcd"]}}`,
			}},
		},
	}
	clients := buildCustomerClientMap([]model.AgentSnapshot{snapshot, {AgentID: "no-xui"}}, nil)
	for _, key := range []string{customerAssignmentKey("backend", 1, "Cranes"), customerAssignmentClientKey("backend", 1, clientID)} {
		ref, found := clients[key]
		if !found || ref.XrayVersion != "26.10.10" {
			t.Fatalf("expected serving core version for client key %q, got %#v", key, ref)
		}
		if !strings.Contains(ref.Client.ShadowrocketImportURL, "fp=chrome140") {
			t.Fatal("admin single-node export must include the Shadowrocket companion URL")
		}
	}
	assignment := model.CustomerAssignment{AgentID: "backend", InboundID: 1, ClientID: clientID, ClientEmail: "old-email"}
	chain := model.ClientChainView{RootAgentID: "backend", RootInboundID: 1, RootClientEmail: "Cranes"}
	link := buildCustomerLinkView(assignment, map[string]model.ClientChainView{customerAssignmentKey("backend", 1, "old-email"): chain}, clients, nil)
	if link.XrayVersion != "26.10.10" {
		t.Fatalf("client-ID lookup must keep the serving Xray version, got %q", link.XrayVersion)
	}
	if !strings.Contains(link.ShadowrocketImportURL, "fp=chrome140") {
		t.Fatal("customer single-node export must include the Shadowrocket companion URL")
	}
}
