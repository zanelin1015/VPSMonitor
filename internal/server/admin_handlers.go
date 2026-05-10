package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (a *App) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/"), "/")
	if strings.HasPrefix(path, "telegram-bots") {
		a.handleAdminTelegramBots(w, r, strings.Split(path, "/")[1:])
		return
	}
	if path == "customers" || strings.HasPrefix(path, "customers/") {
		a.handleAdminCustomers(w, r, strings.Split(path, "/")[1:])
		return
	}
	if strings.HasPrefix(path, "updates") {
		a.handleAdminUpdates(w, r, strings.Split(path, "/")[1:])
		return
	}
	switch path {
	case "login":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminLogin(w, r)
	case "session":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		user, _, ok := a.requireAdmin(w, r)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user, System: serverSystemInfo()})
	case "logout":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminLogout(w, r)
	case "account":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminAccountUpdate(w, r)
	case "audit":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleConfigAuditLogs(w, r)
	case "client-install":
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleClientInstallInfo(w, r)
	case "tags":
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminTags(w, r)
	case "frontend-settings":
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleAdminFrontendSettings(w, r)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (a *App) handleAdminUpdates(w http.ResponseWriter, r *http.Request, parts []string) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, "update route not found")
		return
	}
	if parts[0] == "latest" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		repo := firstNonEmptyString(r.URL.Query().Get("repo"), "zanelin1015/VPSMonitor")
		packagePrefix := firstNonEmptyString(r.URL.Query().Get("package_prefix"), "VPSMonitor")
		latest, err := a.fetchUpdateLatestInfo(repo, packagePrefix)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, latest)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req model.UpdateRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	switch parts[0] {
	case "server":
		latest, err := a.startServerUpdate(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.UpdateResponse{Status: "server update started", Latest: latest})
	case "clients":
		response, err := a.createClientUpdateActions(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		writeError(w, http.StatusNotFound, "update route not found")
	}
}

func (a *App) startServerUpdate(req model.UpdateRequest) (*model.UpdateLatestInfo, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve server executable: %w", err)
	}
	installDir := filepath.Dir(exe)
	repo := firstNonEmptyString(req.Repo, "zanelin1015/VPSMonitor")
	packagePrefix := firstNonEmptyString(req.PackagePrefix, "VPSMonitor")
	latest, err := a.fetchUpdateLatestInfo(repo, packagePrefix)
	if err != nil {
		return nil, err
	}
	version := firstNonEmptyString(req.Version, latest.LatestServerTag, latest.LatestServerVersion, latest.LatestTag, latest.LatestVersion)
	if !isVersionNewer(version, latest.CurrentServerVersion) {
		return latest, fmt.Errorf("server is already up to date: current %s, latest %s", latest.CurrentServerVersion, firstNonEmptyString(latest.LatestServerVersion, latest.LatestVersion))
	}
	scriptURL := firstNonEmptyString(req.ScriptURL, "https://raw.githubusercontent.com/"+repo+"/main/install.sh")
	serviceName := firstNonEmptyString(req.ServiceName, "vpsmonitor-server")
	command := fmt.Sprintf(`(sleep 2; tmp="$(mktemp /tmp/vpsmonitor-server-install.XXXXXX.sh)"; (curl -fsSL %[1]q -o "$tmp" || wget -O "$tmp" %[1]q) && chmod +x "$tmp" && env VPSMONITOR_ASSUME_YES=true VPSMONITOR_VERSION=%[2]q VPSMONITOR_REPO=%[3]q VPSMONITOR_PACKAGE_PREFIX=%[4]q VPSMONITOR_SERVER_DIR=%[5]q VPSMONITOR_SERVER_SERVICE=%[6]q bash "$tmp" server >>/tmp/vpsmonitor-server-update.log 2>&1) >/dev/null 2>&1 &`, scriptURL, version, repo, packagePrefix, installDir, serviceName)
	if err := exec.Command("sh", "-c", command).Start(); err != nil {
		return latest, err
	}
	return latest, nil
}

func (a *App) createClientUpdateActions(req model.UpdateRequest) (model.UpdateResponse, error) {
	agents, err := a.store.ListAgents()
	if err != nil {
		return model.UpdateResponse{}, err
	}
	selected := map[string]struct{}{}
	for _, id := range req.AgentIDs {
		if id = strings.TrimSpace(id); id != "" {
			selected[id] = struct{}{}
		}
	}
	repo := firstNonEmptyString(req.Repo, "zanelin1015/VPSMonitor")
	packagePrefix := firstNonEmptyString(req.PackagePrefix, "VPSMonitor")
	latest, err := a.fetchUpdateLatestInfo(repo, packagePrefix)
	if err != nil {
		return model.UpdateResponse{}, err
	}
	version := firstNonEmptyString(req.Version, latest.LatestClientTag, latest.LatestClientVersion, latest.LatestTag, latest.LatestVersion)
	scriptURL := firstNonEmptyString(req.ScriptURL, "https://raw.githubusercontent.com/"+repo+"/main/install.sh")
	psScriptURL := firstNonEmptyString(req.PSScriptURL, "https://raw.githubusercontent.com/"+repo+"/main/install.ps1")
	serviceName := firstNonEmptyString(req.ServiceName, "")
	count := 0
	skipped := 0
	statuses := make([]model.UpdateAgentStatus, 0, len(agents))
	for _, agent := range agents {
		if len(selected) > 0 {
			if _, ok := selected[agent.AgentID]; !ok {
				continue
			}
		}
		status := buildUpdateAgentStatus(agent, latest.LatestClientVersion, packagePrefix, latest.ClientAssets)
		statuses = append(statuses, status)
		if !status.UpdateAvailable {
			skipped++
			continue
		}
		payload := map[string]any{
			"version":        version,
			"repo":           repo,
			"package_prefix": packagePrefix,
			"script_url":     scriptURL,
			"ps_script_url":  psScriptURL,
			"target_os":      status.OS,
			"target_arch":    status.Arch,
		}
		if serviceName != "" {
			payload["service_name"] = serviceName
		}
		if _, err := a.store.CreateXUIAction(agent.AgentID, model.XUIActionRequest{Kind: model.XUIActionUpdateClient, Payload: payload}); err != nil {
			return model.UpdateResponse{}, err
		}
		count++
	}
	return model.UpdateResponse{
		Status:      "client update tasks created",
		Count:       count,
		Skipped:     skipped,
		Latest:      latest,
		AgentStatus: statuses,
	}, nil
}

func (a *App) fetchUpdateLatestInfo(repo string, packagePrefix string) (*model.UpdateLatestInfo, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" || !strings.Contains(repo, "/") {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	packagePrefix = firstNonEmptyString(packagePrefix, "VPSMonitor")

	ctx := contextWithTimeout(15 * time.Second)
	defer ctx.cancel()
	req, err := http.NewRequestWithContext(ctx.ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases?per_page=20", nil)
	if err != nil {
		return nil, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "VPSMonitor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch releases: http %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Assets  []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	var latest releaseUpdateInfo
	var serverLatest releaseUpdateInfo
	var clientLatest releaseUpdateInfo
	serverPackageName, serverPackageOK := updateServerPackageName(packagePrefix, runtime.GOOS, runtime.GOARCH)
	for _, release := range releases {
		tag := firstNonEmptyString(release.TagName, release.Name)
		version := normalizeVersion(tag)
		if _, ok := parseSemver(version); !ok {
			continue
		}
		assets := make([]string, 0, len(release.Assets))
		for _, asset := range release.Assets {
			if strings.TrimSpace(asset.Name) != "" {
				assets = append(assets, asset.Name)
			}
		}
		candidate := releaseUpdateInfo{Tag: tag, Version: version, Assets: assets}
		if latest.Version == "" || isVersionNewer(version, latest.Version) {
			latest = candidate
		}
		if serverPackageOK && stringInSlice(serverPackageName, assets) && (serverLatest.Version == "" || isVersionNewer(version, serverLatest.Version)) {
			serverLatest = candidate
		}
		if hasClientUpdateAsset(packagePrefix, assets) && (clientLatest.Version == "" || isVersionNewer(version, clientLatest.Version)) {
			clientLatest = candidate
		}
	}
	if latest.Version == "" && len(releases) > 0 {
		release := releases[0]
		assets := make([]string, 0, len(release.Assets))
		for _, asset := range release.Assets {
			if strings.TrimSpace(asset.Name) != "" {
				assets = append(assets, asset.Name)
			}
		}
		latest = releaseUpdateInfo{Tag: firstNonEmptyString(release.TagName, release.Name), Version: normalizeVersion(firstNonEmptyString(release.TagName, release.Name)), Assets: assets}
	}
	if latest.Version == "" {
		return nil, fmt.Errorf("latest release has no version tag")
	}
	if serverLatest.Version == "" {
		serverLatest = latest
	}
	if clientLatest.Version == "" {
		clientLatest = latest
	}

	agents, err := a.store.ListAgents()
	if err != nil {
		return nil, err
	}
	info := &model.UpdateLatestInfo{
		Repo:                  repo,
		PackagePrefix:         packagePrefix,
		CurrentServerVersion:  serverSystemInfo().Version,
		LatestVersion:         latest.Version,
		LatestTag:             latest.Tag,
		LatestServerVersion:   serverLatest.Version,
		LatestServerTag:       serverLatest.Tag,
		LatestClientVersion:   clientLatest.Version,
		LatestClientTag:       clientLatest.Tag,
		ServerUpdateAvailable: isVersionNewer(serverLatest.Version, serverSystemInfo().Version),
		Assets:                latest.Assets,
		ServerAssets:          serverLatest.Assets,
		ClientAssets:          clientLatest.Assets,
		FetchedAt:             time.Now().UTC(),
	}
	for _, agent := range agents {
		status := buildUpdateAgentStatus(agent, clientLatest.Version, packagePrefix, clientLatest.Assets)
		info.AgentStatus = append(info.AgentStatus, status)
		switch {
		case status.OS == "" || status.Arch == "":
			info.UnknownClientCount++
		case !status.Supported:
			info.UnsupportedClientCount++
		default:
			info.SupportedClientCount++
		}
		if status.UpdateAvailable {
			info.ClientUpdateAvailableCount++
		}
	}
	return info, nil
}

type releaseUpdateInfo struct {
	Tag     string
	Version string
	Assets  []string
}

func buildUpdateAgentStatus(agent model.AgentRecord, latestVersion string, packagePrefix string, assets []string) model.UpdateAgentStatus {
	osName := strings.ToLower(strings.TrimSpace(agent.OS))
	arch := strings.ToLower(strings.TrimSpace(agent.Arch))
	status := model.UpdateAgentStatus{
		AgentID:   agent.AgentID,
		AgentName: agent.AgentName,
		Version:   agent.Version,
		OS:        osName,
		Arch:      arch,
	}
	if osName == "" || arch == "" {
		status.Reason = "client has not reported os/arch yet"
		return status
	}
	packageName, ok := updateClientPackageName(packagePrefix, osName, arch)
	status.PackageName = packageName
	if !ok {
		status.Reason = "unsupported client platform"
		return status
	}
	if len(assets) > 0 && !stringInSlice(packageName, assets) {
		status.Reason = "release asset not found"
		return status
	}
	status.Supported = true
	if !isVersionNewer(latestVersion, agent.Version) {
		status.Reason = "client is already up to date"
		return status
	}
	status.UpdateAvailable = true
	status.Reason = "update available"
	return status
}

func updateServerPackageName(packagePrefix string, osName string, arch string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(osName)) {
	case "linux":
		return fmt.Sprintf("%s-server-linux-%s.tar.gz", packagePrefix, strings.ToLower(strings.TrimSpace(arch))), true
	case "windows":
		return fmt.Sprintf("%s-server-windows-%s.zip", packagePrefix, strings.ToLower(strings.TrimSpace(arch))), true
	default:
		return "", false
	}
}

func updateClientPackageName(packagePrefix string, osName string, arch string) (string, bool) {
	switch osName {
	case "linux":
		return fmt.Sprintf("%s-client-linux-%s.tar.gz", packagePrefix, arch), true
	case "windows":
		return fmt.Sprintf("%s-client-windows-%s.zip", packagePrefix, arch), true
	default:
		return "", false
	}
}

func hasClientUpdateAsset(packagePrefix string, assets []string) bool {
	prefix := packagePrefix + "-client-"
	for _, asset := range assets {
		if strings.HasPrefix(asset, prefix) && (strings.HasSuffix(asset, ".tar.gz") || strings.HasSuffix(asset, ".zip")) {
			return true
		}
	}
	return false
}

func stringInSlice(value string, items []string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func isVersionNewer(candidate string, current string) bool {
	candidateParts, candidateOK := parseSemver(candidate)
	currentParts, currentOK := parseSemver(current)
	if !candidateOK || !currentOK {
		return false
	}
	for i := 0; i < len(candidateParts); i++ {
		if candidateParts[i] != currentParts[i] {
			return candidateParts[i] > currentParts[i]
		}
	}
	return false
}

func parseSemver(value string) ([3]int, bool) {
	return parseSemverParts(normalizeVersion(value))
}

func parseSemverParts(value string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return result, false
	}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return result, false
		}
		result[i] = number
	}
	return result, true
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "refs/tags/")
	if version := extractSemver(value); version != "" {
		return version
	}
	value = strings.TrimPrefix(value, "v")
	if index := strings.IndexAny(value, "+-"); index >= 0 {
		value = value[:index]
	}
	return value
}

func extractSemver(value string) string {
	for start := 0; start < len(value); start++ {
		if value[start] < '0' || value[start] > '9' {
			continue
		}
		end := start
		dots := 0
		for end < len(value) {
			ch := value[end]
			if ch == '.' {
				dots++
				end++
				continue
			}
			if ch < '0' || ch > '9' {
				break
			}
			end++
		}
		candidate := value[start:end]
		if dots == 2 {
			if _, ok := parseSemverParts(candidate); ok {
				return candidate
			}
		}
	}
	return ""
}

type timeoutContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func contextWithTimeout(duration time.Duration) timeoutContext {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	return timeoutContext{ctx: ctx, cancel: cancel}
}

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

func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req model.AdminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode login request: %v", err))
		return
	}
	user, ok, err := a.store.AuthenticateAdmin(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	token, session, err := a.store.CreateAdminSession(user.Username, adminSessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setAdminSessionCookie(w, r, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user, System: serverSystemInfo()})
}

func (a *App) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if token := readAdminSessionToken(r); token != "" {
		_ = a.store.DeleteAdminSession(token)
	}
	clearAdminSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *App) handleAdminAccountUpdate(w http.ResponseWriter, r *http.Request) {
	_, token, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var req model.AdminAccountUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode account request: %v", err))
		return
	}
	user, err := a.store.UpdateAdminAccount(req, token)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not initialized") {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.AdminLoginResponse{User: user, System: serverSystemInfo()})
}

func (a *App) handleAdminTelegramBots(w http.ResponseWriter, r *http.Request, parts []string) {
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			bots, err := a.store.ListTelegramBots()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, bots)
		case http.MethodPost:
			var req model.TelegramBotRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode telegram bot: %v", err))
				return
			}
			bot, err := a.store.CreateTelegramBot(req)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, bot)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid telegram bot id")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			var req model.TelegramBotRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode telegram bot: %v", err))
				return
			}
			bot, err := a.store.UpdateTelegramBot(id, req)
			if err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, bot)
		case http.MethodDelete:
			if err := a.store.DeleteTelegramBot(id); err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "test" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		bot, found, err := a.store.GetTelegramBotSecret(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "telegram bot not found")
			return
		}
		text := fmt.Sprintf("✅ NanFengMonitor Telegram 测试消息\n机器人：%s\n时间：%s", bot.Name, time.Now().Format("2006-01-02 15:04:05"))
		if err := a.alerts.sendTelegramMessage(bot.BotToken, bot.ChatID, text); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
		return
	}

	writeError(w, http.StatusNotFound, "route not found")
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (model.AdminUser, string, bool) {
	user, token, ok := a.currentAdmin(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "admin login required")
		return model.AdminUser{}, "", false
	}
	return user, token, true
}

func (a *App) currentAdmin(r *http.Request) (model.AdminUser, string, bool) {
	token := readAdminSessionToken(r)
	user, _, ok, err := a.store.ValidateAdminSession(token)
	if err != nil || !ok {
		return model.AdminUser{}, "", false
	}
	return user, token, true
}

func readAdminSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setAdminSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

func clearAdminSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
