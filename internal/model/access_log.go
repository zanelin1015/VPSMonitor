package model

import "time"

type AccessLogConfig struct {
	Enabled       bool   `json:"enabled,omitempty"`
	Path          string `json:"path,omitempty"`
	RetentionDays int    `json:"retention_days,omitempty"`
	MaxBatch      int    `json:"max_batch,omitempty"`
}

type AccessLogEntry struct {
	ID            int64     `json:"id,omitempty"`
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name,omitempty"`
	InboundID     int       `json:"inbound_id,omitempty"`
	InboundTag    string    `json:"inbound_tag,omitempty"`
	ClientEmail   string    `json:"client_email,omitempty"`
	ClientID      string    `json:"client_id,omitempty"`
	SourceIP      string    `json:"source_ip,omitempty"`
	SourcePort    int       `json:"source_port,omitempty"`
	TargetHost    string    `json:"target_host,omitempty"`
	TargetIP      string    `json:"target_ip,omitempty"`
	TargetPort    int       `json:"target_port,omitempty"`
	Network       string    `json:"network,omitempty"`
	Protocol      string    `json:"protocol,omitempty"`
	OutboundTag   string    `json:"outbound_tag,omitempty"`
	UploadBytes   uint64    `json:"upload_bytes,omitempty"`
	DownloadBytes uint64    `json:"download_bytes,omitempty"`
	DurationMS    int64     `json:"duration_ms,omitempty"`
	RawSummary    string    `json:"raw_summary,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	EndedAt       time.Time `json:"ended_at,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

type AccessLogBatchRequest struct {
	Entries []AccessLogEntry `json:"entries"`
}

type AccessLogListResponse struct {
	Items []AccessLogEntry `json:"items"`
	Total int              `json:"total"`
}
