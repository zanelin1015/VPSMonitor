package server

import (
	"net/url"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestHydrateHAProxyTargetsFromRealmListeners(t *testing.T) {
	cfg := model.ManagedAgentConfig{
		AgentID: "gz-a",
		Entry: model.AgentEntryConfig{HAProxy: model.HAProxyConfig{
			Enabled: true,
			Rules: []model.HAProxyRule{{
				Enabled:    true,
				ListenPort: 20001,
				Primary:    model.HAProxyRealmTarget{AgentID: "hk-b", RealmRuleID: "b-20001"},
				Backups:    []model.HAProxyRealmTarget{{AgentID: "hk-c", Port: 20001}},
			}},
		}},
	}
	agents := []model.AgentRecord{
		haProxyTargetAgent("hk-b", "HK B", "b.example.com", "b-20001", 20001),
		haProxyTargetAgent("hk-c", "HK C", "c.example.com", "c-20001", 20001),
	}
	hydrated, err := hydrateHAProxyTargetsFromAgents(cfg, agents)
	if err != nil {
		t.Fatalf("hydrateHAProxyTargetsFromAgents: %v", err)
	}
	rule := hydrated.Entry.HAProxy.Rules[0]
	if rule.Primary.Address != "b.example.com" || rule.Primary.Port != 20001 {
		t.Fatalf("unexpected primary target: %#v", rule.Primary)
	}
	if rule.Backups[0].Address != "c.example.com" || rule.Backups[0].RealmRuleID != "c-20001" {
		t.Fatalf("unexpected backup target: %#v", rule.Backups[0])
	}
	if rule.CheckIntervalSeconds != 3 || rule.ConnectTimeoutSeconds != 5 || rule.Fall != 3 || rule.Rise != 2 {
		t.Fatalf("expected HAProxy defaults to be hydrated: %#v", rule)
	}
	if err := validateHAProxyConfig(hydrated.Entry.HAProxy, hydrated.Entry.PortForwarding); err != nil {
		t.Fatalf("validateHAProxyConfig: %v", err)
	}
}

func TestHydrateHAProxyTargetsRejectsMissingRealmRule(t *testing.T) {
	cfg := model.ManagedAgentConfig{
		AgentID: "gz-a",
		Entry: model.AgentEntryConfig{HAProxy: model.HAProxyConfig{Rules: []model.HAProxyRule{{
			Enabled: true, ListenPort: 20001,
			Primary: model.HAProxyRealmTarget{AgentID: "hk-b", RealmRuleID: "missing"},
		}}}},
	}
	_, err := hydrateHAProxyTargetsFromAgents(cfg, []model.AgentRecord{haProxyTargetAgent("hk-b", "HK B", "b.example.com", "b-20001", 20001)})
	if err == nil || !strings.Contains(err.Error(), "找不到对应的 Realm") {
		t.Fatalf("expected missing Realm rule error, got %v", err)
	}
}

func TestValidateHAProxyConfigRejectsSimultaneousRealm(t *testing.T) {
	cfg := model.HAProxyConfig{Enabled: true, Rules: []model.HAProxyRule{{
		Enabled: true, ListenAddress: "0.0.0.0", ListenPort: 20001,
		Primary:              model.HAProxyRealmTarget{AgentID: "hk-b", RealmRuleID: "b", Address: "b.example.com", Port: 20001},
		Backups:              []model.HAProxyRealmTarget{{AgentID: "hk-c", RealmRuleID: "c", Address: "c.example.com", Port: 20001}},
		CheckIntervalSeconds: 3, ConnectTimeoutSeconds: 5, Fall: 3, Rise: 2,
	}}}
	realm := model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{Enabled: true, ListenPort: 20001}}}
	if err := validateHAProxyConfig(cfg, realm); err == nil || !strings.Contains(err.Error(), "只能启用一个") {
		t.Fatalf("expected Realm and HAProxy mutual-exclusion error, got %v", err)
	}
}

func TestValidateForwardingFeatureSelectionRejectsRealmAndHAProxy(t *testing.T) {
	err := validateForwardingFeatureSelection(model.AgentFeatureConfig{Realm: true, HAProxy: true})
	if err == nil || !strings.Contains(err.Error(), "只能选择一个") {
		t.Fatalf("expected mutually exclusive feature selection, got %v", err)
	}
	for _, features := range []model.AgentFeatureConfig{{Realm: true}, {HAProxy: true}, {}} {
		if err := validateForwardingFeatureSelection(features); err != nil {
			t.Fatalf("single forwarding mode should be valid: features=%#v err=%v", features, err)
		}
	}
}

func TestResolveHAProxyRulePathsRequiresSameFinalXUINode(t *testing.T) {
	makeRealmAgent := func(agentID, ruleID string, targetPort int) model.DashboardAgentView {
		return model.DashboardAgentView{
			AgentID: agentID,
			Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
				ID: ruleID, Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetPort: targetPort, Network: "tcp",
			}}}},
		}
	}
	context := forwardedOverviewContext{
		agentMap: map[string]model.DashboardAgentView{
			"hk-b": makeRealmAgent("hk-b", "b-20001", 443),
			"hk-c": makeRealmAgent("hk-c", "c-20001", 443),
			"dmit": {AgentID: "dmit"},
		},
		targetOverviewByAgent: map[string]*model.XUIOverview{
			"dmit": {AgentID: "dmit", Nodes: []model.XUINodeView{{ID: 7, Tag: "dmit-in", Protocol: "vless", Port: 443}}},
		},
	}
	rule := model.HAProxyRule{
		Enabled: true, ListenPort: 10001,
		Primary: model.HAProxyRealmTarget{AgentID: "hk-b", RealmRuleID: "b-20001", Port: 20001},
		Backups: []model.HAProxyRealmTarget{{AgentID: "hk-c", RealmRuleID: "c-20001", Port: 20001}},
	}
	paths, err := resolveHAProxyRulePaths(rule, context)
	if err != nil || len(paths) != 2 {
		t.Fatalf("expected both paths to resolve: paths=%#v err=%v", paths, err)
	}
	if err := validateHAProxyResolvedPaths(rule, paths); err != nil {
		t.Fatalf("same final x-ui node should pass validation: %v", err)
	}

	context.agentMap["hk-c"] = makeRealmAgent("hk-c", "c-20001", 8443)
	context.targetOverviewByAgent["dmit"].Nodes = append(context.targetOverviewByAgent["dmit"].Nodes,
		model.XUINodeView{ID: 8, Tag: "other-in", Protocol: "vless", Port: 8443})
	paths, err = resolveHAProxyRulePaths(rule, context)
	if err != nil {
		t.Fatalf("expected both different paths to resolve before compatibility check: %v", err)
	}
	if err := validateHAProxyResolvedPaths(rule, paths); err == nil || !strings.Contains(err.Error(), "最终落点不一致") {
		t.Fatalf("expected incompatible backup to be rejected, got %v", err)
	}
}

func TestAppendForwardedImportURLsMapsHAProxyToFinalClient(t *testing.T) {
	rule := model.HAProxyRule{
		ID: "gz-ha", Enabled: true, ListenAddress: "0.0.0.0", ListenPort: 10001,
		Primary: model.HAProxyRealmTarget{AgentID: "hk-b", RealmRuleID: "b-20001", Port: 20001},
		Backups: []model.HAProxyRealmTarget{{AgentID: "hk-c", RealmRuleID: "c-20001", Port: 20001}},
	}
	realmAgent := func(agentID, ruleID string) model.DashboardAgentView {
		return model.DashboardAgentView{AgentID: agentID, AgentName: agentID, Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
			ID: ruleID, Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetPort: 443, Network: "tcp",
		}}}}}
	}
	context := forwardedOverviewContext{
		agentMap: map[string]model.DashboardAgentView{
			"gz": {
				AgentID: "gz", AgentName: "GZ",
				Entry: model.AgentEntryConfig{
					ImportDomain: "gz.example.com",
					PortForwarding: model.RealmForwardConfig{
						Enabled: false, Backend: "none",
						Rules: []model.RealmForwardRule{{ID: "legacy", Enabled: true, ListenPort: 20002, TargetAgentID: "dmit", TargetPort: 443}},
					},
					HAProxy: model.HAProxyConfig{Enabled: true, Rules: []model.HAProxyRule{rule}},
				},
			},
			"hk-b": realmAgent("hk-b", "b-20001"),
			"hk-c": realmAgent("hk-c", "c-20001"),
			"dmit": {AgentID: "dmit", AgentName: "DMIT"},
		},
		targetOverviewByAgent: map[string]*model.XUIOverview{
			"dmit": {
				AgentID: "dmit",
				Nodes:   []model.XUINodeView{{ID: 7, Tag: "dmit-in", Remark: "DMIT VLESS", Protocol: "vless", Port: 443, Enabled: true}},
				Clients: []model.XUIClientView{{
					InboundID: 7, InboundTag: "dmit-in", Protocol: "vless", Email: "alice@example.com", Enabled: true,
					AuthUUID:  "11111111-1111-1111-1111-111111111111",
					ImportURL: "vless://11111111-1111-1111-1111-111111111111@dmit.example.com:443?security=reality&sni=shop.example.com#DMIT",
					Up:        100, Down: 200,
				}},
			},
		},
	}
	overview := &model.XUIOverview{AgentID: "gz"}
	appendForwardedImportURLsWithContext("gz", overview, context)
	if len(overview.Nodes) != 1 || len(overview.Clients) != 1 {
		t.Fatalf("expected one HAProxy-mapped node and client, got nodes=%#v clients=%#v", overview.Nodes, overview.Clients)
	}
	client := overview.Clients[0]
	parsed, err := url.Parse(client.ImportURL)
	if err != nil || parsed.Host != "gz.example.com:10001" {
		t.Fatalf("expected HAProxy source host and port, got %q (%v)", client.ImportURL, err)
	}
	if client.ForwardType != "haproxy" || client.AuthUUID != "11111111-1111-1111-1111-111111111111" || client.RealmTargetAgentID != "dmit" || client.RealmTargetInboundID != 7 {
		t.Fatalf("expected final client identity and HAProxy mapping metadata to remain intact, got %#v", client)
	}
	if parsed.Query().Get("sni") != "shop.example.com" || client.Up != 100 || client.Down != 200 {
		t.Fatalf("expected Reality parameters and traffic to remain intact, got %#v", client)
	}
}

func TestResolveHAProxyRulePathsRejectsDisabledRealmTarget(t *testing.T) {
	rule := model.HAProxyRule{
		Enabled: true, ListenPort: 10001,
		Primary: model.HAProxyRealmTarget{AgentID: "hk-b", RealmRuleID: "b-20001", Port: 20001},
	}
	context := forwardedOverviewContext{agentMap: map[string]model.DashboardAgentView{
		"hk-b": {
			AgentID: "hk-b", AgentName: "HK B",
			Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{
				Enabled: false, Backend: "none",
				Rules: []model.RealmForwardRule{{ID: "b-20001", Enabled: true, ListenPort: 20001, TargetAgentID: "dmit", TargetPort: 443}},
			}},
		},
	}}
	if _, err := resolveHAProxyRulePaths(rule, context); err == nil || !strings.Contains(err.Error(), "未启用 Realm") {
		t.Fatalf("expected disabled Realm target to be rejected, got %v", err)
	}
}

func haProxyTargetAgent(id, name, domain, ruleID string, port int) model.AgentRecord {
	return model.AgentRecord{
		AgentID:   id,
		AgentName: name,
		Config: model.ManagedAgentConfig{Entry: model.AgentEntryConfig{
			ImportDomain: domain,
			PortForwarding: model.RealmForwardConfig{
				Enabled: true,
				Backend: "realm",
				Rules:   []model.RealmForwardRule{{ID: ruleID, Enabled: true, ListenAddress: "0.0.0.0", ListenPort: port, TargetAddress: "d.example.com", TargetPort: port, Network: "both"}},
			},
		}},
	}
}
