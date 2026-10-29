package model

import "time"

// SingBoxOverview is deliberately close to the JSON file layout so the UI can
// render any current sing-box node or route type without a schema migration.
type SingBoxOverview struct {
	AgentID         string           `json:"agent_id"`
	AgentName       string           `json:"agent_name,omitempty"`
	ConfigPath      string           `json:"config_path,omitempty"`
	ServiceName     string           `json:"service_name,omitempty"`
	BinaryPath      string           `json:"binary_path,omitempty"`
	Running         bool             `json:"running"`
	ReportedAt      time.Time        `json:"reported_at"`
	CollectedAt     time.Time        `json:"collected_at"`
	Nodes           []map[string]any `json:"nodes"`
	Clients         []map[string]any `json:"clients"`
	ForwardingRules []map[string]any `json:"forwarding_rules"`
	RawConfig       map[string]any   `json:"raw_config,omitempty"`
	Error           string           `json:"error,omitempty"`
}
