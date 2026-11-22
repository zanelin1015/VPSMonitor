package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/realmconfig"
)

const realmTrafficTable = "vpsmonitor_realm_traffic"
const realmTrafficMarker = "/var/lib/vpsmonitor/realm_traffic_installed"

var realmTrafficMu sync.Mutex

type realmNFTState struct {
	Exists   bool
	Counters map[string]uint64
	Chains   map[string]bool
	Comments map[string]bool
}

func collectRealmTraffic(ctx context.Context, cfg model.RealmForwardConfig, realm *model.RealmSnapshot) *model.RealmTrafficSnapshot {
	cfg.TrafficEnabled = cfg.TrafficEnabled && cfg.Enabled && !strings.EqualFold(cfg.Backend, "none")
	if !cfg.TrafficEnabled {
		if _, err := os.Stat(realmTrafficMarker); os.IsNotExist(err) {
			return nil
		}
	}
	realmTrafficMu.Lock()
	defer realmTrafficMu.Unlock()
	bootID, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return collectRealmTrafficWith(ctx, cfg, realm, osCommandRunner{}, runtime.GOOS, isOpenWrtLike(), strings.TrimSpace(string(bootID)), realmTrafficMarker)
}

func collectRealmTrafficWith(ctx context.Context, cfg model.RealmForwardConfig, realm *model.RealmSnapshot, runner commandRunner, goos string, openwrt bool, bootID, marker string) *model.RealmTrafficSnapshot {
	cfg.TrafficEnabled = cfg.TrafficEnabled && cfg.Enabled && !strings.EqualFold(cfg.Backend, "none")
	result := &model.RealmTrafficSnapshot{Enabled: cfg.TrafficEnabled, Backend: "nftables", CollectedAt: time.Now().UTC()}
	fail := func(err error) *model.RealmTrafficSnapshot { result.Error = err.Error(); return result }
	if goos != "linux" || openwrt {
		return fail(fmt.Errorf("Realm 端口计量目前支持原生 Linux；OpenWrt/iStoreOS 和容器端口映射需要在宿主机采集"))
	}
	if _, err := runner.LookPath("nft"); err != nil {
		return fail(fmt.Errorf("Realm 流量计量需要 nftables（nft 命令）及 root/CAP_NET_ADMIN 权限"))
	}
	state, err := readRealmNFTState(ctx, runner)
	if err != nil {
		return fail(err)
	}
	if !cfg.TrafficEnabled || realm == nil || !realm.Running || realm.Error != "" {
		if state.Exists {
			if body, err := runner.Run(ctx, "nft", "delete", "table", "inet", realmTrafficTable); err != nil {
				return fail(fmt.Errorf("清理 Realm 计量规则失败: %s: %w", strings.TrimSpace(string(body)), err))
			}
		}
		_ = os.Remove(marker)
		if cfg.TrafficEnabled {
			return fail(fmt.Errorf("Realm 尚未运行或配置采集失败，暂未计量"))
		}
		return result
	}
	if bootID == "" {
		return fail(fmt.Errorf("无法读取 Linux boot_id，不能安全识别计数器重置"))
	}
	rules := observedRealmTrafficRules(cfg, realm)
	if len(rules) == 0 {
		return fail(fmt.Errorf("没有可计量的 Realm 监听规则"))
	}
	counters, script, err := planRealmNFTCounters(rules, state, bootID)
	if err != nil {
		return fail(err)
	}
	// This marker lets a restarted Client remove only its own accounting
	// table when the feature is turned off. No system rules are flushed.
	if _, err := os.Stat(marker); err != nil {
		if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
			return fail(fmt.Errorf("create Realm traffic state directory: %w", err))
		}
		if err := os.WriteFile(marker, []byte("nftables\n"), 0600); err != nil {
			return fail(fmt.Errorf("save Realm traffic marker: %w", err))
		}
	}
	if script != "" {
		file, err := os.CreateTemp("", "vpsmonitor-realm-*.nft")
		if err != nil {
			return fail(err)
		}
		path := file.Name()
		defer os.Remove(path)
		_, writeErr := file.WriteString(script)
		closeErr := file.Close()
		if writeErr != nil {
			return fail(writeErr)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
		if body, err := runner.Run(ctx, "nft", "-f", path); err != nil {
			return fail(fmt.Errorf("安装 Realm 计量规则失败: %s: %w", strings.TrimSpace(string(body)), err))
		}
	}
	result.Counters = counters
	return result
}

func observedRealmTrafficRules(cfg model.RealmForwardConfig, realm *model.RealmSnapshot) []model.RealmForwardRule {
	rules := make([]model.RealmForwardRule, 0)
	for _, observed := range realm.Rules {
		if !observed.Enabled {
			continue
		}
		// Merge each observed listener separately. The general config merge
		// deduplicates IP:port, but distinct TCP/UDP endpoints must both count.
		matching := cfg
		matching.Rules = nil
		var fallback []model.RealmForwardRule
		for _, configured := range cfg.Rules {
			if realmTrafficListenKey(configured) == realmTrafficListenKey(observed) {
				fallback = append(fallback, configured)
				if realmTrafficNetwork(configured.Network) == realmTrafficNetwork(observed.Network) {
					matching.Rules = append(matching.Rules, configured)
				}
			}
		}
		if len(matching.Rules) == 0 && len(fallback) == 1 {
			matching.Rules = fallback
		}
		merged := realmconfig.MergeSnapshotIntoForwardConfig(matching, &model.RealmSnapshot{Rules: []model.RealmForwardRule{observed}})
		if len(merged.Rules) == 0 {
			continue
		}
		rule := merged.Rules[0]
		rule.Network = realmTrafficNetwork(observed.Network)
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool {
		return realmTrafficListenKey(rules[i])+rules[i].Network < realmTrafficListenKey(rules[j])+rules[j].Network
	})
	return rules
}

func realmTrafficListenKey(rule model.RealmForwardRule) string {
	return firstNonEmpty(strings.TrimSpace(rule.ListenAddress), "0.0.0.0") + ":" + strconv.Itoa(rule.ListenPort)
}

func readRealmNFTState(ctx context.Context, runner commandRunner) (realmNFTState, error) {
	state := realmNFTState{Counters: map[string]uint64{}, Chains: map[string]bool{}, Comments: map[string]bool{}}
	body, err := runner.Run(ctx, "nft", "-j", "list", "table", "inet", realmTrafficTable)
	if err != nil {
		// A permission or kernel error is not equivalent to a missing table.
		tables, listErr := runner.Run(ctx, "nft", "-j", "list", "tables")
		if listErr != nil {
			return state, fmt.Errorf("读取 nftables 失败，请检查权限及内核支持: %s: %w", strings.TrimSpace(string(tables)), listErr)
		}
		var listing struct {
			NFTables []struct {
				Table *struct{ Family, Name string }
			} `json:"nftables"`
		}
		if err := json.Unmarshal(tables, &listing); err != nil {
			return state, fmt.Errorf("decode nftables tables: %w", err)
		}
		for _, item := range listing.NFTables {
			if item.Table != nil && item.Table.Family == "inet" && item.Table.Name == realmTrafficTable {
				return state, fmt.Errorf("read Realm traffic table: %w", err)
			}
		}
		return state, nil
	}
	var document struct {
		NFTables []struct {
			Counter *struct {
				Name  string
				Bytes uint64
			} `json:"counter"`
			Chain *struct{ Name string }    `json:"chain"`
			Rule  *struct{ Comment string } `json:"rule"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return state, fmt.Errorf("decode Realm counters: %w", err)
	}
	state.Exists = true
	for _, item := range document.NFTables {
		if item.Counter != nil {
			state.Counters[item.Counter.Name] = item.Counter.Bytes
		}
		if item.Chain != nil {
			state.Chains[item.Chain.Name] = true
		}
		if item.Rule != nil {
			state.Comments[item.Rule.Comment] = true
		}
	}
	return state, nil
}

// The generation is embedded in the kernel counter names, so Client restarts
// preserve counters without a second state file. A new boot or recreated table
// gets a new epoch even when its byte count already exceeds the last sample.
func planRealmNFTCounters(rules []model.RealmForwardRule, state realmNFTState, bootID string) ([]model.RealmTrafficCounter, string, error) {
	if err := validateRealmTrafficListeners(rules); err != nil {
		return nil, "", err
	}
	bootHash := sha256.Sum256([]byte(bootID))
	boot := hex.EncodeToString(bootHash[:8])
	var additions, expressions []string
	needsApply := !state.Exists || !state.Chains["ingress"] || !state.Chains["egress"]
	result := make([]model.RealmTrafficCounter, 0, len(rules))
	for _, rule := range rules {
		address := firstNonEmpty(strings.TrimSpace(rule.ListenAddress), "0.0.0.0")
		ip := net.ParseIP(address)
		if ip == nil || rule.ListenPort <= 0 || rule.ListenPort > 65535 {
			return nil, "", fmt.Errorf("Realm 计量需要有效的监听 IP 与端口")
		}
		network := strings.ToLower(rule.Network)
		if network != "tcp" && network != "udp" && network != "both" && network != "tcp+udp" {
			return nil, "", fmt.Errorf("unsupported Realm accounting network %q", network)
		}
		digest := sha256.Sum256([]byte(rule.ID + "\x00" + address + "\x00" + strconv.Itoa(rule.ListenPort) + "\x00" + network))
		key := hex.EncodeToString(digest[:12])
		prefix := "u_" + boot + "_" + key + "_"
		upName, downName := "", ""
		for name := range state.Counters {
			if strings.HasPrefix(name, prefix) {
				candidate := "d_" + strings.TrimPrefix(name, "u_")
				if _, ok := state.Counters[candidate]; ok {
					if upName == "" || name > upName {
						upName, downName = name, candidate
					}
				}
			}
		}
		if upName == "" {
			var nonce [8]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				return nil, "", err
			}
			upName = prefix + hex.EncodeToString(nonce[:])
			downName = "d_" + strings.TrimPrefix(upName, "u_")
			additions = append(additions, "add counter inet "+realmTrafficTable+" "+upName, "add counter inet "+realmTrafficTable+" "+downName)
			needsApply = true
		}
		protocols := []string{network}
		if network == "both" || network == "tcp+udp" {
			protocols = []string{"tcp", "udp"}
		}
		for _, protocol := range protocols {
			for _, direction := range []string{"up", "down"} {
				chain, portField, ipField, counterName := "ingress", "dport", "daddr", upName
				if direction == "down" {
					chain, portField, ipField, counterName = "egress", "sport", "saddr", downName
				}
				match := ""
				if ip.To4() != nil {
					match = "meta nfproto ipv4 "
					if !ip.IsUnspecified() {
						match = "ip " + ipField + " " + ip.String() + " "
					}
				} else if !ip.IsUnspecified() {
					match = "ip6 " + ipField + " " + ip.String() + " "
				}
				comment := "vmr_" + key + "_" + protocol + "_" + direction
				if !state.Comments[comment] {
					needsApply = true
				}
				expressions = append(expressions, fmt.Sprintf("add rule inet %s %s %s%s %s %d counter name %s comment %s", realmTrafficTable, chain, match, protocol, portField, rule.ListenPort, counterName, strconv.Quote(comment)))
			}
		}
		result = append(result, model.RealmTrafficCounter{Key: key, RuleID: rule.ID, Name: rule.Name, ListenAddress: address, ListenPort: rule.ListenPort,
			Network: network, Epoch: upName, UploadBytes: state.Counters[upName], DownloadBytes: state.Counters[downName]})
	}
	// Detect removed rules, including their obsolete accounting expressions.
	if len(state.Comments) != len(expressions) {
		needsApply = true
	}
	if !needsApply {
		return result, "", nil
	}
	var lines []string
	if !state.Exists {
		lines = append(lines, "add table inet "+realmTrafficTable)
	}
	for _, chain := range []string{"ingress", "egress"} {
		if state.Chains[chain] {
			lines = append(lines, "flush chain inet "+realmTrafficTable+" "+chain)
		} else {
			hook := "input"
			if chain == "egress" {
				hook = "output"
			}
			lines = append(lines, fmt.Sprintf("add chain inet %s %s { type filter hook %s priority -150; policy accept; }", realmTrafficTable, chain, hook))
		}
	}
	lines = append(lines, additions...)
	lines = append(lines, expressions...)
	return result, strings.Join(lines, "\n") + "\n", nil
}

func validateRealmTrafficListeners(rules []model.RealmForwardRule) error {
	for i, rule := range rules {
		ip := net.ParseIP(firstNonEmpty(strings.TrimSpace(rule.ListenAddress), "0.0.0.0"))
		if ip == nil {
			return fmt.Errorf("Realm 计量需要有效的监听 IP")
		}
		for _, previous := range rules[:i] {
			if rule.ListenPort != previous.ListenPort {
				continue
			}
			a, b := realmTrafficNetwork(rule.Network), realmTrafficNetwork(previous.Network)
			if a != b && a != "both" && b != "both" {
				continue
			}
			other := net.ParseIP(firstNonEmpty(strings.TrimSpace(previous.ListenAddress), "0.0.0.0"))
			if other == nil {
				return fmt.Errorf("Realm 计量需要有效的监听 IP")
			}
			// An unspecified IPv6 socket may also receive mapped IPv4 traffic.
			overlaps := ip.Equal(other) || ip.IsUnspecified() && ip.To4() == nil || other.IsUnspecified() && other.To4() == nil || (ip.To4() != nil && other.To4() != nil && (ip.IsUnspecified() || other.IsUnspecified()))
			if overlaps {
				return fmt.Errorf("监听端口 %d 存在地址或协议重叠的规则，无法独立计量，请使用不同的监听地址或端口", rule.ListenPort)
			}
		}
	}
	return nil
}

func realmTrafficNetwork(network string) string {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "udp":
		return "udp"
	case "both", "tcp+udp", "all":
		return "both"
	default:
		return "tcp"
	}
}
