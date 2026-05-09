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
	User AdminUser `json:"user"`
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
