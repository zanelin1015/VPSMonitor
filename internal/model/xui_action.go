package model

import "time"

const (
	XUIActionAddOutbound        = "add_outbound"
	XUIActionAddClient          = "add_client"
	XUIActionAddRoutingRule     = "add_routing_rule"
	XUIActionUpsertRoutingRule  = "upsert_routing_rule"
	XUIActionUpdateClientExpiry = "update_client_expiry"
	XUIActionSetClientEnabled   = "set_client_enabled"
	XUIActionDeleteClient       = "delete_client"
	XUIActionUpdateClient       = "update_client"
	XUIActionRestartXUI         = "restart_xui"
	XUIActionExecuteCommand     = "execute_command"
	XUIActionUpdate3XUI         = "update_3xui"

	XUIActionStatusPending   = "pending"
	XUIActionStatusRunning   = "running"
	XUIActionStatusSucceeded = "succeeded"
	XUIActionStatusFailed    = "failed"
)

type XUIActionRequest struct {
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

type XUIActionResultRequest struct {
	Status string         `json:"status"`
	Result map[string]any `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
}

type XUIActionAuth struct {
	APIToken string `json:"api_token,omitempty"`
}

type XUIAction struct {
	ID          int64          `json:"id"`
	AgentID     string         `json:"agent_id"`
	Kind        string         `json:"kind"`
	Status      string         `json:"status"`
	Payload     map[string]any `json:"payload,omitempty"`
	XUIAuth     *XUIActionAuth `json:"xui_auth,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ClaimedAt   *time.Time     `json:"claimed_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}
