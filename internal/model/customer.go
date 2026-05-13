package model

import "time"

type CustomerUser struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	StyleCode   string    `json:"style_code,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CustomerSession struct {
	CustomerID int64     `json:"customer_id"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type CustomerLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CustomerLoginResponse struct {
	User CustomerUser `json:"user"`
}

type CustomerAccountRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type CustomerAssignment struct {
	ID               int64     `json:"id"`
	CustomerID       int64     `json:"customer_id"`
	AgentID          string    `json:"agent_id"`
	InboundID        int       `json:"inbound_id"`
	InboundTag       string    `json:"inbound_tag,omitempty"`
	ClientEmail      string    `json:"client_email,omitempty"`
	PublicClientName string    `json:"public_client_name,omitempty"`
	Remark           string    `json:"remark,omitempty"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CustomerAssignmentRequest struct {
	AgentID          string   `json:"agent_id"`
	InboundID        int      `json:"inbound_id"`
	InboundTag       string   `json:"inbound_tag,omitempty"`
	ClientEmail      string   `json:"client_email,omitempty"`
	PublicClientName string   `json:"public_client_name,omitempty"`
	RevenueAmount    *float64 `json:"revenue_amount,omitempty"`
	RevenueCurrency  string   `json:"revenue_currency,omitempty"`
	RevenueCycle     string   `json:"revenue_cycle,omitempty"`
	Enabled          *bool    `json:"enabled,omitempty"`
}

type CustomerAdminView struct {
	CustomerUser
	Assignments []CustomerAssignment `json:"assignments"`
}

type CustomerRemarkRequest struct {
	Remark string `json:"remark"`
}

type CustomerStyleRequest struct {
	StyleCode string `json:"style_code"`
}

type CustomerPasswordUpdateRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type CustomerLinkStep struct {
	Role        string `json:"role"`
	Label       string `json:"label"`
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	ExitIP      string `json:"exit_ip,omitempty"`
}

type CustomerLinkView struct {
	AssignmentID     int64              `json:"assignment_id"`
	EntryClientName  string             `json:"entry_client_name"`
	InboundTag       string             `json:"inbound_tag,omitempty"`
	ClientEmail      string             `json:"client_email,omitempty"`
	ClientRemark     string             `json:"client_remark,omitempty"`
	ImportURL        string             `json:"import_url,omitempty"`
	Remark           string             `json:"remark,omitempty"`
	Summary          string             `json:"summary"`
	ExitCountryCode  string             `json:"exit_country_code,omitempty"`
	ExitCountryName  string             `json:"exit_country_name,omitempty"`
	ExitIP           string             `json:"exit_ip,omitempty"`
	Resolved         bool               `json:"resolved"`
	UnresolvedReason string             `json:"unresolved_reason,omitempty"`
	Steps            []CustomerLinkStep `json:"steps"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

type CustomerOverviewResponse struct {
	User        CustomerUser       `json:"user"`
	GeneratedAt time.Time          `json:"generated_at"`
	Links       []CustomerLinkView `json:"links"`
}
