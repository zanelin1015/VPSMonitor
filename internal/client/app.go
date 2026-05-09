package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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
	"bridge-core/internal/version"
)

type App struct {
	config         config.ClientConfig
	httpClient     *http.Client
	requestTimeout time.Duration
	mu             sync.RWMutex
	agentToken     string
	certificates   []model.XUILocalCertificate
	certsScannedAt time.Time
	xuiClient      *panels.XUIClient
	xuiClientKey   string
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
	}, nil
}

func (a *App) RunOnce(ctx context.Context) error {
	effectiveConfig, err := a.loadEffectiveConfig(ctx)
	if err != nil {
		return err
	}
	a.executePendingXUIActions(ctx, effectiveConfig)
	snapshot := a.collect(ctx, effectiveConfig)
	return a.pushSnapshot(ctx, snapshot)
}

func (a *App) executePendingXUIActions(ctx context.Context, effectiveConfig model.ManagedAgentConfig) {
	actionsCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	actions, err := a.fetchPendingXUIActions(actionsCtx)
	cancel()
	if err != nil || len(actions) == 0 {
		return
	}
	var xuiClient *panels.XUIClient
	var xuiErr error
	if effectiveConfig.XUI.Enabled {
		xuiClient, xuiErr = a.xuiClientFor(effectiveConfig.XUI)
	}
	for _, action := range actions {
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
		default:
			if !effectiveConfig.XUI.Enabled {
				result.Status = model.XUIActionStatusFailed
				result.Error = "x-ui config is disabled"
			} else if xuiErr != nil {
				result.Status = model.XUIActionStatusFailed
				result.Error = xuiErr.Error()
			} else {
				actionCtx, actionCancel := context.WithTimeout(ctx, a.requestTimeout)
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
		resultCtx, resultCancel := context.WithTimeout(ctx, a.requestTimeout)
		_ = a.reportXUIActionResult(resultCtx, action.ID, result)
		resultCancel()
	}
}

func (a *App) startSelfUpdate(payload map[string]any) (map[string]any, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	installDir := filepath.Dir(exe)
	version := payloadString(payload, "version", "latest")
	repo := payloadString(payload, "repo", "zanelin1015/VPSMonitor")
	packagePrefix := payloadString(payload, "package_prefix", "VPSMonitor")

	if runtime.GOOS == "windows" {
		scriptURL := payloadString(payload, "ps_script_url", "https://raw.githubusercontent.com/"+repo+"/main/install.ps1")
		serviceName := payloadString(payload, "service_name", "VPSMonitorClient")
		command := fmt.Sprintf(`Start-Sleep -Seconds 2; $env:VPSMONITOR_ASSUME_YES='true'; $env:VPSMONITOR_VERSION=%q; $env:VPSMONITOR_REPO=%q; $env:VPSMONITOR_PACKAGE_PREFIX=%q; $env:VPSMONITOR_CLIENT_DIR=%q; $env:VPSMONITOR_CLIENT_SERVICE=%q; iwr -UseBasicParsing %q -OutFile "$env:TEMP\vpsmonitor-install.ps1"; powershell -NoProfile -ExecutionPolicy Bypass -File "$env:TEMP\vpsmonitor-install.ps1" client *> "$env:TEMP\vpsmonitor-client-update.log"`, version, repo, packagePrefix, installDir, serviceName, scriptURL)
		cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", command)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start windows update: %w", err)
		}
		return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName}, nil
	}

	scriptURL := payloadString(payload, "script_url", "https://raw.githubusercontent.com/"+repo+"/main/install.sh")
	serviceName := payloadString(payload, "service_name", "vpsmonitor-client")
	command := fmt.Sprintf(`(sleep 2; tmp="$(mktemp /tmp/vpsmonitor-install.XXXXXX.sh)"; (curl -fsSL %[1]q -o "$tmp" || wget -O "$tmp" %[1]q) && chmod +x "$tmp" && env VPSMONITOR_ASSUME_YES=true VPSMONITOR_VERSION=%[2]q VPSMONITOR_REPO=%[3]q VPSMONITOR_PACKAGE_PREFIX=%[4]q VPSMONITOR_CLIENT_DIR=%[5]q VPSMONITOR_CLIENT_SERVICE=%[6]q bash "$tmp" client >>/tmp/vpsmonitor-client-update.log 2>&1) >/dev/null 2>&1 &`, scriptURL, version, repo, packagePrefix, installDir, serviceName)
	cmd := exec.Command("sh", "-c", command)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start linux update: %w", err)
	}
	return map[string]any{"status": "started", "install_dir": installDir, "service_name": serviceName}, nil
}

func payloadString(payload map[string]any, key string, fallback string) string {
	if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func (a *App) collect(ctx context.Context, effectiveConfig model.ManagedAgentConfig) model.AgentSnapshot {
	snapshot := model.AgentSnapshot{
		AgentID:    a.config.AgentID,
		AgentName:  firstNonEmpty(effectiveConfig.AgentName, a.config.AgentName, a.config.AgentID),
		Version:    version.Version,
		ReportedAt: time.Now().UTC(),
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

func xuiClientCacheKey(cfg config.XUIConfig) string {
	return strings.Join([]string{
		cfg.BaseURL,
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
		AgentID:   a.config.AgentID,
		AgentName: firstNonEmpty(a.config.AgentName, a.config.AgentID),
		Version:   version.Version,
		Hostname:  currentHostname(),
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
