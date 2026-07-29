package model

import "time"

type XUIOverview struct {
	AgentID           string                `json:"agent_id"`
	AgentName         string                `json:"agent_name,omitempty"`
	BaseURL           string                `json:"base_url,omitempty"`
	ReportedAt        time.Time             `json:"reported_at"`
	CollectedAt       time.Time             `json:"collected_at"`
	Summary           VPSSummary            `json:"summary"`
	NodeCount         int                   `json:"node_count"`
	ClientCount       int                   `json:"client_count"`
	OnlineClientCount int                   `json:"online_client_count"`
	Nodes             []XUINodeView         `json:"nodes"`
	Clients           []XUIClientView       `json:"clients"`
	Outbounds         []XUIOutboundView     `json:"outbounds"`
	Balancers         []XUIBalancerView     `json:"balancers,omitempty"`
	RoutingRules      []XUIRoutingRuleView  `json:"routing_rules"`
	Certificates      []XUILocalCertificate `json:"certificates"`
}

type XUIRouteTrace struct {
	MatchScope        string `json:"match_scope"`
	RuleIndex         int    `json:"rule_index,omitempty"`
	OutboundTag       string `json:"outbound_tag,omitempty"`
	BalancerTag       string `json:"balancer_tag,omitempty"`
	HasGlobalRules    bool   `json:"has_global_rules,omitempty"`
	GlobalRuleIndexes []int  `json:"global_rule_indexes,omitempty"`
	Note              string `json:"note,omitempty"`
}

type XUINodeView struct {
	ID                  int           `json:"id"`
	Tag                 string        `json:"tag,omitempty"`
	Remark              string        `json:"remark,omitempty"`
	Protocol            string        `json:"protocol,omitempty"`
	Listen              string        `json:"listen,omitempty"`
	Port                int           `json:"port,omitempty"`
	Network             string        `json:"network,omitempty"`
	Security            string        `json:"security,omitempty"`
	TLSServerName       string        `json:"tls_server_name,omitempty"`
	ALPN                string        `json:"alpn,omitempty"`
	WSPath              string        `json:"ws_path,omitempty"`
	WSHost              string        `json:"ws_host,omitempty"`
	GRPCService         string        `json:"grpc_service,omitempty"`
	RealityPubKey       string        `json:"reality_public_key,omitempty"`
	RealityShortID      string        `json:"reality_short_id,omitempty"`
	RealityFingerprint  string        `json:"reality_fingerprint,omitempty"`
	RealitySpiderX      string        `json:"reality_spider_x,omitempty"`
	Enabled             bool          `json:"enabled"`
	ExpiryTime          int64         `json:"expiry_time,omitempty"`
	Up                  int64         `json:"up,omitempty"`
	Down                int64         `json:"down,omitempty"`
	Total               int64         `json:"total,omitempty"`
	AllTime             int64         `json:"all_time,omitempty"`
	ClientCount         int           `json:"client_count,omitempty"`
	OnlineCount         int           `json:"online_count,omitempty"`
	Route               XUIRouteTrace `json:"route"`
	AuthKeys            []string      `json:"-"`
	CanAssignAllClients *bool         `json:"can_assign_all_clients,omitempty"`

	RealmTargetAgentID    string `json:"realm_target_agent_id,omitempty"`
	RealmTargetAgentName  string `json:"realm_target_agent_name,omitempty"`
	RealmTargetInboundID  int    `json:"realm_target_inbound_id,omitempty"`
	RealmTargetInboundTag string `json:"realm_target_inbound_tag,omitempty"`
}

type XUIClientView struct {
	InboundID     int           `json:"inbound_id"`
	InboundTag    string        `json:"inbound_tag,omitempty"`
	InboundRemark string        `json:"inbound_remark,omitempty"`
	Protocol      string        `json:"protocol,omitempty"`
	Email         string        `json:"email,omitempty"`
	Comment       string        `json:"comment,omitempty"`
	Enabled       bool          `json:"enabled"`
	AuthUUID      string        `json:"auth_uuid,omitempty"`
	AuthPassword  string        `json:"auth_password,omitempty"`
	Flow          string        `json:"flow,omitempty"`
	ImportURL     string        `json:"import_url,omitempty"`
	LimitIP       int           `json:"limit_ip,omitempty"`
	TotalGB       int64         `json:"total_gb,omitempty"`
	ExpiryTime    int64         `json:"expiry_time,omitempty"`
	SubID         string        `json:"sub_id,omitempty"`
	CreatedAt     int64         `json:"created_at,omitempty"`
	UpdatedAt     int64         `json:"updated_at,omitempty"`
	Up            int64         `json:"up,omitempty"`
	Down          int64         `json:"down,omitempty"`
	AllTime       int64         `json:"all_time,omitempty"`
	TrafficTotal  int64         `json:"traffic_total,omitempty"`
	LastOnline    int64         `json:"last_online,omitempty"`
	Route         XUIRouteTrace `json:"route"`

	ForwardType           string `json:"forward_type,omitempty"`
	IsRealmForwarded      bool   `json:"is_realm_forwarded,omitempty"`
	RealmListenPort       int    `json:"realm_listen_port,omitempty"`
	RealmListenTag        string `json:"realm_listen_tag,omitempty"`
	RealmSourceAgentID    string `json:"realm_source_agent_id,omitempty"`
	RealmTargetAgentID    string `json:"realm_target_agent_id,omitempty"`
	RealmTargetAgentName  string `json:"realm_target_agent_name,omitempty"`
	RealmTargetInboundID  int    `json:"realm_target_inbound_id,omitempty"`
	RealmTargetInboundTag string `json:"realm_target_inbound_tag,omitempty"`
}

type XUIOutboundView struct {
	Tag           string   `json:"tag,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	Target        string   `json:"target,omitempty"`
	Address       string   `json:"address,omitempty"`
	Port          int      `json:"port,omitempty"`
	SendThrough   string   `json:"send_through,omitempty"`
	Network       string   `json:"network,omitempty"`
	Security      string   `json:"security,omitempty"`
	TLSServerName string   `json:"tls_server_name,omitempty"`
	WSPath        string   `json:"ws_path,omitempty"`
	WSHost        string   `json:"ws_host,omitempty"`
	Up            int64    `json:"up,omitempty"`
	Down          int64    `json:"down,omitempty"`
	Total         int64    `json:"total,omitempty"`
	IsDefault     bool     `json:"is_default,omitempty"`
	AuthKeys      []string `json:"-"`
}

type XUIBalancerView struct {
	Tag          string   `json:"tag,omitempty"`
	Selectors    []string `json:"selectors,omitempty"`
	Strategy     string   `json:"strategy,omitempty"`
	FallbackTag  string   `json:"fallback_tag,omitempty"`
	OutboundTags []string `json:"outbound_tags,omitempty"`
}

type XUIRoutingRuleView struct {
	Index       int      `json:"index"`
	Type        string   `json:"type,omitempty"`
	InboundTags []string `json:"inbound_tags,omitempty"`
	Users       []string `json:"users,omitempty"`
	OutboundTag string   `json:"outbound_tag,omitempty"`
	BalancerTag string   `json:"balancer_tag,omitempty"`
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Port        []string `json:"port,omitempty"`
	SourcePort  []string `json:"source_port,omitempty"`
	SourceIP    []string `json:"source_ip,omitempty"`
	Network     []string `json:"network,omitempty"`
	Protocol    []string `json:"protocol,omitempty"`
	VLESSRoute  []string `json:"vless_route,omitempty"`
	Summary     string   `json:"summary,omitempty"`
}
