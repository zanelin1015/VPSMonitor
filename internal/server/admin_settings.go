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
	// Customer announcements are only exposed through the authenticated
	// customer overview endpoint.
	settings.Announcements = nil
	writeJSON(w, http.StatusOK, settings)
}

func (a *App) handleAdminFrontendSettings(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
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
		if err := validateFrontendSettings(req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
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

func validateFrontendSettings(settings model.FrontendSettings) error {
	if len(settings.Announcements) > 20 {
		return fmt.Errorf("customer announcements cannot exceed 20")
	}
	for index, item := range settings.Announcements {
		if len([]rune(item.Title)) > 120 || len([]rune(item.Content)) > 1000 || len([]rune(item.LinkLabel)) > 60 || len([]rune(item.LinkURL)) > 500 {
			return fmt.Errorf("customer announcement %d exceeds the length limit", index+1)
		}
		if item.Enabled && strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("customer announcement %d title is required", index+1)
		}
		if value := strings.TrimSpace(item.LinkURL); value != "" {
			parsed, err := url.Parse(value)
			if err != nil || parsed.Scheme == "" || !allowedAnnouncementURLScheme(parsed.Scheme) {
				return fmt.Errorf("customer announcement %d link is invalid", index+1)
			}
		}
		startsAt, err := parseOptionalAnnouncementTime(item.StartsAt)
		if err != nil {
			return fmt.Errorf("customer announcement %d start time is invalid", index+1)
		}
		endsAt, err := parseOptionalAnnouncementTime(item.EndsAt)
		if err != nil {
			return fmt.Errorf("customer announcement %d end time is invalid", index+1)
		}
		if !startsAt.IsZero() && !endsAt.IsZero() && !endsAt.After(startsAt) {
			return fmt.Errorf("customer announcement %d end time must be after start time", index+1)
		}
	}
	return nil
}

func parseOptionalAnnouncementTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

func allowedAnnouncementURLScheme(scheme string) bool {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "http", "https", "tg", "mailto", "tel":
		return true
	default:
		return false
	}
}

func (a *App) handleAdminScheduledTasks(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, _, err := a.store.GetScheduledTaskSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req model.ScheduledTaskSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode scheduled task settings: %v", err))
			return
		}
		settings, err := a.store.SaveScheduledTaskSettings(req)
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
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
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

func (a *App) handleAdminAreaAgentTags(w http.ResponseWriter, r *http.Request, parts []string) {
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if !isAreaManager(user) {
		writeError(w, http.StatusForbidden, "area manager permission required")
		return
	}
	if len(parts) == 0 || parts[0] == "" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		tags, err := a.store.ListAreaManagerAgentTags(user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, tags)
		return
	}
	agentID := strings.TrimSpace(parts[0])
	if agentID == "" || len(parts) > 1 {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	if !a.adminCanAccessAgent(user, agentID) {
		writeError(w, http.StatusForbidden, "agent is not assigned to this account")
		return
	}
	switch r.Method {
	case http.MethodGet:
		tags, _, err := a.store.GetAreaManagerAgentTags(user.ID, agentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.AreaAgentTagsResponse{AgentID: agentID, Tags: tags})
	case http.MethodPut:
		var req model.AreaAgentTagsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode area agent tags: %v", err))
			return
		}
		tags, err := a.store.SaveAreaManagerAgentTags(user.ID, agentID, req.Tags)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.AreaAgentTagsResponse{AgentID: agentID, Tags: tags})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleConfigAuditLogs(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	agentID := r.URL.Query().Get("agent_id")
	if !isRootAdmin(user) {
		if agentID == "" || !a.adminCanAccessAgent(user, agentID) {
			writeJSON(w, http.StatusOK, []model.ConfigAuditLog{})
			return
		}
	}
	items, err := a.store.ListConfigAuditLogs(agentID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleClientInstallInfo(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
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

const (
	defaultClientInstallScriptURL = "https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh"
	defaultRealmVersion           = "v2.9.4"
)

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
		RealmAutoInstall:      settings.RealmAutoInstall,
		RealmVersion:          firstNonEmptyString(settings.RealmVersion, defaultRealmVersion),
		RealmDownloadBaseURL:  settings.RealmDownloadBaseURL,
		HAProxyAutoInstall:    settings.HAProxyAutoInstall,
		XUIAutoInstall:        false,
	}
}

func validateClientInstallSettings(req model.ClientInstallSettingsRequest) (model.ClientInstallSettingsRequest, error) {
	req.ServerURL = strings.TrimSpace(req.ServerURL)
	req.InstallScriptURL = strings.TrimSpace(req.InstallScriptURL)
	req.PollInterval = strings.TrimSpace(req.PollInterval)
	req.RealmVersion = strings.TrimSpace(req.RealmVersion)
	req.RealmDownloadBaseURL = strings.TrimRight(strings.TrimSpace(req.RealmDownloadBaseURL), "/")
	req.XUIUsername = strings.TrimSpace(req.XUIUsername)
	req.XUIPassword = strings.TrimSpace(req.XUIPassword)
	req.XUIInstallScriptURL = strings.TrimSpace(req.XUIInstallScriptURL)
	req.XUIWebPath = normalizeXUIWebPath(req.XUIWebPath)
	req.XUIAutoInstall = false
	req.XUIUsername = ""
	req.XUIPassword = ""
	req.XUIPanelPort = 0
	req.XUIWebPath = ""
	req.XUIInstallScriptURL = ""
	if req.RealmAutoInstall && req.HAProxyAutoInstall {
		return req, fmt.Errorf("HAProxy 与 Realm 只能选择一个自动安装")
	}
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
	if req.RealmVersion == "" {
		req.RealmVersion = defaultRealmVersion
	}
	if len(req.RealmVersion) > 32 || strings.Trim(req.RealmVersion, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-") != "" {
		return req, fmt.Errorf("realm version contains unsupported characters")
	}
	if req.RealmDownloadBaseURL != "" {
		if err := validateHTTPURL(req.RealmDownloadBaseURL, "realm download base url"); err != nil {
			return req, err
		}
	}
	return req, nil
}

func normalizeXUIWebPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = "/" + strings.Trim(value, "/")
	if value == "/" {
		return ""
	}
	return value + "/"
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
