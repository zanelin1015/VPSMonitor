package singbox

// The sing-box package intentionally deals in map[string]any values.  This
// lets the manager preserve the complete upstream schema, including fields
// added by newer sing-box releases, while still providing safe CRUD helpers
// for the three pieces VPSMonitor owns.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

const (
	DefaultConfigPath  = "/etc/sing-box/config.json"
	DefaultBinaryPath  = "sing-box"
	DefaultServiceName = "sing-box"
	maxConfigBytes     = 32 << 20
)

type Manager struct {
	cfg config.SingBoxConfig
}

func NewManager(cfg config.SingBoxConfig) *Manager {
	cfg.ConfigPath = firstNonEmpty(strings.TrimSpace(cfg.ConfigPath), DefaultConfigPath)
	cfg.BinaryPath = firstNonEmpty(strings.TrimSpace(cfg.BinaryPath), DefaultBinaryPath)
	cfg.ServiceName = firstNonEmpty(strings.TrimSpace(cfg.ServiceName), DefaultServiceName)
	return &Manager{cfg: cfg}
}

func (m *Manager) Config() config.SingBoxConfig {
	if m == nil {
		return config.SingBoxConfig{}
	}
	return m.cfg
}

func (m *Manager) Collect(ctx context.Context) *model.SingBoxSnapshot {
	snapshot := &model.SingBoxSnapshot{
		ConfigPath:  m.cfg.ConfigPath,
		ServiceName: m.cfg.ServiceName,
		BinaryPath:  m.cfg.BinaryPath,
		CollectedAt: time.Now().UTC(),
	}
	data, err := readConfig(m.cfg.ConfigPath)
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		snapshot.Error = fmt.Sprintf("parse sing-box config: %v", err)
		return snapshot
	}
	snapshot.RawConfig = raw
	snapshot.Nodes = cloneObjectList(raw["outbounds"])
	snapshot.Clients = cloneObjectList(raw["inbounds"])
	if route, ok := raw["route"].(map[string]any); ok {
		snapshot.ForwardingRules = cloneObjectList(route["rules"])
	}
	snapshot.Running = serviceRunning(ctx, m.cfg.ServiceName)
	return snapshot
}

func (m *Manager) ExecuteAction(ctx context.Context, action model.XUIAction) (map[string]any, error) {
	if m == nil {
		return nil, errors.New("sing-box manager is nil")
	}
	data, err := readConfig(m.cfg.ConfigPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "no such file") {
			return nil, err
		}
		data = []byte(`{"outbounds":[],"inbounds":[],"route":{"rules":[]}}`)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse sing-box config: %w", err)
	}

	var changed string
	switch action.Kind {
	case model.SingBoxActionAddNode:
		changed, err = addObject(cfg, "outbounds", action.Payload, "node", validateNode)
	case model.SingBoxActionAddClient:
		changed, err = addObject(cfg, "inbounds", action.Payload, "client", validateClient)
	case model.SingBoxActionAddUser:
		changed, err = addUser(cfg, action.Payload)
	case model.SingBoxActionDeleteUser:
		changed, err = deleteUser(cfg, action.Payload)
	case model.SingBoxActionAddForwardingRule:
		changed, err = addForwardingRule(cfg, action.Payload)
	case model.SingBoxActionReload:
		changed = "reload"
	default:
		return nil, fmt.Errorf("unsupported sing-box action kind: %s", action.Kind)
	}
	if err != nil {
		return nil, err
	}
	if changed != "reload" {
		if err := validateConfigShape(cfg); err != nil {
			return nil, err
		}
		if err := writeConfig(m.cfg.ConfigPath, cfg); err != nil {
			return nil, err
		}
	}
	if err := m.validateBinary(ctx); err != nil {
		return nil, err
	}
	if err := reloadService(ctx, m.cfg.ServiceName); err != nil {
		return map[string]any{"changed": changed, "saved": changed == "reload" || changed != "", "restarted": false}, err
	}
	return map[string]any{
		"changed":   changed,
		"saved":     changed == "reload" || changed != "",
		"verified":  true,
		"restarted": true,
	}, nil
}

func (m *Manager) validateBinary(ctx context.Context) error {
	// Validation is best effort when sing-box is not installed yet.  A missing
	// binary must not prevent VPSMonitor from preparing a valid config file.
	if strings.TrimSpace(m.cfg.BinaryPath) == "" {
		return nil
	}
	if _, err := exec.LookPath(m.cfg.BinaryPath); err != nil {
		if filepath.IsAbs(m.cfg.BinaryPath) {
			if _, statErr := os.Stat(m.cfg.BinaryPath); statErr == nil {
				return nil
			}
		}
		return nil
	}
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, m.cfg.BinaryPath, "check", "-c", m.cfg.ConfigPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sing-box config validation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func addObject(cfg map[string]any, listKey string, payload map[string]any, payloadKey string, validate func(map[string]any) error) (string, error) {
	value, ok := payload[payloadKey].(map[string]any)
	if !ok || value == nil {
		return "", fmt.Errorf("%s must be an object", payloadKey)
	}
	item := cloneObject(value)
	if remark := strings.TrimSpace(stringValue(item["remark"])); remark != "" {
		if strings.TrimSpace(stringValue(item["tag"])) == "" {
			item["tag"] = remark
		}
		// sing-box uses tag as the displayable identity; arbitrary remark keys
		// are rejected by sing-box check and must not reach the config file.
		delete(item, "remark")
	}
	if err := validate(item); err != nil {
		return "", err
	}
	if value := cfg[listKey]; value != nil && !isObjectList(value) {
		return "", fmt.Errorf("sing-box %s must be an array", listKey)
	}
	items := objectList(cfg[listKey])
	tag := strings.TrimSpace(stringValue(item["tag"]))
	for _, existing := range items {
		if strings.EqualFold(tag, strings.TrimSpace(stringValue(existing["tag"]))) {
			return "", fmt.Errorf("sing-box %s tag already exists: %s", listKey, tag)
		}
		if listKey == "inbounds" && intValue(item["listen_port"]) > 0 && intValue(existing["listen_port"]) == intValue(item["listen_port"]) {
			return "", fmt.Errorf("sing-box client listen_port already exists: %d", intValue(item["listen_port"]))
		}
	}
	items = append(items, item)
	cfg[listKey] = items
	return tag, nil
}

func addForwardingRule(cfg map[string]any, payload map[string]any) (string, error) {
	value, ok := payload["rule"].(map[string]any)
	if !ok || value == nil {
		return "", errors.New("rule must be an object")
	}
	rule := cloneObject(value)
	if imported, ok := payload["outbound"].(map[string]any); ok && imported != nil {
		if err := addImportedOutbound(cfg, imported); err != nil {
			return "", err
		}
		// Keep the route and copied outbound in sync even when the caller used
		// a stale/generated tag before the import was normalized.
		importedTag := strings.TrimSpace(stringValue(imported["tag"]))
		if importedTag != "" && !outboundTagExists(cfg, strings.TrimSpace(stringValue(rule["outbound"]))) {
			rule["outbound"] = importedTag
		}
	}
	if err := validateForwardingRule(rule, cfg); err != nil {
		return "", err
	}
	route, ok := cfg["route"].(map[string]any)
	if !ok || route == nil {
		route = map[string]any{}
		cfg["route"] = route
	}
	if value := route["rules"]; value != nil && !isObjectList(value) {
		return "", errors.New("sing-box route.rules must be an array")
	}
	rules := objectList(route["rules"])
	fingerprint := objectFingerprint(rule)
	for _, existing := range rules {
		if objectFingerprint(existing) == fingerprint {
			return "", errors.New("equivalent sing-box forwarding rule already exists")
		}
	}
	rules = append(rules, rule)
	route["rules"] = rules
	return fingerprint, nil
}

func outboundTagExists(cfg map[string]any, tag string) bool {
	if tag == "" {
		return false
	}
	for _, outbound := range objectList(cfg["outbounds"]) {
		if strings.TrimSpace(stringValue(outbound["tag"])) == tag {
			return true
		}
	}
	return false
}

func addUser(cfg map[string]any, payload map[string]any) (string, error) {
	inboundTag := strings.TrimSpace(stringValue(payload["inbound_tag"]))
	user, ok := payload["user"].(map[string]any)
	if inboundTag == "" || !ok || user == nil {
		return "", errors.New("sing-box user requires inbound_tag and user object")
	}
	if len(user) == 0 {
		return "", errors.New("sing-box user cannot be empty")
	}
	inbounds := objectList(cfg["inbounds"])
	for _, inbound := range inbounds {
		if strings.TrimSpace(stringValue(inbound["tag"])) != inboundTag {
			continue
		}
		users := objectList(inbound["users"])
		if strings.EqualFold(strings.TrimSpace(stringValue(inbound["type"])), "snell") && strings.TrimSpace(stringValue(user["userkey"])) == "" {
			return "", errors.New("snell user requires userkey")
		}
		key := userIdentity(user)
		if key == "" {
			return "", errors.New("sing-box user requires name, username, email, uuid or password")
		}
		for _, existing := range users {
			if userIdentity(existing) == key {
				return "", fmt.Errorf("sing-box user already exists in inbound: %s", key)
			}
		}
		inbound["users"] = append(users, cloneObject(user))
		return key, nil
	}
	return "", fmt.Errorf("sing-box inbound not found: %s", inboundTag)
}

func userIdentity(user map[string]any) string {
	for _, key := range []string{"name", "userkey", "username", "email", "uuid", "password"} {
		if value := strings.TrimSpace(stringValue(user[key])); value != "" {
			return key + ":" + value
		}
	}
	return ""
}

func deleteUser(cfg map[string]any, payload map[string]any) (string, error) {
	inboundTag := strings.TrimSpace(stringValue(payload["inbound_tag"]))
	userKey := strings.TrimSpace(stringValue(payload["user_key"]))
	if inboundTag == "" || userKey == "" {
		return "", errors.New("sing-box delete user requires inbound_tag and user_key")
	}
	for _, inbound := range objectList(cfg["inbounds"]) {
		if strings.TrimSpace(stringValue(inbound["tag"])) != inboundTag {
			continue
		}
		users := objectList(inbound["users"])
		filtered := make([]map[string]any, 0, len(users))
		removed := false
		for _, user := range users {
			if userIdentity(user) == userKey {
				removed = true
				continue
			}
			filtered = append(filtered, user)
		}
		if !removed {
			return "", fmt.Errorf("sing-box user not found: %s", userKey)
		}
		inbound["users"] = filtered
		return userKey, nil
	}
	return "", fmt.Errorf("sing-box inbound not found: %s", inboundTag)
}

func addImportedOutbound(cfg map[string]any, raw map[string]any) error {
	outbound := cloneObject(raw)
	if remark := strings.TrimSpace(stringValue(outbound["remark"])); remark != "" {
		if strings.TrimSpace(stringValue(outbound["tag"])) == "" {
			outbound["tag"] = remark
		}
		delete(outbound, "remark")
	}
	if err := validateNode(outbound); err != nil {
		return fmt.Errorf("imported outbound: %w", err)
	}
	tag := strings.TrimSpace(stringValue(outbound["tag"]))
	if value := cfg["outbounds"]; value != nil && !isObjectList(value) {
		return errors.New("sing-box outbounds must be an array")
	}
	items := objectList(cfg["outbounds"])
	for _, existing := range items {
		if strings.TrimSpace(stringValue(existing["tag"])) != tag {
			continue
		}
		if objectFingerprint(existing) != objectFingerprint(outbound) {
			return fmt.Errorf("outbound tag already exists with different settings: %s", tag)
		}
		return nil
	}
	cfg["outbounds"] = append(items, outbound)
	return nil
}

func validateNode(node map[string]any) error {
	tag := strings.TrimSpace(stringValue(node["tag"]))
	typ := strings.ToLower(strings.TrimSpace(stringValue(node["type"])))
	if tag == "" || typ == "" {
		return errors.New("node requires non-empty tag and type")
	}
	if typ == "direct" || typ == "block" || typ == "dns" || typ == "selector" || typ == "urltest" {
		return nil
	}
	server := strings.TrimSpace(stringValue(node["server"]))
	if server == "" || net.ParseIP(strings.Trim(server, "[]")) == nil {
		return errors.New("outbound node server must be an IP address")
	}
	port := intValue(node["server_port"])
	if port < 1 || port > 65535 {
		return errors.New("outbound node server_port must be between 1 and 65535")
	}
	node["server_port"] = port
	if typ == "snell" {
		if strings.TrimSpace(stringValue(node["psk"])) == "" {
			return errors.New("snell outbound requires psk")
		}
		version := intValue(node["version"])
		if version == 0 {
			version = 4
		}
		if version != 4 && version != 6 {
			return errors.New("snell outbound version must be 4 or 6")
		}
		node["version"] = version
		if version == 6 && (len([]byte(strings.TrimSpace(stringValue(node["psk"])))) < 12 || len([]byte(strings.TrimSpace(stringValue(node["psk"])))) > 255) {
			return errors.New("snell version 6 outbound psk must be between 12 and 255 bytes")
		}
		if network := strings.ToLower(strings.TrimSpace(stringValue(node["network"]))); network != "" && network != "tcp" && network != "udp" {
			return errors.New("snell outbound network must be tcp or udp")
		} else if network != "" {
			node["network"] = network
		}
		if reuse, ok := node["reuse"]; ok {
			if _, valid := reuse.(bool); !valid {
				return errors.New("snell outbound reuse must be boolean")
			}
		}
		if version == 4 {
			obfsMode := strings.ToLower(strings.TrimSpace(stringValue(node["obfs_mode"])))
			if obfsMode == "" {
				node["obfs_mode"] = "none"
			} else if obfsMode != "none" && obfsMode != "http" {
				return errors.New("snell outbound obfs_mode must be none or http")
			} else {
				node["obfs_mode"] = obfsMode
			}
			if obfsMode == "http" && strings.TrimSpace(stringValue(node["obfs_host"])) == "" {
				node["obfs_host"] = "bing.com"
			}
			delete(node, "mode")
		} else {
			mode := strings.ToLower(strings.TrimSpace(stringValue(node["mode"])))
			if mode == "" {
				node["mode"] = "default"
			} else if mode != "default" && mode != "unshaped" && mode != "unsafe-raw" {
				return errors.New("snell outbound mode must be default, unshaped or unsafe-raw")
			} else {
				node["mode"] = mode
			}
			delete(node, "obfs_mode")
			delete(node, "obfs_host")
		}
	}
	return nil
}

func validateClient(client map[string]any) error {
	if strings.TrimSpace(stringValue(client["tag"])) == "" {
		return errors.New("client requires a non-empty remark/tag")
	}
	typ := strings.ToLower(strings.TrimSpace(stringValue(client["type"])))
	if typ == "" {
		return errors.New("client requires a type")
	}
	if strings.TrimSpace(stringValue(client["listen"])) == "" {
		client["listen"] = "::"
	}
	port := intValue(client["listen_port"])
	if port < 1 || port > 65535 {
		return errors.New("client requires exactly one listen_port between 1 and 65535")
	}
	client["listen_port"] = port
	if typ == "snell" {
		if strings.TrimSpace(stringValue(client["psk"])) == "" {
			return errors.New("snell client requires psk")
		}
		version := intValue(client["version"])
		if version == 0 {
			version = 5
		}
		if version != 5 && version != 6 {
			return errors.New("snell client version must be 5 or 6")
		}
		client["version"] = version
		if version == 6 && (len([]byte(strings.TrimSpace(stringValue(client["psk"])))) < 12 || len([]byte(strings.TrimSpace(stringValue(client["psk"])))) > 255) {
			return errors.New("snell version 6 client psk must be between 12 and 255 bytes")
		}
		if usersValue, exists := client["users"]; exists {
			if usersValue != nil && !isObjectList(usersValue) {
				return errors.New("snell client users must be an array")
			}
			for _, user := range objectList(usersValue) {
				if strings.TrimSpace(stringValue(user["userkey"])) == "" {
					return errors.New("snell client user requires userkey")
				}
			}
		}
		if version == 5 {
			obfsMode := strings.ToLower(strings.TrimSpace(stringValue(client["obfs_mode"])))
			if obfsMode == "" {
				client["obfs_mode"] = "none"
			} else if obfsMode != "none" && obfsMode != "http" {
				return errors.New("snell client obfs_mode must be none or http")
			} else {
				client["obfs_mode"] = obfsMode
			}
			delete(client, "mode")
		} else {
			mode := strings.ToLower(strings.TrimSpace(stringValue(client["mode"])))
			if mode == "" {
				client["mode"] = "default"
			} else if mode != "default" && mode != "unshaped" && mode != "unsafe-raw" {
				return errors.New("snell client mode must be default, unshaped or unsafe-raw")
			} else {
				client["mode"] = mode
			}
			delete(client, "obfs_mode")
		}
	}
	return nil
}

func validateForwardingRule(rule map[string]any, cfg map[string]any) error {
	if len(rule) == 0 {
		return errors.New("forwarding rule cannot be empty")
	}
	outbound := strings.TrimSpace(stringValue(rule["outbound"]))
	action := strings.TrimSpace(stringValue(rule["action"]))
	if outbound == "" && action == "" {
		return errors.New("forwarding rule requires outbound or action")
	}
	if outbound != "" {
		found := false
		for _, item := range objectList(cfg["outbounds"]) {
			if stringValue(item["tag"]) == outbound {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("forwarding rule references unknown outbound: %s", outbound)
		}
	}
	return nil
}

func validateConfigShape(cfg map[string]any) error {
	if cfg == nil {
		return errors.New("sing-box config cannot be null")
	}
	if value, ok := cfg["outbounds"]; ok && value != nil && !isObjectList(value) {
		return errors.New("sing-box outbounds must be an array")
	}
	if value, ok := cfg["inbounds"]; ok && value != nil && !isObjectList(value) {
		return errors.New("sing-box inbounds must be an array")
	}
	if route, ok := cfg["route"].(map[string]any); ok {
		if value, exists := route["rules"]; exists && value != nil && !isObjectList(value) {
			return errors.New("sing-box route.rules must be an array")
		}
	}
	return nil
}

func readConfig(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sing-box config_path is required")
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read sing-box config: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("sing-box config exceeds %d bytes", maxConfigBytes)
	}
	return data, nil
}

func writeConfig(path string, cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sing-box config: %w", err)
	}
	data = append(data, '\n')
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == string(filepath.Separator) || path == "" {
		return errors.New("invalid sing-box config_path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create sing-box config directory: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sing-box-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create sing-box config temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("set sing-box config mode: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write sing-box config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync sing-box config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close sing-box config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace sing-box config: %w", err)
	}
	return nil
}

func reloadService(ctx context.Context, serviceName string) error {
	serviceName = firstNonEmpty(strings.TrimSpace(serviceName), DefaultServiceName)
	if _, err := exec.LookPath("systemctl"); err == nil {
		if output, restartErr := exec.CommandContext(ctx, "systemctl", "restart", serviceName).CombinedOutput(); restartErr != nil {
			return fmt.Errorf("restart sing-box service %s: %w: %s", serviceName, restartErr, strings.TrimSpace(string(output)))
		}
		return nil
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		if output, restartErr := exec.CommandContext(ctx, "rc-service", serviceName, "restart").CombinedOutput(); restartErr != nil {
			return fmt.Errorf("restart sing-box service %s: %w: %s", serviceName, restartErr, strings.TrimSpace(string(output)))
		}
		return nil
	}
	if _, err := exec.LookPath("service"); err == nil {
		if output, restartErr := exec.CommandContext(ctx, "service", serviceName, "restart").CombinedOutput(); restartErr != nil {
			return fmt.Errorf("restart sing-box service %s: %w: %s", serviceName, restartErr, strings.TrimSpace(string(output)))
		}
		return nil
	}
	return errors.New("no supported service manager found for sing-box")
}

func serviceRunning(ctx context.Context, serviceName string) bool {
	serviceName = firstNonEmpty(strings.TrimSpace(serviceName), DefaultServiceName)
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := exec.LookPath("systemctl"); err == nil {
		return exec.CommandContext(checkCtx, "systemctl", "is-active", "--quiet", serviceName).Run() == nil
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		return exec.CommandContext(checkCtx, "rc-service", serviceName, "status").Run() == nil
	}
	if _, err := exec.LookPath("service"); err == nil {
		return exec.CommandContext(checkCtx, "service", serviceName, "status").Run() == nil
	}
	return false
}

func objectList(value any) []map[string]any {
	items, _ := value.([]map[string]any)
	if items != nil {
		return items
	}
	array, _ := value.([]any)
	result := make([]map[string]any, 0, len(array))
	for _, item := range array {
		if object, ok := item.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func cloneObjectList(value any) []map[string]any {
	items := objectList(value)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, cloneObject(item))
	}
	return result
}

func cloneObject(value map[string]any) map[string]any {
	data, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}

func isObjectList(value any) bool {
	if value == nil {
		return true
	}
	if _, ok := value.([]map[string]any); ok {
		return true
	}
	array, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range array {
		if _, ok := item.(map[string]any); !ok {
			return false
		}
	}
	return true
}

func objectFingerprint(value map[string]any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := strconv.Atoi(string(typed))
		return parsed
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
