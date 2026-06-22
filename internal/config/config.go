package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
)

type ServerConfig struct {
	ListenAddr             string            `json:"listen_addr"`
	DataDir                string            `json:"data_dir"`
	DatabasePath           string            `json:"database_path"`
	CredentialKeyPath      string            `json:"credential_key_path"`
	RegistrationToken      string            `json:"registration_token"`
	AdminUsername          string            `json:"admin_username"`
	AdminPassword          string            `json:"admin_password"`
	AdminToken             string            `json:"admin_token"`
	DemoDataSourceURL      string            `json:"demo_data_source_url"`
	SnapshotRetentionDays  int               `json:"snapshot_retention_days"`
	SnapshotRetentionCount int               `json:"snapshot_retention_count"`
	Agents                 []ServerAgentAuth `json:"agents"`
}

type ServerAgentAuth struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

type ClientConfig struct {
	AgentID               string   `json:"agent_id"`
	AgentIDGenerated      bool     `json:"-"`
	AgentName             string   `json:"agent_name"`
	Tags                  []string `json:"tags"`
	AgentToken            string   `json:"agent_token"`
	RegistrationToken     string   `json:"registration_token"`
	ServerURL             string   `json:"server_url"`
	ServerSkipTLSVerify   bool     `json:"server_skip_tls_verify"`
	PollInterval          string   `json:"poll_interval"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
}

const DefaultXUIDBPath = "/etc/x-ui/x-ui.db"

func DefaultXUIDBPathForOS(osName string) string {
	switch strings.ToLower(strings.TrimSpace(osName)) {
	case "", "linux":
		return DefaultXUIDBPath
	default:
		return ""
	}
}

type XUIConfig struct {
	Enabled                bool   `json:"enabled"`
	BaseURL                string `json:"base_url"`
	DBPath                 string `json:"db_path"`
	Username               string `json:"username"`
	Password               string `json:"password"`
	APIToken               string `json:"api_token"`
	TwoFactorCode          string `json:"two_factor_code"`
	SkipTLSVerify          bool   `json:"skip_tls_verify"`
	AutoInstall            bool   `json:"auto_install,omitempty"`
	InstallScriptURL       string `json:"install_script_url,omitempty"`
	PanelPort              int    `json:"panel_port,omitempty"`
	WebPath                string `json:"web_path,omitempty"`
	AccessLogEnabled       bool   `json:"access_log_enabled,omitempty"`
	AccessLogPath          string `json:"access_log_path,omitempty"`
	AccessLogRetentionDays int    `json:"access_log_retention_days,omitempty"`
}

type NezhaConfig struct {
	Enabled       bool   `json:"enabled"`
	BaseURL       string `json:"base_url"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
	ServerID      uint64 `json:"server_id"`
	ServerUUID    string `json:"server_uuid"`
	ServerName    string `json:"server_name"`
}

func LoadServerConfig(path string) (ServerConfig, error) {
	var cfg ServerConfig
	if err := loadJSON(path, &cfg); err != nil {
		return cfg, err
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8090"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = filepath.Join(cfg.DataDir, "bridge.db")
	}
	if cfg.CredentialKeyPath == "" {
		cfg.CredentialKeyPath = filepath.Join(cfg.DataDir, "credential.key")
	}
	if cfg.AdminUsername == "" {
		cfg.AdminUsername = "admin"
	}
	if cfg.SnapshotRetentionDays == 0 {
		cfg.SnapshotRetentionDays = 30
	}
	if cfg.SnapshotRetentionCount == 0 {
		cfg.SnapshotRetentionCount = 5000
	}
	return cfg, nil
}

func LoadClientConfig(path string) (ClientConfig, time.Duration, error) {
	var cfg ClientConfig
	if err := loadJSON(path, &cfg); err != nil {
		return cfg, 0, err
	}
	if cfg.RequestTimeoutSeconds <= 0 {
		cfg.RequestTimeoutSeconds = 15
	}
	if strings.TrimSpace(cfg.AgentID) == "" {
		if value := sanitizeClientAgentID(os.Getenv("VPSMONITOR_AGENT_ID")); value != "" {
			cfg.AgentID = value
		} else {
			cfg.AgentID = defaultClientAgentID()
			cfg.AgentIDGenerated = true
		}
	}
	if cfg.PollInterval == "" {
		cfg.PollInterval = "30s"
	}
	d, err := time.ParseDuration(cfg.PollInterval)
	if err != nil {
		return cfg, 0, fmt.Errorf("parse poll_interval: %w", err)
	}
	if d <= 0 {
		return cfg, 0, fmt.Errorf("poll_interval must be positive")
	}
	return cfg, d, nil
}

func PersistClientAgentIDIfMissing(path string, agentID string) error {
	agentID = sanitizeClientAgentID(agentID)
	if agentID == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var payload map[string]any
	raw := strings.TrimPrefix(string(data), "\ufeff")
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}
	if existing, _ := payload["agent_id"].(string); strings.TrimSpace(existing) != "" {
		return nil
	}
	payload["agent_id"] = agentID
	updated, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	updated = append(updated, '\n')
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, updated, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func defaultClientAgentID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "bridge-client"
	}
	base := sanitizeClientAgentID(hostname)
	if base == "" {
		base = "bridge-client"
	}
	if fingerprint := clientMachineFingerprint(); fingerprint != "" {
		return trimClientAgentID(base + "-" + shortClientHash(fingerprint))
	}
	return base
}

func sanitizeClientAgentID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	value = strings.Trim(builder.String(), "-")
	return trimClientAgentID(value)
}

func trimClientAgentID(value string) string {
	const maxAgentIDLength = 80
	if len(value) <= maxAgentIDLength {
		return value
	}
	return strings.Trim(value[:maxAgentIDLength], "-")
}

func shortClientHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:10]
}

func clientMachineFingerprint() string {
	for _, path := range []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
		"/sys/class/dmi/id/product_uuid",
	} {
		if value := readMachineIDFile(path); value != "" {
			return path + ":" + value
		}
	}
	if runtime.GOOS == "windows" {
		if value := windowsMachineGuid(); value != "" {
			return "windows-machine-guid:" + value
		}
	}
	if values := hardwareMACs(); len(values) > 0 {
		return "mac:" + strings.Join(values, ",")
	}
	return ""
}

func readMachineIDFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(data))
	if value == "" || strings.EqualFold(value, "none") {
		return ""
	}
	return value
}

func windowsMachineGuid() string {
	output, err := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && strings.EqualFold(fields[0], "MachineGuid") {
			return fields[len(fields)-1]
		}
	}
	return ""
}

func hardwareMACs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	values := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) == 0 {
			continue
		}
		values = append(values, strings.ToLower(iface.HardwareAddr.String()))
	}
	sort.Strings(values)
	return values
}

func loadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	raw := strings.TrimPrefix(string(data), "\ufeff")
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return nil
}
