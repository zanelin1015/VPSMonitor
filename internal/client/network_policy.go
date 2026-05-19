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

func (osCommandRunner) LookPath(file string) (string, error) { return exec.LookPath(file) }

func (osCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func applyNetworkPolicy(ctx context.Context, cfg model.NetworkPolicyConfig, runner commandRunner) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	rules := activeNetworkPolicyRules(cfg.Rules)
	if !cfg.Enabled || len(rules) == 0 {
		_ = clearIPTablesPolicy(ctx, runner)
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
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Port != rules[j].Port {
			return rules[i].Port < rules[j].Port
		}
		return rules[i].Protocol < rules[j].Protocol
	})
	return rules
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
	for _, rule := range rules {
		if len(rule.WhitelistIPs) == 0 {
			continue
		}
		for _, proto := range networkPolicyProtocols(rule.Protocol) {
			port := strconv.Itoa(rule.Port)
			_ = runIgnoreError(ctx, runner, "ufw", "delete", "deny", "proto", proto, "from", "any", "to", "any", "port", port)
			for _, ip := range rule.WhitelistIPs {
				_ = runIgnoreError(ctx, runner, "ufw", "delete", "allow", "proto", proto, "from", ip, "to", "any", "port", port)
				if _, err := runner.Run(ctx, "ufw", "allow", "proto", proto, "from", ip, "to", "any", "port", port, "comment", "VPSMonitor whitelist"); err != nil {
					return fmt.Errorf("ufw allow %s/%s from %s: %w", port, proto, ip, err)
				}
			}
			if _, err := runner.Run(ctx, "ufw", "deny", "proto", proto, "from", "any", "to", "any", "port", port, "comment", "VPSMonitor whitelist default deny"); err != nil {
				return fmt.Errorf("ufw deny %s/%s: %w", port, proto, err)
			}
		}
	}
	return nil
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
	if strings.EqualFold(cfg.RateLimitBackend, "none") || !hasRateLimitRule(rules) {
		return nil
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
	if _, err := runner.Run(ctx, "tc", "qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "999"); err != nil {
		return fmt.Errorf("tc qdisc add dev %s: %w", iface, err)
	}
	if _, err := runner.Run(ctx, "tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:999", "htb", "rate", "10000mbit", "ceil", "10000mbit"); err != nil {
		return fmt.Errorf("tc default class dev %s: %w", iface, err)
	}
	classID := 10
	for _, rule := range rules {
		if rule.RateLimitMbps <= 0 {
			continue
		}
		for _, proto := range networkPolicyProtocols(rule.Protocol) {
			id := strconv.Itoa(classID)
			rate := fmt.Sprintf("%gmbit", rule.RateLimitMbps)
			if _, err := runner.Run(ctx, "tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:"+id, "htb", "rate", rate, "ceil", rate); err != nil {
				return fmt.Errorf("tc class %s/%s: %w", strconv.Itoa(rule.Port), proto, err)
			}
			protocolNumber := "6"
			if proto == "udp" {
				protocolNumber = "17"
			}
			if _, err := runner.Run(ctx, "tc", "filter", "add", "dev", iface, "protocol", "ip", "parent", "1:", "prio", strconv.Itoa(classID), "u32", "match", "ip", "protocol", protocolNumber, "0xff", "match", "ip", "sport", strconv.Itoa(rule.Port), "0xffff", "flowid", "1:"+id); err != nil {
				return fmt.Errorf("tc filter %s/%s: %w", strconv.Itoa(rule.Port), proto, err)
			}
			classID++
		}
	}
	_ = writeNetworkPolicyState(iface)
	return nil
}

func clearTCPolicy(ctx context.Context, runner commandRunner, iface string) error {
	if _, err := runner.LookPath("tc"); err != nil {
		return nil
	}
	stateIface := readNetworkPolicyStateInterface()
	if strings.TrimSpace(stateIface) == "" {
		return nil
	}
	if strings.TrimSpace(iface) != "" && strings.TrimSpace(iface) != stateIface {
		return nil
	}
	iface = stateIface
	_ = runIgnoreError(ctx, runner, "tc", "qdisc", "del", "dev", iface, "root")
	_ = os.Remove(networkPolicyStatePath())
	return nil
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

func networkPolicyStatePath() string {
	return "/var/lib/vpsmonitor/network_policy_state.json"
}

func writeNetworkPolicyState(iface string) error {
	path := networkPolicyStatePath()
	if err := os.MkdirAll(strings.TrimSuffix(path, "/network_policy_state.json"), 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"interface": iface})
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func readNetworkPolicyStateInterface() string {
	body, err := os.ReadFile(networkPolicyStatePath())
	if err != nil {
		return ""
	}
	var state map[string]string
	if err := json.Unmarshal(body, &state); err != nil {
		return ""
	}
	return strings.TrimSpace(state["interface"])
}
