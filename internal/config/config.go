package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	AgentName             string   `json:"agent_name"`
	Tags                  []string `json:"tags"`
	AgentToken            string   `json:"agent_token"`
	RegistrationToken     string   `json:"registration_token"`
	ServerURL             string   `json:"server_url"`
	ServerSkipTLSVerify   bool     `json:"server_skip_tls_verify"`
	PollInterval          string   `json:"poll_interval"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
}

type XUIConfig struct {
	Enabled       bool   `json:"enabled"`
	BaseURL       string `json:"base_url"`
	DBPath        string `json:"db_path"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	TwoFactorCode string `json:"two_factor_code"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
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
		cfg.AgentID = defaultClientAgentID()
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

func defaultClientAgentID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "bridge-client"
	}
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	var builder strings.Builder
	lastDash := false
	for _, r := range hostname {
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
	value := strings.Trim(builder.String(), "-")
	if value == "" {
		return "bridge-client"
	}
	return value
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
