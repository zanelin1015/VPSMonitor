package panels

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

type XUIClient struct {
	baseURL       string
	config        config.XUIConfig
	client        *http.Client
	authenticated bool
	localStatus   localStatusSampler
}

var errXUIAuthExpired = errors.New("x-ui authentication expired")
var errXUILocalDBNotFound = errors.New("x-ui local db not found")

var defaultXUIDBPaths = []string{
	"/etc/x-ui/x-ui.db",
	"/etc/x-ui/3x-ui.db",
	"/usr/local/x-ui/x-ui.db",
	"/opt/1panel/docker/compose/3x-ui/db/x-ui.db",
}

type xuiEnvelope struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

type mutableXrayConfig struct {
	config map[string]any
	source string
	dbPath string
}

type xuiHTTPError struct {
	StatusCode  int
	Body        string
	AuthExpired bool
}

func (e xuiHTTPError) Error() string {
	if e.AuthExpired {
		return fmt.Sprintf("%v: http %d: %s", errXUIAuthExpired, e.StatusCode, strings.TrimSpace(e.Body))
	}
	return fmt.Sprintf("http %d: %s", e.StatusCode, e.Body)
}

func (e xuiHTTPError) Unwrap() error {
	if e.AuthExpired {
		return errXUIAuthExpired
	}
	return nil
}

type localStatusSampler struct {
	lastCPU     localCPUCounters
	hasCPU      bool
	lastNet     localNetCounters
	hasNet      bool
	lastNetTime time.Time
}

type localCPUCounters struct {
	idle  uint64
	total uint64
}

type localNetCounters struct {
	rx uint64
	tx uint64
}

func NewXUIClient(cfg config.XUIConfig, timeout time.Duration) (*XUIClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	baseURL := normalizeXUIBaseURL(cfg.BaseURL)
	return &XUIClient{
		baseURL: baseURL,
		config:  cfg,
		client: &http.Client{
			Timeout: timeout,
			Jar:     jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.SkipTLSVerify},
			},
		},
	}, nil
}

func normalizeXUIBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	keep := parts[:0]
	for _, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if lower == "panel" || lower == "login" {
			break
		}
		keep = append(keep, part)
	}
	if len(keep) == 0 {
		parsed.Path = ""
	} else {
		parsed.Path = "/" + strings.Join(keep, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func (c *XUIClient) Collect(ctx context.Context) *model.XUISnapshot {
	snapshot := &model.XUISnapshot{
		BaseURL:     c.baseURL,
		AppVersion:  detectLocal3XUIVersion(ctx),
		CollectedAt: time.Now().UTC(),
	}

	var apiErr error
	if c.canCollectAuthenticated() {
		if err := c.ensureLogin(ctx); err != nil {
			apiErr = err
		} else {
			apiErr = c.collectAuthenticated(ctx, snapshot)
			if isXUIAuthError(apiErr) {
				c.invalidateSession()
				if c.hasAPIToken() {
					// API-token mode has no session to refresh; fall back to local DB.
				} else if loginErr := c.login(ctx); loginErr != nil {
					apiErr = loginErr
				} else {
					apiErr = c.collectAuthenticated(ctx, snapshot)
				}
			}
			if apiErr == nil {
				return snapshot
			}
		}
	}

	if err := c.collectLocal(ctx, snapshot); err == nil {
		return snapshot
	} else if c.config.DBPath != "" || !errors.Is(err, errXUILocalDBNotFound) {
		if apiErr != nil {
			snapshot.Error = fmt.Sprintf("%v; local fallback failed: %v", apiErr, err)
			return snapshot
		}
		snapshot.Error = err.Error()
		return snapshot
	}

	if apiErr != nil {
		snapshot.Error = apiErr.Error()
		return snapshot
	}
	snapshot.Error = errXUILocalDBNotFound.Error()
	return snapshot
}

func (c *XUIClient) canCollectAuthenticated() bool {
	if c.baseURL == "" {
		return false
	}
	if c.hasAPIToken() {
		return true
	}
	return strings.TrimSpace(c.config.Username) != "" || strings.TrimSpace(c.config.Password) != ""
}

func (c *XUIClient) collectAuthenticated(ctx context.Context, snapshot *model.XUISnapshot) error {
	status, err := c.getStatus(ctx)
	if err != nil {
		return err
	}
	snapshot.ServerStatus = status

	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return err
	}
	if clients, err := c.getJSONList(ctx, "/panel/api/clients/list"); err == nil {
		inbounds = mergeXUIClientsIntoInbounds(inbounds, clients)
	}
	snapshot.Inbounds = inbounds

	configJSON, err := c.collectXrayConfig(ctx)
	if err != nil {
		return err
	}
	snapshot.RawConfig = configJSON
	snapshot.Outbounds = extractObjectList(configJSON["outbounds"])
	snapshot.RoutingRules = extractRoutingRules(configJSON["routing"])

	outboundTraffic, err := c.getJSONList(ctx, "/panel/xray/getOutboundsTraffic")
	if err == nil {
		snapshot.OutboundTraffic = outboundTraffic
	}
	return nil
}

func (c *XUIClient) collectXrayConfig(ctx context.Context) (map[string]any, error) {
	configJSON, err := c.getJSONObject(ctx, "/panel/api/server/getConfigJson")
	if err == nil && !xrayConfigNeedsFallback(configJSON) {
		return configJSON, nil
	}

	mutableConfig, fallbackErr := c.getMutableXrayConfig(ctx)
	if fallbackErr == nil {
		if err != nil {
			return mutableConfig.config, nil
		}
		return mergeRicherXrayConfig(configJSON, mutableConfig.config), nil
	}
	if err != nil {
		if fallbackErr != nil {
			return nil, fmt.Errorf("%w (xray template fallback failed: %v)", err, fallbackErr)
		}
		return nil, err
	}
	return configJSON, nil
}

func xrayConfigNeedsFallback(configJSON map[string]any) bool {
	outboundCount, ruleCount := xrayConfigCounts(configJSON)
	return outboundCount == 0 || ruleCount == 0
}

func xrayConfigCounts(configJSON map[string]any) (int, int) {
	return len(extractObjectList(configJSON["outbounds"])), len(extractRoutingRules(configJSON["routing"]))
}

func mergeRicherXrayConfig(primary map[string]any, fallback map[string]any) map[string]any {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	primaryOutbounds, primaryRules := xrayConfigCounts(primary)
	fallbackOutbounds, fallbackRules := xrayConfigCounts(fallback)
	if fallbackOutbounds <= primaryOutbounds && fallbackRules <= primaryRules {
		return primary
	}
	merged := make(map[string]any, len(primary))
	for key, value := range primary {
		merged[key] = value
	}
	if fallbackOutbounds > primaryOutbounds {
		merged["outbounds"] = fallback["outbounds"]
	}
	if fallbackRules > primaryRules {
		merged["routing"] = fallback["routing"]
	}
	return merged
}

func mergeXUIClientsIntoInbounds(inbounds []map[string]any, clients []map[string]any) []map[string]any {
	if len(inbounds) == 0 || len(clients) == 0 {
		return inbounds
	}
	byID := make(map[int]map[string]any, len(inbounds))
	for _, inbound := range inbounds {
		if id := intValue(inbound["id"]); id > 0 {
			byID[id] = inbound
		}
	}
	for _, rawClient := range clients {
		inboundIDs := xuiClientInboundIDs(rawClient)
		if len(inboundIDs) == 0 {
			continue
		}
		for _, inboundID := range inboundIDs {
			inbound := byID[inboundID]
			if inbound == nil {
				continue
			}
			appendClientToInbound(inbound, rawClient)
		}
	}
	return inbounds
}

func xuiClientInboundIDs(client map[string]any) []int {
	keys := []string{"inboundIds", "inbound_ids", "inboundIDs", "inbounds"}
	var result []int
	for _, key := range keys {
		result = append(result, intSliceValue(client[key])...)
	}
	for _, key := range []string{"inboundId", "inbound_id"} {
		if id := intValue(client[key]); id > 0 {
			result = append(result, id)
		}
	}
	if len(result) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(result))
	unique := result[:0]
	for _, id := range result {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func intSliceValue(raw any) []int {
	switch value := raw.(type) {
	case []int:
		return append([]int(nil), value...)
	case []int64:
		result := make([]int, 0, len(value))
		for _, item := range value {
			result = append(result, int(item))
		}
		return result
	case []float64:
		result := make([]int, 0, len(value))
		for _, item := range value {
			result = append(result, int(item))
		}
		return result
	case []any:
		result := make([]int, 0, len(value))
		for _, item := range value {
			if id := intValue(item); id > 0 {
				result = append(result, id)
			}
		}
		return result
	default:
		if id := intValue(raw); id > 0 {
			return []int{id}
		}
		return nil
	}
}

func appendClientToInbound(inbound map[string]any, rawClient map[string]any) {
	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return
	}
	clients := objectSlice(settings["clients"])
	client := normalizeXUIV3ClientForInbound(rawClient)
	if !containsXUIClient(clients, client) {
		clients = append(clients, client)
		settings["clients"] = clients
		if settingsText {
			if data, err := json.Marshal(settings); err == nil {
				inbound["settings"] = string(data)
			}
		} else {
			inbound["settings"] = settings
		}
	}
	stats := objectSlice(inbound["clientStats"])
	stat := xuiV3ClientTrafficStat(rawClient, intValue(inbound["id"]))
	if !containsXUIClientStat(stats, stat) {
		stats = append(stats, stat)
		inbound["clientStats"] = stats
	}
}

func normalizeXUIV3ClientForInbound(raw map[string]any) map[string]any {
	client := make(map[string]any, len(raw))
	for key, value := range raw {
		switch key {
		case "inboundIds", "inbound_ids", "inboundIDs", "inbounds", "inboundId", "inbound_id":
			continue
		default:
			client[key] = value
		}
	}
	if uuid := strings.TrimSpace(stringValue(client["uuid"])); uuid != "" {
		client["id"] = uuid
	} else {
		copyAlias(client, "id", "uuid")
	}
	copyAlias(client, "subId", "sub_id")
	copyAlias(client, "tgId", "tg_id")
	copyAlias(client, "limitIp", "limit_ip")
	copyAlias(client, "totalGB", "total")
	copyAlias(client, "expiryTime", "expiry_time")
	copyAlias(client, "lastOnline", "last_online")
	copyAlias(client, "createdAt", "created_at")
	copyAlias(client, "updatedAt", "updated_at")
	if _, ok := client["enable"]; !ok {
		copyAlias(client, "enable", "enabled")
	}
	return client
}

func xuiV3ClientTrafficStat(raw map[string]any, inboundID int) map[string]any {
	stat := make(map[string]any)
	for _, key := range []string{"id", "email", "up", "down", "allTime", "expiryTime", "total", "reset", "lastOnline", "enable"} {
		if value, ok := raw[key]; ok {
			stat[key] = value
		}
	}
	for _, key := range []string{"all_time", "expiry_time", "last_online"} {
		if value, ok := raw[key]; ok {
			stat[key] = value
		}
	}
	stat["inboundId"] = inboundID
	copyAlias(stat, "allTime", "all_time")
	copyAlias(stat, "expiryTime", "expiry_time")
	copyAlias(stat, "lastOnline", "last_online")
	return stat
}

func copyAlias(target map[string]any, preferred, fallback string) {
	if _, ok := target[preferred]; ok {
		return
	}
	if value, ok := target[fallback]; ok {
		target[preferred] = value
	}
}

func containsXUIClient(clients []map[string]any, candidate map[string]any) bool {
	email := strings.TrimSpace(stringValue(candidate["email"]))
	id := clientPrimaryID(candidate)
	for _, client := range clients {
		if email != "" && strings.EqualFold(strings.TrimSpace(stringValue(client["email"])), email) {
			return true
		}
		if id != "" && clientPrimaryID(client) == id {
			return true
		}
	}
	return false
}

func containsXUIClientStat(stats []map[string]any, candidate map[string]any) bool {
	email := strings.TrimSpace(stringValue(candidate["email"]))
	for _, stat := range stats {
		if email != "" && strings.EqualFold(strings.TrimSpace(stringValue(stat["email"])), email) {
			return true
		}
	}
	return false
}
