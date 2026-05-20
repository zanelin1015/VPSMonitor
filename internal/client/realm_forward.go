package client

import (
	"bufio"
	"bytes"
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
	defaultRealmConfigPath  = "/etc/vpsmonitor/realm.toml"
	defaultRealmServiceName = "vpsmonitor-realm"
)

func (a *App) applyRealmForwardingIfNeeded(ctx context.Context, cfg model.RealmForwardConfig) {
	if a.realmForwardSignature == "" && isEmptyClientRealmForwardConfig(cfg) {
		return
	}
	cfg = normalizeClientRealmForwardConfig(cfg)
	signature := realmForwardSignature(cfg)
	if signature == a.realmForwardSignature && realmForwardConfigFileMatches(cfg) {
		return
	}
	realmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := applyRealmForwarding(realmCtx, cfg, osCommandRunner{}, osRealmFileSystem{}); err != nil {
		log.Printf("apply realm forwarding failed: %v", err)
		return
	}
	a.realmForwardSignature = signature
}

func realmForwardConfigFileMatches(cfg model.RealmForwardConfig) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	if !cfg.Enabled || strings.EqualFold(cfg.Backend, "none") || len(activeRealmForwardRules(cfg.Rules)) == 0 {
		return true
	}
	configPath := firstNonEmpty(cfg.ConfigPath, defaultRealmConfigPath)
	body, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) == strings.TrimSpace(renderRealmConfig(cfg))
}

func hasManagedClientRealmForwardRules(cfg model.RealmForwardConfig) bool {
	cfg = normalizeClientRealmForwardConfig(cfg)
	return cfg.Enabled && strings.EqualFold(cfg.Backend, "realm") && len(cfg.Rules) > 0
}

func isEmptyClientRealmForwardConfig(cfg model.RealmForwardConfig) bool {
	return !cfg.Enabled &&
		strings.TrimSpace(cfg.Backend) == "" &&
		strings.TrimSpace(cfg.BinaryPath) == "" &&
		strings.TrimSpace(cfg.ConfigPath) == "" &&
		strings.TrimSpace(cfg.ServiceName) == "" &&
		strings.TrimSpace(cfg.LogLevel) == "" &&
		len(cfg.Rules) == 0
}

func realmForwardSignature(cfg model.RealmForwardConfig) string {
	body, _ := json.Marshal(cfg)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type realmFileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(name string, data []byte, perm os.FileMode) error
	Chmod(name string, mode os.FileMode) error
}

type osRealmFileSystem struct{}

func (osRealmFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osRealmFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (osRealmFileSystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func applyRealmForwarding(ctx context.Context, cfg model.RealmForwardConfig, runner commandRunner, fs realmFileSystem) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	cfg = normalizeClientRealmForwardConfig(cfg)
	serviceName := firstNonEmpty(cfg.ServiceName, defaultRealmServiceName)
	if !cfg.Enabled || strings.EqualFold(cfg.Backend, "none") || len(activeRealmForwardRules(cfg.Rules)) == 0 {
		return stopRealmForwardService(ctx, serviceName, runner)
	}
	if !strings.EqualFold(cfg.Backend, "realm") {
		return fmt.Errorf("unsupported port forwarding backend %q", cfg.Backend)
	}
	binaryPath, err := resolveRealmBinary(cfg.BinaryPath, runner)
	if err != nil {
		return err
	}
	configPath := firstNonEmpty(cfg.ConfigPath, defaultRealmConfigPath)
	if err := fs.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create realm config dir: %w", err)
	}
	if err := fs.WriteFile(configPath, []byte(renderRealmConfig(cfg)), 0644); err != nil {
		return fmt.Errorf("write realm config: %w", err)
	}
	return installOrRestartRealmService(ctx, serviceName, binaryPath, configPath, runner, fs)
}

func collectRealmSnapshot(cfg model.RealmForwardConfig) *model.RealmSnapshot {
	if runtime.GOOS != "linux" {
		return nil
	}
	paths := discoverRealmConfigPaths(cfg)
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rules, err := parseRealmConfigRules(string(body))
		snapshot := &model.RealmSnapshot{
			ConfigPath:  path,
			ServiceName: firstNonEmpty(strings.TrimSpace(cfg.ServiceName), inferRealmServiceName(path)),
			BinaryPath:  strings.TrimSpace(cfg.BinaryPath),
			CollectedAt: time.Now().UTC(),
			Rules:       rules,
		}
		if err != nil {
			snapshot.Error = err.Error()
		}
		return snapshot
	}
	return nil
}

func discoverRealmConfigPaths(cfg model.RealmForwardConfig) []string {
	candidates := []string{
		strings.TrimSpace(cfg.ConfigPath),
		defaultRealmConfigPath,
		"/etc/realm/config.toml",
		"/usr/local/etc/realm/config.toml",
		"/etc/realm.toml",
		"/root/realm/config.toml",
		"/opt/realm/config.toml",
		"/usr/local/realm/config.toml",
	}
	serviceNames := []string{
		strings.TrimSpace(cfg.ServiceName),
		defaultRealmServiceName,
		"realm",
	}
	for _, serviceName := range serviceNames {
		if serviceName == "" {
			continue
		}
		candidates = append(candidates,
			configPathFromSystemdServiceFile(filepath.Join("/etc/systemd/system", serviceName+".service")),
			configPathFromSystemdServiceFile(filepath.Join("/lib/systemd/system", serviceName+".service")),
			configPathFromOpenRCServiceFile(filepath.Join("/etc/init.d", serviceName)),
		)
	}
	candidates = append(candidates, configPathsFromRealmProcesses("/proc")...)
	seen := make(map[string]struct{}, len(candidates))
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		paths = append(paths, candidate)
	}
	return paths
}

func configPathFromSystemdServiceFile(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		return realmConfigPathFromCommand(strings.TrimPrefix(line, "ExecStart="))
	}
	return ""
}

func configPathFromOpenRCServiceFile(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var commandArgs string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "command_args=") {
			commandArgs = strings.Trim(strings.TrimPrefix(line, "command_args="), `"`)
			break
		}
	}
	return realmConfigPathFromCommand(commandArgs)
}

func realmConfigPathFromCommand(command string) string {
	return realmConfigPathFromFields(splitRealmCommandFields(command))
}

func realmConfigPathFromFields(fields []string) string {
	for index, field := range fields {
		field = strings.Trim(field, `"'`)
		switch {
		case field == "-c" || field == "--config":
			if index+1 < len(fields) {
				return strings.Trim(fields[index+1], `"'`)
			}
		case strings.HasPrefix(field, "-c="):
			return strings.Trim(strings.TrimPrefix(field, "-c="), `"'`)
		case strings.HasPrefix(field, "--config="):
			return strings.Trim(strings.TrimPrefix(field, "--config="), `"'`)
		case strings.HasPrefix(field, "-c") && len(field) > 2:
			return strings.Trim(strings.TrimPrefix(field, "-c"), `"'`)
		}
	}
	return ""
}

func configPathsFromRealmProcesses(procRoot string) []string {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	paths := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !isNumericString(entry.Name()) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(procRoot, entry.Name(), "cmdline"))
		if err != nil || len(body) == 0 {
			continue
		}
		fields := splitProcCmdline(body)
		if !isRealmProcessCommand(fields) {
			continue
		}
		if path := realmConfigPathFromFields(fields); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func splitProcCmdline(body []byte) []string {
	parts := bytes.Split(body, []byte{0})
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(string(part))
		if value != "" {
			fields = append(fields, value)
		}
	}
	return fields
}

func isRealmProcessCommand(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	name := strings.ToLower(filepath.Base(fields[0]))
	return name == "realm" || strings.HasPrefix(name, "realm-")
}

func isNumericString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func splitRealmCommandFields(command string) []string {
	fields := make([]string, 0)
	var builder strings.Builder
	quote := rune(0)
	escaped := false
	for _, r := range command {
		if escaped {
			builder.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			builder.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if builder.Len() > 0 {
				fields = append(fields, builder.String())
				builder.Reset()
			}
			continue
		}
		builder.WriteRune(r)
	}
	if builder.Len() > 0 {
		fields = append(fields, builder.String())
	}
	return fields
}

func inferRealmServiceName(configPath string) string {
	if strings.Contains(configPath, "/vpsmonitor/") {
		return defaultRealmServiceName
	}
	return "realm"
}

func parseRealmConfigRules(raw string) ([]model.RealmForwardRule, error) {
	type parsedEndpoint struct {
		listen string
		remote string
		useUDP *bool
		noTCP  *bool
	}
	var endpoints []parsedEndpoint
	current := -1
	section := ""
	globalUseUDP := false
	globalNoTCP := false
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(stripRealmTOMLComment(scanner.Text()))
		if line == "" {
			continue
		}
		switch line {
		case "[[endpoints]]":
			endpoints = append(endpoints, parsedEndpoint{})
			current = len(endpoints) - 1
			section = "endpoint"
			continue
		case "[endpoints.network]":
			section = "endpoint_network"
			continue
		case "[network]":
			section = "global_network"
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = ""
			continue
		}
		if !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if section == "global_network" {
			switch key {
			case "use_udp":
				globalUseUDP = parseRealmBool(value)
			case "no_tcp":
				globalNoTCP = parseRealmBool(value)
			}
			continue
		}
		if current < 0 {
			continue
		}
		if section == "endpoint_network" {
			switch key {
			case "use_udp":
				endpoints[current].useUDP = boolPtr(parseRealmBool(value))
			case "no_tcp":
				endpoints[current].noTCP = boolPtr(parseRealmBool(value))
			}
			continue
		}
		switch key {
		case "listen":
			endpoints[current].listen = parseRealmString(value)
		case "remote":
			endpoints[current].remote = parseRealmString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	rules := make([]model.RealmForwardRule, 0, len(endpoints))
	for index, endpoint := range endpoints {
		listenHost, listenPort, ok := parseRealmEndpoint(endpoint.listen)
		if !ok {
			continue
		}
		remoteHost, remotePort, ok := parseRealmEndpoint(endpoint.remote)
		if !ok {
			continue
		}
		useUDP := globalUseUDP
		if endpoint.useUDP != nil {
			useUDP = *endpoint.useUDP
		}
		noTCP := globalNoTCP
		if endpoint.noTCP != nil {
			noTCP = *endpoint.noTCP
		}
		rules = append(rules, model.RealmForwardRule{
			ID:            fmt.Sprintf("auto-realm-%d-%d-%d", listenPort, remotePort, index),
			Name:          fmt.Sprintf("realm %d -> %s:%d", listenPort, remoteHost, remotePort),
			Enabled:       true,
			ListenAddress: listenHost,
			ListenPort:    listenPort,
			TargetAddress: remoteHost,
			TargetPort:    remotePort,
			Network:       parsedRealmNetwork(useUDP, noTCP),
		})
	}
	if len(rules) == 0 && strings.Contains(raw, "[[endpoints]]") {
		return nil, errors.New("realm config contains endpoints but no valid listen/remote pairs were parsed")
	}
	return rules, nil
}

func boolPtr(value bool) *bool {
	return &value
}

func stripRealmTOMLComment(line string) string {
	inQuote := false
	escaped := false
	for index, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inQuote {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if r == '#' && !inQuote {
			return line[:index]
		}
	}
	return line
}

func parseRealmString(value string) string {
	value = strings.TrimSpace(value)
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return strings.TrimSpace(decoded)
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func parseRealmBool(value string) bool {
	value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
	return value == "true" || value == "1" || value == "yes"
}

func parseRealmEndpoint(value string) (string, int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0, false
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		index := strings.LastIndex(value, ":")
		if index <= 0 || index == len(value)-1 {
			return "", 0, false
		}
		host = value[:index]
		portText = value[index+1:]
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		host = "0.0.0.0"
	}
	return host, port, true
}

func parsedRealmNetwork(useUDP bool, noTCP bool) string {
	if useUDP && noTCP {
		return "udp"
	}
	if useUDP {
		return "both"
	}
	return "tcp"
}

func normalizeClientRealmForwardConfig(cfg model.RealmForwardConfig) model.RealmForwardConfig {
	cfg.Backend = strings.ToLower(strings.TrimSpace(cfg.Backend))
	if cfg.Backend == "" {
		cfg.Backend = "realm"
	}
	cfg.BinaryPath = strings.TrimSpace(cfg.BinaryPath)
	cfg.ConfigPath = strings.TrimSpace(cfg.ConfigPath)
	cfg.ServiceName = strings.TrimSpace(cfg.ServiceName)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	cfg.Rules = activeRealmForwardRules(cfg.Rules)
	return cfg
}

func activeRealmForwardRules(items []model.RealmForwardRule) []model.RealmForwardRule {
	rules := make([]model.RealmForwardRule, 0, len(items))
	for _, rule := range items {
		if !rule.Enabled || rule.ListenPort <= 0 || rule.ListenPort > 65535 || rule.TargetPort <= 0 || rule.TargetPort > 65535 {
			continue
		}
		rule.ListenAddress = strings.TrimSpace(rule.ListenAddress)
		rule.TargetAddress = strings.TrimSpace(rule.TargetAddress)
		if rule.ListenAddress == "" {
			rule.ListenAddress = "0.0.0.0"
		}
		if rule.TargetAddress == "" {
			continue
		}
		rule.Network = "both"
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].ListenPort != rules[j].ListenPort {
			return rules[i].ListenPort < rules[j].ListenPort
		}
		return rules[i].TargetPort < rules[j].TargetPort
	})
	return rules
}

func clientRealmForwardNetwork(network string) string {
	return "both"
}

func resolveRealmBinary(configured string, runner commandRunner) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return strings.TrimSpace(configured), nil
	}
	if path, err := runner.LookPath("realm"); err == nil && path != "" {
		return path, nil
	}
	for _, candidate := range []string{"/usr/local/bin/realm", "/usr/bin/realm", "/opt/realm/realm"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("realm binary not found; install realm or set binary_path")
}

func renderRealmConfig(cfg model.RealmForwardConfig) string {
	var builder strings.Builder
	level := strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if level == "" {
		level = "info"
	}
	builder.WriteString("[log]\n")
	builder.WriteString("level = ")
	builder.WriteString(quoteTOML(level))
	builder.WriteString("\n\n[network]\nno_tcp = false\nuse_udp = true\n\n")
	for _, rule := range activeRealmForwardRules(cfg.Rules) {
		builder.WriteString("[[endpoints]]\n")
		builder.WriteString("listen = ")
		builder.WriteString(quoteTOML(net.JoinHostPort(rule.ListenAddress, strconv.Itoa(rule.ListenPort))))
		builder.WriteString("\nremote = ")
		builder.WriteString(quoteTOML(net.JoinHostPort(rule.TargetAddress, strconv.Itoa(rule.TargetPort))))
		builder.WriteString("\n\n")
	}
	return builder.String()
}

func quoteTOML(value string) string {
	body, _ := json.Marshal(value)
	return string(body)
}

func installOrRestartRealmService(ctx context.Context, serviceName string, binaryPath string, configPath string, runner commandRunner, fs realmFileSystem) error {
	switch {
	case commandAvailableWithRunner("systemctl", runner):
		serviceFile := filepath.Join("/etc/systemd/system", serviceName+".service")
		content := fmt.Sprintf(`[Unit]
Description=VPSMonitor Realm Port Forwarding
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s -c %s
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`, binaryPath, configPath)
		if err := fs.WriteFile(serviceFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("write systemd service: %w", err)
		}
		if output, err := runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(output)))
		}
		if output, err := runner.Run(ctx, "systemctl", "enable", serviceName); err != nil {
			return fmt.Errorf("systemctl enable %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
		}
		if output, err := runner.Run(ctx, "systemctl", "restart", serviceName); err != nil {
			return fmt.Errorf("systemctl restart %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
		}
		return nil
	case commandAvailableWithRunner("rc-service", runner) && commandAvailableWithRunner("rc-update", runner):
		serviceFile := filepath.Join("/etc/init.d", serviceName)
		content := fmt.Sprintf(`#!/sbin/openrc-run
name="VPSMonitor Realm Port Forwarding"
description="VPSMonitor Realm Port Forwarding"
command=%q
command_args=%q
command_background=true
pidfile="/run/${RC_SVCNAME}.pid"
output_log="/var/log/${RC_SVCNAME}.log"
error_log="/var/log/${RC_SVCNAME}.log"
start_stop_daemon_args="--make-pidfile"

depend() {
  need net
  after firewall
}
`, binaryPath, "-c "+configPath)
		if err := fs.WriteFile(serviceFile, []byte(content), 0755); err != nil {
			return fmt.Errorf("write openrc service: %w", err)
		}
		if err := fs.Chmod(serviceFile, 0755); err != nil {
			return fmt.Errorf("chmod openrc service: %w", err)
		}
		if output, err := runner.Run(ctx, "rc-update", "add", serviceName, "default"); err != nil {
			return fmt.Errorf("rc-update add %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
		}
		if output, err := runner.Run(ctx, "rc-service", serviceName, "restart"); err != nil {
			return fmt.Errorf("rc-service restart %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
		}
		return nil
	default:
		return errors.New("systemd or OpenRC is required to manage realm service")
	}
}

func stopRealmForwardService(ctx context.Context, serviceName string, runner commandRunner) error {
	if commandAvailableWithRunner("systemctl", runner) {
		_, _ = runner.Run(ctx, "systemctl", "stop", serviceName)
		_, _ = runner.Run(ctx, "systemctl", "disable", serviceName)
		return nil
	}
	if commandAvailableWithRunner("rc-service", runner) && commandAvailableWithRunner("rc-update", runner) {
		_, _ = runner.Run(ctx, "rc-service", serviceName, "stop")
		_, _ = runner.Run(ctx, "rc-update", "del", serviceName)
	}
	return nil
}

func commandAvailableWithRunner(name string, runner commandRunner) bool {
	_, err := runner.LookPath(name)
	return err == nil
}
