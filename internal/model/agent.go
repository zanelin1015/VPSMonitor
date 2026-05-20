package model

import (
	"time"

	"bridge-core/internal/config"
)

type ManagedAgentConfig struct {
	AgentID             string           `json:"agent_id,omitempty"`
	AgentName           string           `json:"agent_name,omitempty"`
	CustomerDisplayName string           `json:"customer_display_name,omitempty"`
	SortOrder           int              `json:"sort_order,omitempty"`
	Tags                []string         `json:"tags,omitempty"`
	Renewal             VPSRenewalConfig `json:"renewal,omitempty"`
	Entry               AgentEntryConfig `json:"entry,omitempty"`
	XUI                 config.XUIConfig `json:"xui"`
}

type VPSRenewalConfig struct {
	Enabled                    bool                     `json:"enabled,omitempty"`
	StartDate                  string                   `json:"start_date,omitempty"`
	ExpireDate                 string                   `json:"expire_date,omitempty"`
	Cycle                      string                   `json:"cycle,omitempty"`
	AutoRenew                  bool                     `json:"auto_renew,omitempty"`
	CostAmount                 float64                  `json:"cost_amount,omitempty"`
	CostCurrency               string                   `json:"cost_currency,omitempty"`
	CostCycle                  string                   `json:"cost_cycle,omitempty"`
	ClientBillings             []XUIClientBillingConfig `json:"client_billings,omitempty"`
	TrafficLimitBytes          uint64                   `json:"traffic_limit_bytes,omitempty"`
	BandwidthMbps              float64                  `json:"bandwidth_mbps,omitempty"`
	TrafficBaselineBytes       uint64                   `json:"traffic_baseline_bytes,omitempty"`
	TrafficSentBaselineBytes   uint64                   `json:"traffic_sent_baseline_bytes,omitempty"`
	TrafficRecvBaselineBytes   uint64                   `json:"traffic_recv_baseline_bytes,omitempty"`
	TrafficBaselinePeriodStart string                   `json:"traffic_baseline_period_start,omitempty"`
}

type XUIClientBillingConfig struct {
	InboundID       int     `json:"inbound_id,omitempty"`
	InboundTag      string  `json:"inbound_tag,omitempty"`
	Email           string  `json:"email,omitempty"`
	RevenueAmount   float64 `json:"revenue_amount,omitempty"`
	RevenueCurrency string  `json:"revenue_currency,omitempty"`
	RevenueCycle    string  `json:"revenue_cycle,omitempty"`
	ExpireTime      int64   `json:"expire_time,omitempty"`
	ExpireCycle     string  `json:"expire_cycle,omitempty"`
	ExpireAutoRenew bool    `json:"expire_auto_renew,omitempty"`
}

type AgentEntryConfig struct {
	Addresses      []string            `json:"addresses,omitempty"`
	ImportDomain   string              `json:"import_domain,omitempty"`
	Mappings       []AgentEntryMapping `json:"mappings,omitempty"`
	NetworkPolicy  NetworkPolicyConfig `json:"network_policy,omitempty"`
	PortForwarding RealmForwardConfig  `json:"port_forwarding,omitempty"`
}

type AgentEntryMapping struct {
	Address      string `json:"address,omitempty"`
	ExternalPort int    `json:"external_port,omitempty"`
	InternalPort int    `json:"internal_port,omitempty"`
	Protocol     string `json:"protocol,omitempty"`
	Note         string `json:"note,omitempty"`
}

type NetworkPolicyConfig struct {
	Enabled          bool                    `json:"enabled,omitempty"`
	Interface        string                  `json:"interface,omitempty"`
	FirewallBackend  string                  `json:"firewall_backend,omitempty"`
	RateLimitBackend string                  `json:"rate_limit_backend,omitempty"`
	Rules            []NetworkPortPolicyRule `json:"rules,omitempty"`
}

type NetworkPortPolicyRule struct {
	ID            string   `json:"id,omitempty"`
	Name          string   `json:"name,omitempty"`
	Enabled       bool     `json:"enabled,omitempty"`
	Port          int      `json:"port,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	RateLimitMbps float64  `json:"rate_limit_mbps,omitempty"`
	WhitelistIPs  []string `json:"whitelist_ips,omitempty"`
}

type RealmForwardConfig struct {
	Enabled     bool               `json:"enabled,omitempty"`
	Backend     string             `json:"backend,omitempty"`
	BinaryPath  string             `json:"binary_path,omitempty"`
	ConfigPath  string             `json:"config_path,omitempty"`
	ServiceName string             `json:"service_name,omitempty"`
	LogLevel    string             `json:"log_level,omitempty"`
	Rules       []RealmForwardRule `json:"rules,omitempty"`
}

type RealmForwardRule struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	Enabled       bool   `json:"enabled,omitempty"`
	ListenAddress string `json:"listen_address,omitempty"`
	ListenPort    int    `json:"listen_port,omitempty"`
	TargetAgentID string `json:"target_agent_id,omitempty"`
	TargetAddress string `json:"target_address,omitempty"`
	TargetPort    int    `json:"target_port,omitempty"`
	Network       string `json:"network,omitempty"`
	Note          string `json:"note,omitempty"`
}

type AgentRegisterRequest struct {
	AgentID       string             `json:"agent_id"`
	AgentName     string             `json:"agent_name,omitempty"`
	Version       string             `json:"version,omitempty"`
	OS            string             `json:"os,omitempty"`
	Arch          string             `json:"arch,omitempty"`
	SystemVersion string             `json:"system_version,omitempty"`
	Hostname      string             `json:"hostname,omitempty"`
	PublicIPv4    string             `json:"public_ipv4,omitempty"`
	PublicIPv6    string             `json:"public_ipv6,omitempty"`
	SeedConfig    ManagedAgentConfig `json:"seed_config"`
}

type AgentRegisterResponse struct {
	AgentID      string             `json:"agent_id"`
	AgentName    string             `json:"agent_name,omitempty"`
	AgentToken   string             `json:"agent_token"`
	RegisteredAt time.Time          `json:"registered_at"`
	Config       ManagedAgentConfig `json:"config"`
}

type AgentRecord struct {
	AgentID             string             `json:"agent_id"`
	AgentName           string             `json:"agent_name,omitempty"`
	CustomerDisplayName string             `json:"customer_display_name,omitempty"`
	Version             string             `json:"version,omitempty"`
	OS                  string             `json:"os,omitempty"`
	Arch                string             `json:"arch,omitempty"`
	SystemVersion       string             `json:"system_version,omitempty"`
	SortOrder           int                `json:"sort_order,omitempty"`
	Tags                []string           `json:"tags,omitempty"`
	AgentToken          string             `json:"-"`
	Hostname            string             `json:"hostname,omitempty"`
	PublicIPv4          string             `json:"public_ipv4,omitempty"`
	PublicIPv6          string             `json:"public_ipv6,omitempty"`
	RegisteredAt        time.Time          `json:"registered_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
	LastSeenAt          *time.Time         `json:"last_seen_at,omitempty"`
	ReportedAt          *time.Time         `json:"reported_at,omitempty"`
	Summary             VPSSummary         `json:"summary"`
	HasConfig           bool               `json:"has_config"`
	Config              ManagedAgentConfig `json:"config,omitempty"`
}
