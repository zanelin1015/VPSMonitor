package model

import "time"

type AgentRealtimeMetrics struct {
	AgentID       string              `json:"agent_id"`
	AgentName     string              `json:"agent_name,omitempty"`
	ClientVersion string              `json:"client_version,omitempty"`
	ClientOS      string              `json:"client_os,omitempty"`
	ClientArch    string              `json:"client_arch,omitempty"`
	SystemVersion string              `json:"system_version,omitempty"`
	ReportedAt    time.Time           `json:"reported_at"`
	Summary       VPSSummary          `json:"summary"`
	XUITraffic    *XUIRealtimeTraffic `json:"xui_traffic,omitempty"`
}

// XUIRealtimeTraffic is an internal Client-to-Server payload. The Server must
// remove it before forwarding realtime metrics to any browser.
type XUIRealtimeTraffic struct {
	SampleID    int64                      `json:"sample_id"`
	CollectedAt time.Time                  `json:"collected_at,omitempty"`
	Clients     []XUIRealtimeClientTraffic `json:"clients"`
}

type XUIRealtimeClientTraffic struct {
	InboundID  int    `json:"inbound_id"`
	InboundTag string `json:"inbound_tag,omitempty"`
	Email      string `json:"email"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
}

type DashboardRealtimeMessage struct {
	Type    string                 `json:"type"`
	Metrics []AgentRealtimeMetrics `json:"metrics,omitempty"`
	Metric  *AgentRealtimeMetrics  `json:"metric,omitempty"`
}

const (
	AgentControlCollectNow     = "collect_now"
	AgentControlApplyConfig    = "apply_config"
	AgentControlRestartXUI     = "restart_xui"
	AgentControlExecuteXUI     = "execute_xui_action"
	AgentControlCollectXUI     = "collect_xui_traffic"
	AgentControlDisableClient  = "disable_client_service"
	AgentControlTerminalOpen   = "terminal_open"
	AgentControlTerminalInput  = "terminal_input"
	AgentControlTerminalResize = "terminal_resize"
	AgentControlTerminalClose  = "terminal_close"
)

type AgentControlMessage struct {
	Type     string              `json:"type"`
	Kind     string              `json:"kind,omitempty"`
	ActionID int64               `json:"action_id,omitempty"`
	Payload  map[string]any      `json:"payload,omitempty"`
	XUIAuth  *XUIActionAuth      `json:"xui_auth,omitempty"`
	Config   *ManagedAgentConfig `json:"config,omitempty"`
}

type AgentRefreshResponse struct {
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	Message string `json:"message,omitempty"`
}

const (
	TerminalMessageOpened = "terminal_opened"
	TerminalMessageOutput = "terminal_output"
	TerminalMessageError  = "terminal_error"
	TerminalMessageClosed = "terminal_closed"
)

type TerminalMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id,omitempty"`
	Data      string `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	Cols      int    `json:"cols,omitempty"`
	Shell     string `json:"shell,omitempty"`
}
