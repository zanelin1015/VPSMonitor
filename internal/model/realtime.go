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
	AgentControlCollectNow     = "collect_now"
	AgentControlApplyConfig    = "apply_config"
	AgentControlRestartXUI     = "restart_xui"
	AgentControlExecuteXUI     = "execute_xui_action"
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
