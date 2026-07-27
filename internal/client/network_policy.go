package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const networkPolicyChain = "VPSMONITOR_PORT_GUARD"

func (a *App) applyNetworkPolicyIfNeeded(ctx context.Context, cfg model.NetworkPolicyConfig) {
	signature := networkPolicySignature(cfg)
	if signature == a.networkPolicySignature {
		return
	}
	if isOpenWrtLike() && cfg.Enabled && len(activeNetworkPolicyRules(cfg.Rules)) > 0 {
		log.Printf("apply network policy skipped: OpenWrt/iStoreOS port policies are disabled to protect firewall4 and SQM/Cake configuration")
		a.networkPolicySignature = signature
		return
	}
	policyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := applyNetworkPolicy(policyCtx, cfg, osCommandRunner{}); err != nil {
		log.Printf("apply network policy failed: %v", err)
		return
	}
	a.networkPolicySignature = signature
}

func networkPolicySignature(cfg model.NetworkPolicyConfig) string {
	body, _ := json.Marshal(cfg)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type commandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type osCommandRunner struct{}

func (osCommandRunner) LookPath(file string) (string, error) { return resolveCommandPath(file) }

func (osCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if !strings.Contains(name, "/") {
		if resolved, err := resolveCommandPath(name); err == nil {
			name = resolved
		}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func resolveCommandPath(file string) (string, error) {
	path, err := exec.LookPath(file)
	if err == nil || strings.Contains(file, "/") {
		return path, err
	}
	for _, dir := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin"} {
		candidate := dir + "/" + file
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", err
}

func applyNetworkPolicy(ctx context.Context, cfg model.NetworkPolicyConfig, runner commandRunner) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if isOpenWrtLike() {
		if cfg.Enabled && len(activeNetworkPolicyRules(cfg.Rules)) > 0 {
			return fmt.Errorf("port policies are disabled on OpenWrt/iStoreOS to protect firewall4 and SQM/Cake configuration")
		}
		return nil
	}
	rules := activeNetworkPolicyRules(cfg.Rules)
	if !cfg.Enabled || len(rules) == 0 {
		_ = clearIPTablesPolicy(ctx, runner)
		_ = clearUFWPolicy(ctx, runner)
		_ = clearTCPolicy(ctx, runner, strings.TrimSpace(cfg.Interface))
		return nil
	}
	var errs []string
	if err := applyFirewallPolicy(ctx, cfg, rules, runner); err != nil {
		errs = append(errs, err.Error())
	}
	if err := applyRateLimitPolicy(ctx, cfg, rules, runner); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func activeNetworkPolicyRules(items []model.NetworkPortPolicyRule) []model.NetworkPortPolicyRule {
	rules := make([]model.NetworkPortPolicyRule, 0, len(items))
	for _, rule := range items {
		if !rule.Enabled || rule.Port <= 0 || rule.Port > 65535 {
			continue
		}
		rule.Protocol = networkPolicyProtocol(rule.Protocol)
		rule.WhitelistIPs = validNetworkPolicyIPs(rule.WhitelistIPs)
		if rule.RateLimitMbps <= 0 && len(rule.WhitelistIPs) == 0 {
			continue
		}
		rules = append(rules, rule)
	}
	rules = mergeNetworkPolicyRulesByPort(rules)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Port != rules[j].Port {
			return rules[i].Port < rules[j].Port
		}
		return rules[i].Protocol < rules[j].Protocol
	})
	return rules
}

func mergeNetworkPolicyRulesByPort(items []model.NetworkPortPolicyRule) []model.NetworkPortPolicyRule {
	byPort := make(map[int]model.NetworkPortPolicyRule, len(items))
	order := make([]int, 0, len(items))
	for _, item := range items {
		if item.Port <= 0 || item.Port > 65535 {
			continue
		}
		item.Protocol = networkPolicyProtocol(item.Protocol)
		item.WhitelistIPs = validNetworkPolicyIPs(item.WhitelistIPs)
		if existing, ok := byPort[item.Port]; ok {
			existing.Protocol = mergeNetworkPolicyProtocol(existing.Protocol, item.Protocol)
			existing.WhitelistIPs = validNetworkPolicyIPs(append(existing.WhitelistIPs, item.WhitelistIPs...))
			if existing.RateLimitMbps <= 0 || (item.RateLimitMbps > 0 && item.RateLimitMbps < existing.RateLimitMbps) {
				existing.RateLimitMbps = item.RateLimitMbps
			}
			if existing.Name == "" {
				existing.Name = item.Name
			}
			if existing.ID == "" {
				existing.ID = item.ID
			}
			existing.Enabled = existing.Enabled || item.Enabled
			byPort[item.Port] = existing
			continue
		}
		order = append(order, item.Port)
		byPort[item.Port] = item
	}
	result := make([]model.NetworkPortPolicyRule, 0, len(order))
	for _, port := range order {
		result = append(result, byPort[port])
	}
	return result
}

func mergeNetworkPolicyProtocol(a, b string) string {
	seenTCP := false
	seenUDP := false
	for _, protocol := range []string{a, b} {
		for _, item := range networkPolicyProtocols(protocol) {
			if item == "tcp" {
				seenTCP = true
			}
			if item == "udp" {
				seenUDP = true
			}
		}
	}
	if seenTCP && seenUDP {
		return "both"
	}
	if seenUDP {
		return "udp"
	}
	return "tcp"
}

func networkPolicyProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		return "udp"
	case "both", "all", "tcp+udp":
		return "both"
	default:
		return "tcp"
	}
}

func networkPolicyProtocols(protocol string) []string {
	if networkPolicyProtocol(protocol) == "both" {
		return []string{"tcp", "udp"}
	}
	return []string{networkPolicyProtocol(protocol)}
}

func validNetworkPolicyIPs(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(item); err != nil {
			if net.ParseIP(item) == nil {
				continue
			}
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func applyFirewallPolicy(ctx context.Context, cfg model.NetworkPolicyConfig, rules []model.NetworkPortPolicyRule, runner commandRunner) error {
	if !hasWhitelistRule(rules) || strings.EqualFold(cfg.FirewallBackend, "none") {
		_ = clearIPTablesPolicy(ctx, runner)
		_ = clearUFWPolicy(ctx, runner)
		return nil
	}
	backend := strings.ToLower(strings.TrimSpace(cfg.FirewallBackend))
	autoBackend := backend == "" || backend == "auto"
	if autoBackend {
		if isDebianLike() {
			if _, err := runner.LookPath("ufw"); err == nil {
				backend = "ufw"
			}
		}
		if backend == "" || backend == "auto" {
			if _, err := runner.LookPath("iptables"); err == nil {
				backend = "iptables"
			}
		}
	}
	switch backend {
	case "ufw":
		_ = clearIPTablesPolicy(ctx, runner)
		err := applyUFWPolicy(ctx, rules, runner)
		if err == nil || !autoBackend {
			return err
		}
		if _, lookupErr := runner.LookPath("iptables"); lookupErr == nil {
			if fallbackErr := applyIPTablesPolicy(ctx, rules, runner); fallbackErr == nil {
				return nil
			}
		}
		return err
	case "iptables":
		_ = clearUFWPolicy(ctx, runner)
		return applyIPTablesPolicy(ctx, rules, runner)
	default:
		return fmt.Errorf("no supported firewall backend found")
	}
}

func hasWhitelistRule(rules []model.NetworkPortPolicyRule) bool {
	for _, rule := range rules {
		if len(rule.WhitelistIPs) > 0 {
			return true
		}
	}
	return false
}

func isDebianLike() bool {
	body, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	text := strings.ToLower(string(body))
	return strings.Contains(text, "id=debian") || strings.Contains(text, "id=ubuntu") || strings.Contains(text, "id_like=debian")
}

func applyUFWPolicy(ctx context.Context, rules []model.NetworkPortPolicyRule, runner commandRunner) error {
	status, err := runner.Run(ctx, "ufw", "status")
	if err != nil {
		return fmt.Errorf("ufw status: %w", err)
	}
	if !strings.Contains(strings.ToLower(string(status)), "status: active") {
		return fmt.Errorf("ufw is installed but inactive; enable ufw first or choose iptables backend")
	}
	state := readNetworkPolicyState()
	currentRules := networkPolicyUFWRules(rules)
	cleanupRules := mergeNetworkPolicyRulesByPort(append(append([]model.NetworkPortPolicyRule{}, state.UFWRules...), currentRules...))
	for _, rule := range cleanupRules {
		for _, proto := range networkPolicyProtocols(rule.Protocol) {
			port := strconv.Itoa(rule.Port)
			_ = runIgnoreError(ctx, runner, "ufw", "delete", "deny", "proto", proto, "from", "any", "to", "any", "port", port)
			for _, ip := range rule.WhitelistIPs {
				_ = runIgnoreError(ctx, runner, "ufw", "delete", "allow", "proto", proto, "from", ip, "to", "any", "port", port)
			}
		}
	}
	for _, rule := range currentRules {
		for _, proto := range networkPolicyProtocols(rule.Protocol) {
			port := strconv.Itoa(rule.Port)
			for _, ip := range rule.WhitelistIPs {
				if _, err := runner.Run(ctx, "ufw", "allow", "proto", proto, "from", ip, "to", "any", "port", port, "comment", "VPSMonitor whitelist"); err != nil {
					return fmt.Errorf("ufw allow %s/%s from %s: %w", port, proto, ip, err)
				}
			}
			if _, err := runner.Run(ctx, "ufw", "deny", "proto", proto, "from", "any", "to", "any", "port", port, "comment", "VPSMonitor whitelist default deny"); err != nil {
				return fmt.Errorf("ufw deny %s/%s: %w", port, proto, err)
			}
		}
	}
	state.UFWRules = currentRules
	_ = writeNetworkPolicyState(state)
	return nil
}

func clearUFWPolicy(ctx context.Context, runner commandRunner) error {
	state := readNetworkPolicyState()
	if len(state.UFWRules) == 0 {
		return nil
	}
	if _, err := runner.LookPath("ufw"); err != nil {
		return nil
	}
	status, err := runner.Run(ctx, "ufw", "status")
	if err != nil || !strings.Contains(strings.ToLower(string(status)), "status: active") {
		return nil
	}
	for _, rule := range networkPolicyUFWRules(state.UFWRules) {
		for _, proto := range networkPolicyProtocols(rule.Protocol) {
			port := strconv.Itoa(rule.Port)
			_ = runIgnoreError(ctx, runner, "ufw", "delete", "deny", "proto", proto, "from", "any", "to", "any", "port", port)
			for _, ip := range rule.WhitelistIPs {
				_ = runIgnoreError(ctx, runner, "ufw", "delete", "allow", "proto", proto, "from", ip, "to", "any", "port", port)
			}
		}
	}
	state.UFWRules = nil
	_ = writeNetworkPolicyState(state)
	return nil
}

func networkPolicyUFWRules(rules []model.NetworkPortPolicyRule) []model.NetworkPortPolicyRule {
	filtered := make([]model.NetworkPortPolicyRule, 0, len(rules))
	for _, rule := range rules {
		rule.Protocol = networkPolicyProtocol(rule.Protocol)
		rule.WhitelistIPs = validNetworkPolicyIPs(rule.WhitelistIPs)
		if rule.Port <= 0 || rule.Port > 65535 || len(rule.WhitelistIPs) == 0 {
			continue
		}
		filtered = append(filtered, rule)
	}
	return mergeNetworkPolicyRulesByPort(filtered)
}

func applyIPTablesPolicy(ctx context.Context, rules []model.NetworkPortPolicyRule, runner commandRunner) error {
	if _, err := runner.LookPath("iptables"); err != nil {
		return err
	}
	_ = runIgnoreError(ctx, runner, "iptables", "-N", networkPolicyChain)
	if _, err := runner.Run(ctx, "iptables", "-C", "INPUT", "-j", networkPolicyChain); err != nil {
		if _, err := runner.Run(ctx, "iptables", "-I", "INPUT", "1", "-j", networkPolicyChain); err != nil {
			return fmt.Errorf("insert iptables chain: %w", err)
		}
	}
	if _, err := runner.Run(ctx, "iptables", "-F", networkPolicyChain); err != nil {
		return fmt.Errorf("flush iptables chain: %w", err)
	}
	for _, rule := range rules {
		if len(rule.WhitelistIPs) == 0 {
			continue
		}
		for _, proto := range networkPolicyProtocols(rule.Protocol) {
			port := strconv.Itoa(rule.Port)
			for _, ip := range rule.WhitelistIPs {
				if _, err := runner.Run(ctx, "iptables", "-A", networkPolicyChain, "-p", proto, "--dport", port, "-s", ip, "-j", "ACCEPT"); err != nil {
					return fmt.Errorf("iptables allow %s/%s from %s: %w", port, proto, ip, err)
				}
			}
			if _, err := runner.Run(ctx, "iptables", "-A", networkPolicyChain, "-p", proto, "--dport", port, "-j", "DROP"); err != nil {
				return fmt.Errorf("iptables drop %s/%s: %w", port, proto, err)
			}
		}
	}
	return nil
}

func clearIPTablesPolicy(ctx context.Context, runner commandRunner) error {
	if _, err := runner.LookPath("iptables"); err != nil {
		return nil
	}
	_ = runIgnoreError(ctx, runner, "iptables", "-D", "INPUT", "-j", networkPolicyChain)
	_ = runIgnoreError(ctx, runner, "iptables", "-F", networkPolicyChain)
	_ = runIgnoreError(ctx, runner, "iptables", "-X", networkPolicyChain)
	return nil
}

func applyRateLimitPolicy(ctx context.Context, cfg model.NetworkPolicyConfig, rules []model.NetworkPortPolicyRule, runner commandRunner) error {
	if strings.EqualFold(cfg.RateLimitBackend, "none") {
		return nil
	}
	if !hasRateLimitRule(rules) {
		return clearTCPolicy(ctx, runner, strings.TrimSpace(cfg.Interface))
	}
	if _, err := runner.LookPath("tc"); err != nil {
		return fmt.Errorf("tc not found for port rate limit")
	}
	iface := strings.TrimSpace(cfg.Interface)
	if iface == "" {
		var err error
		iface, err = defaultNetworkInterface(ctx, runner)
		if err != nil {
			return err
		}
	}
	if iface == "" {
		return fmt.Errorf("network interface is required for port rate limit")
	}
	_ = runIgnoreError(ctx, runner, "tc", "qdisc", "del", "dev", iface, "root")
	_ = runIgnoreError(ctx, runner, "tc", "qdisc", "del", "dev", iface, "ingress")
	if _, err := runner.Run(ctx, "tc", "qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "999"); err != nil {
		return fmt.Errorf("tc qdisc add dev %s: %w", iface, err)
	}
	if _, err := runner.Run(ctx, "tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:999", "htb", "rate", "10000mbit", "ceil", "10000mbit"); err != nil {
		return fmt.Errorf("tc default class dev %s: %w", iface, err)
	}
	if _, err := runner.Run(ctx, "tc", "qdisc", "add", "dev", iface, "handle", "ffff:", "ingress"); err != nil {
		return fmt.Errorf("tc ingress qdisc add dev %s: %w", iface, err)
	}
	classID := 10
	for _, rule := range rules {
		if rule.RateLimitMbps <= 0 {
			continue
		}
		id := strconv.Itoa(classID)
		rate := fmt.Sprintf("%gmbit", rule.RateLimitMbps)
		if _, err := runner.Run(ctx, "tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:"+id, "htb", "rate", rate, "ceil", rate, "burst", "15k", "cburst", "15k"); err != nil {
			return fmt.Errorf("tc class %s: %w", strconv.Itoa(rule.Port), err)
		}
		rootArgs := append([]string{"filter", "add", "dev", iface, "protocol", "ip", "parent", "1:", "prio", strconv.Itoa(classID), "u32"}, tcProtocolMatchArgs(rule.Protocol)...)
		rootArgs = append(rootArgs, "match", "ip", "sport", strconv.Itoa(rule.Port), "0xffff", "flowid", "1:"+id)
		if _, err := runner.Run(ctx, "tc", rootArgs...); err != nil {
			return fmt.Errorf("tc root filter %s: %w", strconv.Itoa(rule.Port), err)
		}
		ingressArgs := append([]string{"filter", "add", "dev", iface, "parent", "ffff:", "protocol", "ip", "prio", strconv.Itoa(classID), "u32"}, tcProtocolMatchArgs(rule.Protocol)...)
		ingressArgs = append(ingressArgs, "match", "ip", "dport", strconv.Itoa(rule.Port), "0xffff", "action", "police", "rate", rate, "burst", "200k", "drop", "flowid", ":1")
		if _, err := runner.Run(ctx, "tc", ingressArgs...); err != nil {
			return fmt.Errorf("tc ingress filter %s: %w", strconv.Itoa(rule.Port), err)
		}
		classID++
	}
	state := readNetworkPolicyState()
	state.Interface = iface
	_ = writeNetworkPolicyState(state)
	return nil
}

func tcProtocolMatchArgs(protocol string) []string {
	switch networkPolicyProtocol(protocol) {
	case "tcp":
		return []string{"match", "ip", "protocol", "6", "0xff"}
	case "udp":
		return []string{"match", "ip", "protocol", "17", "0xff"}
	default:
		return nil
	}
}

func clearTCPolicy(ctx context.Context, runner commandRunner, iface string) error {
	if _, err := runner.LookPath("tc"); err != nil {
		return nil
	}
	iface = strings.TrimSpace(iface)
	stateIface := readNetworkPolicyStateInterface()
	if iface == "" {
		iface = strings.TrimSpace(stateIface)
	}
	if iface == "" {
		return nil
	}
	if strings.TrimSpace(stateIface) != "" && iface != strings.TrimSpace(stateIface) {
		return nil
	}
	_ = runIgnoreError(ctx, runner, "tc", "qdisc", "del", "dev", iface, "root")
	_ = runIgnoreError(ctx, runner, "tc", "qdisc", "del", "dev", iface, "ingress")
	state := readNetworkPolicyState()
	state.Interface = ""
	_ = writeNetworkPolicyState(state)
	return nil
}

func collectNetworkPolicySnapshot(ctx context.Context, cfg model.NetworkPolicyConfig) *model.NetworkPolicySnapshot {
	if runtime.GOOS != "linux" || isOpenWrtLike() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return collectNetworkPolicySnapshotWithRunner(ctx, cfg, osCommandRunner{})
}

func collectNetworkPolicySnapshotWithRunner(ctx context.Context, cfg model.NetworkPolicyConfig, runner commandRunner) *model.NetworkPolicySnapshot {
	var snapshot *model.NetworkPolicySnapshot
	appendSnapshot := func(next *model.NetworkPolicySnapshot) {
		if next == nil {
			return
		}
		if snapshot == nil {
			snapshot = &model.NetworkPolicySnapshot{CollectedAt: next.CollectedAt}
		}
		if snapshot.CollectedAt.IsZero() {
			snapshot.CollectedAt = next.CollectedAt
		}
		if snapshot.Interface == "" {
			snapshot.Interface = next.Interface
		}
		if snapshot.FirewallBackend == "" {
			snapshot.FirewallBackend = next.FirewallBackend
		}
		if snapshot.RateLimitBackend == "" {
			snapshot.RateLimitBackend = next.RateLimitBackend
		}
		if next.Error != "" {
			if snapshot.Error != "" {
				snapshot.Error += "; "
			}
			snapshot.Error += next.Error
		}
		snapshot.Rules = append(snapshot.Rules, next.Rules...)
	}
	appendSnapshot(collectTCRateLimitSnapshot(ctx, cfg, runner))
	appendSnapshot(collectUFWWhitelistSnapshot(ctx, runner))
	if snapshot == nil {
		return nil
	}
	snapshot.Rules = mergeNetworkPolicyRulesByPort(snapshot.Rules)
	sort.Slice(snapshot.Rules, func(i, j int) bool {
		if snapshot.Rules[i].Port != snapshot.Rules[j].Port {
			return snapshot.Rules[i].Port < snapshot.Rules[j].Port
		}
		return snapshot.Rules[i].Protocol < snapshot.Rules[j].Protocol
	})
	return snapshot
}

func collectTCRateLimitSnapshot(ctx context.Context, cfg model.NetworkPolicyConfig, runner commandRunner) *model.NetworkPolicySnapshot {
	snapshot := &model.NetworkPolicySnapshot{
		CollectedAt:      time.Now().UTC(),
		RateLimitBackend: "tc",
	}
	if _, err := runner.LookPath("tc"); err != nil {
		return nil
	}
	iface := strings.TrimSpace(cfg.Interface)
	if iface == "" {
		iface = readNetworkPolicyStateInterface()
	}
	if iface == "" {
		detected, err := defaultNetworkInterface(ctx, runner)
		if err != nil {
			snapshot.Error = err.Error()
			return snapshot
		}
		iface = detected
	}
	snapshot.Interface = iface

	qdiscOutput, err := runner.Run(ctx, "tc", "qdisc", "show", "dev", iface)
	if err != nil {
		snapshot.Error = fmt.Sprintf("tc qdisc show dev %s: %v", iface, err)
		return snapshot
	}
	if !strings.Contains(string(qdiscOutput), " htb ") && !strings.Contains(string(qdiscOutput), " htb") {
		return snapshot
	}
	classOutput, err := runner.Run(ctx, "tc", "class", "show", "dev", iface)
	if err != nil {
		snapshot.Error = fmt.Sprintf("tc class show dev %s: %v", iface, err)
		return snapshot
	}
	filterOutput, err := runner.Run(ctx, "tc", "filter", "show", "dev", iface)
	if err != nil {
		snapshot.Error = fmt.Sprintf("tc filter show dev %s: %v", iface, err)
		return snapshot
	}
	snapshot.Rules = parseTCRateLimitRules(string(classOutput), string(filterOutput))
	return snapshot
}

var (
	tcClassRatePattern        = regexp.MustCompile(`class\s+htb\s+1:([0-9a-fA-F]+)\b.*\brate\s+([0-9.]+)\s*([kKmMgG]?bit)`)
	tcFilterFlowIDPattern     = regexp.MustCompile(`\bflowid\s+1:([0-9a-fA-F]+)\b`)
	tcFilterProtocolPattern   = regexp.MustCompile(`match\s+ip\s+protocol\s+([0-9]+)\s+0xff`)
	tcFilterPortPattern       = regexp.MustCompile(`match\s+ip\s+sport\s+([0-9]+)\s+0xffff`)
	tcFilterHexProtocolRegexp = regexp.MustCompile(`match\s+([0-9a-fA-F]{4})0000/00ff0000\s+at\s+8`)
	tcFilterHexPortPattern    = regexp.MustCompile(`match\s+([0-9a-fA-F]{4})0000/ffff0000\s+at\s+20`)
)

func parseTCRateLimitRules(classOutput, filterOutput string) []model.NetworkPortPolicyRule {
	ratesByClass := parseTCClassRates(classOutput)
	type filterRef struct {
		classID  string
		protocol string
		port     int
	}
	refs := make([]filterRef, 0)
	current := filterRef{}
	flush := func() {
		if current.classID != "" && current.port > 0 {
			if current.protocol == "" {
				current.protocol = "both"
			}
			refs = append(refs, current)
		}
		current = filterRef{}
	}
	for _, line := range strings.Split(filterOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "filter ") {
			flush()
		}
		if match := tcFilterFlowIDPattern.FindStringSubmatch(trimmed); len(match) == 2 {
			current.classID = strings.ToLower(match[1])
		}
		if match := tcFilterProtocolPattern.FindStringSubmatch(trimmed); len(match) == 2 {
			current.protocol = protocolNameFromNumber(match[1])
		} else if match := tcFilterHexProtocolRegexp.FindStringSubmatch(trimmed); len(match) == 2 {
			if parsed, err := strconv.ParseInt(match[1], 16, 64); err == nil {
				current.protocol = protocolNameFromNumber(strconv.FormatInt(parsed, 10))
			}
		}
		if match := tcFilterPortPattern.FindStringSubmatch(trimmed); len(match) == 2 {
			current.port = intValueFromString(match[1], 10)
		} else if match := tcFilterHexPortPattern.FindStringSubmatch(trimmed); len(match) == 2 {
			current.port = intValueFromString(match[1], 16)
		}
	}
	flush()

	rules := make([]model.NetworkPortPolicyRule, 0, len(refs))
	for _, ref := range refs {
		if strings.EqualFold(ref.classID, "999") {
			continue
		}
		rate := ratesByClass[strings.ToLower(ref.classID)]
		if rate <= 0 {
			continue
		}
		rules = append(rules, model.NetworkPortPolicyRule{
			ID:            fmt.Sprintf("tc-%d-%s", ref.port, ref.protocol),
			Name:          strconv.Itoa(ref.port),
			Enabled:       true,
			Port:          ref.port,
			Protocol:      ref.protocol,
			RateLimitMbps: rate,
		})
	}
	rules = mergeNetworkPolicyRulesByPort(rules)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Port != rules[j].Port {
			return rules[i].Port < rules[j].Port
		}
		return rules[i].Protocol < rules[j].Protocol
	})
	return rules
}

func collectUFWWhitelistSnapshot(ctx context.Context, runner commandRunner) *model.NetworkPolicySnapshot {
	if _, err := runner.LookPath("ufw"); err != nil {
		return nil
	}
	output, err := runner.Run(ctx, "ufw", "status", "numbered")
	if err != nil {
		return &model.NetworkPolicySnapshot{
			CollectedAt:     time.Now().UTC(),
			FirewallBackend: "ufw",
			Error:           fmt.Sprintf("ufw status numbered: %v", err),
		}
	}
	if !strings.Contains(strings.ToLower(string(output)), "status: active") {
		return nil
	}
	rules := parseUFWWhitelistRules(string(output))
	if len(rules) == 0 {
		return &model.NetworkPolicySnapshot{
			CollectedAt:     time.Now().UTC(),
			FirewallBackend: "ufw",
		}
	}
	return &model.NetworkPolicySnapshot{
		CollectedAt:     time.Now().UTC(),
		FirewallBackend: "ufw",
		Rules:           rules,
	}
}

func parseUFWWhitelistRules(output string) []model.NetworkPortPolicyRule {
	rules := make([]model.NetworkPortPolicyRule, 0)
	for _, line := range strings.Split(output, "\n") {
		to, action, direction, from := parseUFWStatusLine(line)
		if !strings.EqualFold(action, "ALLOW") || !strings.EqualFold(direction, "IN") {
			continue
		}
		port, protocol := parseUFWToPortProtocol(to)
		if port <= 0 {
			continue
		}
		from = strings.TrimSpace(from)
		if from == "" || strings.EqualFold(from, "Anywhere") || strings.EqualFold(from, "Anywhere (v6)") {
			continue
		}
		rules = append(rules, model.NetworkPortPolicyRule{
			ID:           fmt.Sprintf("ufw-%d-%s", port, protocol),
			Name:         strconv.Itoa(port),
			Enabled:      true,
			Port:         port,
			Protocol:     protocol,
			WhitelistIPs: []string{from},
		})
	}
	rules = mergeNetworkPolicyRulesByPort(rules)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Port != rules[j].Port {
			return rules[i].Port < rules[j].Port
		}
		return rules[i].Protocol < rules[j].Protocol
	})
	return rules
}

func parseUFWStatusLine(line string) (to, action, direction, from string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "Status:") || strings.HasPrefix(trimmed, "To ") || strings.HasPrefix(trimmed, "--") {
		return "", "", "", ""
	}
	if strings.HasPrefix(trimmed, "[") {
		if end := strings.Index(trimmed, "]"); end >= 0 {
			trimmed = strings.TrimSpace(trimmed[end+1:])
		}
	}
	fields := strings.Fields(trimmed)
	for index := 0; index+2 < len(fields); index++ {
		if isUFWAction(fields[index]) && (strings.EqualFold(fields[index+1], "IN") || strings.EqualFold(fields[index+1], "OUT")) {
			return strings.Join(fields[:index], " "), fields[index], fields[index+1], strings.Join(fields[index+2:], " ")
		}
	}
	return "", "", "", ""
}

func isUFWAction(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ALLOW", "DENY", "REJECT", "LIMIT":
		return true
	default:
		return false
	}
}

func parseUFWToPortProtocol(value string) (int, string) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "Anywhere") || strings.Contains(value, ":") {
		return 0, ""
	}
	protocol := "tcp"
	if parts := strings.Split(value, "/"); len(parts) == 2 {
		value = strings.TrimSpace(parts[0])
		protocol = networkPolicyProtocol(parts[1])
	} else if len(parts) > 2 {
		return 0, ""
	}
	if strings.Contains(value, ",") {
		return 0, ""
	}
	port := intValueFromString(value, 10)
	if port <= 0 {
		return 0, ""
	}
	return port, protocol
}

func parseTCClassRates(output string) map[string]float64 {
	result := make(map[string]float64)
	for _, line := range strings.Split(output, "\n") {
		match := tcClassRatePattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		classID := strings.ToLower(match[1])
		if classID == "999" {
			continue
		}
		rate, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(match[3]) {
		case "kbit":
			rate = rate / 1000
		case "gbit":
			rate = rate * 1000
		}
		result[classID] = rate
	}
	return result
}

func protocolNameFromNumber(value string) string {
	switch strings.TrimSpace(value) {
	case "17":
		return "udp"
	case "6":
		return "tcp"
	default:
		return ""
	}
}

func intValueFromString(value string, base int) int {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), base, 64)
	if err != nil || parsed <= 0 || parsed > 65535 {
		return 0
	}
	return int(parsed)
}

func hasRateLimitRule(rules []model.NetworkPortPolicyRule) bool {
	for _, rule := range rules {
		if rule.RateLimitMbps > 0 {
			return true
		}
	}
	return false
}

func defaultNetworkInterface(ctx context.Context, runner commandRunner) (string, error) {
	if _, err := runner.LookPath("ip"); err != nil {
		return "", err
	}
	output, err := runner.Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("detect default interface: %w", err)
	}
	fields := strings.Fields(string(output))
	for index, field := range fields {
		if field == "dev" && index+1 < len(fields) {
			return fields[index+1], nil
		}
	}
	return "", fmt.Errorf("default interface not found")
}

func runIgnoreError(ctx context.Context, runner commandRunner, name string, args ...string) error {
	_, err := runner.Run(ctx, name, args...)
	return err
}

type networkPolicyState struct {
	Interface string                        `json:"interface,omitempty"`
	UFWRules  []model.NetworkPortPolicyRule `json:"ufw_rules,omitempty"`
}

var networkPolicyStatePathValue = "/var/lib/vpsmonitor/network_policy_state.json"

func networkPolicyStatePath() string {
	return networkPolicyStatePathValue
}

func writeNetworkPolicyState(state networkPolicyState) error {
	state.Interface = strings.TrimSpace(state.Interface)
	state.UFWRules = networkPolicyUFWRules(state.UFWRules)
	path := networkPolicyStatePath()
	if state.Interface == "" && len(state.UFWRules) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func readNetworkPolicyState() networkPolicyState {
	body, err := os.ReadFile(networkPolicyStatePath())
	if err != nil {
		return networkPolicyState{}
	}
	var state networkPolicyState
	if err := json.Unmarshal(body, &state); err != nil {
		return networkPolicyState{}
	}
	state.Interface = strings.TrimSpace(state.Interface)
	state.UFWRules = networkPolicyUFWRules(state.UFWRules)
	return state
}

func readNetworkPolicyStateInterface() string {
	return readNetworkPolicyState().Interface
}
