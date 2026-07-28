package server

import (
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

func TestValidateHAProxyConfigRejectsRealmListenConflict(t *testing.T) {
	cfg := model.HAProxyConfig{Enabled: true, Rules: []model.HAProxyRule{{
		Enabled: true, ListenAddress: "0.0.0.0", ListenPort: 20001,
		Primary:              model.HAProxyRealmTarget{AgentID: "hk-b", RealmRuleID: "b", Address: "b.example.com", Port: 20001},
		Backups:              []model.HAProxyRealmTarget{{AgentID: "hk-c", RealmRuleID: "c", Address: "c.example.com", Port: 20001}},
		CheckIntervalSeconds: 3, ConnectTimeoutSeconds: 5, Fall: 3, Rise: 2,
	}}}
	realm := model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{Enabled: true, ListenPort: 20001}}}
	if err := validateHAProxyConfig(cfg, realm); err == nil || !strings.Contains(err.Error(), "已被当前 Client 的 Realm 使用") {
		t.Fatalf("expected Realm listen conflict, got %v", err)
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
