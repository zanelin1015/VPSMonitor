package model

import "time"

type AgentRealtimeMetrics struct {
	AgentID       string     `json:"agent_id"`
	AgentName     string     `json:"agent_name,omitempty"`
	ClientVersion string     `json:"client_version,omitempty"`
	ClientOS      string     `json:"client_os,omitempty"`
	ClientArch    string     `json:"client_arch,omitempty"`
	SystemVersion string     `json:"system_version,omitempty"`
	ReportedAt    time.Time  `json:"reported_at"`
	Summary       VPSSummary `json:"summary"`
}

type DashboardRealtimeMessage struct {
	Type    string                 `json:"type"`
	Metrics []AgentRealtimeMetrics `json:"metrics,omitempty"`
	Metric  *AgentRealtimeMetrics  `json:"metric,omitempty"`
}

const (
	AgentControlCollectNow = "collect_now"
	AgentControlRestartXUI = "restart_xui"
)

type AgentControlMessage struct {
	Type     string         `json:"type"`
	ActionID int64          `json:"action_id,omitempty"`
	Payload  map[string]any `json:"payload,omitempty"`
}

type AgentRefreshResponse struct {
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	Message string `json:"message,omitempty"`
}
