package model

import "time"

const (
	// Retained only to retire queued actions and reject pre-upgrade requests.
	XUIActionResetClientTraffic       = "reset_client_traffic"
	XUIActionManualResetClientTraffic = "manual_reset_client_traffic"
	XUIActionAddOutbound              = "add_outbound"
	XUIActionAddClient                = "add_client"
	XUIActionAddRoutingRule           = "add_routing_rule"
	XUIActionUpsertRoutingRule        = "upsert_routing_rule"
	XUIActionUpdateRoutingRule        = "update_routing_rule"
	XUIActionDeleteRoutingRules       = "delete_routing_rules"
	XUIActionDeleteOutbounds          = "delete_outbounds"
	XUIActionUpdateClientExpiry       = "update_client_expiry"
	XUIActionUpdateClientTraffic      = "update_client_traffic_limit"
	XUIActionUpdateClientTrafficReset = "update_client_traffic_reset"
	XUIActionUpdateClientConfig       = "update_client_config"
	XUIActionSetClientEnabled         = "set_client_enabled"
	XUIActionDeleteClient             = "delete_client"
	XUIActionUpdateClient             = "update_client"
	XUIActionRestartXUI               = "restart_xui"
	XUIActionExecuteCommand           = "execute_command"
	XUIActionUpdate3XUI               = "update_3xui"

	// Sing-box actions share the durable action queue and realtime transport
	// with the historical x-ui actions.  The prefix keeps the two executors
	// unambiguous for old Clients.
	SingBoxActionAddNode           = "singbox_add_node"
	SingBoxActionAddClient         = "singbox_add_client"
	SingBoxActionAddUser           = "singbox_add_user"
	SingBoxActionDeleteUser        = "singbox_delete_user"
	SingBoxActionAddForwardingRule = "singbox_add_forwarding_rule"
	SingBoxActionReload            = "singbox_reload"
	// Compatibility aliases for callers that group actions by XUI.
	XUIActionSingBoxAddNode           = SingBoxActionAddNode
	XUIActionSingBoxAddClient         = SingBoxActionAddClient
	XUIActionSingBoxAddForwardingRule = SingBoxActionAddForwardingRule
	SingBoxActionAddRule              = SingBoxActionAddForwardingRule
	SingBoxActionAddForwardRule       = SingBoxActionAddForwardingRule

	XUIActionStatusPending   = "pending"
	XUIActionStatusRunning   = "running"
	XUIActionStatusSucceeded = "succeeded"
	XUIActionStatusFailed    = "failed"

	LegacyTrafficResetRemovedMessage = "系统周期流量重置已移除，请使用 x-ui 客户端自动重置"
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

type XUIActionActor struct {
	Role      string `json:"role,omitempty"`
	AccountID int64  `json:"account_id,omitempty"`
	Username  string `json:"username,omitempty"`
}

type XUIAction struct {
	ID                 int64          `json:"id"`
	AgentID            string         `json:"agent_id"`
	Kind               string         `json:"kind"`
	Status             string         `json:"status"`
	CreatedByRole      string         `json:"created_by_role,omitempty"`
	CreatedByAccountID int64          `json:"created_by_account_id,omitempty"`
	CreatedByUsername  string         `json:"created_by_username,omitempty"`
	Payload            map[string]any `json:"payload,omitempty"`
	XUIAuth            *XUIActionAuth `json:"xui_auth,omitempty"`
	Result             map[string]any `json:"result,omitempty"`
	Error              string         `json:"error,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	ClaimedAt          *time.Time     `json:"claimed_at,omitempty"`
	CompletedAt        *time.Time     `json:"completed_at,omitempty"`
}
