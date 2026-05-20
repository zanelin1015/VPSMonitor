package server

import (
	"net/url"
	"testing"

	"bridge-core/internal/model"
)

func TestCustomerExitInfoPrefersMatchedAgentObservedGeo(t *testing.T) {
	chain := model.ClientChainView{
		RootAgentID: "entry",
		Steps: []model.ClientChainStep{
			{
				StepType: "outbound",
				AgentID:  "entry",
				TargetIP: "142.91.98.131",
				TargetGeo: &model.IPGeoView{
					IP:          "142.91.98.131",
					CountryCode: "SG",
					CountryName: "Singapore",
				},
			},
			{
				StepType: "match",
				AgentID:  "nat",
			},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"nat": {
			AgentID: "nat",
			Geo: &model.IPGeoView{
				IP:          "24.248.245.100",
				CountryCode: "US",
				CountryName: "United States",
			},
		},
	}

	ip, countryCode, countryName := customerExitInfo(chain, agentMap)
	if ip != "24.248.245.100" || countryCode != "US" || countryName != "United States" {
		t.Fatalf("expected matched NAT geo, got ip=%q countryCode=%q countryName=%q", ip, countryCode, countryName)
	}
}

func TestCustomerExitInfoFallsBackToOutboundTargetGeo(t *testing.T) {
	chain := model.ClientChainView{
		RootAgentID: "entry",
		Steps: []model.ClientChainStep{
			{
				StepType: "outbound",
				AgentID:  "entry",
				TargetIP: "142.91.98.131",
				TargetGeo: &model.IPGeoView{
					IP:          "142.91.98.131",
					CountryCode: "SG",
					CountryName: "Singapore",
				},
			},
		},
	}

	ip, countryCode, countryName := customerExitInfo(chain, nil)
	if ip != "142.91.98.131" || countryCode != "SG" || countryName != "Singapore" {
		t.Fatalf("expected outbound target geo, got ip=%q countryCode=%q countryName=%q", ip, countryCode, countryName)
	}
}

func TestBuildCustomerLinkViewUsesCustomerDisplayNames(t *testing.T) {
	assignment := model.CustomerAssignment{
		ID:               7,
		AgentID:          "entry",
		InboundID:        1001,
		InboundTag:       "vless-in",
		ClientEmail:      "alice@example.com",
		PublicClientName: "Internal Entry - alice@example.com",
	}
	chainMap := map[string]model.ClientChainView{
		customerAssignmentKey("entry", 1001, "alice@example.com"): {
			RootAgentID: "entry",
			Steps: []model.ClientChainStep{
				{StepType: "match", AgentID: "relay", AgentName: "Internal Relay"},
			},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"entry": {AgentID: "entry", AgentName: "Internal Entry", CustomerDisplayName: "Customer Entry"},
		"relay": {AgentID: "relay", AgentName: "Internal Relay", CustomerDisplayName: "Customer Relay"},
	}

	link := buildCustomerLinkView(assignment, chainMap, nil, agentMap)
	if link.EntryClientName != "Customer Entry" {
		t.Fatalf("expected customer entry name, got %q", link.EntryClientName)
	}
	if len(link.Steps) < 2 || link.Steps[0].Label != "Customer Entry" || link.Steps[1].Label != "Customer Relay" {
		t.Fatalf("expected customer-only step labels, got %#v", link.Steps)
	}
}

func TestBuildCustomerLinkViewIncludesBillingAndExpiry(t *testing.T) {
	assignment := model.CustomerAssignment{
		ID:          7,
		AgentID:     "entry",
		InboundID:   1001,
		InboundTag:  "vless-in",
		ClientEmail: "alice@example.com",
	}
	chainMap := map[string]model.ClientChainView{
		customerAssignmentKey("entry", 1001, "alice@example.com"): {
			RootAgentID:      "entry",
			RootClientEmail:  "alice@example.com",
			RootInboundTag:   "vless-in",
			MatchedLinkCount: 1,
			Steps:            []model.ClientChainStep{{StepType: "client", AgentID: "entry", Label: "alice@example.com"}},
		},
	}
	clientMap := map[string]customerClientRef{
		customerAssignmentKey("entry", 1001, "alice@example.com"): {
			Client: model.XUIClientView{InboundID: 1001, InboundTag: "vless-in", Email: "alice@example.com", ExpiryTime: 1760000000000},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"entry": {
			AgentID: "entry",
			Renewal: model.VPSRenewalConfig{
				ClientBillings: []model.XUIClientBillingConfig{
					{
						InboundID:       1001,
						InboundTag:      "vless-in",
						Email:           "alice@example.com",
						RevenueAmount:   88.5,
						RevenueCurrency: "USDT",
						RevenueCycle:    "quarter",
						ExpireTime:      1770000000000,
						ExpireCycle:     "year",
						ExpireAutoRenew: true,
					},
				},
			},
		},
	}

	link := buildCustomerLinkView(assignment, chainMap, clientMap, agentMap)
	if link.RevenueAmount == nil || *link.RevenueAmount != 88.5 || link.RevenueCurrency != "USDT" || link.RevenueCycle != "quarter" {
		t.Fatalf("expected billing revenue on customer link, got %#v", link)
	}
	if link.ExpireTime != 1770000000000 || link.ExpireCycle != "year" || !link.ExpireAutoRenew {
		t.Fatalf("expected billing expiry to override client expiry, got %#v", link)
	}
}

func TestBuildCustomerLinkViewRewritesImportURLToRealmEntry(t *testing.T) {
	assignment := model.CustomerAssignment{
		ID:          7,
		AgentID:     "hk",
		InboundID:   1001,
		InboundTag:  "hk-vless",
		ClientEmail: "alice@example.com",
	}
	chainMap := map[string]model.ClientChainView{
		customerAssignmentKey("hk", 1001, "alice@example.com"): {
			RootAgentID:     "hk",
			RootClientEmail: "alice@example.com",
			RootInboundTag:  "hk-vless",
			Steps: []model.ClientChainStep{
				{StepType: "client", AgentID: "hk", Label: "alice@example.com"},
				{StepType: "inbound", AgentID: "hk", Label: "HK VLESS", Protocol: "vless", Port: 443},
				{StepType: "outbound", AgentID: "hk", Label: "to-third", Port: 8443},
			},
		},
	}
	clientMap := map[string]customerClientRef{
		customerAssignmentKey("hk", 1001, "alice@example.com"): {
			Client: model.XUIClientView{
				InboundID:  1001,
				InboundTag: "hk-vless",
				Email:      "alice@example.com",
				ImportURL:  "vless://11111111-1111-1111-1111-111111111111@hk.example.com:443?encryption=none&security=tls&type=tcp&sni=hk.example.com#HK",
			},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"gz": {
			AgentID: "gz",
			Entry: model.AgentEntryConfig{
				Addresses: []string{"gz.example.com"},
				PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{{
					Enabled:       true,
					ListenAddress: "0.0.0.0",
					ListenPort:    2443,
					TargetAgentID: "hk",
					TargetPort:    443,
					Network:       "tcp",
				}}},
			},
		},
		"hk": {
			AgentID: "hk",
			Entry:   model.AgentEntryConfig{Addresses: []string{"hk.example.com"}},
		},
	}

	link := buildCustomerLinkView(assignment, chainMap, clientMap, agentMap)
	parsed, err := url.Parse(link.ImportURL)
	if err != nil {
		t.Fatalf("parse import url: %v", err)
	}
	if parsed.Host != "gz.example.com:2443" {
		t.Fatalf("expected Guangzhou entry host, got %q from %q", parsed.Host, link.ImportURL)
	}
	if parsed.Query().Get("sni") != "hk.example.com" {
		t.Fatalf("expected HK-produced stream parameters to stay unchanged, got %q", link.ImportURL)
	}
}
