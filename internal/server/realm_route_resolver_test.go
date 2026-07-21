package server

import (
	"net/url"
	"testing"

	"bridge-core/internal/model"
)

func TestResolveRealmForwardTargetAcrossMultipleHops(t *testing.T) {
	for _, protocol := range []string{"vless", "shadowsocks"} {
		t.Run(protocol, func(t *testing.T) {
			gzRule := model.RealmForwardRule{Enabled: true, ListenPort: 20001, TargetAgentID: "hk", TargetAddress: "hk.example.com", TargetPort: 20001}
			hkRule := model.RealmForwardRule{Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetAddress: "dmit.example.com", TargetPort: 443}
			agents := map[string]model.DashboardAgentView{
				"gz":   {AgentID: "gz", AgentName: "Guangzhou"},
				"hk":   {AgentID: "hk", AgentName: "Hong Kong", Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{hkRule}}}},
				"dmit": {AgentID: "dmit", AgentName: "DMIT"},
			}
			overviews := map[string]*model.XUIOverview{
				"dmit": {AgentID: "dmit", Nodes: []model.XUINodeView{{ID: 7, Tag: "dmit-in", Protocol: protocol, Port: 443}}},
			}

			resolved := resolveRealmForwardTarget("gz", gzRule, agents, overviews)
			if !resolved.Resolved || resolved.FinalAgentID != "dmit" || resolved.FinalNode.Protocol != protocol || len(resolved.Hops) != 2 {
				t.Fatalf("unexpected multi-hop resolution: %#v", resolved)
			}
		})
	}
}

func TestResolveRealmForwardTargetDetectsLoop(t *testing.T) {
	gzRule := model.RealmForwardRule{Enabled: true, ListenPort: 20001, TargetAgentID: "hk", TargetPort: 20001}
	hkRule := model.RealmForwardRule{Enabled: true, ListenPort: 20001, TargetAgentID: "gz", TargetPort: 20001}
	agents := map[string]model.DashboardAgentView{
		"gz": {AgentID: "gz", Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{gzRule}}}},
		"hk": {AgentID: "hk", Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{hkRule}}}},
	}
	resolved := resolveRealmForwardTarget("gz", gzRule, agents, nil)
	if resolved.Resolved || !resolved.LoopDetected || resolved.UnresolvedReason == "" {
		t.Fatalf("expected Realm loop detection, got %#v", resolved)
	}
}

func TestCustomerRealmPublicEntryUsesOutermostHop(t *testing.T) {
	assignment := model.CustomerAssignment{AgentID: "dmit", InboundID: 7, InboundTag: "dmit-in", ClientEmail: "alice@example.com"}
	chain := model.ClientChainView{RootAgentID: "dmit", Steps: []model.ClientChainStep{{StepType: "inbound", AgentID: "dmit", Port: 443}}}
	realmAgent := func(agentID, domain string, rule model.RealmForwardRule) model.DashboardAgentView {
		return model.DashboardAgentView{
			AgentID: agentID,
			Entry: model.AgentEntryConfig{
				ImportDomain:   domain,
				PortForwarding: model.RealmForwardConfig{Rules: []model.RealmForwardRule{rule}},
			},
		}
	}
	agents := map[string]model.DashboardAgentView{
		"gz": realmAgent("gz", "gz.example.com", model.RealmForwardRule{
			Enabled: true, ListenPort: 20001, TargetAgentID: "hk", TargetPort: 20001,
		}),
		"hk": realmAgent("hk", "hk.example.com", model.RealmForwardRule{
			Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetPort: 443,
		}),
		"dmit": {AgentID: "dmit", Entry: model.AgentEntryConfig{ImportDomain: "dmit.example.com"}},
	}

	entry, ok := customerRealmPublicEntry(assignment, chain, agents)
	if !ok || entry.Host != "gz.example.com" || entry.Port != 20001 {
		t.Fatalf("expected outermost Guangzhou Realm entry, got %#v, ok=%v", entry, ok)
	}
	raw := "vless://11111111-1111-1111-1111-111111111111@dmit.example.com:443?security=reality&sni=example.com#DMIT"
	rewritten := rewriteCustomerImportURL(raw, entry.Host, entry.Port)
	parsed, err := url.Parse(rewritten)
	if err != nil || parsed.Host != "gz.example.com:20001" || parsed.Query().Get("sni") != "example.com" {
		t.Fatalf("unexpected outer-entry URL rewrite: %q (%v)", rewritten, err)
	}
}

func TestAreaManagerRealmPathIncludesIntermediateAgentOnlyForAuthorizedFinalNode(t *testing.T) {
	final := model.TopologyInboundRef{AgentID: "dmit", InboundID: 7, InboundTag: "dmit-in", Protocol: "vless", Port: 443}
	hkRealm := model.TopologyInboundRef{AgentID: "hk", InboundID: 20001, InboundTag: "realm:hk", Protocol: "realm", Port: 20001}
	links := []model.TopologyLinkView{
		{Key: "gz::realm:gz", Source: model.TopologyOutboundRef{AgentID: "gz", OutboundTag: "realm:gz", Protocol: "realm", ListenPort: 20001}, Target: hkRealm, FinalTarget: &final, RealmHops: []model.TopologyInboundRef{hkRealm}},
		{Key: "hk::realm:hk", Source: model.TopologyOutboundRef{AgentID: "hk", OutboundTag: "realm:hk", Protocol: "realm", ListenPort: 20001}, Target: final, FinalTarget: &final},
	}
	scope := areaManagerClientScope{
		exactClients: map[string]struct{}{},
		inbounds:     map[string]struct{}{areaClientInboundKey("dmit", 7, "dmit-in"): {}},
		realmPorts:   map[string]struct{}{areaRealmPortKey("gz", 20001): {}},
		agents:       map[string]struct{}{"gz": {}, "dmit": {}},
	}
	visible := areaManagerRealmPathLinkKeys(links, scope)
	if len(visible) != 2 {
		t.Fatalf("expected both Realm hops to be visible, got %#v", visible)
	}
	allowed := cloneAgentSet(scope.agents)
	expandAreaManagerRealmPathAgents(allowed, links, scope)
	if _, ok := allowed["hk"]; !ok {
		t.Fatalf("expected intermediate HK agent to be included in topology visibility, got %#v", allowed)
	}

	delete(scope.inbounds, areaClientInboundKey("dmit", 7, "dmit-in"))
	if got := areaManagerRealmPathLinkKeys(links, scope); len(got) != 0 {
		t.Fatalf("Realm authorization must not reveal an unauthorized final node, got %#v", got)
	}
}
