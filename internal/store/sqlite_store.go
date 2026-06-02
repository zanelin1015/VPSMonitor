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
	"net"
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

func (s *SQLiteStore) parseManagedConfig(agentID, agentName, customerDisplayName string, sortOrder int, tagsJSON, xuiJSON, nezhaJSON, renewalJSON, entryJSON string) (model.ManagedAgentConfig, error) {
	cfg := model.ManagedAgentConfig{
		AgentID:             agentID,
		AgentName:           agentName,
		CustomerDisplayName: strings.TrimSpace(customerDisplayName),
		SortOrder:           sortOrder,
	}
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &cfg.Tags)
		cfg.Tags = normalizeTags(cfg.Tags)
		if strings.HasPrefix(strings.TrimSpace(tagsJSON), "{") {
			var tagPayload struct {
				Tags     []string                 `json:"tags"`
				Features model.AgentFeatureConfig `json:"features"`
			}
			if err := json.Unmarshal([]byte(tagsJSON), &tagPayload); err == nil {
				cfg.Tags = normalizeTags(tagPayload.Tags)
				cfg.Features = tagPayload.Features
			}
		}
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

func managedTagsJSON(cfg model.ManagedAgentConfig) string {
	if cfg.Features.Configured || hasAgentFeatures(cfg.Features) {
		return mustJSON(struct {
			Tags     []string                 `json:"tags"`
			Features model.AgentFeatureConfig `json:"features"`
		}{
			Tags:     cfg.Tags,
			Features: cfg.Features,
		})
	}
	return mustJSON(cfg.Tags)
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
	return hasXUIConfig(cfg.XUI) || hasRenewalConfig(cfg.Renewal) || hasEntryConfig(cfg.Entry) || hasAgentFeatures(cfg.Features)
}

func hasAgentFeatures(features model.AgentFeatureConfig) bool {
	return features.XUI || features.Realm || features.NAT || features.PortPolicy
}

func mergeAgentFeatures(base model.AgentFeatureConfig, incoming model.AgentFeatureConfig) model.AgentFeatureConfig {
	return model.AgentFeatureConfig{
		XUI:        base.XUI || incoming.XUI,
		Realm:      base.Realm || incoming.Realm,
		NAT:        base.NAT || incoming.NAT,
		PortPolicy: base.PortPolicy || incoming.PortPolicy,
	}
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
	cfg.ClientBillings = normalizeClientBillings(cfg.ClientBillings)
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

func normalizeClientBillings(items []model.XUIClientBillingConfig) []model.XUIClientBillingConfig {
	normalized := make([]model.XUIClientBillingConfig, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item.InboundTag = strings.TrimSpace(item.InboundTag)
		item.Email = strings.TrimSpace(item.Email)
		if item.InboundID <= 0 && item.InboundTag == "" && item.Email == "" {
			continue
		}
		if item.RevenueAmount < 0 {
			item.RevenueAmount = 0
		}
		item.RevenueCurrency = strings.ToUpper(strings.TrimSpace(item.RevenueCurrency))
		if item.RevenueCurrency != "USDT" {
			item.RevenueCurrency = "CNY"
		}
		switch strings.ToLower(strings.TrimSpace(item.RevenueCycle)) {
		case "quarter", "quarterly", "season":
			item.RevenueCycle = "quarter"
		case "year", "yearly":
			item.RevenueCycle = "year"
		default:
			item.RevenueCycle = "month"
		}
		if item.StartTime < 0 {
			item.StartTime = 0
		}
		if item.ExpireTime < 0 {
			item.ExpireTime = 0
		}
		// Client time periods follow the price cycle so billing and expiry stay in sync.
		item.ExpireCycle = item.RevenueCycle
		if item.StartTime > 0 {
			item.ExpireTime = calculateClientBillingExpireTime(item.StartTime, item.RevenueCycle, item.ExpireAutoRenew, time.Now())
		}
		key := fmt.Sprintf("%d\x00%s\x00%s", item.InboundID, item.InboundTag, item.Email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func calculateClientBillingExpireTime(startMillis int64, cycle string, autoRenew bool, now time.Time) int64 {
	if startMillis <= 0 {
		return 0
	}
	periodStart := time.UnixMilli(startMillis)
	nextStart := addClientBillingCycle(periodStart, cycle)
	periodEnd := nextStart.Add(-time.Second)
	for autoRenew && !periodEnd.After(now) {
		periodStart = nextStart
		nextStart = addClientBillingCycle(periodStart, cycle)
		periodEnd = nextStart.Add(-time.Second)
	}
	return periodEnd.UnixMilli()
}

func addClientBillingCycle(value time.Time, cycle string) time.Time {
	switch cycle {
	case "quarter":
		return value.AddDate(0, 3, 0)
	case "year":
		return value.AddDate(1, 0, 0)
	default:
		return value.AddDate(0, 1, 0)
	}
}

func hasRenewalConfig(cfg model.VPSRenewalConfig) bool {
	cfg = normalizeRenewalConfig(cfg)
	return cfg.Enabled || cfg.StartDate != "" || cfg.ExpireDate != "" || cfg.Cycle != "" || cfg.AutoRenew || cfg.CostAmount > 0 || len(cfg.ClientBillings) > 0 || cfg.TrafficLimitBytes > 0 || cfg.BandwidthMbps > 0
}

func normalizeEntryConfig(cfg model.AgentEntryConfig) model.AgentEntryConfig {
	cfg.Addresses = normalizeEntryAddresses(cfg.Addresses)
	cfg.ImportDomain = normalizeEntryImportDomain(cfg.ImportDomain)
	cfg.NetworkPolicy = normalizeNetworkPolicyConfig(cfg.NetworkPolicy)
	cfg.PortForwarding = normalizeRealmForwardConfig(cfg.PortForwarding)
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

func normalizeEntryImportDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	if slash := strings.Index(domain, "/"); slash >= 0 {
		domain = domain[:slash]
	}
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = strings.Trim(host, "[]")
	}
	domain = strings.Trim(domain, "[]")
	if domain == "" || strings.Contains(domain, " ") || strings.Contains(domain, "*") || strings.Contains(domain, ":") {
		return ""
	}
	if net.ParseIP(domain) != nil {
		return ""
	}
	return domain
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
	return len(cfg.Addresses) > 0 || cfg.ImportDomain != "" || len(cfg.Mappings) > 0 || hasNetworkPolicyConfig(cfg.NetworkPolicy) || hasRealmForwardConfig(cfg.PortForwarding)
}

func normalizeRealmForwardConfig(cfg model.RealmForwardConfig) model.RealmForwardConfig {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "realm", "none":
		cfg.Backend = strings.ToLower(strings.TrimSpace(cfg.Backend))
	default:
		cfg.Backend = "realm"
	}
	cfg.BinaryPath = strings.TrimSpace(cfg.BinaryPath)
	cfg.ConfigPath = strings.TrimSpace(cfg.ConfigPath)
	cfg.ServiceName = strings.TrimSpace(cfg.ServiceName)
	switch strings.ToLower(strings.TrimSpace(cfg.LogLevel)) {
	case "trace", "debug", "warn", "error":
		cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	default:
		cfg.LogLevel = "info"
	}
	rules := make([]model.RealmForwardRule, 0, len(cfg.Rules))
	seen := make(map[string]struct{}, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.ListenAddress = strings.TrimSpace(rule.ListenAddress)
		rule.TargetAgentID = strings.TrimSpace(rule.TargetAgentID)
		rule.TargetAddress = strings.TrimSpace(rule.TargetAddress)
		rule.Network = normalizeRealmForwardNetwork(rule.Network)
		rule.Note = strings.TrimSpace(rule.Note)
		if rule.ListenPort <= 0 || rule.ListenPort > 65535 || rule.TargetPort <= 0 || rule.TargetPort > 65535 {
			continue
		}
		if rule.TargetAddress == "" && rule.TargetAgentID == "" {
			continue
		}
		if rule.ListenAddress == "" {
			rule.ListenAddress = "0.0.0.0"
		}
		if rule.ID == "" {
			rule.ID = fmt.Sprintf("%s-%d-%d-%s", sanitizeRealmForwardID(rule.Name), rule.ListenPort, rule.TargetPort, rule.Network)
		}
		key := fmt.Sprintf("%s:%s:%d:%s:%d:%s", strings.ToLower(rule.ID), strings.ToLower(rule.ListenAddress), rule.ListenPort, strings.ToLower(rule.TargetAddress), rule.TargetPort, rule.Network)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].ListenPort != rules[j].ListenPort {
			return rules[i].ListenPort < rules[j].ListenPort
		}
		return strings.ToLower(rules[i].Name) < strings.ToLower(rules[j].Name)
	})
	cfg.Rules = rules
	if !cfg.Enabled && len(rules) == 0 {
		cfg.Backend = ""
		cfg.BinaryPath = ""
		cfg.ConfigPath = ""
		cfg.ServiceName = ""
		cfg.LogLevel = ""
	}
	return cfg
}

func normalizeRealmForwardNetwork(network string) string {
	return "both"
}

func sanitizeRealmForwardID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		if r == ' ' || r == '.' || r == ':' || r == '/' {
			return '-'
		}
		return -1
	}, value)
	value = strings.Trim(value, "-_")
	if value == "" {
		return "realm"
	}
	return value
}

func normalizeNetworkPolicyConfig(cfg model.NetworkPolicyConfig) model.NetworkPolicyConfig {
	cfg.Interface = strings.TrimSpace(cfg.Interface)
	switch strings.ToLower(strings.TrimSpace(cfg.FirewallBackend)) {
	case "ufw", "iptables", "none":
		cfg.FirewallBackend = strings.ToLower(strings.TrimSpace(cfg.FirewallBackend))
	default:
		cfg.FirewallBackend = "auto"
	}
	switch strings.ToLower(strings.TrimSpace(cfg.RateLimitBackend)) {
	case "tc", "none":
		cfg.RateLimitBackend = strings.ToLower(strings.TrimSpace(cfg.RateLimitBackend))
	default:
		cfg.RateLimitBackend = "auto"
	}
	rulesByPort := make(map[int]model.NetworkPortPolicyRule, len(cfg.Rules))
	portOrder := make([]int, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Protocol = normalizeNetworkPolicyProtocol(rule.Protocol)
		if rule.Port <= 0 || rule.Port > 65535 {
			continue
		}
		if rule.RateLimitMbps < 0 {
			rule.RateLimitMbps = 0
		}
		rule.WhitelistIPs = normalizeNetworkPolicyIPs(rule.WhitelistIPs)
		if !rule.Enabled && rule.RateLimitMbps <= 0 && len(rule.WhitelistIPs) == 0 {
			continue
		}
		if rule.ID == "" {
			rule.ID = fmt.Sprintf("%s-%d-%s", rule.Protocol, rule.Port, strings.ToLower(strings.ReplaceAll(rule.Name, " ", "-")))
		}
		if existing, ok := rulesByPort[rule.Port]; ok {
			existing.Protocol = mergeNetworkPolicyProtocol(existing.Protocol, rule.Protocol)
			existing.WhitelistIPs = normalizeNetworkPolicyIPs(append(existing.WhitelistIPs, rule.WhitelistIPs...))
			if existing.RateLimitMbps <= 0 || (rule.RateLimitMbps > 0 && rule.RateLimitMbps < existing.RateLimitMbps) {
				existing.RateLimitMbps = rule.RateLimitMbps
			}
			if existing.Name == "" {
				existing.Name = rule.Name
			}
			if existing.ID == "" {
				existing.ID = rule.ID
			}
			existing.Enabled = existing.Enabled || rule.Enabled
			rulesByPort[rule.Port] = existing
			continue
		}
		portOrder = append(portOrder, rule.Port)
		rulesByPort[rule.Port] = rule
	}
	rules := make([]model.NetworkPortPolicyRule, 0, len(portOrder))
	for _, port := range portOrder {
		rules = append(rules, rulesByPort[port])
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Port != rules[j].Port {
			return rules[i].Port < rules[j].Port
		}
		return rules[i].Protocol < rules[j].Protocol
	})
	cfg.Rules = rules
	if !cfg.Enabled && len(rules) == 0 {
		cfg.Interface = ""
		cfg.FirewallBackend = ""
		cfg.RateLimitBackend = ""
	}
	return cfg
}

func mergeNetworkPolicyProtocol(a, b string) string {
	seenTCP := false
	seenUDP := false
	for _, protocol := range []string{normalizeNetworkPolicyProtocol(a), normalizeNetworkPolicyProtocol(b)} {
		if protocol == "both" || protocol == "tcp" {
			seenTCP = true
		}
		if protocol == "both" || protocol == "udp" {
			seenUDP = true
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

func normalizeNetworkPolicyProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		return "udp"
	case "both", "all", "tcp+udp":
		return "both"
	default:
		return "tcp"
	}
}

func normalizeNetworkPolicyIPs(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(item)]; ok {
			continue
		}
		seen[strings.ToLower(item)] = struct{}{}
		result = append(result, item)
	}
	return result
}

func hasNetworkPolicyConfig(cfg model.NetworkPolicyConfig) bool {
	cfg = normalizeNetworkPolicyConfig(cfg)
	if !cfg.Enabled {
		return false
	}
	return len(cfg.Rules) > 0 || cfg.Interface != "" || cfg.FirewallBackend != "" || cfg.RateLimitBackend != ""
}

func hasRealmForwardConfig(cfg model.RealmForwardConfig) bool {
	cfg = normalizeRealmForwardConfig(cfg)
	if !cfg.Enabled {
		return false
	}
	return len(cfg.Rules) > 0 || cfg.BinaryPath != "" || cfg.ConfigPath != "" || cfg.ServiceName != ""
}

func hasXUIConfig(cfg config.XUIConfig) bool {
	return cfg.Enabled || cfg.BaseURL != "" || cfg.Username != "" || cfg.Password != "" || cfg.APIToken != "" || cfg.TwoFactorCode != "" || cfg.SkipTLSVerify
}

func isValidXUIActionKind(kind string) bool {
	switch kind {
	case model.XUIActionAddOutbound, model.XUIActionAddClient, model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule, model.XUIActionUpdateClientExpiry, model.XUIActionSetClientEnabled, model.XUIActionDeleteClient, model.XUIActionUpdateClient, model.XUIActionRestartXUI, model.XUIActionExecuteCommand, model.XUIActionUpdate3XUI:
		return true
	default:
		return false
	}
}
