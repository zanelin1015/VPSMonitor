package model

import "time"

// Counters measure the client-facing side of a Realm listener, including IP
// headers and retransmissions. They do not add the relay's outgoing leg again.
type RealmTrafficCounter struct {
	Key           string `json:"key"`
	RuleID        string `json:"rule_id"`
	Name          string `json:"name,omitempty"`
	ListenAddress string `json:"listen_address"`
	ListenPort    int    `json:"listen_port"`
	Network       string `json:"network"`
	Epoch         string `json:"epoch"`
	UploadBytes   uint64 `json:"upload_bytes"`
	DownloadBytes uint64 `json:"download_bytes"`
}

type RealmTrafficSnapshot struct {
	Enabled     bool                  `json:"enabled"`
	Backend     string                `json:"backend,omitempty"`
	CollectedAt time.Time             `json:"collected_at"`
	Error       string                `json:"error,omitempty"`
	Counters    []RealmTrafficCounter `json:"counters,omitempty"`
}

type RealmTrafficUsage struct {
	Key             string    `json:"key"`
	RuleID          string    `json:"rule_id"`
	Name            string    `json:"name,omitempty"`
	ListenAddress   string    `json:"listen_address"`
	ListenPort      int       `json:"listen_port"`
	Network         string    `json:"network"`
	UploadBytes     uint64    `json:"upload_bytes"`
	DownloadBytes   uint64    `json:"download_bytes"`
	TotalBytes      uint64    `json:"total_bytes"`
	FirstObservedAt time.Time `json:"first_observed_at"`
	LastObservedAt  time.Time `json:"last_observed_at"`
}

type RealmTrafficUsageResponse struct {
	From        string              `json:"from"`
	To          string              `json:"to"`
	Timezone    string              `json:"timezone"`
	Enabled     bool                `json:"enabled"`
	Reported    bool                `json:"reported"`
	CollectedAt *time.Time          `json:"collected_at,omitempty"`
	Error       string              `json:"error,omitempty"`
	Items       []RealmTrafficUsage `json:"items"`
}
