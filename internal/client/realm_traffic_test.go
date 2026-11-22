package client

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

// This fake models named kernel objects, rather than resetting counters on
// each configuration application. All command payloads are inspected below.
type realmNFTRunner struct {
	state   realmNFTState
	scripts []string
	calls   []string
	denied  bool
}

func (f *realmNFTRunner) LookPath(name string) (string, error) { return name, nil }
func (f *realmNFTRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if f.denied {
		return []byte("Operation not permitted"), errors.New("denied")
	}
	if name != "nft" {
		return nil, errors.New("unexpected command")
	}
	if reflect.DeepEqual(args, []string{"-j", "list", "tables"}) {
		return []byte(`{"nftables":[]}`), nil
	}
	if len(args) > 0 && args[0] == "delete" {
		f.state = realmNFTState{}
		return nil, nil
	}
	if len(args) > 0 && args[0] == "-f" {
		body, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		f.scripts = append(f.scripts, string(body))
		if f.state.Counters == nil {
			f.state.Counters = map[string]uint64{}
		}
		if f.state.Chains == nil {
			f.state.Chains = map[string]bool{}
		}
		if f.state.Comments == nil {
			f.state.Comments = map[string]bool{}
		}
		for _, line := range strings.Split(string(body), "\n") {
			parts := strings.Fields(line)
			if len(parts) < 4 {
				continue
			}
			switch parts[0] + " " + parts[1] {
			case "add table":
				f.state.Exists = true
			case "add chain":
				f.state.Chains[parts[4]] = true
			case "flush chain":
				f.state.Comments = map[string]bool{}
			case "add counter":
				f.state.Counters[parts[4]] = 0
			case "add rule":
				f.state.Comments[strings.Trim(parts[len(parts)-1], `"`)] = true
			}
		}
		return nil, nil
	}
	if !f.state.Exists {
		return []byte("No such file or directory"), errors.New("missing")
	}
	objects := []any{}
	for name, bytes := range f.state.Counters {
		objects = append(objects, map[string]any{"counter": map[string]any{"name": name, "bytes": bytes}})
	}
	for name := range f.state.Chains {
		objects = append(objects, map[string]any{"chain": map[string]any{"name": name}})
	}
	for comment := range f.state.Comments {
		objects = append(objects, map[string]any{"rule": map[string]any{"comment": comment}})
	}
	return json.Marshal(map[string]any{"nftables": objects})
}

func realmTrafficFixture() (model.RealmForwardConfig, *model.RealmSnapshot) {
	rule := model.RealmForwardRule{ID: "stable-rule", Name: "GZ -> HK", Enabled: true, ListenAddress: "0.0.0.0", ListenPort: 20002, TargetAddress: "hk.example.com", TargetPort: 443, Network: "both"}
	cfg := model.RealmForwardConfig{Enabled: true, Backend: "realm", TrafficEnabled: true, Rules: []model.RealmForwardRule{rule}}
	return cfg, &model.RealmSnapshot{Running: true, Rules: []model.RealmForwardRule{rule}}
}

func TestRealmTrafficCounterLifecycleAndEpochs(t *testing.T) {
	cfg, realm := realmTrafficFixture()
	runner := &realmNFTRunner{}
	marker := filepath.Join(t.TempDir(), "installed")
	collect := func(boot string) *model.RealmTrafficSnapshot {
		t.Helper()
		result := collectRealmTrafficWith(context.Background(), cfg, realm, runner, "linux", false, boot, marker)
		if result.Error != "" || len(result.Counters) != 1 {
			t.Fatalf("collect: %#v", result)
		}
		return result
	}
	first := collect("boot-a")
	if len(runner.scripts) != 1 {
		t.Fatal("expected initial installation")
	}
	for name := range runner.state.Counters {
		if strings.HasPrefix(name, "u_") {
			runner.state.Counters[name] = 1000
		} else {
			runner.state.Counters[name] = 2000
		}
	}
	second := collect("boot-a")
	if second.Counters[0].Epoch != first.Counters[0].Epoch || second.Counters[0].UploadBytes != 1000 || second.Counters[0].DownloadBytes != 2000 || len(runner.scripts) != 1 {
		t.Fatalf("Client restart/poll must reuse kernel counters: %#v", second)
	}
	// Changing a label does not reset a port's measurement identity.
	cfg.Rules[0].Name = "Renamed"
	if third := collect("boot-a"); third.Counters[0].Key != first.Counters[0].Key || third.Counters[0].Epoch != first.Counters[0].Epoch {
		t.Fatal("rename reset counter")
	}
	// Rebuild missing accounting expressions without losing named counters.
	runner.state.Comments = map[string]bool{}
	if restored := collect("boot-a"); restored.Counters[0].UploadBytes != 1000 || restored.Counters[0].Epoch != first.Counters[0].Epoch {
		t.Fatal("rule repair reset counter")
	}
	rebooted := collect("boot-b")
	if rebooted.Counters[0].Epoch == first.Counters[0].Epoch || rebooted.Counters[0].UploadBytes != 0 {
		t.Fatal("boot must get a fresh counter epoch")
	}
	runner.state = realmNFTState{}
	recreated := collect("boot-b")
	if recreated.Counters[0].Epoch == rebooted.Counters[0].Epoch {
		t.Fatal("table recreation must get a fresh epoch")
	}
	cfg.TrafficEnabled = false
	disabled := collectRealmTrafficWith(context.Background(), cfg, realm, runner, "linux", false, "boot-b", marker)
	if disabled.Error != "" || runner.state.Exists {
		t.Fatal("disable must remove own accounting table")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("disable must remove marker")
	}
}

func TestRealmTrafficRulesCountOnlyListenerSideAndDoNotFilter(t *testing.T) {
	cfg, realm := realmTrafficFixture()
	runner := &realmNFTRunner{}
	result := collectRealmTrafficWith(context.Background(), cfg, realm, runner, "linux", false, "boot", filepath.Join(t.TempDir(), "marker"))
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	script := runner.scripts[0]
	for _, want := range []string{"tcp dport 20002", "udp dport 20002", "tcp sport 20002", "udp sport 20002", "hook input", "hook output"} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %q: %s", want, script)
		}
	}
	for _, forbidden := range []string{"flush ruleset", " drop", " reject", " dport 443", " sport 443", "hook forward", "add rule inet filter"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("unsafe/double counting rule %q: %s", forbidden, script)
		}
	}
	if strings.Count(script, "add counter") != 2 || strings.Count(script, "add rule") != 4 {
		t.Fatal("TCP and UDP should share one pair of byte counters")
	}
	before := realmForwardSignature(cfg)
	cfg.TrafficEnabled = false
	if before != realmForwardSignature(cfg) {
		t.Fatal("accounting switch must not restart Realm")
	}
}

func TestRealmTrafficErrorsDoNotClaimZeroUsageOrTouchOpenWrt(t *testing.T) {
	cfg, realm := realmTrafficFixture()
	for _, target := range []struct {
		os      string
		openwrt bool
	}{{"windows", false}, {"linux", true}} {
		runner := &realmNFTRunner{}
		result := collectRealmTrafficWith(context.Background(), cfg, realm, runner, target.os, target.openwrt, "boot", filepath.Join(t.TempDir(), "marker"))
		if result.Error == "" || len(runner.calls) != 0 || result.Counters != nil {
			t.Fatalf("unsupported host must stay untouched: %#v", result)
		}
	}
	runner := &realmNFTRunner{denied: true}
	result := collectRealmTrafficWith(context.Background(), cfg, realm, runner, "linux", false, "boot", filepath.Join(t.TempDir(), "marker"))
	if result.Error == "" || result.Counters != nil || len(runner.scripts) != 0 {
		t.Fatalf("permission failure must not report zeros: %#v", result)
	}
	if _, script, err := planRealmNFTCounters([]model.RealmForwardRule{{ID: "bad", ListenAddress: "0.0.0.0; flush ruleset", ListenPort: 80, Network: "tcp"}}, realmNFTState{}, "boot"); err == nil || script != "" {
		t.Fatal("unvalidated address entered nft script")
	}
}

func TestRealmTrafficObservedProtocolsIPv6AndOverlappingListeners(t *testing.T) {
	cfg, realm := realmTrafficFixture()
	realm.Rules[0].Network = "tcp"
	rules := observedRealmTrafficRules(cfg, realm)
	if len(rules) != 1 || rules[0].Network != "tcp" {
		t.Fatal("accounting must use actual TCP-only protocol, not configured defaults")
	}
	_, script, err := planRealmNFTCounters(rules, realmNFTState{}, "boot")
	if err != nil || strings.Contains(script, "udp ") {
		t.Fatalf("counted another UDP service on a TCP-only port: %s %v", script, err)
	}
	rules[0].ListenAddress = "2001:db8::1"
	_, script, err = planRealmNFTCounters(rules, realmNFTState{}, "boot")
	if err != nil || !strings.Contains(script, "ip6 daddr 2001:db8::1") || !strings.Contains(script, "ip6 saddr 2001:db8::1") {
		t.Fatalf("IPv6 listener accounting: %s %v", script, err)
	}
	rules[0].ListenAddress = "0.0.0.0"
	other := rules[0]
	other.ID, other.ListenAddress = "other", "192.0.2.1"
	if _, script, err := planRealmNFTCounters(append(rules, other), realmNFTState{}, "boot"); err == nil || script != "" {
		t.Fatal("overlapping listener rules must not double count")
	}
	other.ListenAddress = "::"
	if _, _, err := planRealmNFTCounters(append(rules, other), realmNFTState{}, "boot"); err == nil {
		t.Fatal("dual stack wildcard overlap must not double count")
	}
	other.ListenAddress, other.Network = "0.0.0.0", "udp"
	if _, _, err := planRealmNFTCounters(append(rules, other), realmNFTState{}, "boot"); err != nil {
		t.Fatal("disjoint TCP and UDP listeners should be measurable independently", err)
	}
}
