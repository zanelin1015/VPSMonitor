package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	observedIP             string
	observedIPAt           time.Time
	runOnceMu              sync.Mutex
	networkPolicySignature string
	xuiBootstrapSignature  string
	realmForwardSignature  string
	haProxySignature       string
	accessLogState         accessLogTailState
	capabilities           model.AgentCapabilities
}

func New(cfg config.ClientConfig) (*App, error) {
	timeout := time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	return &App{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.ServerSkipTLSVerify},
			},
		},
		requestTimeout: timeout,
		agentToken:     cfg.AgentToken,
		capabilities:   detectAgentCapabilities(osCommandRunner{}),
	}, nil
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
		_ = a.reportXUIActionResult(resultCtx, action.ID, result)
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
	version := payloadString(payload, "version", "latest")
	repo := payloadString(payload, "repo", "zanelin1015/VPSMonitor")
	packagePrefix := payloadString(payload, "package_prefix", "VPSMonitor")

	if runtime.GOOS == "windows" {
		scriptURL := payloadString(payload, "ps_script_url", "https://raw.githubusercontent.com/"+repo+"/main/install.ps1")
		serviceName := payloadString(payload, "service_name", "VPSMonitorClient")
		command := buildWindowsSelfUpdateCommand(scriptURL, version, repo, packagePrefix, installDir, serviceName)
		cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", command)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start windows update: %w", err)
		}
		return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName}, nil
	}

	scriptURL := payloadString(payload, "script_url", "https://raw.githubusercontent.com/"+repo+"/main/install.sh")
	serviceName := payloadString(payload, "service_name", "vpsmonitor-client")
	realmAutoInstall := payloadBool(payload, "realm_auto_install", false)
	realmVersion := payloadString(payload, "realm_version", "v2.9.4")
	realmDownloadBaseURL := payloadString(payload, "realm_download_base_url", "")
	haProxyAutoInstall := payloadBool(payload, "haproxy_auto_install", false)
	if haProxyAutoInstall {
		realmAutoInstall = false
	}
	if isOpenWrtLike() {
		openWrtScriptURL := payloadString(payload, "openwrt_script_url", openWrtInstallerURL(scriptURL))
		command := buildUnixSelfUpdateCommand(openWrtScriptURL, version, repo, packagePrefix, installDir, serviceName, realmAutoInstall, realmVersion, realmDownloadBaseURL, haProxyAutoInstall, true)
		cmd := exec.Command("sh", "-c", command)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start OpenWrt update: %w", err)
		}
		return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName, "service_manager": "procd"}, nil
	}
	command := buildUnixSelfUpdateCommand(scriptURL, version, repo, packagePrefix, installDir, serviceName, realmAutoInstall, realmVersion, realmDownloadBaseURL, haProxyAutoInstall, false)
	cmd := exec.Command("sh", "-c", command)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start linux update: %w", err)
	}
	return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName}, nil
}

func buildWindowsSelfUpdateCommand(scriptURL, version, repo, packagePrefix, installDir, serviceName string) string {
	return fmt.Sprintf(`Start-Sleep -Seconds 2; $env:VPSMONITOR_ASSUME_YES='true'; $env:VPSMONITOR_VERSION=%q; $env:VPSMONITOR_REPO=%q; $env:VPSMONITOR_PACKAGE_PREFIX=%q; $env:VPSMONITOR_CLIENT_DIR=%q; $env:VPSMONITOR_CLIENT_SERVICE=%q; $scriptPath=Join-Path $env:TEMP ('vpsmonitor-install-' + [guid]::NewGuid().ToString('N') + '.ps1'); try { iwr -UseBasicParsing %q -OutFile $scriptPath; powershell -NoProfile -ExecutionPolicy Bypass -File $scriptPath client *> "$env:TEMP\vpsmonitor-client-update.log" } finally { Remove-Item -Force -ErrorAction SilentlyContinue $scriptPath }`, version, repo, packagePrefix, installDir, serviceName, scriptURL)
}

func buildUnixSelfUpdateCommand(scriptURL, version, repo, packagePrefix, installDir, serviceName string, realmAutoInstall bool, realmVersion, realmDownloadBaseURL string, haProxyAutoInstall, openWrt bool) string {
	downloadCommand := fmt.Sprintf(`(curl -fsSL %[1]q -o "$tmp" || wget -O "$tmp" %[1]q)`, scriptURL)
	installerShell := "bash"
	if openWrt {
		downloadCommand = fmt.Sprintf(`if command -v uclient-fetch >/dev/null 2>&1; then uclient-fetch -O "$tmp" %[1]q; elif command -v wget >/dev/null 2>&1; then wget -O "$tmp" %[1]q; else curl -fL %[1]q -o "$tmp"; fi`, scriptURL)
		installerShell = "sh"
	}
	return fmt.Sprintf(`(sleep 2; { tmp=""; trap 'if [ -n "$tmp" ]; then rm -f "$tmp"; fi' EXIT; tmp_base="${VPSMONITOR_TMP_DIR:-/var/tmp}"; tmp="$(mktemp "$tmp_base/vpsmonitor-install.XXXXXX.sh" 2>/dev/null || mktemp /tmp/vpsmonitor-install.XXXXXX.sh)" || exit 1; %[1]s && exec 3<"$tmp" && rm -f "$tmp" && tmp="" && env VPSMONITOR_ASSUME_YES=true VPSMONITOR_VERSION=%[2]q VPSMONITOR_REPO=%[3]q VPSMONITOR_PACKAGE_PREFIX=%[4]q VPSMONITOR_CLIENT_DIR=%[5]q VPSMONITOR_CLIENT_SERVICE=%[6]q VPSMONITOR_REALM_AUTO_INSTALL=%[7]q VPSMONITOR_REALM_VERSION=%[8]q VPSMONITOR_REALM_DOWNLOAD_BASE_URL=%[9]q VPSMONITOR_HAPROXY_AUTO_INSTALL=%[10]q %[11]s -s -- client <&3; } >>/tmp/vpsmonitor-client-update.log 2>&1) >/dev/null 2>&1 &`, downloadCommand, version, repo, packagePrefix, installDir, serviceName, strconv.FormatBool(realmAutoInstall), realmVersion, realmDownloadBaseURL, strconv.FormatBool(haProxyAutoInstall), installerShell)
}

func openWrtInstallerURL(scriptURL string) string {
	scriptURL = strings.TrimSpace(scriptURL)
	if scriptURL == "" {
		return scriptURL
	}
	base := scriptURL
	suffix := ""
	if index := strings.IndexAny(base, "?#"); index >= 0 {
		suffix = base[index:]
		base = base[:index]
	}
	if strings.HasSuffix(base, "/install.sh") {
		return strings.TrimSuffix(base, "/install.sh") + "/install-openwrt.sh" + suffix
	}
	return scriptURL
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
		AgentID:       a.config.AgentID,
		AgentName:     firstNonEmpty(effectiveConfig.AgentName, a.config.AgentName, a.config.AgentID),
		Version:       version.Version,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		SystemVersion: currentSystemVersion(),
		ReportedAt:    time.Now().UTC(),
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
	snapshot.NetworkPolicy = collectNetworkPolicySnapshot(ctx, effectiveConfig.Entry.NetworkPolicy)

	snapshot.Summary = buildSummary(snapshot)
	if outboundIP := a.detectOutboundIP(ctx); outboundIP != "" {
		snapshot.Summary.ObservedIP = outboundIP
	}
	if len(lastErrs) > 0 {
		snapshot.Summary.LastCollectionErr = strings.Join(lastErrs, "; ")
	}
	return snapshot
}

func (a *App) detectOutboundIP(ctx context.Context) string {
	a.mu.RLock()
	if a.observedIP != "" && time.Since(a.observedIPAt) < 10*time.Minute {
		cached := a.observedIP
		a.mu.RUnlock()
		return cached
	}
	a.mu.RUnlock()

	timeout := 3 * time.Second
	if a.requestTimeout > 0 && a.requestTimeout < timeout {
		timeout = a.requestTimeout
	}
	detectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, endpoint := range []string{"https://ipinfo.io/json", "https://api.ipify.org?format=json"} {
		ip := a.fetchOutboundIP(detectCtx, endpoint)
		if ip == "" {
			continue
		}
		a.mu.Lock()
		a.observedIP = ip
		a.observedIPAt = time.Now()
		a.mu.Unlock()
		return ip
	}
	return ""
}

func (a *App) fetchOutboundIP(ctx context.Context, endpoint string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "VPSMonitor")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return ""
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if ip := net.ParseIP(strings.TrimSpace(payload.IP)); ip != nil {
		return ip.String()
	}
	return ""
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
	if a.config.RegistrationToken != "" {
		registerCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
		defer cancel()

		response, err := a.register(registerCtx)
		if err != nil {
			return model.ManagedAgentConfig{}, err
		}
		a.setAgentToken(response.AgentToken)
		return normalizeManagedConfig(response.Config, response.AgentID, response.AgentName), nil
	}

	if firstNonEmpty(a.currentAgentToken(), a.config.AgentToken) != "" {
		configCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
		defer cancel()
		return a.fetchManagedConfig(configCtx)
	}

	return model.ManagedAgentConfig{}, fmt.Errorf("registration_token or agent_token is required")
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

	body, err := io.ReadAll(resp.Body)
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
