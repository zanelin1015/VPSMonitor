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
	TLSCertFile            string            `json:"tls_cert_file"`
	TLSKeyFile             string            `json:"tls_key_file"`
	AllowInsecureHTTP      bool              `json:"allow_insecure_http"`
	DataDir                string            `json:"data_dir"`
	DatabasePath           string            `json:"database_path"`
	CredentialKeyPath      string            `json:"credential_key_path"`
	RegistrationToken      string            `json:"registration_token"`
	TrustedProxyCIDRs      []string          `json:"trusted_proxy_cidrs"`
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
	ConfigPath            string   `json:"-"`
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
	hasTLSCert := strings.TrimSpace(cfg.TLSCertFile) != ""
	hasTLSKey := strings.TrimSpace(cfg.TLSKeyFile) != ""
	if hasTLSCert != hasTLSKey {
		return cfg, fmt.Errorf("tls_cert_file and tls_key_file must be configured together")
	}
	if !hasTLSCert && !cfg.AllowInsecureHTTP && len(cfg.TrustedProxyCIDRs) == 0 {
		return cfg, fmt.Errorf("TLS is required: configure tls_cert_file/tls_key_file, configure trusted_proxy_cidrs for a TLS reverse proxy, or explicitly set allow_insecure_http for local development")
	}
	for _, agent := range cfg.Agents {
		if agent.ID != "" && !IsValidAgentID(agent.ID) {
			return cfg, fmt.Errorf("agents entry %q has an invalid id", agent.ID)
		}
	}
	return cfg, nil
}

func LoadClientConfig(path string) (ClientConfig, time.Duration, error) {
	var cfg ClientConfig
	if err := loadJSON(path, &cfg); err != nil {
		return cfg, 0, err
	}
	cfg.ConfigPath = path
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
	if !IsValidAgentID(cfg.AgentID) {
		return cfg, 0, fmt.Errorf("agent_id must be 1-80 characters using only letters, digits, dots, underscores, or hyphens")
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
	return persistClientConfigPayload(path, payload)
}

// PersistClientRegistration stores the issued per-agent token and removes the
// shared bootstrap token after the first successful registration.
func PersistClientRegistration(path string, agentID string, agentToken string) error {
	agentToken = strings.TrimSpace(agentToken)
	if strings.TrimSpace(path) == "" || strings.TrimSpace(agentID) == "" || agentToken == "" {
		return fmt.Errorf("config path, agent_id, and agent_token are required")
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
	payload["agent_id"] = agentID
	payload["agent_token"] = agentToken
	payload["registration_token"] = ""
	return persistClientConfigPayload(path, payload)
}

func persistClientConfigPayload(path string, payload map[string]any) error {
	updated, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	updated = append(updated, '\n')
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm() & 0o600
	}
	if mode == 0 {
		mode = 0o600
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set temporary config permissions: %w", err)
	}
	if _, err := tmp.Write(updated); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
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

// IsValidAgentID reports whether an ID can be safely embedded in the
// server's path-based agent routes. It accepts identifier forms used by
// existing deployments, including uppercase letters and dots.
func IsValidAgentID(value string) bool {
	if len(value) == 0 || len(value) > 80 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
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
