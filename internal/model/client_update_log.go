package model

// ClientUpdateLog stores the dispatch decision separately from the installer
// task result. A successful task receipt only means the updater was launched.
type ClientUpdateLog struct {
	ID            int64  `json:"id"`
	BatchID       string `json:"batch_id"`
	AgentID       string `json:"agent_id"`
	AgentName     string `json:"agent_name"`
	Version       string `json:"version"`
	TargetVersion string `json:"target_version"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	Decision      string `json:"decision"`
	Force         bool   `json:"force,omitempty"`
	ReasonCode    string `json:"reason_code"`
	Reason        string `json:"reason"`
	ActionID      int64  `json:"action_id,omitempty"`
	CreatedAt     string `json:"created_at"`
	TaskStatus    string `json:"task_status,omitempty"`
	TaskError     string `json:"task_error,omitempty"`
	ClaimedAt     string `json:"claimed_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	ConfirmedAt   string `json:"confirmed_at,omitempty"`
}
