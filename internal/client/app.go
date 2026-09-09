package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/panels"
	"bridge-core/internal/realmconfig"
	"bridge-core/internal/version"
)

type App struct {
	config                 config.ClientConfig
	httpClient             *http.Client
	requestTimeout         time.Duration
	mu                     sync.RWMutex
	agentToken             string
	certificates           []model.XUILocalCertificate
	certsScannedAt         time.Time
	xuiClient              *panels.XUIClient
	xuiClientKey           string
	runOnceMu              sync.Mutex
	operationMu            sync.Mutex
	configLoadMu           sync.Mutex
	registrationPersisted  bool
	networkPolicySignature string
	xuiBootstrapSignature  string
	realmForwardSignature  string
	haProxySignature       string
	accessLogState         accessLogTailState
	capabilities           model.AgentCapabilities
}

const maxServerAPIResponseBytes = 16 << 20

func New(cfg config.ClientConfig) (*App, error) {
	timeout := time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	return &App{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.ServerSkipTLSVerify},
			},
			CheckRedirect: rejectCrossOriginServerRedirect,
		},
		requestTimeout: timeout,
		agentToken:     cfg.AgentToken,
		capabilities:   detectAgentCapabilities(osCommandRunner{}),
	}, nil
}

// rejectCrossOriginServerRedirect prevents agent credentials in custom
// headers from being forwarded to a destination other than the configured
// server. Same-host HTTP-to-HTTPS redirects remain supported.
func rejectCrossOriginServerRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || via[0].URL == nil || req == nil || req.URL == nil {
		return http.ErrUseLastResponse
	}
	if !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) {
		return http.ErrUseLastResponse
	}
	if len(via) >= 3 {
		return http.ErrUseLastResponse
	}
	return nil
}

func (a *App) RunOnce(ctx context.Context) error {
	a.runOnceMu.Lock()
	defer a.runOnceMu.Unlock()

	effectiveConfig, err := a.loadEffectiveConfig(ctx)
	if err != nil {
		return err
	}
	return a.runOnceWithConfig(ctx, effectiveConfig)
}

func (a *App) runOnceWithConfig(ctx context.Context, effectiveConfig model.ManagedAgentConfig) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()

	effectiveConfig = normalizeManagedConfig(effectiveConfig, a.config.AgentID, a.config.AgentName)
	effectiveConfig = enforceExclusiveForwardingMode(effectiveConfig)
	effectiveConfig.Entry = mergeLocalRealmConfigIntoEntry(effectiveConfig.Entry)
	a.applyNetworkPolicyIfNeeded(ctx, effectiveConfig.Entry.NetworkPolicy)
	a.applyRealmForwardingIfNeeded(ctx, effectiveConfig.Entry.PortForwarding)
	a.applyHAProxyIfNeeded(ctx, effectiveConfig.Entry.HAProxy)
	a.ensureXUIBootstrapIfNeeded(ctx, effectiveConfig.XUI)
	a.executePendingXUIActions(ctx, effectiveConfig)
	snapshot := a.collect(ctx, effectiveConfig)
	if err := a.pushSnapshot(ctx, snapshot); err != nil {
		return err
	}
	a.collectAndPushAccessLogs(ctx, effectiveConfig.XUI)
	return nil
}

func enforceExclusiveForwardingMode(cfg model.ManagedAgentConfig) model.ManagedAgentConfig {
	switch {
	case cfg.Features.HAProxy:
		cfg.Entry.PortForwarding.Enabled = false
		cfg.Entry.PortForwarding.Backend = "none"
	case cfg.Features.Realm:
		cfg.Entry.HAProxy.Enabled = false
	case cfg.Entry.HAProxy.Enabled:
		cfg.Entry.PortForwarding.Enabled = false
		cfg.Entry.PortForwarding.Backend = "none"
	case cfg.Entry.PortForwarding.Enabled && !strings.EqualFold(strings.TrimSpace(cfg.Entry.PortForwarding.Backend), "none"):
		cfg.Entry.HAProxy.Enabled = false
	}
	return cfg
}

func mergeLocalRealmConfigIntoEntry(entry model.AgentEntryConfig) model.AgentEntryConfig {
	if hasManagedClientRealmForwardRules(entry.PortForwarding) {
		return entry
	}
	return realmconfig.MergeSnapshotIntoEntry(entry, collectRealmSnapshot(entry.PortForwarding))
}

func (a *App) executePendingXUIActions(ctx context.Context, effectiveConfig model.ManagedAgentConfig) {
	actionsCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	actions, err := a.fetchPendingXUIActions(actionsCtx)
	cancel()
	if err != nil || len(actions) == 0 {
		return
	}
	for _, action := range actions {
		var xuiClient *panels.XUIClient
		var xuiErr error
		if effectiveConfig.XUI.Enabled {
			xuiClient, xuiErr = a.xuiClientForAction(effectiveConfig.XUI, action.XUIAuth)
		}
		action.XUIAuth = nil
		result := a.executeXUIAction(ctx, effectiveConfig, xuiClient, xuiErr, action)
		resultCtx, resultCancel := context.WithTimeout(ctx, a.requestTimeout)
		if reportErr := a.reportXUIActionResult(resultCtx, action.ID, result); reportErr != nil {
			log.Printf("report polled x-ui action %d result failed: %v", action.ID, reportErr)
		}
		resultCancel()
	}
}

func (a *App) executeXUIAction(ctx context.Context, effectiveConfig model.ManagedAgentConfig, xuiClient *panels.XUIClient, xuiErr error, action model.XUIAction) model.XUIActionResultRequest {
	result := model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded}
	switch action.Kind {
	case model.XUIActionUpdateClient:
		output, actionErr := a.startSelfUpdate(action.Payload)
		if actionErr != nil {
			result.Status = model.XUIActionStatusFailed
			result.Error = actionErr.Error()
		} else {
			result.Result = output
		}
	case model.XUIActionRestartXUI:
		output, actionErr := restartXUIService(ctx, action.Payload)
		if actionErr != nil {
			result.Status = model.XUIActionStatusFailed
			result.Error = actionErr.Error()
			result.Result = output
		} else {
			result.Result = output
		}
	case model.XUIActionExecuteCommand:
		output, actionErr := executeRemoteCommand(ctx, action.Payload)
		if actionErr != nil {
			result.Status = model.XUIActionStatusFailed
			result.Error = actionErr.Error()
			result.Result = output
		} else {
			result.Result = output
		}
	case model.XUIActionUpdate3XUI:
		output, actionErr := update3XUI(ctx, action.Payload)
		if actionErr != nil {
			result.Status = model.XUIActionStatusFailed
			result.Error = actionErr.Error()
			result.Result = output
		} else {
			result.Result = output
		}
	default:
		if !effectiveConfig.XUI.Enabled {
			result.Status = model.XUIActionStatusFailed
			result.Error = "x-ui config is disabled"
		} else if xuiErr != nil {
			result.Status = model.XUIActionStatusFailed
			result.Error = xuiErr.Error()
		} else {
			actionCtx, actionCancel := context.WithTimeout(ctx, a.xuiActionTimeout(action.Kind))
			output, actionErr := xuiClient.ExecuteAction(actionCtx, action)
			actionCancel()
			if actionErr != nil {
				result.Status = model.XUIActionStatusFailed
				result.Error = actionErr.Error()
			} else {
				result.Result = output
			}
		}
	}
	return result
}

func (a *App) xuiActionTimeout(kind string) time.Duration {
	timeout := a.requestTimeout
	switch kind {
	case model.XUIActionAddOutbound,
		model.XUIActionAddClient,
		model.XUIActionAddRoutingRule,
		model.XUIActionUpsertRoutingRule,
		model.XUIActionUpdateClientExpiry,
		model.XUIActionUpdateClientTraffic,
		model.XUIActionSetClientEnabled,
		model.XUIActionDeleteClient:
		if timeout < 90*time.Second {
			timeout = 90 * time.Second
		}
	}
	return timeout
}

func (a *App) startSelfUpdate(payload map[string]any) (map[string]any, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	targetOS := payloadString(payload, "target_os", "")
	targetArch := payloadString(payload, "target_arch", "")
	if targetOS != "" && targetOS != runtime.GOOS {
		return nil, fmt.Errorf("update target os mismatch: target=%s current=%s", targetOS, runtime.GOOS)
	}
	if targetArch != "" && targetArch != runtime.GOARCH {
		return nil, fmt.Errorf("update target arch mismatch: target=%s current=%s", targetArch, runtime.GOARCH)
	}
	installDir := filepath.Dir(exe)
	version := payloadString(payload, "version", "")
	repo := payloadString(payload, "repo", "zanelin1015/VPSMonitor")
	packagePrefix := payloadString(payload, "package_prefix", "VPSMonitor")
	if !isSafeUpdateRepository(repo) || !isSafeUpdateReleaseTag(version) {
		return nil, fmt.Errorf("update requires a verified repository and release tag")
	}
	if packagePrefix != officialClientUpdatePackagePrefix {
		return nil, fmt.Errorf("update requires the official package prefix %s", officialClientUpdatePackagePrefix)
	}
	packageSHA256, err := requiredUpdatePackageSHA256(payloadString(payload, "package_sha256", ""))
	if err != nil {
		return nil, err
	}
	packageName, err := clientUpdatePackageName(packagePrefix)
	if err != nil {
		return nil, err
	}
	packageURL, err := verifiedUpdatePackageURL(repo, version, packageName)
	if err != nil {
		return nil, err
	}

	if runtime.GOOS == "windows" {
		serviceName := payloadString(payload, "service_name", "VPSMonitorClient")
		command := buildWindowsSelfUpdateCommand(packageURL, installDir, serviceName, packageSHA256)
		cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", command)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start windows update: %w", err)
		}
		return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName}, nil
	}

	serviceName := payloadString(payload, "service_name", "vpsmonitor-client")
	if isOpenWrtLike() {
		command := buildUnixSelfUpdateCommand(packageURL, installDir, serviceName, packageSHA256, true)
		cmd := exec.Command("sh", "-c", command)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start OpenWrt update: %w", err)
		}
		return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName, "service_manager": "procd"}, nil
	}
	command := buildUnixSelfUpdateCommand(packageURL, installDir, serviceName, packageSHA256, false)
	cmd := exec.Command("sh", "-c", command)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start linux update: %w", err)
	}
	return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName}, nil
}

func buildWindowsSelfUpdateCommand(packageURL, installDir, serviceName, packageSHA256 string) string {
	return fmt.Sprintf(`Start-Sleep -Seconds 2
$packageUrl = %q
$expectedHash = %q
$installDir = %q
$serviceName = %q
$tempDir = Join-Path $env:TEMP ('vpsmonitor-update-' + [guid]::NewGuid().ToString('N'))
try {
  New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
  $packagePath = Join-Path $tempDir 'client.zip'
  Invoke-WebRequest -Uri $packageUrl -OutFile $packagePath -UseBasicParsing
  $actualHash = (Get-FileHash -Algorithm SHA256 -Path $packagePath).Hash.ToLowerInvariant()
  if ($actualHash -ne $expectedHash) { throw 'Downloaded client package SHA-256 does not match the verified release digest.' }
  Expand-Archive -Path $packagePath -DestinationPath $tempDir -Force
  $newBinary = Get-ChildItem -Path $tempDir -Filter 'bridge-client.exe' -Recurse | Select-Object -First 1
  if (-not $newBinary) { throw 'bridge-client.exe was not found in the verified package.' }
  $existingService = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
  if ($existingService) {
    Stop-Service -Name $serviceName -Force -ErrorAction Stop
    for ($i = 0; $i -lt 40; $i++) {
      if ((Get-Service -Name $serviceName -ErrorAction SilentlyContinue).Status -eq 'Stopped') { break }
      Start-Sleep -Milliseconds 500
    }
  }
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  Copy-Item -Path $newBinary.FullName -Destination (Join-Path $installDir 'bridge-client.exe') -Force
  if ($existingService) { Start-Service -Name $serviceName -ErrorAction Stop }
}
finally {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tempDir
}`, packageURL, packageSHA256, installDir, serviceName)
}

func buildUnixSelfUpdateCommand(packageURL, installDir, serviceName, packageSHA256 string, openWrt bool) string {
	restartCommand := `if command -v systemctl >/dev/null 2>&1; then systemctl restart "$service_name";
elif command -v rc-service >/dev/null 2>&1; then rc-service "$service_name" restart;
else echo "no supported service manager found" >&2; exit 1; fi`
	if openWrt {
		restartCommand = `service="/etc/init.d/$service_name"
[ -x "$service" ] || { echo "OpenWrt service was not found: $service" >&2; exit 1; }
"$service" restart || "$service" start`
	}
	return fmt.Sprintf(`(sleep 2; {
set -eu
tmp="$(mktemp -d "${VPSMONITOR_TMP_DIR:-/var/tmp}/vpsmonitor-client-update.XXXXXX" 2>/dev/null || mktemp -d /tmp/vpsmonitor-client-update.XXXXXX)"
trap 'rm -rf "$tmp"' EXIT
package="$tmp/package.tar.gz"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL %[1]q -o "$package"
elif command -v uclient-fetch >/dev/null 2>&1; then
  uclient-fetch -O "$package" %[1]q
elif command -v wget >/dev/null 2>&1; then
  wget -O "$package" %[1]q
else
  echo "curl, uclient-fetch, or wget is required for the client update" >&2; exit 127
fi
expected=%[2]q
if command -v sha256sum >/dev/null 2>&1; then actual="$(sha256sum "$package" | awk '{print $1}')";
elif command -v shasum >/dev/null 2>&1; then actual="$(shasum -a 256 "$package" | awk '{print $1}')";
elif command -v openssl >/dev/null 2>&1; then actual="$(openssl dgst -sha256 "$package" | awk '{print $NF}')";
else echo "a SHA-256 utility is required for verified updates" >&2; exit 127; fi
[ "$(printf '%%s' "$actual" | tr '[:upper:]' '[:lower:]')" = "$expected" ] || { echo "client package SHA-256 mismatch" >&2; exit 1; }
tar -xzf "$package" -C "$tmp"
binary="$(find "$tmp" -type f -name bridge-client | head -n 1)"
[ -n "$binary" ] || { echo "bridge-client not found in verified package" >&2; exit 1; }
install_dir=%[3]q
mkdir -p "$install_dir"
cp "$binary" "$install_dir/.bridge-client.new"
chmod 0755 "$install_dir/.bridge-client.new"
mv -f "$install_dir/.bridge-client.new" "$install_dir/bridge-client"
service_name=%[4]q
%[5]s
} >>/tmp/vpsmonitor-client-update.log 2>&1) >/dev/null 2>&1 &`, packageURL, packageSHA256, installDir, serviceName, restartCommand)
}

const officialClientUpdateRepository = "zanelin1015/VPSMonitor"
const officialClientUpdatePackagePrefix = "VPSMonitor"

func isSafeUpdateRepository(repo string) bool { return repo == officialClientUpdateRepository }

func isSafeUpdateReleaseTag(tag string) bool {
	if tag == "" || len(tag) > 100 || !strings.HasPrefix(tag, "v") {
		return false
	}
	if _, ok := parseSemver3(strings.TrimPrefix(tag, "v")); !ok {
		return false
	}
	return isSafeUpdateIdentifier(tag)
}

func isSafeUpdateIdentifier(value string) bool {
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

func verifiedUpdatePackageURL(repo, tag, packageName string) (string, error) {
	if !isSafeUpdateRepository(repo) || !isSafeUpdateReleaseTag(tag) || !isSafeUpdatePackageName(packageName) {
		return "", fmt.Errorf("invalid verified update source")
	}
	return "https://github.com/" + repo + "/releases/download/" + tag + "/" + packageName, nil
}

func clientUpdatePackageName(packagePrefix string) (string, error) {
	arch := runtime.GOARCH
	switch runtime.GOOS {
	case "windows":
		if arch != "amd64" && arch != "arm64" {
			return "", fmt.Errorf("client update is unsupported on windows/%s", arch)
		}
		return packagePrefix + "-client-windows-" + arch + ".zip", nil
	case "linux":
		if arch != "amd64" && arch != "arm64" && arch != "arm" {
			return "", fmt.Errorf("client update is unsupported on linux/%s", arch)
		}
		return packagePrefix + "-client-linux-" + arch + ".tar.gz", nil
	default:
		return "", fmt.Errorf("client update is unsupported on %s/%s", runtime.GOOS, arch)
	}
}

func isSafeUpdatePackageName(value string) bool {
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

func requiredUpdatePackageSHA256(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 64 {
		return "", fmt.Errorf("update is missing a SHA-256 package digest")
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return "", fmt.Errorf("update has an invalid SHA-256 package digest")
	}
	return value, nil
}

func payloadString(payload map[string]any, key string, fallback string) string {
	if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func payloadBool(payload map[string]any, key string, fallback bool) bool {
	if value, ok := payload[key].(bool); ok {
		return value
	}
	return fallback
}

func (a *App) collect(ctx context.Context, effectiveConfig model.ManagedAgentConfig) model.AgentSnapshot {
	snapshot := model.AgentSnapshot{
		AgentID:            a.config.AgentID,
		AgentName:          firstNonEmpty(effectiveConfig.AgentName, a.config.AgentName, a.config.AgentID),
		Version:            version.Version,
		VerifiedSelfUpdate: true,
		OS:                 runtime.GOOS,
		Arch:               runtime.GOARCH,
		SystemVersion:      currentSystemVersion(),
		ReportedAt:         time.Now().UTC(),
		Summary: model.VPSSummary{
			Hostname: currentHostname(),
		},
	}

	var lastErrs []string
	if effectiveConfig.XUI.Enabled {
		xuiClient, err := a.xuiClientFor(effectiveConfig.XUI)
		if err != nil {
			xuiErr := "x-ui: " + err.Error()
			lastErrs = append(lastErrs, xuiErr)
			snapshot.Logs = append(snapshot.Logs, xuiLogEntry(xuiErr))
		} else {
			xuiCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
			snapshot.XUI = xuiClient.Collect(xuiCtx)
			cancel()
			if snapshot.XUI != nil {
				snapshot.XUI.Certificates = a.localCertificates()
			}
			if snapshot.XUI != nil && snapshot.XUI.Error != "" {
				xuiErr := "x-ui: " + snapshot.XUI.Error
				lastErrs = append(lastErrs, xuiErr)
				snapshot.Logs = append(snapshot.Logs, xuiLogEntry(xuiErr))
			}
		}
	}
	snapshot.Realm = collectRealmSnapshot(effectiveConfig.Entry.PortForwarding)
	snapshot.HAProxy = collectHAProxySnapshot(ctx, effectiveConfig.Entry.HAProxy)
	if snapshot.HAProxy != nil && snapshot.HAProxy.Error != "" {
		haProxyErr := "haproxy: " + snapshot.HAProxy.Error
		snapshot.Logs = append(snapshot.Logs, model.AgentLogEntry{
			Time:    time.Now().UTC(),
			Level:   "error",
			Source:  "haproxy",
			Message: haProxyErr,
		})
	}
	snapshot.NetworkPolicy = collectNetworkPolicySnapshot(ctx, effectiveConfig.Entry.NetworkPolicy)

	snapshot.Summary = buildSummary(snapshot)
	if len(lastErrs) > 0 {
		snapshot.Summary.LastCollectionErr = strings.Join(lastErrs, "; ")
	}
	return snapshot
}
func xuiLogEntry(message string) model.AgentLogEntry {
	return model.AgentLogEntry{
		Time:    time.Now().UTC(),
		Level:   "error",
		Source:  "x-ui",
		Message: strings.TrimSpace(message),
	}
}

func (a *App) xuiClientFor(cfg config.XUIConfig) (*panels.XUIClient, error) {
	if strings.TrimSpace(cfg.APIToken) != "" {
		return panels.NewXUIClient(cfg, a.requestTimeout)
	}
	key := xuiClientCacheKey(cfg)
	if a.xuiClient != nil && a.xuiClientKey == key {
		return a.xuiClient, nil
	}
	xuiClient, err := panels.NewXUIClient(cfg, a.requestTimeout)
	if err != nil {
		return nil, err
	}
	a.xuiClient = xuiClient
	a.xuiClientKey = key
	return xuiClient, nil
}

func (a *App) xuiClientForAction(cfg config.XUIConfig, auth *model.XUIActionAuth) (*panels.XUIClient, error) {
	if auth == nil || strings.TrimSpace(auth.APIToken) == "" {
		return a.xuiClientFor(cfg)
	}
	cfg.APIToken = auth.APIToken
	return panels.NewXUIClient(cfg, a.requestTimeout)
}

func xuiClientCacheKey(cfg config.XUIConfig) string {
	return strings.Join([]string{
		cfg.BaseURL,
		cfg.DBPath,
		cfg.Username,
		cfg.Password,
		cfg.TwoFactorCode,
		fmt.Sprintf("%t", cfg.SkipTLSVerify),
	}, "\x00")
}

func (a *App) loadEffectiveConfig(ctx context.Context) (model.ManagedAgentConfig, error) {
	// Serialize bootstrap so concurrent polling and realtime requests cannot
	// register the same identity before its issued token is available.
	a.configLoadMu.Lock()
	defer a.configLoadMu.Unlock()

	token := firstNonEmpty(a.currentAgentToken(), a.config.AgentToken)
	var registered *model.AgentRegisterResponse
	if token == "" && a.config.RegistrationToken != "" {
		registerCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
		response, err := a.register(registerCtx)
		cancel()
		if err != nil {
			return model.ManagedAgentConfig{}, err
		}
		if response.AgentID != a.config.AgentID || strings.TrimSpace(response.AgentToken) == "" {
			return model.ManagedAgentConfig{}, fmt.Errorf("invalid registration credentials returned by server")
		}
		token = response.AgentToken
		a.setAgentToken(response.AgentToken)
		registered = &response
	}
	if token == "" {
		return model.ManagedAgentConfig{}, fmt.Errorf("registration_token or agent_token is required")
	}

	if !a.registrationPersisted && a.config.RegistrationToken != "" && a.config.ConfigPath != "" {
		if err := config.PersistClientRegistration(a.config.ConfigPath, a.config.AgentID, token); err != nil {
			return model.ManagedAgentConfig{}, fmt.Errorf("persist registration credentials: %w", err)
		}
		a.registrationPersisted = true
	}
	if registered != nil {
		return normalizeManagedConfig(registered.Config, registered.AgentID, registered.AgentName), nil
	}
	configCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	defer cancel()
	return a.fetchManagedConfig(configCtx)
}

func normalizeManagedConfig(cfg model.ManagedAgentConfig, fallbackAgentID string, fallbackAgentName string) model.ManagedAgentConfig {
	if cfg.AgentID == "" {
		cfg.AgentID = fallbackAgentID
	}
	if cfg.AgentName == "" {
		cfg.AgentName = firstNonEmpty(fallbackAgentName, fallbackAgentID)
	}
	return cfg
}

func (a *App) register(ctx context.Context) (model.AgentRegisterResponse, error) {
	reqBody := model.AgentRegisterRequest{
		AgentID:       a.config.AgentID,
		AgentName:     firstNonEmpty(a.config.AgentName, a.config.AgentID),
		Version:       version.Version,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		SystemVersion: currentSystemVersion(),
		Hostname:      currentHostname(),
		Capabilities:  a.capabilities,
		SeedConfig: model.ManagedAgentConfig{
			AgentID:   a.config.AgentID,
			AgentName: firstNonEmpty(a.config.AgentName, a.config.AgentID),
			Tags:      cloneStrings(a.config.Tags),
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return model.AgentRegisterResponse{}, fmt.Errorf("marshal register request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.config.ServerURL, "/")+"/api/v1/agents/register", bytes.NewReader(body))
	if err != nil {
		return model.AgentRegisterResponse{}, fmt.Errorf("build register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Registration-Token", a.config.RegistrationToken)
	if token := firstNonEmpty(a.currentAgentToken(), a.config.AgentToken); token != "" {
		req.Header.Set("X-Agent-Token", token)
	}

	var response model.AgentRegisterResponse
	if err := a.doJSON(req, &response); err != nil {
		return model.AgentRegisterResponse{}, fmt.Errorf("register agent: %w", err)
	}
	return response, nil
}

func (a *App) fetchManagedConfig(ctx context.Context) (model.ManagedAgentConfig, error) {
	url := strings.TrimRight(a.config.ServerURL, "/") + "/api/v1/agents/" + a.config.AgentID + "/config"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.ManagedAgentConfig{}, fmt.Errorf("build config request: %w", err)
	}
	req.Header.Set("X-Agent-Token", firstNonEmpty(a.currentAgentToken(), a.config.AgentToken))

	var cfg model.ManagedAgentConfig
	if err := a.doJSON(req, &cfg); err != nil {
		return model.ManagedAgentConfig{}, fmt.Errorf("fetch managed config: %w", err)
	}
	return normalizeManagedConfig(cfg, a.config.AgentID, a.config.AgentName), nil
}

func (a *App) pushSnapshot(ctx context.Context, snapshot model.AgentSnapshot) error {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	url := strings.TrimRight(a.config.ServerURL, "/") + "/api/v1/agents/" + a.config.AgentID + "/heartbeat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", firstNonEmpty(a.currentAgentToken(), a.config.AgentToken))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("heartbeat rejected with status %d", resp.StatusCode)
	}
	return nil
}

func (a *App) fetchPendingXUIActions(ctx context.Context) ([]model.XUIAction, error) {
	url := strings.TrimRight(a.config.ServerURL, "/") + "/api/v1/agents/" + a.config.AgentID + "/xui/actions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build x-ui action request: %w", err)
	}
	req.Header.Set("X-Agent-Token", firstNonEmpty(a.currentAgentToken(), a.config.AgentToken))

	var actions []model.XUIAction
	if err := a.doJSON(req, &actions); err != nil {
		return nil, fmt.Errorf("fetch x-ui actions: %w", err)
	}
	return actions, nil
}

func (a *App) reportXUIActionResult(ctx context.Context, actionID int64, result model.XUIActionResultRequest) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal x-ui action result: %w", err)
	}
	url := fmt.Sprintf("%s/api/v1/agents/%s/xui/actions/%d/result", strings.TrimRight(a.config.ServerURL, "/"), a.config.AgentID, actionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build x-ui action result request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", firstNonEmpty(a.currentAgentToken(), a.config.AgentToken))

	var action model.XUIAction
	if err := a.doJSON(req, &action); err != nil {
		return fmt.Errorf("report x-ui action result: %w", err)
	}
	return nil
}

func (a *App) doJSON(req *http.Request, target any) error {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := readServerAPIResponse(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func readServerAPIResponse(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxServerAPIResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxServerAPIResponseBytes {
		return nil, fmt.Errorf("server API response exceeds %d bytes", maxServerAPIResponseBytes)
	}
	return data, nil
}

func buildSummary(snapshot model.AgentSnapshot) model.VPSSummary {
	summary := snapshot.Summary

	if snapshot.XUI != nil {
		summary.PublicIPv4 = snapshot.XUI.ServerStatus.PublicIP.IPv4
		summary.PublicIPv6 = snapshot.XUI.ServerStatus.PublicIP.IPv6
		summary.CPU = snapshot.XUI.ServerStatus.CPU
		summary.MemUsed = snapshot.XUI.ServerStatus.Mem.Current
		summary.MemTotal = snapshot.XUI.ServerStatus.Mem.Total
		if snapshot.XUI.ServerStatus.Disk.Total > 0 {
			summary.DiskUsed = snapshot.XUI.ServerStatus.Disk.Current
			summary.DiskTotal = snapshot.XUI.ServerStatus.Disk.Total
		}
		summary.NetTrafficSent = snapshot.XUI.ServerStatus.NetTraffic.Sent
		summary.NetTrafficRecv = snapshot.XUI.ServerStatus.NetTraffic.Recv
		summary.NetTrafficTotal = snapshot.XUI.ServerStatus.NetTraffic.Sent + snapshot.XUI.ServerStatus.NetTraffic.Recv
		summary.NetIOUp = snapshot.XUI.ServerStatus.NetIO.Up
		summary.NetIODown = snapshot.XUI.ServerStatus.NetIO.Down
		summary.XrayState = snapshot.XUI.ServerStatus.Xray.State
		summary.InboundCount = len(snapshot.XUI.Inbounds)
		summary.OutboundCount = len(snapshot.XUI.Outbounds)
		summary.RoutingRuleCount = len(snapshot.XUI.RoutingRules)
	}
	if summary.DiskTotal == 0 {
		if diskUsed, diskTotal, ok := readDiskUsage(); ok {
			summary.DiskUsed = diskUsed
			summary.DiskTotal = diskTotal
		}
	}

	return summary
}

func currentHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return hostname
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (a *App) currentAgentToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentToken
}

func (a *App) setAgentToken(token string) {
	if token == "" {
		return
	}
	a.mu.Lock()
	a.agentToken = token
	a.mu.Unlock()
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func floatValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func uintValue(v any) uint64 {
	switch n := v.(type) {
	case float64:
		return uint64(n)
	case int:
		return uint64(n)
	case int64:
		return uint64(n)
	case uint64:
		return n
	default:
		return 0
	}
}
