package model

import (
	"encoding/json"
	"time"
)

type AgentSnapshot struct {
	AgentID       string          `json:"agent_id"`
	AgentName     string          `json:"agent_name,omitempty"`
	Version       string          `json:"version,omitempty"`
	OS            string          `json:"os,omitempty"`
	Arch          string          `json:"arch,omitempty"`
	SystemVersion string          `json:"system_version,omitempty"`
	ReportedAt    time.Time       `json:"reported_at"`
	Summary       VPSSummary      `json:"summary"`
	XUI           *XUISnapshot    `json:"xui,omitempty"`
	Realm         *RealmSnapshot  `json:"realm,omitempty"`
	Nezha         *NezhaSnapshot  `json:"nezha,omitempty"`
	Logs          []AgentLogEntry `json:"logs,omitempty"`
}

type AgentLogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level,omitempty"`
	Source  string    `json:"source,omitempty"`
	Message string    `json:"message"`
}

type AgentLogsResponse struct {
	AgentID           string          `json:"agent_id"`
	ReportedAt        time.Time       `json:"reported_at"`
	LastCollectionErr string          `json:"last_collection_err,omitempty"`
	Logs              []AgentLogEntry `json:"logs"`
}

type VPSSummary struct {
	Hostname          string  `json:"hostname,omitempty"`
	ObservedIP        string  `json:"observed_ip,omitempty"`
	PublicIPv4        string  `json:"public_ipv4,omitempty"`
	PublicIPv6        string  `json:"public_ipv6,omitempty"`
	CPU               float64 `json:"cpu,omitempty"`
	MemUsed           uint64  `json:"mem_used,omitempty"`
	MemTotal          uint64  `json:"mem_total,omitempty"`
	NetTrafficSent    uint64  `json:"net_traffic_sent,omitempty"`
	NetTrafficRecv    uint64  `json:"net_traffic_recv,omitempty"`
	NetTrafficTotal   uint64  `json:"net_traffic_total,omitempty"`
	NetIOUp           uint64  `json:"net_io_up,omitempty"`
	NetIODown         uint64  `json:"net_io_down,omitempty"`
	XrayState         string  `json:"xray_state,omitempty"`
	InboundCount      int     `json:"inbound_count,omitempty"`
	OutboundCount     int     `json:"outbound_count,omitempty"`
	RoutingRuleCount  int     `json:"routing_rule_count,omitempty"`
	NezhaServerID     uint64  `json:"nezha_server_id,omitempty"`
	NezhaServerName   string  `json:"nezha_server_name,omitempty"`
	LastCollectionErr string  `json:"last_collection_err,omitempty"`
}

type XUISnapshot struct {
	BaseURL         string                `json:"base_url"`
	AppVersion      string                `json:"app_version,omitempty"`
	CollectedAt     time.Time             `json:"collected_at"`
	Error           string                `json:"error,omitempty"`
	ServerStatus    XUIServerStatus       `json:"server_status"`
	Inbounds        []map[string]any      `json:"inbounds,omitempty"`
	Outbounds       []map[string]any      `json:"outbounds,omitempty"`
	RoutingRules    []map[string]any      `json:"routing_rules,omitempty"`
	OutboundTraffic []map[string]any      `json:"outbound_traffic,omitempty"`
	Certificates    []XUILocalCertificate `json:"certificates,omitempty"`
	RawConfig       map[string]any        `json:"raw_config,omitempty"`
}

type XUILocalCertificate struct {
	ID        string     `json:"id"`
	Name      string     `json:"name,omitempty"`
	Subject   string     `json:"subject,omitempty"`
	Issuer    string     `json:"issuer,omitempty"`
	DNSNames  []string   `json:"dns_names,omitempty"`
	CertPath  string     `json:"cert_path,omitempty"`
	KeyPath   string     `json:"key_path,omitempty"`
	SourceDir string     `json:"source_dir,omitempty"`
	NotAfter  *time.Time `json:"not_after,omitempty"`
}

type RealmSnapshot struct {
	ConfigPath  string             `json:"config_path,omitempty"`
	ServiceName string             `json:"service_name,omitempty"`
	BinaryPath  string             `json:"binary_path,omitempty"`
	CollectedAt time.Time          `json:"collected_at"`
	Error       string             `json:"error,omitempty"`
	Rules       []RealmForwardRule `json:"rules,omitempty"`
}

type XUIServerStatus struct {
	CPU        float64       `json:"cpu"`
	Uptime     uint64        `json:"uptime"`
	Loads      []float64     `json:"loads,omitempty"`
	TCPCount   int           `json:"tcp_count"`
	UDPCount   int           `json:"udp_count"`
	PublicIP   XUIPublicIP   `json:"public_ip"`
	Mem        XUIUsage      `json:"mem"`
	Swap       XUIUsage      `json:"swap"`
	Disk       XUIUsage      `json:"disk"`
	NetIO      XUINetIO      `json:"net_io"`
	NetTraffic XUINetTraffic `json:"net_traffic"`
	Xray       XUIXrayStatus `json:"xray"`
	AppStats   XUIAppStats   `json:"app_stats"`
}

func (s *XUIServerStatus) UnmarshalJSON(data []byte) error {
	type alias XUIServerStatus
	var raw struct {
		alias
		PublicIP   *XUIPublicIP   `json:"publicIP"`
		TCPCount   *int           `json:"tcpCount"`
		UDPCount   *int           `json:"udpCount"`
		NetIO      *XUINetIO      `json:"netIO"`
		NetTraffic *XUINetTraffic `json:"netTraffic"`
		Xray       *XUIXrayStatus `json:"xray"`
		AppStats   *XUIAppStats   `json:"appStats"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = XUIServerStatus(raw.alias)
	if raw.PublicIP != nil {
		s.PublicIP = *raw.PublicIP
	}
	if raw.TCPCount != nil {
		s.TCPCount = *raw.TCPCount
	}
	if raw.UDPCount != nil {
		s.UDPCount = *raw.UDPCount
	}
	if raw.NetIO != nil {
		s.NetIO = *raw.NetIO
	}
	if raw.NetTraffic != nil {
		s.NetTraffic = *raw.NetTraffic
	}
	if raw.Xray != nil {
		s.Xray = *raw.Xray
	}
	if raw.AppStats != nil {
		s.AppStats = *raw.AppStats
	}
	return nil
}

type XUIPublicIP struct {
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
}

type XUIUsage struct {
	Current uint64 `json:"current"`
	Total   uint64 `json:"total"`
}

type XUINetIO struct {
	Up   uint64 `json:"up"`
	Down uint64 `json:"down"`
}

type XUINetTraffic struct {
	Sent uint64 `json:"sent"`
	Recv uint64 `json:"recv"`
}

type XUIXrayStatus struct {
	State    string `json:"state"`
	ErrorMsg string `json:"error_msg,omitempty"`
	Version  string `json:"version,omitempty"`
}

type XUIAppStats struct {
	Threads uint32 `json:"threads"`
	Mem     uint64 `json:"mem"`
	Uptime  uint64 `json:"uptime"`
}

type NezhaSnapshot struct {
	BaseURL     string         `json:"base_url"`
	CollectedAt time.Time      `json:"collected_at"`
	Error       string         `json:"error,omitempty"`
	ServerID    uint64         `json:"server_id,omitempty"`
	ServerUUID  string         `json:"server_uuid,omitempty"`
	ServerName  string         `json:"server_name,omitempty"`
	RawServer   map[string]any `json:"raw_server,omitempty"`
}

type AgentListItem struct {
	AgentID             string           `json:"agent_id"`
	AgentName           string           `json:"agent_name,omitempty"`
	CustomerDisplayName string           `json:"customer_display_name,omitempty"`
	ClientVersion       string           `json:"client_version,omitempty"`
	ClientOS            string           `json:"client_os,omitempty"`
	ClientArch          string           `json:"client_arch,omitempty"`
	SystemVersion       string           `json:"system_version,omitempty"`
	SortOrder           int              `json:"sort_order,omitempty"`
	Tags                []string         `json:"tags,omitempty"`
	Renewal             VPSRenewalConfig `json:"renewal,omitempty"`
	Entry               AgentEntryConfig `json:"entry,omitempty"`
	ReportedAt          *time.Time       `json:"reported_at,omitempty"`
	RealtimeAt          *time.Time       `json:"realtime_at,omitempty"`
	RegisteredAt        *time.Time       `json:"registered_at,omitempty"`
	UpdatedAt           *time.Time       `json:"updated_at,omitempty"`
	LastSeenAt          *time.Time       `json:"last_seen_at,omitempty"`
	HasConfig           bool             `json:"has_config"`
	Summary             VPSSummary       `json:"summary"`
	Geo                 *IPGeoView       `json:"geo,omitempty"`
}
