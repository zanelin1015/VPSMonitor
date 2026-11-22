package model

import "time"

type NetworkInterfaceCounter struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Index         int    `json:"index"`
	MAC           string `json:"mac,omitempty"`
	Kind          string `json:"kind"`
	Up            bool   `json:"up"`
	Loopback      bool   `json:"loopback"`
	Primary       bool   `json:"primary"`
	Epoch         string `json:"epoch"`
	ReceivedBytes uint64 `json:"received_bytes"`
	SentBytes     uint64 `json:"sent_bytes"`
}

type NetworkInterfacesSnapshot struct {
	CollectedAt time.Time                 `json:"collected_at"`
	Error       string                    `json:"error,omitempty"`
	Warnings    []string                  `json:"warnings,omitempty"`
	Interfaces  []NetworkInterfaceCounter `json:"interfaces"`
}

type NetworkInterfaceTrafficUsage struct {
	Key             string    `json:"key"`
	Name            string    `json:"name"`
	Index           int       `json:"index"`
	MAC             string    `json:"mac,omitempty"`
	Kind            string    `json:"kind"`
	Up              bool      `json:"up"`
	Loopback        bool      `json:"loopback"`
	Primary         bool      `json:"primary"`
	Present         bool      `json:"present"`
	ReceivedBytes   uint64    `json:"received_bytes"`
	SentBytes       uint64    `json:"sent_bytes"`
	TotalBytes      uint64    `json:"total_bytes"`
	FirstObservedAt time.Time `json:"first_observed_at"`
	LastObservedAt  time.Time `json:"last_observed_at"`
}

type NetworkInterfaceTrafficResponse struct {
	From        string                         `json:"from"`
	To          string                         `json:"to"`
	Timezone    string                         `json:"timezone"`
	Reported    bool                           `json:"reported"`
	CollectedAt *time.Time                     `json:"collected_at,omitempty"`
	Error       string                         `json:"error,omitempty"`
	Warnings    []string                       `json:"warnings,omitempty"`
	Items       []NetworkInterfaceTrafficUsage `json:"items"`
}
