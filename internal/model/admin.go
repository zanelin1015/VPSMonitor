package model

import "time"

type AdminUser struct {
	Username  string    `json:"username"`
	UpdatedAt time.Time `json:"updated_at"`
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
	CurrentPassword string `json:"current_password"`
	NewUsername     string `json:"new_username"`
	NewPassword     string `json:"new_password"`
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
	Status string `json:"status"`
	Count  int    `json:"count,omitempty"`
}
