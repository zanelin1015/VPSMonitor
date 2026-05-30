package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vpsmonitor-network-policy-*")
	if err == nil {
		networkPolicyStatePathValue = filepath.Join(dir, "network_policy_state.json")
	}
	code := m.Run()
	if err == nil {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

func resetNetworkPolicyState(t *testing.T) {
	t.Helper()
	_ = os.Remove(networkPolicyStatePath())
}

type fakeCommandRunner struct {
	paths    map[string]bool
	outputs  map[string]string
	commands []string
}

func (f *fakeCommandRunner) LookPath(file string) (string, error) {
	if f.paths[file] {
		return "/usr/sbin/" + file, nil
	}
	return "", fmt.Errorf("not found")
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	command := name + " " + strings.Join(args, " ")
	f.commands = append(f.commands, command)
	if output, ok := f.outputs[command]; ok {
		return []byte(output), nil
	}
	return []byte("ok"), nil
}

func TestApplyNetworkPolicyUsesIPTablesWhitelist(t *testing.T) {
	runner := &fakeCommandRunner{paths: map[string]bool{"iptables": true}}
	err := applyIPTablesPolicy(context.Background(), []model.NetworkPortPolicyRule{{
		Enabled:      true,
		Port:         443,
		Protocol:     "tcp",
		WhitelistIPs: []string{"1.2.3.4", "10.0.0.0/24"},
	}}, runner)
	if err != nil {
		t.Fatalf("applyNetworkPolicy: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"iptables -N VPSMONITOR_PORT_GUARD",
		"iptables -A VPSMONITOR_PORT_GUARD -p tcp --dport 443 -s 1.2.3.4 -j ACCEPT",
		"iptables -A VPSMONITOR_PORT_GUARD -p tcp --dport 443 -j DROP",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command %q in:\n%s", want, joined)
		}
	}
}

func TestApplyNetworkPolicyUsesTCForRateLimit(t *testing.T) {
	resetNetworkPolicyState(t)
	runner := &fakeCommandRunner{paths: map[string]bool{"tc": true}}
	err := applyRateLimitPolicy(context.Background(), model.NetworkPolicyConfig{Interface: "eth0", RateLimitBackend: "tc"}, []model.NetworkPortPolicyRule{{
		Enabled:       true,
		Port:          8443,
		Protocol:      "tcp",
		RateLimitMbps: 20,
	}}, runner)
	if err != nil {
		t.Fatalf("applyNetworkPolicy: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"tc qdisc add dev eth0 root handle 1: htb default 999",
		"tc qdisc add dev eth0 handle ffff: ingress",
		"tc class add dev eth0 parent 1: classid 1:10 htb rate 20mbit ceil 20mbit burst 15k cburst 15k",
		"tc filter add dev eth0 protocol ip parent 1: prio 10 u32 match ip protocol 6 0xff match ip sport 8443 0xffff flowid 1:10",
		"tc filter add dev eth0 parent ffff: protocol ip prio 10 u32 match ip protocol 6 0xff match ip dport 8443 0xffff action police rate 20mbit burst 200k drop flowid :1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command %q in:\n%s", want, joined)
		}
	}
}

func TestApplyNetworkPolicyDedupesRateLimitByPort(t *testing.T) {
	resetNetworkPolicyState(t)
	runner := &fakeCommandRunner{paths: map[string]bool{"tc": true}}
	rules := activeNetworkPolicyRules([]model.NetworkPortPolicyRule{
		{Enabled: true, Port: 20002, Protocol: "tcp", RateLimitMbps: 100},
		{Enabled: true, Port: 20002, Protocol: "udp", RateLimitMbps: 100},
		{Enabled: true, Port: 20002, Protocol: "tcp", RateLimitMbps: 100},
	})
	if len(rules) != 1 {
		t.Fatalf("expected one deduped rule, got %#v", rules)
	}
	if rules[0].Protocol != "both" {
		t.Fatalf("expected tcp+udp protocol, got %#v", rules[0])
	}
	err := applyRateLimitPolicy(context.Background(), model.NetworkPolicyConfig{Interface: "eth0", RateLimitBackend: "tc"}, rules, runner)
	if err != nil {
		t.Fatalf("applyNetworkPolicy: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	if got := strings.Count(joined, "match ip sport 20002"); got != 1 {
		t.Fatalf("expected one root filter for port 20002, got %d in:\n%s", got, joined)
	}
	if got := strings.Count(joined, "match ip dport 20002"); got != 1 {
		t.Fatalf("expected one ingress filter for port 20002, got %d in:\n%s", got, joined)
	}
	if strings.Contains(joined, "match ip protocol 6 0xff") || strings.Contains(joined, "match ip protocol 17 0xff") {
		t.Fatalf("expected deduped tcp+udp port rule to avoid duplicate protocol filters, got:\n%s", joined)
	}
}

func TestApplyNetworkPolicyClearsTCWhenRateLimitRemoved(t *testing.T) {
	resetNetworkPolicyState(t)
	runner := &fakeCommandRunner{paths: map[string]bool{"tc": true}}
	err := applyRateLimitPolicy(context.Background(), model.NetworkPolicyConfig{Interface: "eth0", RateLimitBackend: "tc"}, []model.NetworkPortPolicyRule{{
		Enabled:      true,
		Port:         20002,
		Protocol:     "tcp",
		WhitelistIPs: []string{"1.2.3.4"},
	}}, runner)
	if err != nil {
		t.Fatalf("applyNetworkPolicy: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "tc qdisc del dev eth0 root") {
		t.Fatalf("expected tc qdisc to be cleared when rate limit is removed, got:\n%s", joined)
	}
	if !strings.Contains(joined, "tc qdisc del dev eth0 ingress") {
		t.Fatalf("expected tc ingress qdisc to be cleared when rate limit is removed, got:\n%s", joined)
	}
}

func TestApplyUFWPolicyReconcilesPreviousManagedRules(t *testing.T) {
	resetNetworkPolicyState(t)
	if err := writeNetworkPolicyState(networkPolicyState{UFWRules: []model.NetworkPortPolicyRule{{
		Enabled:      true,
		Port:         20010,
		Protocol:     "tcp",
		WhitelistIPs: []string{"1.1.1.1"},
	}}}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	runner := &fakeCommandRunner{
		paths: map[string]bool{"ufw": true},
		outputs: map[string]string{
			"ufw status": "Status: active",
		},
	}
	err := applyUFWPolicy(context.Background(), []model.NetworkPortPolicyRule{{
		Enabled:      true,
		Port:         20010,
		Protocol:     "tcp",
		WhitelistIPs: []string{"2.2.2.2"},
	}}, runner)
	if err != nil {
		t.Fatalf("applyUFWPolicy: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"ufw delete allow proto tcp from 1.1.1.1 to any port 20010",
		"ufw delete allow proto tcp from 2.2.2.2 to any port 20010",
		"ufw allow proto tcp from 2.2.2.2 to any port 20010 comment VPSMonitor whitelist",
		"ufw deny proto tcp from any to any port 20010 comment VPSMonitor whitelist default deny",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command %q in:\n%s", want, joined)
		}
	}
	state := readNetworkPolicyState()
	if len(state.UFWRules) != 1 || state.UFWRules[0].Port != 20010 || state.UFWRules[0].WhitelistIPs[0] != "2.2.2.2" {
		t.Fatalf("unexpected stored ufw state: %#v", state.UFWRules)
	}
}

func TestApplyNetworkPolicyClearsUFWWhenWhitelistRemoved(t *testing.T) {
	resetNetworkPolicyState(t)
	if err := writeNetworkPolicyState(networkPolicyState{UFWRules: []model.NetworkPortPolicyRule{{
		Enabled:      true,
		Port:         20010,
		Protocol:     "tcp",
		WhitelistIPs: []string{"1.1.1.1"},
	}}}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	runner := &fakeCommandRunner{
		paths: map[string]bool{"ufw": true},
		outputs: map[string]string{
			"ufw status": "Status: active",
		},
	}
	err := applyFirewallPolicy(context.Background(), model.NetworkPolicyConfig{FirewallBackend: "ufw"}, []model.NetworkPortPolicyRule{{
		Enabled:       true,
		Port:          20010,
		Protocol:      "tcp",
		RateLimitMbps: 50,
	}}, runner)
	if err != nil {
		t.Fatalf("applyNetworkPolicy: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"ufw delete deny proto tcp from any to any port 20010",
		"ufw delete allow proto tcp from 1.1.1.1 to any port 20010",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command %q in:\n%s", want, joined)
		}
	}
	if state := readNetworkPolicyState(); len(state.UFWRules) != 0 {
		t.Fatalf("expected ufw state to be cleared, got %#v", state.UFWRules)
	}
}

func TestCollectTCRateLimitSnapshotParsesCurrentRules(t *testing.T) {
	runner := &fakeCommandRunner{
		paths: map[string]bool{"tc": true},
		outputs: map[string]string{
			"tc qdisc show dev eth0": "qdisc htb 1: root refcnt 3 r2q 10 default 0x999 direct_packets_stat 0 direct_qlen 1000\n",
			"tc class show dev eth0": strings.Join([]string{
				"class htb 1:10 root prio 0 rate 100Mbit ceil 100Mbit burst 1600b cburst 1600b",
				"class htb 1:11 root prio 0 rate 100Mbit ceil 100Mbit burst 1600b cburst 1600b",
				"class htb 1:12 root prio 0 rate 100Mbit ceil 100Mbit burst 1600b cburst 1600b",
				"class htb 1:999 root prio 0 rate 10Gbit ceil 10Gbit burst 0b cburst 0b",
			}, "\n"),
			"tc filter show dev eth0": strings.Join([]string{
				"filter protocol ip pref 10 u32 chain 0 fh 800::800 order 2048 key ht 800 bkt 0 *flowid 1:10 not_in_hw",
				"  match 00060000/00ff0000 at 8",
				"  match 4e220000/ffff0000 at 20",
				"filter protocol ip pref 11 u32 chain 0 fh 801::800 order 2048 key ht 801 bkt 0 *flowid 1:11 not_in_hw",
				"  match 00110000/00ff0000 at 8",
				"  match 4e220000/ffff0000 at 20",
				"filter protocol ip pref 12 u32 chain 0 fh 802::800 order 2048 key ht 802 bkt 0 *flowid 1:12 not_in_hw",
				"  match ip protocol 6 0xff",
				"  match ip sport 20004 0xffff",
			}, "\n"),
		},
	}

	snapshot := collectTCRateLimitSnapshot(context.Background(), model.NetworkPolicyConfig{Interface: "eth0"}, runner)
	if snapshot == nil {
		t.Fatalf("expected snapshot")
	}
	if snapshot.Interface != "eth0" {
		t.Fatalf("expected eth0 interface, got %q", snapshot.Interface)
	}
	if len(snapshot.Rules) != 2 {
		t.Fatalf("expected 2 deduped tc rules, got %#v", snapshot.Rules)
	}
	if snapshot.Rules[0].Port != 20002 || snapshot.Rules[0].Protocol != "both" || snapshot.Rules[0].RateLimitMbps != 100 {
		t.Fatalf("unexpected first rule: %#v", snapshot.Rules[0])
	}
	if snapshot.Rules[1].Port != 20004 || snapshot.Rules[1].Protocol != "tcp" || snapshot.Rules[1].RateLimitMbps != 100 {
		t.Fatalf("unexpected second rule: %#v", snapshot.Rules[1])
	}
}

func TestCollectTCRateLimitSnapshotParsesProtocolAgnosticPortRule(t *testing.T) {
	rules := parseTCRateLimitRules(
		"class htb 1:10 root prio 0 rate 50Mbit ceil 50Mbit burst 15k cburst 15k",
		strings.Join([]string{
			"filter protocol ip pref 10 u32 chain 0 fh 800::800 order 2048 key ht 800 bkt 0 *flowid 1:10 not_in_hw",
			"  match 4e220000/ffff0000 at 20",
		}, "\n"),
	)
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %#v", rules)
	}
	if rules[0].Port != 20002 || rules[0].Protocol != "both" || rules[0].RateLimitMbps != 50 {
		t.Fatalf("unexpected parsed rule: %#v", rules[0])
	}
}

func TestParseUFWWhitelistRulesParsesAllowRules(t *testing.T) {
	rules := parseUFWWhitelistRules(strings.Join([]string{
		"Status: active",
		"[ 1] 20010/tcp                  ALLOW IN    104.194.70.102",
		"[ 2] 20010/udp                  ALLOW IN    104.194.70.102",
		"[ 3] 20006/tcp                  ALLOW IN    154.17.31.165",
		"[ 4] 20006/udp                  ALLOW IN    154.17.31.165",
		"[ 5] 20006/tcp                  DENY IN     Anywhere",
		"[ 6] 22/tcp                     ALLOW IN    Anywhere",
	}, "\n"))
	if len(rules) != 2 {
		t.Fatalf("expected 2 whitelist rules, got %#v", rules)
	}
	if rules[0].Port != 20006 || rules[0].Protocol != "both" || len(rules[0].WhitelistIPs) != 1 || rules[0].WhitelistIPs[0] != "154.17.31.165" {
		t.Fatalf("unexpected 20006 rule: %#v", rules[0])
	}
	if rules[1].Port != 20010 || rules[1].Protocol != "both" || len(rules[1].WhitelistIPs) != 1 || rules[1].WhitelistIPs[0] != "104.194.70.102" {
		t.Fatalf("unexpected 20010 rule: %#v", rules[1])
	}
}

func TestCollectNetworkPolicySnapshotMergesUFWAndTC(t *testing.T) {
	runner := &fakeCommandRunner{
		paths: map[string]bool{"tc": true, "ufw": true},
		outputs: map[string]string{
			"tc qdisc show dev eth0": "qdisc htb 1: root refcnt 3 r2q 10 default 0x999 direct_packets_stat 0 direct_qlen 1000\n",
			"tc class show dev eth0": "class htb 1:10 root prio 0 rate 50Mbit ceil 50Mbit burst 15Kb cburst 15Kb",
			"tc filter show dev eth0": strings.Join([]string{
				"filter protocol ip pref 10 u32 chain 0 fh 800::800 order 2048 key ht 800 bkt 0 *flowid 1:10 not_in_hw",
				"  match 4e280000/ffff0000 at 20",
			}, "\n"),
			"ufw status numbered": strings.Join([]string{
				"Status: active",
				"[ 1] 20006/tcp                  ALLOW IN    154.17.31.165",
				"[ 2] 20006/udp                  ALLOW IN    154.17.31.165",
			}, "\n"),
		},
	}

	snapshot := collectNetworkPolicySnapshotWithRunner(context.Background(), model.NetworkPolicyConfig{Interface: "eth0"}, runner)
	if snapshot == nil {
		t.Fatalf("expected snapshot")
	}
	if snapshot.FirewallBackend != "ufw" || snapshot.RateLimitBackend != "tc" {
		t.Fatalf("unexpected backends: %#v", snapshot)
	}
	if len(snapshot.Rules) != 2 {
		t.Fatalf("expected merged ufw+tc rules, got %#v", snapshot.Rules)
	}
	if snapshot.Rules[0].Port != 20006 || snapshot.Rules[0].Protocol != "both" || len(snapshot.Rules[0].WhitelistIPs) != 1 {
		t.Fatalf("unexpected ufw rule: %#v", snapshot.Rules[0])
	}
	if snapshot.Rules[1].Port != 20008 || snapshot.Rules[1].RateLimitMbps != 50 {
		t.Fatalf("unexpected tc rule: %#v", snapshot.Rules[1])
	}
}
