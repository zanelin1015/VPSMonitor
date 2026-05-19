package client

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

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
		"tc class add dev eth0 parent 1: classid 1:10 htb rate 20mbit ceil 20mbit",
		"tc filter add dev eth0 protocol ip parent 1: prio 10 u32 match ip protocol 6 0xff match ip sport 8443 0xffff flowid 1:10",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command %q in:\n%s", want, joined)
		}
	}
}
