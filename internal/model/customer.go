package model

import "time"

type CustomerUser struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	StyleCode   string    `json:"style_code,omitempty"`
	OwnerType   string    `json:"owner_type,omitempty"`
	OwnerID     int64     `json:"owner_id,omitempty"`
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
	Username          string  `json:"username"`
	Password          string  `json:"password,omitempty"`
	DisplayName       string  `json:"display_name,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	FrontProxyNodeIDs []int64 `json:"front_proxy_node_ids,omitempty"`
}

type CustomerAssignment struct {
	ID               int64                `json:"id"`
	CustomerID       int64                `json:"customer_id"`
	AgentID          string               `json:"agent_id"`
	InboundID        int                  `json:"inbound_id"`
	InboundTag       string               `json:"inbound_tag,omitempty"`
	ClientEmail      string               `json:"client_email,omitempty"`
	PublicClientName string               `json:"public_client_name,omitempty"`
	Remark           string               `json:"remark,omitempty"`
	Enabled          bool                 `json:"enabled"`
	FrontProxies     []FrontProxyNodeView `json:"front_proxies,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

type CustomerAssignmentRequest struct {
	AgentID           string   `json:"agent_id"`
	InboundID         int      `json:"inbound_id"`
	InboundTag        string   `json:"inbound_tag,omitempty"`
	ClientEmail       string   `json:"client_email,omitempty"`
	PublicClientName  string   `json:"public_client_name,omitempty"`
	TrafficMultiplier *float64 `json:"traffic_multiplier,omitempty"`
	RevenueAmount     *float64 `json:"revenue_amount,omitempty"`
	RevenueCurrency   string   `json:"revenue_currency,omitempty"`
	RevenueCycle      string   `json:"revenue_cycle,omitempty"`
	Enabled           *bool    `json:"enabled,omitempty"`
	FrontProxyNodeIDs []int64  `json:"front_proxy_node_ids,omitempty"`
}

type CustomerAssignmentSourceView struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name,omitempty"`
}

type CustomerAdminView struct {
	CustomerUser
	Assignments  []CustomerAssignment `json:"assignments"`
	FrontProxies []FrontProxyNodeView `json:"front_proxies,omitempty"`
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
	AssignmentID      int64                    `json:"assignment_id"`
	EntryClientName   string                   `json:"entry_client_name"`
	InboundTag        string                   `json:"inbound_tag,omitempty"`
	ClientEmail       string                   `json:"client_email,omitempty"`
	ClientRemark      string                   `json:"client_remark,omitempty"`
	ImportURL         string                   `json:"import_url,omitempty"`
	Remark            string                   `json:"remark,omitempty"`
	Summary           string                   `json:"summary"`
	ExitCountryCode   string                   `json:"exit_country_code,omitempty"`
	ExitCountryName   string                   `json:"exit_country_name,omitempty"`
	ExitIP            string                   `json:"exit_ip,omitempty"`
	Resolved          bool                     `json:"resolved"`
	UnresolvedReason  string                   `json:"unresolved_reason,omitempty"`
	RevenueAmount     *float64                 `json:"revenue_amount,omitempty"`
	RevenueCurrency   string                   `json:"revenue_currency,omitempty"`
	RevenueCycle      string                   `json:"revenue_cycle,omitempty"`
	TrafficMultiplier float64                  `json:"traffic_multiplier,omitempty"`
	TrafficUsedBytes  int64                    `json:"traffic_used_bytes,omitempty"`
	TrafficLimitBytes int64                    `json:"traffic_limit_bytes,omitempty"`
	NodeExpireTime    int64                    `json:"node_expire_time,omitempty"`
	StartTime         int64                    `json:"start_time,omitempty"`
	ExpireTime        int64                    `json:"expire_time,omitempty"`
	ExpireCycle       string                   `json:"expire_cycle,omitempty"`
	ExpireAutoRenew   bool                     `json:"expire_auto_renew,omitempty"`
	Steps             []CustomerLinkStep       `json:"steps"`
	FrontProxies      []CustomerLinkFrontProxy `json:"front_proxies,omitempty"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

type CustomerLinkFrontProxy struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol,omitempty"`
	ShareURL string `json:"-"`
}

type CustomerOverviewResponse struct {
	User                  CustomerUser           `json:"user"`
	GeneratedAt           time.Time              `json:"generated_at"`
	ClashSubscriptionURL  string                 `json:"clash_subscription_url,omitempty"`
	MihomoSubscriptionURL string                 `json:"mihomo_subscription_url,omitempty"`
	Announcements         []CustomerAnnouncement `json:"announcements,omitempty"`
	Links                 []CustomerLinkView     `json:"links"`
}
