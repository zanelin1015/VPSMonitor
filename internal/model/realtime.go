package model

import "time"

type AgentRealtimeMetrics struct {
	AgentID    string     `json:"agent_id"`
	AgentName  string     `json:"agent_name,omitempty"`
	ReportedAt time.Time  `json:"reported_at"`
	Summary    VPSSummary `json:"summary"`
}

type DashboardRealtimeMessage struct {
	Type    string                 `json:"type"`
	Metrics []AgentRealtimeMetrics `json:"metrics,omitempty"`
	Metric  *AgentRealtimeMetrics  `json:"metric,omitempty"`
}
