package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (a *App) handlePublicFrontendSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	settings, _, err := a.store.GetFrontendSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *App) handleAdminFrontendSettings(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, _, err := a.store.GetFrontendSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req model.FrontendSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode frontend settings: %v", err))
			return
		}
		settings, err := a.store.SaveFrontendSettings(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleAdminTags(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		tags, _, err := a.store.GetTagSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.TagSettingsResponse{Tags: tags})
	case http.MethodPut:
		var req model.TagSettingsResponse
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode tags request: %v", err))
			return
		}
		tags, err := a.store.SaveTagSettings(req.Tags)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.TagSettingsResponse{Tags: tags})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleConfigAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	agentID := r.URL.Query().Get("agent_id")
	items, err := a.store.ListConfigAuditLogs(agentID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleClientInstallInfo(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	if r.Method == http.MethodPut {
		var req model.ClientInstallSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode client install settings: %v", err))
			return
		}
		settings, err := validateClientInstallSettings(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := a.store.SaveClientInstallSettings(settings)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a.clientInstallInfo(r, saved))
		return
	}

	settings, found, err := a.store.GetClientInstallSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		settings = model.ClientInstallSettingsRequest{}
	}
	writeJSON(w, http.StatusOK, a.clientInstallInfo(r, settings))
}

const defaultClientInstallScriptURL = "https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh"

func (a *App) clientInstallInfo(r *http.Request, settings model.ClientInstallSettingsRequest) model.ClientInstallInfo {
	serverURL := firstNonEmptyString(settings.ServerURL, requestPublicBaseURL(r))
	installScriptURL := firstNonEmptyString(settings.InstallScriptURL, defaultClientInstallScriptURL)
	pollInterval := firstNonEmptyString(settings.PollInterval, "30s")
	requestTimeoutSeconds := settings.RequestTimeoutSeconds
	if requestTimeoutSeconds <= 0 {
		requestTimeoutSeconds = 15
	}
	return model.ClientInstallInfo{
		ServerURL:             serverURL,
		RegistrationToken:     a.config.RegistrationToken,
		InstallScriptURL:      installScriptURL,
		PollInterval:          pollInterval,
		RequestTimeoutSeconds: requestTimeoutSeconds,
		ServerSkipTLSVerify:   settings.ServerSkipTLSVerify,
	}
}

func validateClientInstallSettings(req model.ClientInstallSettingsRequest) (model.ClientInstallSettingsRequest, error) {
	req.ServerURL = strings.TrimSpace(req.ServerURL)
	req.InstallScriptURL = strings.TrimSpace(req.InstallScriptURL)
	req.PollInterval = strings.TrimSpace(req.PollInterval)
	if req.ServerURL == "" {
		return req, fmt.Errorf("server url is required")
	}
	if err := validateHTTPURL(req.ServerURL, "server url"); err != nil {
		return req, err
	}
	if req.InstallScriptURL == "" {
		return req, fmt.Errorf("install script url is required")
	}
	if err := validateHTTPURL(req.InstallScriptURL, "install script url"); err != nil {
		return req, err
	}
	if req.PollInterval == "" {
		req.PollInterval = "30s"
	}
	if d, err := time.ParseDuration(req.PollInterval); err != nil || d <= 0 {
		return req, fmt.Errorf("poll interval must be a positive Go duration, e.g. 30s")
	}
	if req.RequestTimeoutSeconds <= 0 {
		req.RequestTimeoutSeconds = 15
	}
	return req, nil
}

func validateHTTPURL(value, label string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be a valid URL", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must start with http:// or https://", label)
	}
	return nil
}

func requestPublicBaseURL(r *http.Request) string {
	proto := firstNonEmptyString(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0], r.Header.Get("X-Forwarded-Protocol"), r.Header.Get("X-Scheme"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := firstNonEmptyString(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0], r.Host)
	if host == "" {
		host = "SERVER_IP:8090"
	}
	return strings.TrimRight(strings.TrimSpace(proto), ":/") + "://" + strings.TrimSpace(host)
}
