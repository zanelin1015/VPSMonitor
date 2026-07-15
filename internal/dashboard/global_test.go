package dashboard

import (
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestBuildGlobalDashboardWithOptionsUsesCachedGeoWithoutNetwork(t *testing.T) {
	originalHostLookup := topologyLookupHostIPs
	originalGeoLookup := topologyLookupIPGeo
	topologyLookupHostIPs = func(host string) []string {
		t.Fatalf("unexpected DNS lookup for %s", host)
		return nil
	}
	topologyLookupIPGeo = func(ip string) (model.IPGeoView, bool) {
		t.Fatalf("unexpected GeoIP lookup for %s", ip)
		return model.IPGeoView{}, false
	}
	defer func() {
		topologyLookupHostIPs = originalHostLookup
		topologyLookupIPGeo = originalGeoLookup
	}()

	now := time.Now().UTC()
	view := BuildGlobalDashboardWithOptions([]model.AgentRecord{{
		AgentID:      "agent-a",
		AgentName:    "Agent A",
		RegisteredAt: now,
		UpdatedAt:    now,
		Summary: model.VPSSummary{
			PublicIPv4: "8.8.8.8",
		},
	}}, nil, GlobalDashboardOptions{
		IncludeTopology:    false,
		IncludeGeo:         true,
		AllowNetworkLookup: false,
		ResolverData: TopologyResolverData{
			Geos: map[string]model.IPGeoView{
				"8.8.8.8": {IP: "8.8.8.8", CountryCode: "US", CountryName: "United States"},
			},
		},
	})

	if len(view.Agents) != 1 || view.Agents[0].Geo == nil || view.Agents[0].Geo.CountryCode != "US" {
		t.Fatalf("expected cached geo to be used, got %#v", view.Agents)
	}
	if view.Totals.LinkCount != 0 || view.Totals.ChainCount != 0 || len(view.Links) != 0 || len(view.ClientChains) != 0 {
		t.Fatalf("expected lightweight dashboard without topology, got totals=%#v", view.Totals)
	}
}

func TestBuildGlobalDashboardIncludesNetworkPolicySnapshot(t *testing.T) {
	now := time.Now().UTC()
	view := BuildGlobalDashboardWithOptions([]model.AgentRecord{{
		AgentID:      "agent-a",
		AgentName:    "Agent A",
		RegisteredAt: now,
		UpdatedAt:    now,
	}}, []model.AgentSnapshot{{
		AgentID:    "agent-a",
		ReportedAt: now,
		NetworkPolicy: &model.NetworkPolicySnapshot{
			CollectedAt:     now,
			FirewallBackend: "ufw",
			Rules: []model.NetworkPortPolicyRule{{
				Enabled:      true,
				Port:         20010,
				Protocol:     "both",
				WhitelistIPs: []string{"104.194.70.102"},
			}},
		},
	}}, GlobalDashboardOptions{IncludeTopology: false})

	if len(view.Agents) != 1 || view.Agents[0].NetworkPolicy == nil {
		t.Fatalf("expected network policy snapshot in dashboard agent, got %#v", view.Agents)
	}
	if got := view.Agents[0].NetworkPolicy.Rules; len(got) != 1 || got[0].Port != 20010 || len(got[0].WhitelistIPs) != 1 {
		t.Fatalf("unexpected network policy rules: %#v", got)
	}
}

func TestBuildGlobalDashboardMatchesCrossClientTopology(t *testing.T) {
	now := time.Now().UTC()
	originalLookup := topologyLookupHostIPs
	topologyLookupHostIPs = func(host string) []string {
		switch host {
		case "b.example.com":
			return []string{"203.0.113.20"}
		default:
			return nil
		}
	}
	defer func() {
		topologyLookupHostIPs = originalLookup
	}()

	agents := []model.AgentRecord{
		{
			AgentID:      "agent-a",
			AgentName:    "Agent A",
			Tags:         []string{"edge", "hk"},
			RegisteredAt: now,
			UpdatedAt:    now,
			Summary: model.VPSSummary{
				PublicIPv4:       "203.0.113.10",
				InboundCount:     1,
				OutboundCount:    1,
				RoutingRuleCount: 1,
			},
		},
		{
			AgentID:      "agent-b",
			AgentName:    "Agent B",
			Tags:         []string{"transit"},
			RegisteredAt: now,
			UpdatedAt:    now,
			Summary: model.VPSSummary{
				PublicIPv4:       "203.0.113.20",
				InboundCount:     1,
				OutboundCount:    1,
				RoutingRuleCount: 1,
			},
		},
	}

	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "agent-a",
			AgentName:  "Agent A",
			ReportedAt: now,
			Summary:    agents[0].Summary,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":             1,
						"tag":            "in-a",
						"remark":         "A Entry",
						"protocol":       "vless",
						"port":           443,
						"enable":         true,
						"settings":       `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"alice@example.com","enable":true}]}`,
						"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"a.example.com"}}`,
						"clientStats": []map[string]any{
							{"email": "alice@example.com", "enable": true, "lastOnline": now.Add(-2 * time.Minute).UnixMilli()},
						},
					},
				},
				Outbounds: []map[string]any{
					{
						"tag":      "relay-b",
						"protocol": "vless",
						"settings": map[string]any{
							"vnext": []map[string]any{
								{"address": "b.example.com", "port": 8443},
							},
						},
						"streamSettings": map[string]any{
							"network":  "tcp",
							"security": "tls",
							"tlsSettings": map[string]any{
								"serverName": "b.example.com",
							},
						},
					},
				},
				RoutingRules: []map[string]any{
					{"type": "field", "user": []string{"alice@example.com"}, "outboundTag": "relay-b"},
				},
			},
		},
		{
			AgentID:    "agent-b",
			AgentName:  "Agent B",
			ReportedAt: now,
			Summary:    agents[1].Summary,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Certificates: []model.XUILocalCertificate{
					{ID: "cert-b", DNSNames: []string{"b.example.com"}},
				},
				Inbounds: []map[string]any{
					{
						"id":             2,
						"tag":            "in-b",
						"remark":         "B Transit",
						"protocol":       "vless",
						"port":           8443,
						"enable":         true,
						"settings":       `{"clients":[]}`,
						"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"b.example.com"}}`,
					},
				},
				Outbounds: []map[string]any{
					{"tag": "direct", "protocol": "freedom"},
				},
				RoutingRules: []map[string]any{
					{"type": "field", "inboundTag": []string{"in-b"}, "outboundTag": "direct"},
				},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if view.Totals.AgentCount != 2 {
		t.Fatalf("expected 2 agents, got %d", view.Totals.AgentCount)
	}
	if view.Totals.ClientCount != 1 {
		t.Fatalf("expected 1 client, got %d", view.Totals.ClientCount)
	}
	if view.Totals.LinkCount != 1 {
		t.Fatalf("expected 1 topology link, got %d", view.Totals.LinkCount)
	}
	if len(view.ClientChains) != 1 {
		t.Fatalf("expected 1 client chain, got %d", len(view.ClientChains))
	}
	if view.ClientChains[0].MatchedLinkCount != 1 {
		t.Fatalf("expected client chain to include one matched link, got %#v", view.ClientChains[0])
	}
	if view.ClientChains[0].RootInboundID != 1 {
		t.Fatalf("expected client chain root inbound id 1, got %#v", view.ClientChains[0])
	}
	if view.Links[0].Target.AgentID != "agent-b" {
		t.Fatalf("expected outbound to match agent-b inbound, got %#v", view.Links[0])
	}

	lightweight := BuildGlobalDashboardWithOptions(agents, snapshots, GlobalDashboardOptions{IncludeTopology: false})
	if len(lightweight.ClientChains) != 0 {
		t.Fatalf("expected lightweight dashboard without topology chains, got %#v", lightweight.ClientChains)
	}
	if !lightweight.Agents[0].FinanceClientsReady || len(lightweight.Agents[0].FinanceClients) != 1 {
		t.Fatalf("expected lightweight dashboard finance client state, got %#v", lightweight.Agents[0].FinanceClients)
	}
	financeClient := lightweight.Agents[0].FinanceClients[0]
	if financeClient.InboundID != 1 || financeClient.InboundTag != "in-a" || financeClient.Email != "alice@example.com" || !financeClient.NodeEnabled || !financeClient.Enabled {
		t.Fatalf("unexpected lightweight finance client: %#v", financeClient)
	}
	if !view.ClientChains[0].RootInboundEnabled {
		t.Fatalf("expected client chain to retain enabled root inbound, got %#v", view.ClientChains[0])
	}
}

func TestBuildGlobalDashboardFinanceClientTracksDisabledInbound(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{{
		AgentID:      "agent-disabled",
		RegisteredAt: now,
		UpdatedAt:    now,
	}}
	snapshots := []model.AgentSnapshot{{
		AgentID:    "agent-disabled",
		ReportedAt: now,
		XUI: &model.XUISnapshot{
			CollectedAt: now,
			Inbounds: []map[string]any{{
				"id":       9,
				"tag":      "disabled-inbound",
				"protocol": "vless",
				"enable":   false,
				"settings": `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"enabled-client","enable":true}]}`,
			}},
		},
	}}

	view := BuildGlobalDashboardWithOptions(agents, snapshots, GlobalDashboardOptions{IncludeTopology: false})
	if len(view.Agents) != 1 || len(view.Agents[0].FinanceClients) != 1 {
		t.Fatalf("expected one finance client, got %#v", view.Agents)
	}
	client := view.Agents[0].FinanceClients[0]
	if client.NodeEnabled || !client.Enabled {
		t.Fatalf("expected enabled client on disabled node, got %#v", client)
	}
}

func TestBuildGlobalDashboardMergesRealmSnapshotIntoAgentEntry(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{{
		AgentID:      "gz",
		AgentName:    "Guangzhou",
		RegisteredAt: now,
		UpdatedAt:    now,
		Config: model.ManagedAgentConfig{
			Entry: model.AgentEntryConfig{Addresses: []string{"gz.example.com"}},
		},
	}}
	snapshots := []model.AgentSnapshot{{
		AgentID:    "gz",
		AgentName:  "Guangzhou",
		ReportedAt: now,
		Realm: &model.RealmSnapshot{
			ConfigPath:  "/etc/realm/config.toml",
			CollectedAt: now,
			Rules: []model.RealmForwardRule{{
				Enabled:       true,
				ListenAddress: "0.0.0.0",
				ListenPort:    2443,
				TargetAddress: "hk.example.com",
				TargetPort:    443,
				Network:       "tcp",
			}},
		},
	}}

	view := BuildGlobalDashboard(agents, snapshots)
	if len(view.Agents) != 1 || len(view.Agents[0].Entry.PortForwarding.Rules) != 1 {
		t.Fatalf("expected realm snapshot rule on dashboard agent, got %#v", view.Agents)
	}
	rule := view.Agents[0].Entry.PortForwarding.Rules[0]
	if !view.Agents[0].Entry.PortForwarding.Enabled || rule.ListenPort != 2443 || rule.TargetAddress != "hk.example.com" {
		t.Fatalf("unexpected merged realm config: %#v", view.Agents[0].Entry.PortForwarding)
	}
}

func TestBuildGlobalDashboardPrefersCollectedRealmConfigForSameListenPort(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{{
		AgentID:      "gz",
		AgentName:    "Guangzhou",
		RegisteredAt: now,
		UpdatedAt:    now,
		Config: model.ManagedAgentConfig{
			Entry: model.AgentEntryConfig{
				PortForwarding: model.RealmForwardConfig{
					Enabled:     true,
					Backend:     "realm",
					ConfigPath:  "/etc/vpsmonitor/realm.toml",
					ServiceName: "vpsmonitor-realm",
					Rules: []model.RealmForwardRule{{
						ID:            "operator-rule",
						Enabled:       true,
						ListenAddress: "0.0.0.0",
						ListenPort:    20001,
						TargetAgentID: "hk",
						TargetAddress: "old.example.com",
						TargetPort:    20001,
						Network:       "tcp",
						Note:          "keep target agent metadata",
					}},
				},
			},
		},
	}}
	snapshots := []model.AgentSnapshot{{
		AgentID:    "gz",
		AgentName:  "Guangzhou",
		ReportedAt: now,
		Realm: &model.RealmSnapshot{
			ConfigPath:  "/etc/realm/config.toml",
			ServiceName: "realm",
			CollectedAt: now,
			Rules: []model.RealmForwardRule{{
				ID:            "auto-realm-20001-20001-0",
				Enabled:       true,
				ListenAddress: "0.0.0.0",
				ListenPort:    20001,
				TargetAddress: "47.239.135.242",
				TargetPort:    20001,
				Network:       "tcp",
			}},
		},
	}}

	view := BuildGlobalDashboard(agents, snapshots)
	got := view.Agents[0].Entry.PortForwarding
	if got.ConfigPath != "/etc/realm/config.toml" || got.ServiceName != "realm" {
		t.Fatalf("expected collected realm metadata to win, got %#v", got)
	}
	if len(got.Rules) != 1 {
		t.Fatalf("expected collected realm rule to replace same listen port, got %#v", got.Rules)
	}
	rule := got.Rules[0]
	if rule.TargetAddress != "47.239.135.242" || rule.TargetAgentID != "hk" || rule.Note != "keep target agent metadata" {
		t.Fatalf("expected collected target with preserved operator metadata, got %#v", rule)
	}
}

func TestBuildGlobalDashboardMatchesRealmToShadowsocksInbound(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{
		{
			AgentID:      "gz",
			AgentName:    "Guangzhou",
			RegisteredAt: now,
			UpdatedAt:    now,
		},
		{
			AgentID:      "hk",
			AgentName:    "Hong Kong",
			RegisteredAt: now,
			UpdatedAt:    now,
			Summary: model.VPSSummary{
				PublicIPv4: "47.239.135.242",
			},
		},
	}
	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "gz",
			AgentName:  "Guangzhou",
			ReportedAt: now,
			Realm: &model.RealmSnapshot{
				ConfigPath:  "/etc/realm/config.toml",
				ServiceName: "realm",
				CollectedAt: now,
				Rules: []model.RealmForwardRule{{
					ID:            "auto-realm-20003-20003-2",
					Enabled:       true,
					ListenAddress: "0.0.0.0",
					ListenPort:    20003,
					TargetAddress: "47.239.135.242",
					TargetPort:    20003,
					Network:       "both",
				}},
			},
		},
		{
			AgentID:    "hk",
			AgentName:  "Hong Kong",
			ReportedAt: now,
			Summary:    model.VPSSummary{PublicIPv4: "47.239.135.242"},
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{{
					"id":       3,
					"tag":      "inbound-20003",
					"remark":   "CN-HK-SS",
					"protocol": "shadowsocks",
					"port":     20003,
					"enable":   true,
					"settings": `{"clients":[{"email":"ss-client","enable":true}]}`,
				}},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if view.Totals.LinkCount != 1 {
		t.Fatalf("expected realm to match shadowsocks inbound, got %d links: %#v", view.Totals.LinkCount, view.Links)
	}
	link := view.Links[0]
	if link.Source.Protocol != "realm" || link.Target.Protocol != "shadowsocks" || link.Target.Port != 20003 {
		t.Fatalf("unexpected realm shadowsocks link: %#v", link)
	}
	if !containsString(link.MatchFields, "address_ip") || !containsString(link.MatchFields, "port") {
		t.Fatalf("expected address_ip and port match fields, got %#v", link.MatchFields)
	}
}

func TestBuildGlobalDashboardMatchesDirectIPOutbound(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{
		{AgentID: "edge", AgentName: "Edge", RegisteredAt: now, UpdatedAt: now},
		{
			AgentID:      "landing",
			AgentName:    "Landing",
			RegisteredAt: now,
			UpdatedAt:    now,
			Summary: model.VPSSummary{
				PublicIPv4: "203.0.113.20",
			},
		},
	}

	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "edge",
			AgentName:  "Edge",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":       1,
						"tag":      "edge-in",
						"protocol": "vmess",
						"port":     443,
						"settings": `{"clients":[{"email":"alice","enable":true}]}`,
					},
				},
				Outbounds: []map[string]any{
					{
						"tag":      "relay-by-ip",
						"protocol": "vmess",
						"settings": map[string]any{
							"vnext": []map[string]any{
								{"address": "203.0.113.20", "port": 20001},
							},
						},
						"streamSettings": map[string]any{
							"network": "tcp",
						},
					},
				},
				RoutingRules: []map[string]any{
					{"type": "field", "user": []string{"alice"}, "outboundTag": "relay-by-ip"},
				},
			},
		},
		{
			AgentID:    "landing",
			AgentName:  "Landing",
			ReportedAt: now,
			Summary:    agents[1].Summary,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":             2,
						"tag":            "landing-vmess",
						"remark":         "Landing VMess",
						"protocol":       "vmess",
						"port":           20001,
						"settings":       `{"clients":[]}`,
						"streamSettings": `{"network":"tcp"}`,
					},
				},
				Outbounds: []map[string]any{
					{"tag": "direct", "protocol": "freedom"},
				},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if view.Totals.LinkCount != 1 {
		t.Fatalf("expected direct IP outbound to match one link, got %d", view.Totals.LinkCount)
	}
	link := view.Links[0]
	if link.Target.AgentID != "landing" {
		t.Fatalf("expected landing target, got %#v", link)
	}
	if !containsString(link.MatchFields, "address_ip") {
		t.Fatalf("expected address_ip match field, got %#v", link.MatchFields)
	}
	if !containsString(link.MatchFields, "port") {
		t.Fatalf("expected port match field, got %#v", link.MatchFields)
	}
}

func TestBuildGlobalDashboardExpandsBalancerRoute(t *testing.T) {
	now := time.Now().UTC()

	agents := []model.AgentRecord{
		{
			AgentID:      "edge-a",
			AgentName:    "Edge A",
			RegisteredAt: now,
			UpdatedAt:    now,
		},
		{
			AgentID:      "relay-b",
			AgentName:    "Relay B",
			RegisteredAt: now,
			UpdatedAt:    now,
		},
	}

	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "edge-a",
			AgentName:  "Edge A",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":             1,
						"tag":            "edge-in",
						"remark":         "Edge Entry",
						"protocol":       "vless",
						"port":           443,
						"enable":         true,
						"settings":       `{"clients":[{"email":"front@example.com","enable":true}]}`,
						"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"edge.example.com"}}`,
					},
				},
				Outbounds: []map[string]any{
					{
						"tag":      "relay-b",
						"protocol": "vless",
						"settings": map[string]any{
							"vnext": []map[string]any{
								{"address": "relay.example.com", "port": 9443},
							},
						},
					},
				},
				RoutingRules: []map[string]any{
					{"type": "field", "user": []string{"front@example.com"}, "balancerTag": "relay-balancer"},
				},
				RawConfig: map[string]any{
					"routing": map[string]any{
						"balancers": []map[string]any{
							{"tag": "relay-balancer", "selector": []string{"relay-"}},
						},
					},
				},
			},
		},
		{
			AgentID:    "relay-b",
			AgentName:  "Relay B",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":             2,
						"tag":            "relay-in",
						"remark":         "Relay Entry",
						"protocol":       "vless",
						"port":           9443,
						"enable":         true,
						"settings":       `{"clients":[]}`,
						"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"relay.example.com"}}`,
					},
				},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if len(view.ClientChains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(view.ClientChains))
	}
	chain := view.ClientChains[0]
	seenBalancer := false
	for _, step := range chain.Steps {
		if step.StepType == "balancer" && step.Label == "relay-balancer" {
			seenBalancer = true
		}
	}
	if !seenBalancer {
		t.Fatalf("expected balancer step in chain: %#v", chain.Steps)
	}
	if chain.MatchedLinkCount != 1 {
		t.Fatalf("expected balancer chain to match one link, got %#v", chain)
	}
}

func TestBuildGlobalDashboardMatchesByCredentialWithMappedPort(t *testing.T) {
	now := time.Now().UTC()
	originalLookup := topologyLookupHostIPs
	topologyLookupHostIPs = func(host string) []string {
		if host == "landing.example.com" {
			return []string{"203.0.113.30"}
		}
		return nil
	}
	defer func() {
		topologyLookupHostIPs = originalLookup
	}()

	agents := []model.AgentRecord{
		{AgentID: "edge", AgentName: "Edge", RegisteredAt: now, UpdatedAt: now},
		{
			AgentID:      "landing",
			AgentName:    "Landing",
			RegisteredAt: now,
			UpdatedAt:    now,
			Summary: model.VPSSummary{
				PublicIPv4: "203.0.113.30",
			},
		},
	}
	clientID := "22222222-2222-2222-2222-222222222222"
	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "edge",
			AgentName:  "Edge",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":       1,
						"tag":      "edge-in",
						"protocol": "vless",
						"port":     443,
						"settings": `{"clients":[{"email":"alice","enable":true}]}`,
					},
				},
				Outbounds: []map[string]any{
					{
						"tag":      "mapped-relay",
						"protocol": "vless",
						"settings": map[string]any{
							"address": "landing.example.com",
							"port":    20108,
							"id":      clientID,
						},
					},
				},
				RoutingRules: []map[string]any{
					{"type": "field", "user": []string{"alice"}, "outboundTag": "mapped-relay"},
				},
			},
		},
		{
			AgentID:    "landing",
			AgentName:  "Landing",
			ReportedAt: now,
			Summary:    agents[1].Summary,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":       2,
						"tag":      "landing-in",
						"protocol": "vless",
						"port":     20008,
						"settings": `{"clients":[{"id":"22222222-2222-2222-2222-222222222222","email":"relay-user","enable":true}]}`,
					},
				},
				Outbounds: []map[string]any{
					{"tag": "direct", "protocol": "freedom"},
				},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if view.Totals.LinkCount != 1 {
		t.Fatalf("expected one credential-matched link, got %d", view.Totals.LinkCount)
	}
	link := view.Links[0]
	if link.Target.AgentID != "landing" {
		t.Fatalf("expected landing target, got %#v", link)
	}
	if !containsString(link.MatchFields, "credential") {
		t.Fatalf("expected credential match field, got %#v", link.MatchFields)
	}
	if !containsString(link.MatchFields, "port_mapped") {
		t.Fatalf("expected mapped port match field, got %#v", link.MatchFields)
	}
	if view.ClientChains[0].MatchedLinkCount != 1 {
		t.Fatalf("expected chain to follow credential link, got %#v", view.ClientChains[0])
	}
}

func TestBuildGlobalDashboardMatchesConfiguredEntryMapping(t *testing.T) {
	now := time.Now().UTC()
	originalLookup := topologyLookupHostIPs
	topologyLookupHostIPs = func(host string) []string {
		if host == "nat.example.com" {
			return []string{"198.51.100.88"}
		}
		return nil
	}
	defer func() {
		topologyLookupHostIPs = originalLookup
	}()

	agents := []model.AgentRecord{
		{AgentID: "edge", AgentName: "Edge", RegisteredAt: now, UpdatedAt: now},
		{
			AgentID:      "landing",
			AgentName:    "Landing NAT",
			RegisteredAt: now,
			UpdatedAt:    now,
			Summary: model.VPSSummary{
				PublicIPv4: "203.0.113.44",
			},
			Config: model.ManagedAgentConfig{
				Entry: model.AgentEntryConfig{
					Addresses: []string{"nat.example.com"},
					Mappings: []model.AgentEntryMapping{
						{Address: "nat.example.com", ExternalPort: 20002, InternalPort: 1080, Protocol: "socks", Note: "NAT S5"},
					},
				},
			},
		},
	}

	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "edge",
			AgentName:  "Edge",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":       1,
						"tag":      "edge-in",
						"protocol": "vless",
						"port":     443,
						"settings": `{"clients":[{"email":"alice","enable":true}]}`,
					},
				},
				Outbounds: []map[string]any{
					{
						"tag":      "att-s5",
						"protocol": "socks",
						"settings": map[string]any{
							"servers": []map[string]any{
								{"address": "nat.example.com", "port": 20002},
							},
						},
					},
				},
				RoutingRules: []map[string]any{
					{"type": "field", "user": []string{"alice"}, "outboundTag": "att-s5"},
				},
			},
		},
		{
			AgentID:    "landing",
			AgentName:  "Landing NAT",
			ReportedAt: now,
			Summary:    agents[1].Summary,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{
					{
						"id":       2,
						"tag":      "socks-in",
						"remark":   "ATT-S5-落地",
						"protocol": "socks",
						"port":     1080,
						"settings": `{"accounts":[]}`,
					},
				},
				Outbounds: []map[string]any{
					{"tag": "direct", "protocol": "freedom"},
				},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if view.Totals.LinkCount != 1 {
		t.Fatalf("expected one entry-mapped link, got %d", view.Totals.LinkCount)
	}
	link := view.Links[0]
	if link.Target.AgentID != "landing" {
		t.Fatalf("expected landing target, got %#v", link)
	}
	if !containsString(link.MatchFields, "entry_mapping") {
		t.Fatalf("expected entry_mapping match field, got %#v", link.MatchFields)
	}
	if link.MatchConfidence != "high" {
		t.Fatalf("expected high confidence, got %q", link.MatchConfidence)
	}
}

func TestBuildGlobalDashboardMatchesImportDomain(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{
		{
			AgentID:      "edge",
			AgentName:    "Edge",
			RegisteredAt: now,
			UpdatedAt:    now,
		},
		{
			AgentID:      "landing",
			AgentName:    "Landing",
			RegisteredAt: now,
			UpdatedAt:    now,
			Config: model.ManagedAgentConfig{
				Entry: model.AgentEntryConfig{
					ImportDomain: "landing.example.com",
				},
			},
		},
	}
	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "edge",
			AgentName:  "Edge",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{{
					"id":       1,
					"tag":      "edge-in",
					"protocol": "vmess",
					"port":     20005,
					"settings": `{"clients":[{"email":"alice","enable":true}]}`,
				}},
				Outbounds: []map[string]any{{
					"tag":      "to-landing",
					"protocol": "vless",
					"settings": map[string]any{
						"vnext": []map[string]any{{"address": "landing.example.com", "port": 20106}},
					},
					"streamSettings": map[string]any{"network": "tcp", "security": "reality"},
				}},
				RoutingRules: []map[string]any{{"type": "field", "user": []string{"alice"}, "outboundTag": "to-landing"}},
			},
		},
		{
			AgentID:    "landing",
			AgentName:  "Landing",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{{
					"id":             19,
					"tag":            "inbound-20106",
					"remark":         "Anilam",
					"protocol":       "vless",
					"port":           20106,
					"settings":       `{"clients":[]}`,
					"streamSettings": `{"network":"tcp","security":"reality"}`,
				}},
				Outbounds: []map[string]any{{"tag": "direct", "protocol": "freedom"}},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if view.Totals.LinkCount != 1 {
		t.Fatalf("expected import-domain link, got %d", view.Totals.LinkCount)
	}
	link := view.Links[0]
	if link.Target.AgentID != "landing" || !containsString(link.MatchFields, "address_domain") {
		t.Fatalf("expected landing import domain match, got %#v", link)
	}
	if len(view.ClientChains) != 1 || view.ClientChains[0].MatchedLinkCount != 1 {
		t.Fatalf("expected client chain to follow import-domain link, got %#v", view.ClientChains)
	}
}

func TestBuildGlobalDashboardMatchesNonPrimaryCertificateDomain(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{
		{
			AgentID:      "edge",
			AgentName:    "Edge",
			RegisteredAt: now,
			UpdatedAt:    now,
		},
		{
			AgentID:      "landing",
			AgentName:    "Landing",
			RegisteredAt: now,
			UpdatedAt:    now,
			Config: model.ManagedAgentConfig{
				Entry: model.AgentEntryConfig{
					ImportDomain: "primary.example.com",
				},
			},
		},
	}
	snapshots := []model.AgentSnapshot{
		{
			AgentID:    "edge",
			AgentName:  "Edge",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Inbounds: []map[string]any{{
					"id":       1,
					"tag":      "edge-in",
					"protocol": "vmess",
					"port":     20005,
					"settings": `{"clients":[{"email":"alice","enable":true}]}`,
				}},
				Outbounds: []map[string]any{{
					"tag":      "att-s5",
					"protocol": "socks",
					"settings": map[string]any{
						"servers": []map[string]any{{"address": "other.example.com", "port": 20002}},
					},
				}},
				RoutingRules: []map[string]any{{"type": "field", "user": []string{"alice"}, "outboundTag": "att-s5"}},
			},
		},
		{
			AgentID:    "landing",
			AgentName:  "Landing",
			ReportedAt: now,
			XUI: &model.XUISnapshot{
				CollectedAt: now,
				Certificates: []model.XUILocalCertificate{
					{ID: "primary", DNSNames: []string{"primary.example.com"}},
					{ID: "other", DNSNames: []string{"other.example.com"}},
				},
				Inbounds: []map[string]any{{
					"id":       2,
					"tag":      "socks-in",
					"remark":   "ATT-S5",
					"protocol": "socks",
					"port":     20002,
					"settings": `{"accounts":[]}`,
				}},
				Outbounds: []map[string]any{{"tag": "direct", "protocol": "freedom"}},
			},
		},
	}

	view := BuildGlobalDashboard(agents, snapshots)
	if view.Totals.LinkCount != 1 {
		t.Fatalf("expected non-primary cert-domain link, got %d", view.Totals.LinkCount)
	}
	link := view.Links[0]
	if link.Target.AgentID != "landing" || !containsString(link.MatchFields, "address_domain") {
		t.Fatalf("expected landing cert domain match, got %#v", link)
	}
	if len(view.ClientChains) != 1 || view.ClientChains[0].MatchedLinkCount != 1 {
		t.Fatalf("expected client chain to follow cert-domain link, got %#v", view.ClientChains)
	}
}
