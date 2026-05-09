package model

import "time"

const (
	XUIActionAddOutbound        = "add_outbound"
	XUIActionAddRoutingRule     = "add_routing_rule"
	XUIActionUpdateClientExpiry = "update_client_expiry"
	XUIActionUpdateClient       = "update_client"

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

type XUIAction struct {
	ID          int64          `json:"id"`
	AgentID     string         `json:"agent_id"`
	Kind        string         `json:"kind"`
	Status      string         `json:"status"`
	Payload     map[string]any `json:"payload,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ClaimedAt   *time.Time     `json:"claimed_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}
