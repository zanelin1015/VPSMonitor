package dashboard

import (
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestBuildGlobalDashboardResolvesMultiHopRealmToXUIInbound(t *testing.T) {
	for _, protocol := range []string{"vless", "shadowsocks", "http"} {
		t.Run(protocol, func(t *testing.T) {
			now := time.Now().UTC()
			inboundSettings := `{"clients":[{"email":"alice@example.com","enable":true}]}`
			if protocol == "http" {
				inboundSettings = `{"accounts":[{"user":"proxy-user","pass":"proxy-pass"}]}`
			}
			agents := []model.AgentRecord{
				{
					AgentID: "gz", AgentName: "Guangzhou", RegisteredAt: now, UpdatedAt: now,
					Summary: model.VPSSummary{PublicIPv4: "192.0.2.10"},
					Config: model.ManagedAgentConfig{Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{{
						ID: "gz-20001", Enabled: true, ListenPort: 20001, TargetAgentID: "hk", TargetAddress: "192.0.2.20", TargetPort: 20001, Network: "tcp",
					}}}}},
				},
				{
					AgentID: "hk", AgentName: "Hong Kong Relay", RegisteredAt: now, UpdatedAt: now,
					Summary: model.VPSSummary{PublicIPv4: "192.0.2.20"},
					Config: model.ManagedAgentConfig{Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{{
						ID: "hk-20001", Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetAddress: "192.0.2.30", TargetPort: 443, Network: "tcp",
					}}}}},
				},
				{AgentID: "dmit", AgentName: "DMIT", RegisteredAt: now, UpdatedAt: now, Summary: model.VPSSummary{PublicIPv4: "192.0.2.30"}},
			}
			snapshots := []model.AgentSnapshot{{
				AgentID: "dmit", AgentName: "DMIT", ReportedAt: now, Summary: model.VPSSummary{PublicIPv4: "192.0.2.30"},
				XUI: &model.XUISnapshot{CollectedAt: now, Inbounds: []map[string]any{{
					"id": 7, "tag": "dmit-in", "remark": "DMIT inbound", "protocol": protocol, "port": 443, "enable": true,
					"settings": inboundSettings,
				}}},
			}}

			view := BuildGlobalDashboardWithOptions(agents, snapshots, GlobalDashboardOptions{IncludeTopology: true})
			if len(view.Links) != 2 {
				t.Fatalf("expected two Realm hops, got %d: %#v", len(view.Links), view.Links)
			}
			if len(view.ClientChains) != 1 {
				t.Fatalf("expected final %s account to produce one customer chain, got %#v", protocol, view.ClientChains)
			}
			gzLink := topologyLinkFromAgent(t, view.Links, "gz")
			if gzLink.Target.AgentID != "hk" || gzLink.Target.Protocol != "realm" {
				t.Fatalf("expected GZ to target the HK Realm listener, got %#v", gzLink.Target)
			}
			if gzLink.FinalTarget == nil || gzLink.FinalTarget.AgentID != "dmit" || gzLink.FinalTarget.Protocol != protocol || gzLink.FinalTarget.Port != 443 {
				t.Fatalf("expected GZ Realm chain to resolve to the final %s inbound, got %#v", protocol, gzLink.FinalTarget)
			}
			if len(gzLink.RealmHops) != 1 || gzLink.RealmHops[0].AgentID != "hk" {
				t.Fatalf("expected HK as the intermediate Realm hop, got %#v", gzLink.RealmHops)
			}
			hkLink := topologyLinkFromAgent(t, view.Links, "hk")
			if hkLink.FinalTarget == nil || hkLink.FinalTarget.AgentID != "dmit" || len(hkLink.RealmHops) != 0 {
				t.Fatalf("expected HK Realm to resolve directly to DMIT, got %#v", hkLink)
			}
		})
	}
}

func TestBuildGlobalDashboardResolvesHAProxyPrimaryRealmPathToFinalInbound(t *testing.T) {
	now := time.Now().UTC()
	agents := []model.AgentRecord{
		{
			AgentID: "gz", AgentName: "Guangzhou HAProxy", RegisteredAt: now, UpdatedAt: now,
			Summary: model.VPSSummary{PublicIPv4: "192.0.2.10"},
			Config: model.ManagedAgentConfig{Entry: model.AgentEntryConfig{HAProxy: model.HAProxyConfig{
				Enabled: true,
				Rules: []model.HAProxyRule{{
					ID: "gz-ha-20001", Enabled: true, ListenPort: 10001,
					Primary: model.HAProxyRealmTarget{AgentID: "hk-b", Address: "b.example.com", Port: 20001},
					Backups: []model.HAProxyRealmTarget{{AgentID: "hk-c", Address: "c.example.com", Port: 20001}},
				}},
			}}},
		},
		{
			AgentID: "hk-b", AgentName: "HK B", RegisteredAt: now, UpdatedAt: now,
			Summary: model.VPSSummary{PublicIPv4: "192.0.2.20"},
			Config: model.ManagedAgentConfig{Entry: model.AgentEntryConfig{ImportDomain: "b.example.com", PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{{
				ID: "b-20001", Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetAddress: "d.example.com", TargetPort: 443, Network: "tcp",
			}}}}},
		},
		{
			AgentID: "hk-c", AgentName: "HK C", RegisteredAt: now, UpdatedAt: now,
			Summary: model.VPSSummary{PublicIPv4: "192.0.2.21"},
			Config: model.ManagedAgentConfig{Entry: model.AgentEntryConfig{ImportDomain: "c.example.com", PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{{
				ID: "c-20001", Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetAddress: "d.example.com", TargetPort: 443, Network: "tcp",
			}}}}},
		},
		{
			AgentID: "dmit", AgentName: "DMIT", RegisteredAt: now, UpdatedAt: now,
			Summary: model.VPSSummary{PublicIPv4: "192.0.2.30"},
			Config:  model.ManagedAgentConfig{Entry: model.AgentEntryConfig{ImportDomain: "d.example.com"}},
		},
	}
	snapshots := []model.AgentSnapshot{{
		AgentID: "dmit", AgentName: "DMIT", ReportedAt: now, Summary: model.VPSSummary{PublicIPv4: "192.0.2.30"},
		XUI: &model.XUISnapshot{CollectedAt: now, Inbounds: []map[string]any{{
			"id": 7, "tag": "dmit-in", "remark": "DMIT inbound", "protocol": "vless", "port": 443, "enable": true,
			"settings": `{"clients":[{"email":"alice@example.com","enable":true}]}`,
		}}},
	}}

	view := BuildGlobalDashboardWithOptions(agents, snapshots, GlobalDashboardOptions{IncludeTopology: true})
	gzLink := topologyLinkFromAgent(t, view.Links, "gz")
	if gzLink.Source.Protocol != "haproxy" || gzLink.Target.AgentID != "hk-b" || gzLink.Target.Protocol != "realm" {
		t.Fatalf("expected HAProxy to follow its primary HK B Realm listener, got %#v", gzLink)
	}
	if gzLink.FinalTarget == nil || gzLink.FinalTarget.AgentID != "dmit" || gzLink.FinalTarget.InboundID != 7 {
		t.Fatalf("expected HAProxy path to resolve to the final DMIT inbound, got %#v", gzLink.FinalTarget)
	}
	if len(gzLink.RealmHops) != 1 || gzLink.RealmHops[0].AgentID != "hk-b" {
		t.Fatalf("expected HK B as the primary runtime path, got %#v", gzLink.RealmHops)
	}
}

func TestBuildGlobalDashboardMarksRealmLoopAndBrokenMiddleHop(t *testing.T) {
	now := time.Now().UTC()
	baseAgent := func(id, ip string, rule model.RealmForwardRule) model.AgentRecord {
		return model.AgentRecord{
			AgentID: id, AgentName: id, RegisteredAt: now, UpdatedAt: now, Summary: model.VPSSummary{PublicIPv4: ip},
			Config: model.ManagedAgentConfig{Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{rule}}}},
		}
	}

	loopAgents := []model.AgentRecord{
		baseAgent("gz", "192.0.2.10", model.RealmForwardRule{ID: "gz", Enabled: true, ListenPort: 20001, TargetAgentID: "hk", TargetAddress: "192.0.2.20", TargetPort: 20001}),
		baseAgent("hk", "192.0.2.20", model.RealmForwardRule{ID: "hk", Enabled: true, ListenPort: 20001, TargetAgentID: "gz", TargetAddress: "192.0.2.10", TargetPort: 20001}),
	}
	loopView := BuildGlobalDashboardWithOptions(loopAgents, nil, GlobalDashboardOptions{IncludeTopology: true})
	if len(loopView.Links) != 2 {
		t.Fatalf("expected both sides of the Realm loop, got %#v", loopView.Links)
	}
	for _, link := range loopView.Links {
		if !link.LoopDetected || link.UnresolvedReason == "" || link.FinalTarget != nil {
			t.Fatalf("expected loop metadata without a final target, got %#v", link)
		}
	}

	brokenAgents := []model.AgentRecord{
		baseAgent("gz", "192.0.2.10", model.RealmForwardRule{ID: "gz", Enabled: true, ListenPort: 20001, TargetAgentID: "hk", TargetAddress: "192.0.2.20", TargetPort: 20001}),
		baseAgent("hk", "192.0.2.20", model.RealmForwardRule{ID: "hk", Enabled: true, ListenPort: 20001, TargetAddress: "198.51.100.99", TargetPort: 443}),
	}
	brokenView := BuildGlobalDashboardWithOptions(brokenAgents, nil, GlobalDashboardOptions{IncludeTopology: true})
	if len(brokenView.Links) != 1 {
		t.Fatalf("expected the resolvable first hop only, got %#v", brokenView.Links)
	}
	broken := brokenView.Links[0]
	if broken.Source.AgentID != "gz" || broken.FinalTarget != nil || broken.UnresolvedReason == "" {
		t.Fatalf("expected GZ link to report the unresolved HK hop, got %#v", broken)
	}
}

func topologyLinkFromAgent(t *testing.T, links []model.TopologyLinkView, agentID string) model.TopologyLinkView {
	t.Helper()
	for _, link := range links {
		if link.Source.AgentID == agentID {
			return link
		}
	}
	t.Fatalf("topology link from %s not found in %#v", agentID, links)
	return model.TopologyLinkView{}
}
