package store

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db        *sql.DB
	secrets   *CredentialCipher
	retention SnapshotRetentionPolicy
}

type Option func(*SQLiteStore)

type SnapshotRetentionPolicy struct {
	MaxAge      time.Duration
	MaxPerAgent int
}

func WithCredentialCipher(cipher *CredentialCipher) Option {
	return func(s *SQLiteStore) {
		s.secrets = cipher
	}
}

func WithSnapshotRetention(policy SnapshotRetentionPolicy) Option {
	return func(s *SQLiteStore) {
		s.retention = policy
	}
}

const (
	adminAccountID         = 1
	adminSessionTokenBytes = 32
	adminPasswordSaltBytes = 16
	adminPasswordKeyBytes  = 32
	adminPasswordIter      = 210000
	adminPasswordMinLength = 8
)

func NewSQLiteStore(databasePath string, opts ...Option) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &SQLiteStore{db: db}
	for _, opt := range opts {
		opt(store)
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.encryptPlaintextCredentials(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

type rowScanner interface {
	Scan(dest ...any) error
}

type rowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

func (s *SQLiteStore) parseManagedConfig(agentID, agentName string, sortOrder int, tagsJSON, xuiJSON, nezhaJSON, renewalJSON, entryJSON string) (model.ManagedAgentConfig, error) {
	cfg := model.ManagedAgentConfig{
		AgentID:   agentID,
		AgentName: agentName,
		SortOrder: sortOrder,
	}
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &cfg.Tags)
		cfg.Tags = normalizeTags(cfg.Tags)
	}
	if xuiJSON != "" {
		_ = json.Unmarshal([]byte(xuiJSON), &cfg.XUI)
		var err error
		cfg.XUI, err = s.decryptXUIConfig(cfg.XUI)
		if err != nil {
			return model.ManagedAgentConfig{}, fmt.Errorf("decrypt x-ui config for agent %s: %w", agentID, err)
		}
	}
	if renewalJSON != "" {
		_ = json.Unmarshal([]byte(renewalJSON), &cfg.Renewal)
		cfg.Renewal = normalizeRenewalConfig(cfg.Renewal)
	}
	if entryJSON != "" {
		_ = json.Unmarshal([]byte(entryJSON), &cfg.Entry)
		cfg.Entry = normalizeEntryConfig(cfg.Entry)
	}
	return cfg, nil
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func randomToken() (string, error) {
	return randomTokenBytes(24)
}

func randomTokenBytes(size int) (string, error) {
	buf, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomBytes(size int) ([]byte, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate random bytes: %w", err)
	}
	return buf, nil
}

func hashPassword(password string) (string, error) {
	salt, err := randomBytes(adminPasswordSaltBytes)
	if err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, adminPasswordIter, adminPasswordKeyBytes)
	if err != nil {
		return "", fmt.Errorf("derive password hash: %w", err)
	}
	return fmt.Sprintf(
		"pbkdf2_sha256$%d$%s$%s",
		adminPasswordIter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false, fmt.Errorf("unsupported password hash")
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil {
		return false, fmt.Errorf("parse password hash iterations: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("decode password hash salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("decode password hash key: %w", err)
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return false, fmt.Errorf("derive password hash: %w", err)
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	return normalized
}

func hasManagedConfig(cfg model.ManagedAgentConfig) bool {
	return hasXUIConfig(cfg.XUI) || hasRenewalConfig(cfg.Renewal) || hasEntryConfig(cfg.Entry)
}

func normalizeRenewalConfig(cfg model.VPSRenewalConfig) model.VPSRenewalConfig {
	cfg.StartDate = strings.TrimSpace(cfg.StartDate)
	cfg.ExpireDate = strings.TrimSpace(cfg.ExpireDate)
	cfg.TrafficBaselinePeriodStart = strings.TrimSpace(cfg.TrafficBaselinePeriodStart)
	switch strings.ToLower(strings.TrimSpace(cfg.Cycle)) {
	case "week", "weekly":
		cfg.Cycle = "week"
	case "month", "monthly":
		cfg.Cycle = "month"
	case "quarter", "quarterly", "season":
		cfg.Cycle = "quarter"
	case "year", "yearly":
		cfg.Cycle = "year"
	default:
		cfg.Cycle = ""
	}
	cfg.CostCurrency = strings.ToUpper(strings.TrimSpace(cfg.CostCurrency))
	if cfg.CostCurrency == "" && cfg.CostAmount > 0 {
		cfg.CostCurrency = "USD"
	}
	switch strings.ToLower(strings.TrimSpace(cfg.CostCycle)) {
	case "month", "monthly":
		cfg.CostCycle = "month"
	case "quarter", "quarterly", "season":
		cfg.CostCycle = "quarter"
	case "year", "yearly":
		cfg.CostCycle = "year"
	default:
		cfg.CostCycle = ""
	}
	if cfg.CostCycle == "" && cfg.CostAmount > 0 {
		switch cfg.Cycle {
		case "quarter", "year":
			cfg.CostCycle = cfg.Cycle
		default:
			cfg.CostCycle = "month"
		}
	}
	if cfg.CostAmount < 0 {
		cfg.CostAmount = 0
	}
	cfg.RevenueCurrency = strings.ToUpper(strings.TrimSpace(cfg.RevenueCurrency))
	if cfg.RevenueCurrency == "" && cfg.RevenueAmount > 0 {
		cfg.RevenueCurrency = "CNY"
	}
	if cfg.RevenueCurrency != "" && cfg.RevenueCurrency != "CNY" && cfg.RevenueCurrency != "USDT" {
		cfg.RevenueCurrency = "CNY"
	}
	switch strings.ToLower(strings.TrimSpace(cfg.RevenueCycle)) {
	case "month", "monthly":
		cfg.RevenueCycle = "month"
	case "quarter", "quarterly", "season":
		cfg.RevenueCycle = "quarter"
	case "year", "yearly":
		cfg.RevenueCycle = "year"
	default:
		cfg.RevenueCycle = ""
	}
	if cfg.RevenueCycle == "" && cfg.RevenueAmount > 0 {
		cfg.RevenueCycle = "month"
	}
	if cfg.RevenueAmount < 0 {
		cfg.RevenueAmount = 0
	}
	if cfg.Cycle == "" {
		cfg.AutoRenew = false
	}
	if cfg.TrafficLimitBytes == 0 {
		cfg.TrafficBaselineBytes = 0
		cfg.TrafficSentBaselineBytes = 0
		cfg.TrafficRecvBaselineBytes = 0
		cfg.TrafficBaselinePeriodStart = ""
	}
	if cfg.BandwidthMbps < 0 {
		cfg.BandwidthMbps = 0
	}
	if cfg.StartDate == "" && cfg.ExpireDate == "" {
		cfg.Enabled = false
	}
	return cfg
}

func hasRenewalConfig(cfg model.VPSRenewalConfig) bool {
	cfg = normalizeRenewalConfig(cfg)
	return cfg.Enabled || cfg.StartDate != "" || cfg.ExpireDate != "" || cfg.Cycle != "" || cfg.AutoRenew || cfg.CostAmount > 0 || cfg.RevenueAmount > 0 || cfg.TrafficLimitBytes > 0 || cfg.BandwidthMbps > 0
}

func normalizeEntryConfig(cfg model.AgentEntryConfig) model.AgentEntryConfig {
	cfg.Addresses = normalizeEntryAddresses(cfg.Addresses)
	mappings := make([]model.AgentEntryMapping, 0, len(cfg.Mappings))
	seen := make(map[string]struct{}, len(cfg.Mappings))
	for _, mapping := range cfg.Mappings {
		mapping.Address = strings.TrimSpace(mapping.Address)
		mapping.Protocol = normalizeEntryProtocol(mapping.Protocol)
		mapping.Note = strings.TrimSpace(mapping.Note)
		if mapping.ExternalPort < 0 {
			mapping.ExternalPort = 0
		}
		if mapping.InternalPort < 0 {
			mapping.InternalPort = 0
		}
		if mapping.Address == "" || mapping.ExternalPort == 0 || mapping.Protocol == "" {
			continue
		}
		if mapping.InternalPort == 0 {
			mapping.InternalPort = mapping.ExternalPort
		}
		key := fmt.Sprintf("%s:%d:%d:%s", strings.ToLower(mapping.Address), mapping.ExternalPort, mapping.InternalPort, mapping.Protocol)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		mappings = append(mappings, mapping)
	}
	sort.Slice(mappings, func(i, j int) bool {
		if strings.ToLower(mappings[i].Address) != strings.ToLower(mappings[j].Address) {
			return strings.ToLower(mappings[i].Address) < strings.ToLower(mappings[j].Address)
		}
		if mappings[i].ExternalPort != mappings[j].ExternalPort {
			return mappings[i].ExternalPort < mappings[j].ExternalPort
		}
		if mappings[i].InternalPort != mappings[j].InternalPort {
			return mappings[i].InternalPort < mappings[j].InternalPort
		}
		return mappings[i].Protocol < mappings[j].Protocol
	})
	cfg.Mappings = mappings
	return cfg
}

func normalizeEntryAddresses(addresses []string) []string {
	if len(addresses) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(addresses))
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		key := strings.ToLower(address)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, address)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

func normalizeEntryProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "vless":
		return "vless"
	case "vmess":
		return "vmess"
	case "http":
		return "http"
	case "socks", "socks5":
		return "socks"
	default:
		return ""
	}
}

func hasEntryConfig(cfg model.AgentEntryConfig) bool {
	cfg = normalizeEntryConfig(cfg)
	return len(cfg.Addresses) > 0 || len(cfg.Mappings) > 0
}

func hasXUIConfig(cfg config.XUIConfig) bool {
	return cfg.Enabled || cfg.BaseURL != "" || cfg.Username != "" || cfg.Password != "" || cfg.TwoFactorCode != "" || cfg.SkipTLSVerify
}

func isValidXUIActionKind(kind string) bool {
	switch kind {
	case model.XUIActionAddOutbound, model.XUIActionAddRoutingRule:
		return true
	default:
		return false
	}
}
