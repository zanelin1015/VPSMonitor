package server

import (
	"net/url"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestActiveCustomerAnnouncementsFiltersDisabledAndScheduledItems(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	items := []model.CustomerAnnouncement{
		{ID: "active", Enabled: true, Title: "Active"},
		{ID: "disabled", Enabled: false, Title: "Disabled"},
		{ID: "future", Enabled: true, Title: "Future", StartsAt: now.Add(time.Hour).Format(time.RFC3339)},
		{ID: "expired", Enabled: true, Title: "Expired", EndsAt: now.Format(time.RFC3339)},
		{ID: "window", Enabled: true, Title: "Window", StartsAt: now.Add(-time.Hour).Format(time.RFC3339), EndsAt: now.Add(time.Hour).Format(time.RFC3339)},
	}

	active := activeCustomerAnnouncements(items, now)
	if len(active) != 2 || active[0].ID != "active" || active[1].ID != "window" {
		t.Fatalf("unexpected active announcements: %#v", active)
	}
}

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

func TestBuildCustomerLinkViewShowsUnmatchedOutboundRoute(t *testing.T) {
	assignment := model.CustomerAssignment{
		ID:               7,
		AgentID:          "entry",
		InboundID:        20005,
		InboundTag:       "inbound-20005",
		ClientEmail:      "alice@example.com",
		PublicClientName: "US-200M - AnilamVM",
	}
	chainMap := map[string]model.ClientChainView{
		customerAssignmentKey("entry", 20005, "alice@example.com"): {
			RootAgentID:      "entry",
			RootClientEmail:  "alice@example.com",
			RootInboundTag:   "inbound-20005",
			UnresolvedReason: "the outbound target did not match any registered client inbound",
			Steps: []model.ClientChainStep{
				{StepType: "client", AgentID: "entry", Label: "alice@example.com"},
				{StepType: "inbound", AgentID: "entry", Label: "AnilamVM", Port: 20005},
				{StepType: "outbound", AgentID: "entry", Label: "COX-Anilam", OutboundTag: "COX-Anilam", Target: "cox.example.com:443"},
			},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"entry": {AgentID: "entry", AgentName: "US-VMISS-1"},
	}

	link := buildCustomerLinkView(assignment, chainMap, nil, agentMap)
	if len(link.Steps) < 2 || link.Steps[1].Role != "relay" || link.Steps[1].Label != "COX-Anilam" {
		t.Fatalf("expected unmatched outbound to appear as relay step, got %#v", link.Steps)
	}
	if link.Summary != "US-200M - AnilamVM 转发 COX-Anilam 出口 未知" {
		t.Fatalf("expected summary to include outbound route, got %q", link.Summary)
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
			Client: model.XUIClientView{
				InboundID: 1001, InboundTag: "vless-in", Email: "alice@example.com", ExpiryTime: 1760000000000,
				TotalGB: 100 * 1024 * 1024 * 1024, Up: 4 * 1024 * 1024 * 1024, Down: 6 * 1024 * 1024 * 1024,
			},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"entry": {
			AgentID: "entry",
			Renewal: model.VPSRenewalConfig{
				Enabled:    true,
				ExpireDate: "2099-01-02",
				ClientBillings: []model.XUIClientBillingConfig{
					{
						InboundID:         1001,
						InboundTag:        "vless-in",
						Email:             "alice@example.com",
						TrafficMultiplier: 2,
						RevenueAmount:     88.5,
						RevenueCurrency:   "USDT",
						RevenueCycle:      "quarter",
						ExpireTime:        1770000000000,
						ExpireCycle:       "year",
						ExpireAutoRenew:   true,
					},
				},
			},
		},
	}

	link := buildCustomerLinkView(assignment, chainMap, clientMap, agentMap)
	expectedNodeExpiry, ok := parseDate("2099-01-02")
	if !ok || link.NodeExpireTime != expectedNodeExpiry.UnixMilli() {
		t.Fatalf("expected node expiry from agent renewal, got %#v", link)
	}
	if link.RevenueAmount == nil || *link.RevenueAmount != 88.5 || link.RevenueCurrency != "USDT" || link.RevenueCycle != "quarter" {
		t.Fatalf("expected billing revenue on customer link, got %#v", link)
	}
	if link.ExpireTime != 1770000000000 || link.ExpireCycle != "year" || !link.ExpireAutoRenew {
		t.Fatalf("expected billing expiry to override client expiry, got %#v", link)
	}
	if link.TrafficMultiplier != 2 || link.TrafficUsedBytes != 20*1024*1024*1024 || link.TrafficLimitBytes != 200*1024*1024*1024 {
		t.Fatalf("expected customer traffic to use configured multiplier, got %#v", link)
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
			Summary: model.VPSSummary{
				PublicIPv4: "1.1.1.1",
				ObservedIP: "1.1.1.1",
			},
			Entry: model.AgentEntryConfig{
				ImportDomain: "gz.example.com",
				Addresses:    []string{"1.1.1.1"},
				PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
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

func TestBuildCustomerLinkViewResolvesFirstRealmHopToFinalClient(t *testing.T) {
	finalInbound := model.TopologyInboundRef{
		AgentID:    "dmit",
		InboundID:  1,
		InboundTag: "DMIT",
		Protocol:   "vless",
		Port:       20001,
	}
	hkRealm := model.TopologyInboundRef{
		AgentID:    "hk",
		InboundID:  20001,
		InboundTag: "realm:hk:20001",
		Protocol:   "realm",
		Port:       20001,
	}
	chains := []model.ClientChainView{{
		Key:               customerAssignmentKey("dmit", 1, "xq"),
		RootAgentID:       "dmit",
		RootInboundID:     1,
		RootInboundTag:    "DMIT",
		RootClientEmail:   "xq",
		RootClientRemark:  "DMIT customer",
		MatchedLinkCount:  1,
		RootClientEnabled: true,
		Steps: []model.ClientChainStep{
			{StepType: "client", AgentID: "dmit", Label: "xq"},
			{StepType: "inbound", AgentID: "dmit", Label: "DMIT", Protocol: "vless", Port: 20001},
		},
	}}
	links := []model.TopologyLinkView{
		{
			Key:         "gz::realm:20001",
			Source:      model.TopologyOutboundRef{AgentID: "gz", Protocol: "realm", ListenPort: 20001},
			Target:      hkRealm,
			FinalTarget: &finalInbound,
			RealmHops:   []model.TopologyInboundRef{hkRealm},
		},
		{
			Key:         "hk::realm:20001",
			Source:      model.TopologyOutboundRef{AgentID: "hk", Protocol: "realm", ListenPort: 20001},
			Target:      finalInbound,
			FinalTarget: &finalInbound,
		},
	}
	chainMap := buildCustomerChainMap(chains, links)
	clientMap := map[string]customerClientRef{
		customerAssignmentKey("dmit", 1, "xq"): {
			Client: model.XUIClientView{
				InboundID:  1,
				InboundTag: "DMIT",
				Email:      "xq",
				Comment:    "final client",
				ImportURL:  "vless://6dd06a03-4987-4ffb-9cbf-6f04daa05d82@dmit.example.com:20001?encryption=none&fp=chrome&pbk=public-key&security=reality&sid=bc&sni=shop.example.com&type=tcp#DMIT",
				TotalGB:    100 * 1024 * 1024 * 1024,
				Up:         2 * 1024 * 1024 * 1024,
				Down:       3 * 1024 * 1024 * 1024,
			},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"gz": {
			AgentID:             "gz",
			CustomerDisplayName: "Guangzhou Entry",
			Entry: model.AgentEntryConfig{
				ImportDomain: "gz.example.com",
				PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
					Enabled:       true,
					ListenPort:    20001,
					TargetAgentID: "hk",
					TargetPort:    20001,
				}}},
			},
		},
		"hk": {
			AgentID: "hk",
			Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
				Enabled:       true,
				ListenPort:    20001,
				TargetAgentID: "dmit",
				TargetPort:    20001,
			}}}},
		},
		"dmit": {AgentID: "dmit"},
	}
	assignment := model.CustomerAssignment{
		AgentID:     "gz",
		InboundID:   20001,
		InboundTag:  "DMIT",
		ClientEmail: "xq",
	}

	link := buildCustomerLinkView(assignment, chainMap, clientMap, agentMap)
	if !link.Resolved || link.UnresolvedReason != "" {
		t.Fatalf("expected first Realm assignment to resolve, got %#v", link)
	}
	parsed, err := url.Parse(link.ImportURL)
	if err != nil {
		t.Fatalf("parse rewritten import URL: %v", err)
	}
	if parsed.Host != "gz.example.com:20001" {
		t.Fatalf("expected first Realm entry, got %q from %q", parsed.Host, link.ImportURL)
	}
	if parsed.User.Username() != "6dd06a03-4987-4ffb-9cbf-6f04daa05d82" || parsed.Query().Get("pbk") != "public-key" || parsed.Query().Get("sni") != "shop.example.com" {
		t.Fatalf("expected final client credentials and Reality parameters to remain unchanged, got %q", link.ImportURL)
	}
	if link.ClientRemark != "final client" || link.TrafficUsedBytes != 5*1024*1024*1024 || link.TrafficLimitBytes != 100*1024*1024*1024 {
		t.Fatalf("expected final client metadata and traffic, got %#v", link)
	}

	unauthorized := assignment
	unauthorized.ClientEmail = "other-user"
	unauthorizedLink := buildCustomerLinkView(unauthorized, chainMap, clientMap, agentMap)
	if unauthorizedLink.Resolved || unauthorizedLink.ImportURL != "" {
		t.Fatalf("must not resolve another client on the same Realm port, got %#v", unauthorizedLink)
	}
}

func TestBuildCustomerLinkViewExportsHAProxyEntryForFinalClient(t *testing.T) {
	finalInbound := model.TopologyInboundRef{AgentID: "dmit", InboundID: 7, InboundTag: "dmit-in", Protocol: "vless", Port: 443}
	hkRealm := model.TopologyInboundRef{AgentID: "hk-b", InboundID: 20001, InboundTag: "realm:b-20001", Protocol: "realm", Port: 20001}
	chains := []model.ClientChainView{{
		Key: customerAssignmentKey("dmit", 7, "alice@example.com"), RootAgentID: "dmit", RootInboundID: 7,
		RootInboundTag: "dmit-in", RootClientEmail: "alice@example.com", RootClientEnabled: true,
		Steps: []model.ClientChainStep{{StepType: "client", AgentID: "dmit"}, {StepType: "inbound", AgentID: "dmit", Protocol: "vless", Port: 443}},
	}}
	links := []model.TopologyLinkView{{
		Key:    "gz::haproxy:10001",
		Source: model.TopologyOutboundRef{AgentID: "gz", OutboundTag: "haproxy:10001", Protocol: "haproxy", ListenPort: 10001},
		Target: hkRealm, FinalTarget: &finalInbound, RealmHops: []model.TopologyInboundRef{hkRealm},
	}}
	chainMap := buildCustomerChainMap(chains, links)
	clientMap := map[string]customerClientRef{
		customerAssignmentKey("dmit", 7, "alice@example.com"): {
			Client: model.XUIClientView{
				InboundID: 7, InboundTag: "dmit-in", Email: "alice@example.com", AuthUUID: "11111111-1111-1111-1111-111111111111",
				ImportURL: "vless://11111111-1111-1111-1111-111111111111@dmit.example.com:443?security=reality&pbk=public-key&sni=shop.example.com#DMIT",
			},
		},
	}
	agentMap := map[string]model.DashboardAgentView{
		"gz": {
			AgentID: "gz", CustomerDisplayName: "Guangzhou Entry",
			Entry: model.AgentEntryConfig{ImportDomain: "gz.example.com", HAProxy: model.HAProxyConfig{Enabled: true, Rules: []model.HAProxyRule{{
				Enabled: true, ListenPort: 10001,
				Primary: model.HAProxyRealmTarget{AgentID: "hk-b", Port: 20001},
				Backups: []model.HAProxyRealmTarget{{AgentID: "hk-c", Port: 20001}},
			}}}},
		},
		"hk-b": {AgentID: "hk-b"},
		"hk-c": {AgentID: "hk-c"},
		"dmit": {AgentID: "dmit"},
	}
	assignment := model.CustomerAssignment{AgentID: "gz", InboundID: 10001, InboundTag: "haproxy:10001", ClientEmail: "alice@example.com"}

	link := buildCustomerLinkView(assignment, chainMap, clientMap, agentMap)
	if !link.Resolved || link.UnresolvedReason != "" {
		t.Fatalf("expected HAProxy assignment to resolve, got %#v", link)
	}
	parsed, err := url.Parse(link.ImportURL)
	if err != nil || parsed.Host != "gz.example.com:10001" {
		t.Fatalf("expected Guangzhou HAProxy entry URL, got %q (%v)", link.ImportURL, err)
	}
	if parsed.User.Username() != "11111111-1111-1111-1111-111111111111" || parsed.Query().Get("pbk") != "public-key" || parsed.Query().Get("sni") != "shop.example.com" {
		t.Fatalf("expected final UUID and Reality settings to remain unchanged, got %q", link.ImportURL)
	}
}

func TestRewriteCustomerHTTPImportURLPreservesAccount(t *testing.T) {
	raw := "http://proxy-user:p%40ss%3Aword@hk.example.com:18080#HTTP"
	rewritten := rewriteCustomerImportURL(raw, "gz.example.com", 20080)
	parsed, err := url.Parse(rewritten)
	if err != nil {
		t.Fatalf("parse rewritten HTTP URL: %v", err)
	}
	password, hasPassword := parsed.User.Password()
	if parsed.Scheme != "http" || parsed.Host != "gz.example.com:20080" || parsed.User.Username() != "proxy-user" || !hasPassword || password != "p@ss:word" {
		t.Fatalf("expected Realm rewrite to preserve HTTP credentials, got %q", rewritten)
	}
}
