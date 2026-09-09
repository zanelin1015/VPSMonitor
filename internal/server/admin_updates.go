package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const (
	officialVPSMonitorRepository = "zanelin1015/VPSMonitor"
	officialVPSMonitorPrefix     = "VPSMonitor"
)

func (a *App) handleAdminUpdates(w http.ResponseWriter, r *http.Request, parts []string) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
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
		latest, err := a.cachedUpdateLatestInfo(repo, packagePrefix)
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
	case "3xui":
		response, err := a.create3XUIUpdateActions(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		writeError(w, http.StatusNotFound, "update route not found")
	}
}

func (a *App) cachedUpdateLatestInfo(repo string, packagePrefix string) (*model.UpdateLatestInfo, error) {
	cacheKey := strings.Trim(strings.TrimSpace(repo), "/") + "|" + firstNonEmptyString(packagePrefix, "VPSMonitor")
	now := time.Now()
	a.updateLatestMu.Lock()
	if a.updateLatestCache == nil {
		a.updateLatestCache = make(map[string]updateLatestCacheEntry)
	}
	if entry, ok := a.updateLatestCache[cacheKey]; ok && entry.info != nil && now.Before(entry.expiresAt) {
		info := cloneUpdateLatestInfo(entry.info)
		info.CacheExpiresAt = &entry.expiresAt
		a.updateLatestMu.Unlock()
		return info, nil
	}
	a.updateLatestMu.Unlock()

	info, err := a.fetchUpdateLatestInfo(repo, packagePrefix)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(updateLatestCacheTTL)
	a.updateLatestMu.Lock()
	a.updateLatestCache[cacheKey] = updateLatestCacheEntry{expiresAt: expiresAt, info: cloneUpdateLatestInfo(info)}
	a.updateLatestMu.Unlock()
	info.CacheExpiresAt = &expiresAt
	return info, nil
}

func cloneUpdateLatestInfo(info *model.UpdateLatestInfo) *model.UpdateLatestInfo {
	if info == nil {
		return nil
	}
	cloned := *info
	cloned.Assets = cloneStringSlice(info.Assets)
	cloned.ServerAssets = cloneStringSlice(info.ServerAssets)
	cloned.ClientAssets = cloneStringSlice(info.ClientAssets)
	cloned.ServerAssetDigests = cloneStringMap(info.ServerAssetDigests)
	cloned.ClientAssetDigests = cloneStringMap(info.ClientAssetDigests)
	cloned.AgentStatus = append([]model.UpdateAgentStatus(nil), info.AgentStatus...)
	cloned.XUIAgentStatus = append([]model.UpdateAgentStatus(nil), info.XUIAgentStatus...)
	if info.CacheExpiresAt != nil {
		value := *info.CacheExpiresAt
		cloned.CacheExpiresAt = &value
	}
	if info.RateLimitResetAt != nil {
		value := *info.RateLimitResetAt
		cloned.RateLimitResetAt = &value
	}
	return &cloned
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

// Custom installer URLs bypass the release asset digest obtained from GitHub's
// release API. Refuse them in automated updates so an admin action cannot
// silently turn into an unchecked remote-script execution path.
func rejectCustomUpdateScripts(req model.UpdateRequest) error {
	if strings.TrimSpace(req.ScriptURL) != "" || strings.TrimSpace(req.PSScriptURL) != "" {
		return fmt.Errorf("custom installer URLs are not allowed for automated updates")
	}
	return nil
}

func officialUpdateRepository(value string) (string, error) {
	value = firstNonEmptyString(value, officialVPSMonitorRepository)
	if value != officialVPSMonitorRepository {
		return "", fmt.Errorf("automated updates only support the official repository %s", officialVPSMonitorRepository)
	}
	return value, nil
}

func officialUpdatePackagePrefix(value string) (string, error) {
	value = firstNonEmptyString(value, officialVPSMonitorPrefix)
	if value != officialVPSMonitorPrefix {
		return "", fmt.Errorf("automated updates only support package prefix %s", officialVPSMonitorPrefix)
	}
	return value, nil
}

func selectedReleaseTag(requested, latestTag, latestVersion string) (string, error) {
	latestTag = strings.TrimSpace(latestTag)
	latestVersion = normalizeVersion(latestVersion)
	if latestTag == "" || latestVersion == "" || !isSafeReleaseTag(latestTag) {
		return "", fmt.Errorf("latest release has no safe semantic-version tag")
	}
	if requested = strings.TrimSpace(requested); requested != "" && normalizeVersion(requested) != latestVersion {
		return "", fmt.Errorf("only the verified latest release %s can be installed automatically", latestTag)
	}
	return latestTag, nil
}

func verifiedReleaseAssetURL(repo, tag, assetName string) (string, error) {
	if !isSafeGitHubRepository(repo) || !isSafeReleaseTag(tag) || !isSafeReleaseAssetName(assetName) {
		return "", fmt.Errorf("invalid verified update source")
	}
	return "https://github.com/" + repo + "/releases/download/" + tag + "/" + assetName, nil
}

func isSafeGitHubRepository(repo string) bool {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) != 2 {
		return false
	}
	return isSafeGitHubIdentifier(parts[0]) && isSafeGitHubIdentifier(parts[1])
}

func isSafeGitHubIdentifier(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafeReleaseTag(tag string) bool {
	if tag == "" || len(tag) > 100 {
		return false
	}
	version := strings.TrimPrefix(tag, "v")
	if _, ok := parseSemverParts(version); !ok {
		return false
	}
	for _, char := range tag {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafeReleaseAssetName(value string) bool {
	if value == "" || len(value) > 200 || strings.Contains(value, "/") {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func requiredReleaseAssetDigest(digests map[string]string, assetName string) (string, error) {
	digest := normalizeReleaseAssetDigest(digests[assetName])
	if digest == "" {
		return "", fmt.Errorf("release asset %s is missing a SHA-256 digest; refusing unchecked update", assetName)
	}
	return digest, nil
}

func normalizeReleaseAssetDigest(value string) string {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
	if len(value) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

func (a *App) startServerUpdate(req model.UpdateRequest) (*model.UpdateLatestInfo, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve server executable: %w", err)
	}
	installDir := filepath.Dir(exe)
	repo, err := officialUpdateRepository(req.Repo)
	if err != nil {
		return nil, err
	}
	packagePrefix, err := officialUpdatePackagePrefix(req.PackagePrefix)
	if err != nil {
		return nil, err
	}
	if err := rejectCustomUpdateScripts(req); err != nil {
		return nil, err
	}
	latest, err := a.fetchUpdateLatestInfo(repo, packagePrefix)
	if err != nil {
		return nil, err
	}
	version, err := selectedReleaseTag(req.Version, latest.LatestServerTag, latest.LatestServerVersion)
	if err != nil {
		return latest, err
	}
	if !isVersionNewer(version, latest.CurrentServerVersion) {
		return latest, fmt.Errorf("server is already up to date: current %s, latest %s", latest.CurrentServerVersion, firstNonEmptyString(latest.LatestServerVersion, latest.LatestVersion))
	}
	packageName, packageOK := updateServerPackageName(packagePrefix, runtime.GOOS, runtime.GOARCH)
	if !packageOK {
		return latest, fmt.Errorf("server update is unsupported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	packageSHA256, err := requiredReleaseAssetDigest(latest.ServerAssetDigests, packageName)
	if err != nil {
		return latest, err
	}
	packageURL, err := verifiedReleaseAssetURL(repo, version, packageName)
	if err != nil {
		return latest, err
	}
	serviceName := firstNonEmptyString(req.ServiceName, "vpsmonitor-server")
	command := buildServerSelfUpdateCommand(packageURL, installDir, serviceName, packageSHA256)
	if err := exec.Command("sh", "-c", command).Start(); err != nil {
		return latest, err
	}
	return latest, nil
}

func buildServerSelfUpdateCommand(packageURL, installDir, serviceName, packageSHA256 string) string {
	return fmt.Sprintf(`(sleep 2; {
set -eu
tmp="$(mktemp -d "${VPSMONITOR_TMP_DIR:-/var/tmp}/vpsmonitor-server-update.XXXXXX" 2>/dev/null || mktemp -d /tmp/vpsmonitor-server-update.XXXXXX)"
trap 'rm -rf "$tmp"' EXIT
package="$tmp/package.tar.gz"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL %[1]q -o "$package"
elif command -v wget >/dev/null 2>&1; then
  wget -O "$package" %[1]q
else
  echo "curl or wget is required for the server update" >&2; exit 127
fi
expected=%[2]q
if command -v sha256sum >/dev/null 2>&1; then actual="$(sha256sum "$package" | awk '{print $1}')";
elif command -v shasum >/dev/null 2>&1; then actual="$(shasum -a 256 "$package" | awk '{print $1}')";
else echo "sha256sum or shasum is required for verified updates" >&2; exit 127; fi
[ "$(printf '%%s' "$actual" | tr '[:upper:]' '[:lower:]')" = "$expected" ] || { echo "server package SHA-256 mismatch" >&2; exit 1; }
tar -xzf "$package" -C "$tmp"
binary="$(find "$tmp" -type f -name bridge-server | head -n 1)"
[ -n "$binary" ] || { echo "bridge-server not found in verified package" >&2; exit 1; }
install_dir=%[3]q
mkdir -p "$install_dir"
cp "$binary" "$install_dir/.bridge-server.new"
chmod 0755 "$install_dir/.bridge-server.new"
mv -f "$install_dir/.bridge-server.new" "$install_dir/bridge-server"
service_name=%[4]q
if command -v systemctl >/dev/null 2>&1; then systemctl restart "$service_name";
elif command -v rc-service >/dev/null 2>&1; then rc-service "$service_name" restart;
else echo "no supported service manager found" >&2; exit 1; fi
} >>/tmp/vpsmonitor-server-update.log 2>&1) >/dev/null 2>&1 &`, packageURL, packageSHA256, installDir, serviceName)
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
	repo, err := officialUpdateRepository(req.Repo)
	if err != nil {
		return model.UpdateResponse{}, err
	}
	packagePrefix, err := officialUpdatePackagePrefix(req.PackagePrefix)
	if err != nil {
		return model.UpdateResponse{}, err
	}
	if err := rejectCustomUpdateScripts(req); err != nil {
		return model.UpdateResponse{}, err
	}
	latest, err := a.fetchUpdateLatestInfo(repo, packagePrefix)
	if err != nil {
		return model.UpdateResponse{}, err
	}
	version, err := selectedReleaseTag(req.Version, latest.LatestClientTag, latest.LatestClientVersion)
	if err != nil {
		return model.UpdateResponse{}, err
	}
	serviceName := firstNonEmptyString(req.ServiceName, "")
	installSettings, _, err := a.store.GetClientInstallSettings()
	if err != nil {
		return model.UpdateResponse{}, err
	}
	latestSnapshots := make(map[string]model.AgentSnapshot)
	for _, snapshot := range a.store.ListLatest() {
		latestSnapshots[snapshot.AgentID] = snapshot
	}
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
		if !supportsVerifiedAutomaticUpdate(latestSnapshots[agent.AgentID]) {
			status.Reason = "client requires a one-time manual upgrade before verified automatic updates are available"
			statuses[len(statuses)-1] = status
			skipped++
			continue
		}
		packageSHA256, digestErr := requiredReleaseAssetDigest(latest.ClientAssetDigests, status.PackageName)
		if digestErr != nil {
			return model.UpdateResponse{}, digestErr
		}
		payload := map[string]any{
			"version":                 version,
			"repo":                    repo,
			"package_prefix":          packagePrefix,
			"package_sha256":          packageSHA256,
			"target_os":               status.OS,
			"target_arch":             status.Arch,
			"realm_auto_install":      installSettings.RealmAutoInstall,
			"realm_version":           firstNonEmptyString(installSettings.RealmVersion, defaultRealmVersion),
			"realm_download_base_url": installSettings.RealmDownloadBaseURL,
			"haproxy_auto_install":    installSettings.HAProxyAutoInstall,
		}
		if serviceName != "" {
			payload["service_name"] = serviceName
		}
		action, err := a.store.CreateXUIAction(agent.AgentID, model.XUIActionRequest{Kind: model.XUIActionUpdateClient, Payload: payload})
		if err != nil {
			return model.UpdateResponse{}, err
		}
		a.dispatchXUIActionRealtime(agent.AgentID, action)
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

func supportsVerifiedAutomaticUpdate(snapshot model.AgentSnapshot) bool {
	return snapshot.VerifiedSelfUpdate
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
	addGitHubAuthHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, githubHTTPError("fetch releases", resp)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Assets  []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	body, err := readExternalJSONResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read releases: %w", err)
	}
	if err := json.Unmarshal(body, &releases); err != nil {
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
		digests := make(map[string]string, len(release.Assets))
		for _, asset := range release.Assets {
			name := strings.TrimSpace(asset.Name)
			if name != "" {
				assets = append(assets, name)
				if digest := normalizeReleaseAssetDigest(asset.Digest); digest != "" {
					digests[name] = digest
				}
			}
		}
		candidate := releaseUpdateInfo{Tag: tag, Version: version, Assets: assets, AssetDigests: digests}
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
		digests := make(map[string]string, len(release.Assets))
		for _, asset := range release.Assets {
			name := strings.TrimSpace(asset.Name)
			if name != "" {
				assets = append(assets, name)
				if digest := normalizeReleaseAssetDigest(asset.Digest); digest != "" {
					digests[name] = digest
				}
			}
		}
		latest = releaseUpdateInfo{Tag: firstNonEmptyString(release.TagName, release.Name), Version: normalizeVersion(firstNonEmptyString(release.TagName, release.Name)), Assets: assets, AssetDigests: digests}
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
		ServerAssetDigests:    cloneStringMap(serverLatest.AssetDigests),
		ClientAssetDigests:    cloneStringMap(clientLatest.AssetDigests),
		FetchedAt:             time.Now().UTC(),
		Authenticated:         githubToken() != "",
	}
	applyGitHubRateLimit(info, resp)
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
	a.populate3XUIUpdateInfo(info, agents)
	return info, nil
}

func (a *App) create3XUIUpdateActions(req model.UpdateRequest) (model.UpdateResponse, error) {
	_ = a
	_ = req
	return model.UpdateResponse{}, fmt.Errorf("automatic 3x-ui updates are disabled because the upstream updater does not publish a verifiable package digest")
}

func shouldCreate3XUIUpdateAction(status model.UpdateAgentStatus, force bool) bool {
	if force {
		return status.Supported
	}
	return status.UpdateAvailable || (status.Supported && status.Version == "")
}

func (a *App) populate3XUIUpdateInfo(info *model.UpdateLatestInfo, agents []model.AgentRecord) {
	latest, err := fetchLatest3XUIRelease()
	if err != nil {
		info.Latest3XUIError = err.Error()
		return
	}
	info.Latest3XUIVersion = latest.Version
	info.Latest3XUITag = latest.Tag
	info.XUIAgentStatus = a.build3XUIUpdateStatuses(agents, latest.Version)
	for _, status := range info.XUIAgentStatus {
		switch {
		case status.Version == "":
			info.UnknownXUICount++
		case !status.Supported:
			info.UnsupportedXUICount++
		default:
			info.SupportedXUICount++
		}
		if status.UpdateAvailable {
			info.XUIUpdateAvailableCount++
		}
	}
}

func (a *App) build3XUIUpdateStatuses(agents []model.AgentRecord, latestVersion string) []model.UpdateAgentStatus {
	snapshots := a.store.ListLatest()
	snapshotByAgent := make(map[string]model.AgentSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotByAgent[snapshot.AgentID] = snapshot
	}
	statuses := make([]model.UpdateAgentStatus, 0, len(agents))
	for _, agent := range agents {
		statuses = append(statuses, build3XUIUpdateAgentStatus(agent, snapshotByAgent[agent.AgentID], latestVersion))
	}
	return statuses
}

func build3XUIUpdateAgentStatus(agent model.AgentRecord, snapshot model.AgentSnapshot, latestVersion string) model.UpdateAgentStatus {
	osName := strings.ToLower(strings.TrimSpace(firstNonEmptyString(snapshot.OS, agent.OS)))
	version := ""
	if snapshot.XUI != nil {
		version = normalizeVersion(snapshot.XUI.AppVersion)
	}
	status := model.UpdateAgentStatus{
		AgentID:     agent.AgentID,
		AgentName:   agent.AgentName,
		Version:     version,
		OS:          osName,
		Arch:        strings.ToLower(strings.TrimSpace(firstNonEmptyString(snapshot.Arch, agent.Arch))),
		PackageName: "3x-ui official update.sh",
	}
	if !agent.Config.XUI.Enabled {
		status.Reason = "x-ui config is disabled"
		return status
	}
	if osName == "" {
		status.Reason = "client has not reported os yet"
		return status
	}
	if osName != "linux" {
		status.Reason = "3x-ui official updater only supports linux"
		return status
	}
	status.Supported = true
	if version == "" {
		status.Reason = "3x-ui version has not been reported yet"
		return status
	}
	if latestVersion == "" {
		status.Reason = "latest 3x-ui version unavailable"
		return status
	}
	if !isVersionNewer(latestVersion, version) {
		status.Reason = "3x-ui is already up to date"
		return status
	}
	status.UpdateAvailable = true
	status.Reason = "update available"
	return status
}

func fetchLatest3XUIRelease() (releaseUpdateInfo, error) {
	return fetchLatestSemverRelease("MHSanaei/3x-ui")
}

func fetchLatestSemverRelease(repo string) (releaseUpdateInfo, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" || !strings.Contains(repo, "/") {
		return releaseUpdateInfo{}, fmt.Errorf("invalid repo: %s", repo)
	}
	ctx := contextWithTimeout(15 * time.Second)
	defer ctx.cancel()
	req, err := http.NewRequestWithContext(ctx.ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases?per_page=20", nil)
	if err != nil {
		return releaseUpdateInfo{}, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "VPSMonitor")
	addGitHubAuthHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return releaseUpdateInfo{}, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return releaseUpdateInfo{}, githubHTTPError("fetch releases", resp)
	}
	var releases []struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Assets  []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	body, err := readExternalJSONResponse(resp.Body)
	if err != nil {
		return releaseUpdateInfo{}, fmt.Errorf("read releases: %w", err)
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return releaseUpdateInfo{}, fmt.Errorf("decode releases: %w", err)
	}
	var latest releaseUpdateInfo
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
		if latest.Version == "" || isVersionNewer(version, latest.Version) {
			latest = releaseUpdateInfo{Tag: tag, Version: version, Assets: assets}
		}
	}
	if latest.Version == "" {
		return releaseUpdateInfo{}, fmt.Errorf("latest release has no semver tag")
	}
	return latest, nil
}

func addGitHubAuthHeader(req *http.Request) {
	if req == nil {
		return
	}
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func githubToken() string {
	return strings.TrimSpace(firstNonEmptyString(
		os.Getenv("VPSMONITOR_GITHUB_TOKEN"),
		os.Getenv("GITHUB_TOKEN"),
	))
}

func githubHTTPError(action string, resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("%s: empty response", action)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	resetText := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset"))
	if resetText != "" {
		if seconds, err := strconv.ParseInt(resetText, 10, 64); err == nil && seconds > 0 {
			resetAt := time.Unix(seconds, 0).UTC().Format(time.RFC3339)
			return fmt.Errorf("%s: http %d: %s (GitHub rate limit remaining=%s reset=%s; set VPSMONITOR_GITHUB_TOKEN to raise the limit)",
				action,
				resp.StatusCode,
				message,
				firstNonEmptyString(resp.Header.Get("X-RateLimit-Remaining"), "unknown"),
				resetAt,
			)
		}
	}
	return fmt.Errorf("%s: http %d: %s", action, resp.StatusCode, message)
}

func applyGitHubRateLimit(info *model.UpdateLatestInfo, resp *http.Response) {
	if info == nil || resp == nil {
		return
	}
	info.RateLimitRemaining = resp.Header.Get("X-RateLimit-Remaining")
	info.RateLimitLimit = resp.Header.Get("X-RateLimit-Limit")
	info.RateLimitResource = resp.Header.Get("X-RateLimit-Resource")
	resetText := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset"))
	if resetText == "" {
		return
	}
	seconds, err := strconv.ParseInt(resetText, 10, 64)
	if err != nil || seconds <= 0 {
		return
	}
	resetAt := time.Unix(seconds, 0).UTC()
	info.RateLimitResetAt = &resetAt
	if info.RateLimitRemaining == "0" {
		info.RateLimitError = fmt.Sprintf("GitHub API rate limit exhausted until %s", resetAt.Format(time.RFC3339))
	}
}

type releaseUpdateInfo struct {
	Tag          string
	Version      string
	Assets       []string
	AssetDigests map[string]string
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
