package model

import "time"

type DashboardTotals struct {
	AgentCount        int `json:"agent_count"`
	TaggedAgentCount  int `json:"tagged_agent_count"`
	NodeCount         int `json:"node_count"`
	ClientCount       int `json:"client_count"`
	OnlineClientCount int `json:"online_client_count"`
	OutboundCount     int `json:"outbound_count"`
	RoutingRuleCount  int `json:"routing_rule_count"`
	LinkCount         int `json:"link_count"`
	ChainCount        int `json:"chain_count"`
}

type DashboardTagView struct {
	Tag               string `json:"tag"`
	AgentCount        int    `json:"agent_count"`
	NodeCount         int    `json:"node_count"`
	ClientCount       int    `json:"client_count"`
	OnlineClientCount int    `json:"online_client_count"`
}

type DashboardAgentView struct {
	AgentID           string           `json:"agent_id"`
	AgentName         string           `json:"agent_name,omitempty"`
	SortOrder         int              `json:"sort_order,omitempty"`
	Tags              []string         `json:"tags,omitempty"`
	Renewal           VPSRenewalConfig `json:"renewal,omitempty"`
	Entry             AgentEntryConfig `json:"entry,omitempty"`
	ReportedAt        *time.Time       `json:"reported_at,omitempty"`
	RealtimeAt        *time.Time       `json:"realtime_at,omitempty"`
	RegisteredAt      *time.Time       `json:"registered_at,omitempty"`
	UpdatedAt         *time.Time       `json:"updated_at,omitempty"`
	LastSeenAt        *time.Time       `json:"last_seen_at,omitempty"`
	HasConfig         bool             `json:"has_config"`
	Summary           VPSSummary       `json:"summary"`
	Geo               *IPGeoView       `json:"geo,omitempty"`
	NodeCount         int              `json:"node_count"`
	ClientCount       int              `json:"client_count"`
	OnlineClientCount int              `json:"online_client_count"`
	OutboundCount     int              `json:"outbound_count"`
	RoutingRuleCount  int              `json:"routing_rule_count"`
}

type IPGeoView struct {
	IP          string `json:"ip,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	RegionName  string `json:"region_name,omitempty"`
	City        string `json:"city,omitempty"`
}

type TopologyInboundRef struct {
	AgentID        string                 `json:"agent_id"`
	AgentName      string                 `json:"agent_name,omitempty"`
	AgentTags      []string               `json:"agent_tags,omitempty"`
	InboundID      int                    `json:"inbound_id"`
	InboundTag     string                 `json:"inbound_tag,omitempty"`
	InboundName    string                 `json:"inbound_name,omitempty"`
	Protocol       string                 `json:"protocol,omitempty"`
	Port           int                    `json:"port,omitempty"`
	Network        string                 `json:"network,omitempty"`
	Security       string                 `json:"security,omitempty"`
	WSPath         string                 `json:"ws_path,omitempty"`
	WSHost         string                 `json:"ws_host,omitempty"`
	Domains        []string               `json:"domains,omitempty"`
	IPs            []string               `json:"ips,omitempty"`
	ResolvedIPs    []string               `json:"resolved_ips,omitempty"`
	EntryAddresses []string               `json:"entry_addresses,omitempty"`
	EntryIPs       []string               `json:"entry_ips,omitempty"`
	EntryMappings  []TopologyEntryMapping `json:"entry_mappings,omitempty"`
	AuthKeys       []string               `json:"-"`
}

type TopologyEntryMapping struct {
	Address      string   `json:"address,omitempty"`
	ExternalPort int      `json:"external_port,omitempty"`
	InternalPort int      `json:"internal_port,omitempty"`
	Protocol     string   `json:"protocol,omitempty"`
	Note         string   `json:"note,omitempty"`
	ResolvedIPs  []string `json:"resolved_ips,omitempty"`
}

type TopologyOutboundRef struct {
	AgentID       string     `json:"agent_id"`
	AgentName     string     `json:"agent_name,omitempty"`
	AgentTags     []string   `json:"agent_tags,omitempty"`
	OutboundTag   string     `json:"outbound_tag,omitempty"`
	Protocol      string     `json:"protocol,omitempty"`
	Target        string     `json:"target,omitempty"`
	Address       string     `json:"address,omitempty"`
	Port          int        `json:"port,omitempty"`
	Network       string     `json:"network,omitempty"`
	Security      string     `json:"security,omitempty"`
	TLSServerName string     `json:"tls_server_name,omitempty"`
	WSPath        string     `json:"ws_path,omitempty"`
	WSHost        string     `json:"ws_host,omitempty"`
	ResolvedIPs   []string   `json:"resolved_ips,omitempty"`
	TargetIP      string     `json:"target_ip,omitempty"`
	TargetGeo     *IPGeoView `json:"target_geo,omitempty"`
	AuthKeys      []string   `json:"-"`
}

type TopologyLinkView struct {
	Key              string              `json:"key"`
	Source           TopologyOutboundRef `json:"source"`
	Target           TopologyInboundRef  `json:"target"`
	MatchScore       int                 `json:"match_score"`
	MatchConfidence  string              `json:"match_confidence,omitempty"`
	MatchReason      string              `json:"match_reason,omitempty"`
	MatchExplanation string              `json:"match_explanation,omitempty"`
	MatchFields      []string            `json:"match_fields,omitempty"`
}

type ClientChainStep struct {
	StepType    string     `json:"step_type"`
	AgentID     string     `json:"agent_id"`
	AgentName   string     `json:"agent_name,omitempty"`
	AgentTags   []string   `json:"agent_tags,omitempty"`
	Label       string     `json:"label"`
	Detail      string     `json:"detail,omitempty"`
	Protocol    string     `json:"protocol,omitempty"`
	Port        int        `json:"port,omitempty"`
	RouteScope  string     `json:"route_scope,omitempty"`
	RuleIndex   int        `json:"rule_index,omitempty"`
	OutboundTag string     `json:"outbound_tag,omitempty"`
	Target      string     `json:"target,omitempty"`
	TargetIP    string     `json:"target_ip,omitempty"`
	TargetGeo   *IPGeoView `json:"target_geo,omitempty"`
	MatchReason string     `json:"match_reason,omitempty"`
}

type ClientChainView struct {
	Key              string            `json:"key"`
	RootAgentID      string            `json:"root_agent_id"`
	RootAgentName    string            `json:"root_agent_name,omitempty"`
	RootAgentTags    []string          `json:"root_agent_tags,omitempty"`
	RootClientEmail  string            `json:"root_client_email,omitempty"`
	RootClientRemark string            `json:"root_client_remark,omitempty"`
	RootInboundTag   string            `json:"root_inbound_tag,omitempty"`
	MatchedLinkCount int               `json:"matched_link_count"`
	LoopDetected     bool              `json:"loop_detected,omitempty"`
	UnresolvedReason string            `json:"unresolved_reason,omitempty"`
	Steps            []ClientChainStep `json:"steps"`
}

type GlobalDashboardView struct {
	GeneratedAt  time.Time            `json:"generated_at"`
	Totals       DashboardTotals      `json:"totals"`
	Tags         []DashboardTagView   `json:"tags"`
	Agents       []DashboardAgentView `json:"agents"`
	Links        []TopologyLinkView   `json:"links"`
	ClientChains []ClientChainView    `json:"client_chains"`
}
