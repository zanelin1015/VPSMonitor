package model

import "time"

const (
	AdminRoleRoot        = "admin"
	AdminRoleAreaManager = "area_manager"
)

type AdminUser struct {
	ID          int64     `json:"id,omitempty"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Role        string    `json:"role,omitempty"`
	AgentIDs    []string  `json:"agent_ids,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AdminSession struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AdminLoginResponse struct {
	User   AdminUser  `json:"user"`
	System SystemInfo `json:"system,omitempty"`
}

type SystemInfo struct {
	Role      string `json:"role"`
	Version   string `json:"version"`
	BuildTime string `json:"build_time,omitempty"`
	GitCommit string `json:"git_commit,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

type AdminAccountUpdateRequest struct {
	CurrentPassword string  `json:"current_password"`
	NewUsername     string  `json:"new_username"`
	NewPassword     string  `json:"new_password"`
	AvatarURL       *string `json:"avatar_url,omitempty"`
}

type AreaManagerAccountRequest struct {
	Username    string   `json:"username"`
	Password    string   `json:"password,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
	AgentIDs    []string `json:"agent_ids,omitempty"`
}

type AreaManagerAdminView struct {
	ID          int64               `json:"id"`
	Username    string              `json:"username"`
	DisplayName string              `json:"display_name,omitempty"`
	Enabled     bool                `json:"enabled"`
	AgentIDs    []string            `json:"agent_ids,omitempty"`
	Customers   []CustomerAdminView `json:"customers,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type ClientInstallInfo struct {
	ServerURL             string `json:"server_url"`
	RegistrationToken     string `json:"registration_token"`
	InstallScriptURL      string `json:"install_script_url"`
	PollInterval          string `json:"poll_interval"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	ServerSkipTLSVerify   bool   `json:"server_skip_tls_verify"`
}

type ClientInstallSettingsRequest struct {
	ServerURL             string `json:"server_url"`
	InstallScriptURL      string `json:"install_script_url"`
	PollInterval          string `json:"poll_interval"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	ServerSkipTLSVerify   bool   `json:"server_skip_tls_verify"`
}

type ExchangeRatesResponse struct {
	Base      string             `json:"base"`
	Date      string             `json:"date"`
	Rates     map[string]float64 `json:"rates"`
	Source    string             `json:"source,omitempty"`
	FetchedAt time.Time          `json:"fetched_at"`
	Stale     bool               `json:"stale,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type TagSettingsResponse struct {
	Tags []string `json:"tags"`
}

type AreaAgentTagsRequest struct {
	Tags []string `json:"tags"`
}

type AreaAgentTagsResponse struct {
	AgentID string   `json:"agent_id"`
	Tags    []string `json:"tags"`
}

type FrontendSettings struct {
	CustomCode string `json:"custom_code"`
}

type UpdateRequest struct {
	Version       string   `json:"version,omitempty"`
	Repo          string   `json:"repo,omitempty"`
	PackagePrefix string   `json:"package_prefix,omitempty"`
	ScriptURL     string   `json:"script_url,omitempty"`
	PSScriptURL   string   `json:"ps_script_url,omitempty"`
	ServiceName   string   `json:"service_name,omitempty"`
	AgentIDs      []string `json:"agent_ids,omitempty"`
}

type UpdateResponse struct {
	Status      string              `json:"status"`
	Count       int                 `json:"count,omitempty"`
	Skipped     int                 `json:"skipped,omitempty"`
	Latest      *UpdateLatestInfo   `json:"latest,omitempty"`
	AgentStatus []UpdateAgentStatus `json:"agent_status,omitempty"`
}

type UpdateLatestInfo struct {
	Repo                       string              `json:"repo"`
	PackagePrefix              string              `json:"package_prefix"`
	CurrentServerVersion       string              `json:"current_server_version"`
	LatestVersion              string              `json:"latest_version"`
	LatestTag                  string              `json:"latest_tag"`
	LatestServerVersion        string              `json:"latest_server_version,omitempty"`
	LatestServerTag            string              `json:"latest_server_tag,omitempty"`
	LatestClientVersion        string              `json:"latest_client_version,omitempty"`
	LatestClientTag            string              `json:"latest_client_tag,omitempty"`
	ServerUpdateAvailable      bool                `json:"server_update_available"`
	ClientUpdateAvailableCount int                 `json:"client_update_available_count"`
	SupportedClientCount       int                 `json:"supported_client_count"`
	UnknownClientCount         int                 `json:"unknown_client_count"`
	UnsupportedClientCount     int                 `json:"unsupported_client_count"`
	Assets                     []string            `json:"assets,omitempty"`
	ServerAssets               []string            `json:"server_assets,omitempty"`
	ClientAssets               []string            `json:"client_assets,omitempty"`
	AgentStatus                []UpdateAgentStatus `json:"agent_status,omitempty"`
	FetchedAt                  time.Time           `json:"fetched_at"`
}

type UpdateAgentStatus struct {
	AgentID         string `json:"agent_id"`
	AgentName       string `json:"agent_name,omitempty"`
	Version         string `json:"version,omitempty"`
	OS              string `json:"os,omitempty"`
	Arch            string `json:"arch,omitempty"`
	PackageName     string `json:"package_name,omitempty"`
	Supported       bool   `json:"supported"`
	UpdateAvailable bool   `json:"update_available"`
	Reason          string `json:"reason,omitempty"`
}
