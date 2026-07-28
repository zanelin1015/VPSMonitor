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
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const (
	defaultHAProxyConfigPath  = "/etc/vpsmonitor/haproxy.cfg"
	defaultHAProxyServiceName = "vpsmonitor-haproxy"
)

var haProxyRuntimeGOOS = runtime.GOOS

func (a *App) applyHAProxyIfNeeded(ctx context.Context, cfg model.HAProxyConfig) {
	if a.haProxySignature == "" && isEmptyClientHAProxyConfig(cfg) && !managedHAProxyArtifactsExist(cfg) {
		return
	}
	cfg = normalizeClientHAProxyConfig(cfg)
	signature := haProxyConfigSignature(cfg)
	if signature == a.haProxySignature && haProxyConfigFileMatches(cfg) {
		return
	}
	applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := applyHAProxy(applyCtx, cfg, osCommandRunner{}, osHAProxyFileSystem{}); err != nil {
		log.Printf("apply HAProxy failed: %v", err)
		return
	}
	a.haProxySignature = signature
}

func managedHAProxyArtifactsExist(cfg model.HAProxyConfig) bool {
	if haProxyRuntimeGOOS != "linux" {
		return false
	}
	configPath := firstNonEmpty(strings.TrimSpace(cfg.ConfigPath), defaultHAProxyConfigPath)
	serviceName := firstNonEmpty(strings.TrimSpace(cfg.ServiceName), defaultHAProxyServiceName)
	for _, path := range []string{
		configPath,
		filepath.Join("/etc/systemd/system", serviceName+".service"),
		filepath.Join("/etc/init.d", serviceName),
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func isEmptyClientHAProxyConfig(cfg model.HAProxyConfig) bool {
	return !cfg.Enabled && strings.TrimSpace(cfg.BinaryPath) == "" && strings.TrimSpace(cfg.ConfigPath) == "" && strings.TrimSpace(cfg.ServiceName) == "" && len(cfg.Rules) == 0
}

func normalizeClientHAProxyConfig(cfg model.HAProxyConfig) model.HAProxyConfig {
	cfg.BinaryPath = strings.TrimSpace(cfg.BinaryPath)
	cfg.ConfigPath = strings.TrimSpace(cfg.ConfigPath)
	cfg.ServiceName = strings.TrimSpace(cfg.ServiceName)
	for index := range cfg.Rules {
		rule := &cfg.Rules[index]
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.ListenAddress = strings.TrimSpace(rule.ListenAddress)
		if rule.ListenAddress == "" {
			rule.ListenAddress = "0.0.0.0"
		}
		if rule.CheckIntervalSeconds <= 0 {
			rule.CheckIntervalSeconds = 3
		}
		if rule.ConnectTimeoutSeconds <= 0 {
			rule.ConnectTimeoutSeconds = 5
		}
		if rule.Fall <= 0 {
			rule.Fall = 3
		}
		if rule.Rise <= 0 {
			rule.Rise = 2
		}
		rule.Primary = normalizeClientHAProxyTarget(rule.Primary)
		for targetIndex := range rule.Backups {
			rule.Backups[targetIndex] = normalizeClientHAProxyTarget(rule.Backups[targetIndex])
		}
	}
	sort.SliceStable(cfg.Rules, func(i, j int) bool { return cfg.Rules[i].ListenPort < cfg.Rules[j].ListenPort })
	return cfg
}

func normalizeClientHAProxyTarget(target model.HAProxyRealmTarget) model.HAProxyRealmTarget {
	target.AgentID = strings.TrimSpace(target.AgentID)
	target.RealmRuleID = strings.TrimSpace(target.RealmRuleID)
	target.Address = strings.TrimSpace(target.Address)
	return target
}

func activeHAProxyRules(rules []model.HAProxyRule) []model.HAProxyRule {
	active := make([]model.HAProxyRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			active = append(active, rule)
		}
	}
	return active
}

func haProxyConfigSignature(cfg model.HAProxyConfig) string {
	body, _ := json.Marshal(cfg)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func haProxyConfigFileMatches(cfg model.HAProxyConfig) bool {
	if haProxyRuntimeGOOS != "linux" || !cfg.Enabled || len(activeHAProxyRules(cfg.Rules)) == 0 {
		return true
	}
	body, err := os.ReadFile(firstNonEmpty(cfg.ConfigPath, defaultHAProxyConfigPath))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) == strings.TrimSpace(renderHAProxyConfig(cfg))
}

type haProxyFileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(name string, data []byte, perm os.FileMode) error
	Rename(oldPath, newPath string) error
	Remove(name string) error
	Chmod(name string, mode os.FileMode) error
}

type osHAProxyFileSystem struct{}

func (osHAProxyFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}
func (osHAProxyFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (osHAProxyFileSystem) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (osHAProxyFileSystem) Remove(name string) error             { return os.Remove(name) }
func (osHAProxyFileSystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func applyHAProxy(ctx context.Context, cfg model.HAProxyConfig, runner commandRunner, fs haProxyFileSystem) error {
	if haProxyRuntimeGOOS != "linux" {
		return nil
	}
	cfg = normalizeClientHAProxyConfig(cfg)
	serviceName := firstNonEmpty(cfg.ServiceName, defaultHAProxyServiceName)
	rules := activeHAProxyRules(cfg.Rules)
	if !cfg.Enabled || len(rules) == 0 {
		return stopHAProxyService(ctx, serviceName, runner)
	}
	if err := validateClientHAProxyRules(rules); err != nil {
		return err
	}
	binaryPath, err := resolveHAProxyBinary(cfg.BinaryPath, runner)
	if err != nil {
		return err
	}
	configPath := firstNonEmpty(cfg.ConfigPath, defaultHAProxyConfigPath)
	if err := fs.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create HAProxy config dir: %w", err)
	}
	tempPath := configPath + ".tmp"
	defer func() { _ = fs.Remove(tempPath) }()
	if err := fs.WriteFile(tempPath, []byte(renderHAProxyConfig(cfg)), 0644); err != nil {
		return fmt.Errorf("write HAProxy temporary config: %w", err)
	}
	if output, err := runner.Run(ctx, binaryPath, "-c", "-f", tempPath); err != nil {
		return fmt.Errorf("validate HAProxy config: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := fs.Rename(tempPath, configPath); err != nil {
		return fmt.Errorf("replace HAProxy config: %w", err)
	}
	return installOrReloadHAProxyService(ctx, serviceName, binaryPath, configPath, runner, fs)
}

func validateClientHAProxyRules(rules []model.HAProxyRule) error {
	listenPorts := make(map[int]struct{}, len(rules))
	for _, rule := range rules {
		if rule.ListenPort <= 0 || rule.ListenPort > 65535 {
			return fmt.Errorf("HAProxy rule %q has invalid listen port", rule.Name)
		}
		if _, exists := listenPorts[rule.ListenPort]; exists {
			return fmt.Errorf("HAProxy listen port %d is duplicated", rule.ListenPort)
		}
		listenPorts[rule.ListenPort] = struct{}{}
		if err := validateClientHAProxyTarget(rule.Primary); err != nil {
			return fmt.Errorf("HAProxy rule %q primary: %w", rule.Name, err)
		}
		if len(rule.Backups) == 0 {
			return fmt.Errorf("HAProxy rule %q requires at least one backup", rule.Name)
		}
		seen := map[string]struct{}{clientHAProxyTargetKey(rule.Primary): {}}
		for _, target := range rule.Backups {
			if err := validateClientHAProxyTarget(target); err != nil {
				return fmt.Errorf("HAProxy rule %q backup: %w", rule.Name, err)
			}
			key := clientHAProxyTargetKey(target)
			if _, exists := seen[key]; exists {
				return fmt.Errorf("HAProxy rule %q contains duplicate target %s:%d", rule.Name, target.Address, target.Port)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

func validateClientHAProxyTarget(target model.HAProxyRealmTarget) error {
	if strings.TrimSpace(target.Address) == "" {
		return errors.New("target address is empty")
	}
	if target.Port <= 0 || target.Port > 65535 {
		return errors.New("target port is invalid")
	}
	return nil
}

func clientHAProxyTargetKey(target model.HAProxyRealmTarget) string {
	return strings.ToLower(strings.TrimSpace(target.Address)) + "\x00" + strconv.Itoa(target.Port)
}

func resolveHAProxyBinary(configured string, runner commandRunner) (string, error) {
	if path := strings.TrimSpace(configured); path != "" {
		return path, nil
	}
	if path, err := runner.LookPath("haproxy"); err == nil && path != "" {
		return path, nil
	}
	for _, candidate := range []string{"/usr/sbin/haproxy", "/usr/local/sbin/haproxy", "/usr/bin/haproxy"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("HAProxy binary not found; install haproxy or set binary_path")
}

func renderHAProxyConfig(cfg model.HAProxyConfig) string {
	var builder strings.Builder
	builder.WriteString("global\n")
	builder.WriteString("  log stdout format raw local0\n")
	builder.WriteString("  maxconn 100000\n\n")
	builder.WriteString("defaults\n")
	builder.WriteString("  log global\n")
	builder.WriteString("  mode tcp\n")
	builder.WriteString("  option tcplog\n")
	builder.WriteString("  timeout connect 5s\n")
	builder.WriteString("  timeout client 1h\n")
	builder.WriteString("  timeout server 1h\n")
	builder.WriteString("  timeout check 5s\n\n")
	for index, rule := range activeHAProxyRules(cfg.Rules) {
		name := fmt.Sprintf("vpsm_%d_%s", rule.ListenPort, sanitizeHAProxyName(firstNonEmpty(rule.ID, rule.Name, fmt.Sprintf("rule_%d", index+1))))
		builder.WriteString("frontend ")
		builder.WriteString(name)
		builder.WriteString("_frontend\n  bind ")
		builder.WriteString(net.JoinHostPort(rule.ListenAddress, strconv.Itoa(rule.ListenPort)))
		builder.WriteString("\n  default_backend ")
		builder.WriteString(name)
		builder.WriteString("_backend\n\nbackend ")
		builder.WriteString(name)
		builder.WriteString("_backend\n  mode tcp\n  option tcp-check\n  timeout connect ")
		builder.WriteString(strconv.Itoa(rule.ConnectTimeoutSeconds))
		builder.WriteString("s\n  default-server inter ")
		builder.WriteString(strconv.Itoa(rule.CheckIntervalSeconds))
		builder.WriteString("s fall ")
		builder.WriteString(strconv.Itoa(rule.Fall))
		builder.WriteString(" rise ")
		builder.WriteString(strconv.Itoa(rule.Rise))
		builder.WriteString("\n")
		writeHAProxyServer(&builder, "primary", rule.Primary, false)
		for backupIndex, target := range rule.Backups {
			writeHAProxyServer(&builder, fmt.Sprintf("backup_%d", backupIndex+1), target, true)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func writeHAProxyServer(builder *strings.Builder, prefix string, target model.HAProxyRealmTarget, backup bool) {
	name := sanitizeHAProxyName(prefix + "_" + target.AgentID + "_" + target.RealmRuleID)
	builder.WriteString("  server ")
	builder.WriteString(name)
	builder.WriteString(" ")
	builder.WriteString(net.JoinHostPort(target.Address, strconv.Itoa(target.Port)))
	builder.WriteString(" check")
	if backup {
		builder.WriteString(" backup")
	}
	builder.WriteString("\n")
}

func sanitizeHAProxyName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, value)
	value = strings.Trim(value, "_-")
	if value == "" {
		return "target"
	}
	return value
}

func installOrReloadHAProxyService(ctx context.Context, serviceName, binaryPath, configPath string, runner commandRunner, fs haProxyFileSystem) error {
	switch {
	case commandAvailableWithRunner("systemctl", runner):
		serviceFile := filepath.Join("/etc/systemd/system", serviceName+".service")
		content := fmt.Sprintf(`[Unit]
Description=VPSMonitor HAProxy Failover
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s -Ws -f %s -p /run/%s.pid
ExecReload=/bin/kill -USR2 $MAINPID
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`, binaryPath, configPath, serviceName)
		if err := fs.WriteFile(serviceFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("write HAProxy systemd service: %w", err)
		}
		if output, err := runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(output)))
		}
		if output, err := runner.Run(ctx, "systemctl", "enable", serviceName); err != nil {
			return fmt.Errorf("systemctl enable %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
		}
		if _, err := runner.Run(ctx, "systemctl", "is-active", "--quiet", serviceName); err == nil {
			if _, reloadErr := runner.Run(ctx, "systemctl", "reload", serviceName); reloadErr == nil {
				return nil
			} else if output, restartErr := runner.Run(ctx, "systemctl", "restart", serviceName); restartErr != nil {
				return fmt.Errorf("systemctl restart %s after reload failure: %w: %s", serviceName, restartErr, strings.TrimSpace(string(output)))
			}
			return nil
		}
		if output, err := runner.Run(ctx, "systemctl", "start", serviceName); err != nil {
			return fmt.Errorf("systemctl start %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
		}
		return nil
	case commandAvailableWithRunner("rc-service", runner) && commandAvailableWithRunner("rc-update", runner):
		serviceFile := filepath.Join("/etc/init.d", serviceName)
		content := fmt.Sprintf(`#!/sbin/openrc-run
name="VPSMonitor HAProxy Failover"
description="VPSMonitor HAProxy Failover"
command=%q
command_args=%q
pidfile=%q
respawn_delay=3

depend() {
  need net
  after firewall
}
`, binaryPath, "-Ws -f "+configPath+" -p /run/"+serviceName+".pid", "/run/"+serviceName+".pid")
		if err := fs.WriteFile(serviceFile, []byte(content), 0755); err != nil {
			return fmt.Errorf("write HAProxy OpenRC service: %w", err)
		}
		if err := fs.Chmod(serviceFile, 0755); err != nil {
			return fmt.Errorf("chmod HAProxy OpenRC service: %w", err)
		}
		_, _ = runner.Run(ctx, "rc-update", "add", serviceName, "default")
		if output, err := runner.Run(ctx, "rc-service", serviceName, "restart"); err != nil {
			return fmt.Errorf("restart HAProxy OpenRC service: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	case commandAvailableWithRunner("procd", runner) && commandAvailableWithRunner("ubus", runner):
		serviceFile := filepath.Join("/etc/init.d", serviceName)
		content := fmt.Sprintf(`#!/bin/sh /etc/rc.common

START=95
STOP=10
USE_PROCD=1

start_service() {
  procd_open_instance
  procd_set_param command %q -Ws -f %q -p %q
  procd_set_param respawn 3600 5 5
  procd_set_param stdout 1
  procd_set_param stderr 1
  procd_set_param file %q
  procd_close_instance
}
`, binaryPath, configPath, "/var/run/"+serviceName+".pid", configPath)
		if err := fs.WriteFile(serviceFile, []byte(content), 0755); err != nil {
			return fmt.Errorf("write HAProxy procd service: %w", err)
		}
		if err := fs.Chmod(serviceFile, 0755); err != nil {
			return fmt.Errorf("chmod HAProxy procd service: %w", err)
		}
		_, _ = runner.Run(ctx, serviceFile, "enable")
		if output, err := runner.Run(ctx, serviceFile, "restart"); err != nil {
			return fmt.Errorf("restart HAProxy procd service: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	default:
		return errors.New("systemd, OpenRC, or OpenWrt procd is required to manage HAProxy service")
	}
}

func stopHAProxyService(ctx context.Context, serviceName string, runner commandRunner) error {
	if commandAvailableWithRunner("systemctl", runner) {
		_, _ = runner.Run(ctx, "systemctl", "stop", serviceName)
		_, _ = runner.Run(ctx, "systemctl", "disable", serviceName)
		return nil
	}
	if commandAvailableWithRunner("rc-service", runner) && commandAvailableWithRunner("rc-update", runner) {
		_, _ = runner.Run(ctx, "rc-service", serviceName, "stop")
		_, _ = runner.Run(ctx, "rc-update", "del", serviceName)
		return nil
	}
	if commandAvailableWithRunner("procd", runner) && commandAvailableWithRunner("ubus", runner) {
		serviceFile := filepath.Join("/etc/init.d", serviceName)
		_, _ = runner.Run(ctx, serviceFile, "stop")
		_, _ = runner.Run(ctx, serviceFile, "disable")
	}
	return nil
}
