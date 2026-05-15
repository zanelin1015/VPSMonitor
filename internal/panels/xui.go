package panels

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"

	_ "modernc.org/sqlite"
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
		if lower == "panel" || lower == "xui" || lower == "login" {
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
		CollectedAt: time.Now().UTC(),
	}

	if err := c.collectLocal(ctx, snapshot); err == nil {
		return snapshot
	} else if c.config.DBPath != "" || !errors.Is(err, errXUILocalDBNotFound) {
		snapshot.Error = err.Error()
		return snapshot
	}

	if err := c.ensureLogin(ctx); err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	if err := c.collectAuthenticated(ctx, snapshot); err != nil {
		if isXUIAuthError(err) {
			c.invalidateSession()
			if loginErr := c.login(ctx); loginErr != nil {
				snapshot.Error = loginErr.Error()
				return snapshot
			}
			err = c.collectAuthenticated(ctx, snapshot)
		}
		if err != nil {
			snapshot.Error = err.Error()
		}
	}
	return snapshot
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
	snapshot.Inbounds = inbounds

	configJSON, err := c.collectXrayConfig(ctx)
	if err != nil {
		return err
	}
	snapshot.RawConfig = configJSON
	snapshot.Outbounds = extractObjectList(configJSON["outbounds"])
	snapshot.RoutingRules = extractRoutingRules(configJSON["routing"])

	outboundTraffic, err := c.getJSONList(ctx, "/panel/xray/getOutboundsTraffic")
	if err != nil {
		return err
	}
	snapshot.OutboundTraffic = outboundTraffic
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

func (c *XUIClient) ExecuteAction(ctx context.Context, action model.XUIAction) (map[string]any, error) {
	if err := c.ensureActionSession(ctx); err != nil {
		if !actionCanUseLocalXrayFallback(action.Kind) {
			return nil, err
		}
		c.invalidateSession()
	}
	result, err := c.executeActionAuthenticated(ctx, action)
	if err == nil || !isXUIAuthError(err) {
		return result, err
	}
	c.invalidateSession()
	if loginErr := c.login(ctx); loginErr != nil {
		return nil, loginErr
	}
	return c.executeActionAuthenticated(ctx, action)
}

func actionCanUseLocalXrayFallback(kind string) bool {
	switch kind {
	case model.XUIActionAddOutbound, model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule:
		return true
	default:
		return false
	}
}

func (c *XUIClient) executeActionAuthenticated(ctx context.Context, action model.XUIAction) (map[string]any, error) {
	switch action.Kind {
	case model.XUIActionAddOutbound:
		return c.addOutbound(ctx, action.Payload)
	case model.XUIActionAddRoutingRule:
		return c.addRoutingRule(ctx, action.Payload)
	case model.XUIActionUpsertRoutingRule:
		return c.upsertRoutingRule(ctx, action.Payload)
	case model.XUIActionUpdateClientExpiry:
		return c.updateClientExpiry(ctx, action.Payload)
	default:
		return nil, fmt.Errorf("unsupported x-ui action kind: %s", action.Kind)
	}
}

func (c *XUIClient) addInbound(ctx context.Context, payload map[string]any, localCertificates []model.XUILocalCertificate) (map[string]any, error) {
	inbound, err := payloadObject(payload, "inbound")
	if err != nil {
		return nil, err
	}
	resolvedCertificate, err := injectLocalCertificate(inbound, payload, localCertificates)
	if err != nil {
		return nil, err
	}
	result, err := c.postJSON(ctx, "/panel/api/inbounds/add", inbound)
	if err != nil {
		return nil, err
	}
	response := map[string]any{
		"message": result.Msg,
		"obj":     decodeEnvelopeObject(result.Obj),
	}
	if resolvedCertificate != nil {
		response["certificate"] = resolvedCertificate
	}
	return response, nil
}

func (c *XUIClient) addOutbound(ctx context.Context, payload map[string]any) (map[string]any, error) {
	outbound, err := payloadObject(payload, "outbound")
	if err != nil {
		return nil, err
	}
	tag := stringFromMap(outbound, "tag")
	if tag == "" {
		return nil, fmt.Errorf("outbound.tag is required")
	}
	if err := validateOutboundConfig(outbound); err != nil {
		return nil, err
	}

	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	configJSON := mutableConfig.config
	outbounds := objectSlice(configJSON["outbounds"])
	for _, existing := range outbounds {
		if stringFromMap(existing, "tag") == tag {
			return nil, fmt.Errorf("outbound tag already exists: %s", tag)
		}
	}
	configJSON["outbounds"] = append(outbounds, outbound)

	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"outbound_tag": tag,
		"restarted":    true,
	}, nil
}

func (c *XUIClient) addRoutingRule(ctx context.Context, payload map[string]any) (map[string]any, error) {
	rule, err := payloadObject(payload, "rule")
	if err != nil {
		return nil, err
	}
	if stringFromMap(rule, "type") == "" {
		rule["type"] = "field"
	}
	if stringFromMap(rule, "outboundTag") == "" && stringFromMap(rule, "balancerTag") == "" {
		return nil, fmt.Errorf("rule.outboundTag or rule.balancerTag is required")
	}

	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	configJSON := mutableConfig.config
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	rules = append(rules, rule)
	routing["rules"] = rules
	configJSON["routing"] = routing

	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"rule_index": len(rules),
		"restarted":  true,
	}, nil
}

func (c *XUIClient) upsertRoutingRule(ctx context.Context, payload map[string]any) (map[string]any, error) {
	rule, err := payloadObject(payload, "rule")
	if err != nil {
		return nil, err
	}
	if stringFromMap(rule, "type") == "" {
		rule["type"] = "field"
	}
	if stringFromMap(rule, "outboundTag") == "" && stringFromMap(rule, "balancerTag") == "" {
		return nil, fmt.Errorf("rule.outboundTag or rule.balancerTag is required")
	}

	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	configJSON := mutableConfig.config

	outboundAdded := false
	outboundUpdated := false
	if rawOutbound, ok := payload["outbound"]; ok && rawOutbound != nil {
		outbound, ok := rawOutbound.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("outbound must be an object")
		}
		normalizeOutboundForXUI(outbound)
		tag := stringFromMap(outbound, "tag")
		if tag == "" {
			return nil, fmt.Errorf("outbound.tag is required")
		}
		if err := validateOutboundConfig(outbound); err != nil {
			return nil, err
		}
		outbounds := objectSlice(configJSON["outbounds"])
		foundIndex := -1
		for index, existing := range outbounds {
			if stringFromMap(existing, "tag") == tag {
				foundIndex = index
				break
			}
		}
		if foundIndex >= 0 {
			outbounds[foundIndex] = outbound
			configJSON["outbounds"] = outbounds
			outboundUpdated = true
		} else {
			configJSON["outbounds"] = append(outbounds, outbound)
			outboundAdded = true
		}
	}
	if outboundTag := stringFromMap(rule, "outboundTag"); outboundTag != "" {
		found := false
		for _, existing := range objectSlice(configJSON["outbounds"]) {
			if stringFromMap(existing, "tag") == outboundTag {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("outbound tag not found: %s", outboundTag)
		}
	}

	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	ruleIndex := intValue(payload["rule_index"])
	updated := false
	if ruleIndex > 0 {
		if ruleIndex > len(rules) {
			return nil, fmt.Errorf("routing rule index out of range: %d", ruleIndex)
		}
		rules[ruleIndex-1] = rule
		updated = true
	} else {
		rules = append(rules, rule)
		ruleIndex = len(rules)
	}
	routing["rules"] = rules
	configJSON["routing"] = routing

	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"rule_index":       ruleIndex,
		"updated":          updated,
		"outbound_added":   outboundAdded,
		"outbound_updated": outboundUpdated,
		"restarted":        true,
	}, nil
}

func (c *XUIClient) updateClientExpiry(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	email := strings.TrimSpace(stringFromMap(payload, "email"))
	expiryTime := int64Value(payload["expiry_time"])
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if expiryTime <= 0 {
		return nil, fmt.Errorf("expiry_time is required")
	}

	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, err
	}
	var inbound map[string]any
	for _, item := range inbounds {
		if inboundID > 0 && intValue(item["id"]) == inboundID {
			inbound = item
			break
		}
		if inboundTag != "" && stringValue(item["tag"]) == inboundTag {
			inbound = item
			break
		}
	}
	if inbound == nil {
		return nil, fmt.Errorf("inbound not found for client %s", email)
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	var updatedClient map[string]any
	for _, client := range clients {
		if strings.TrimSpace(stringValue(client["email"])) == email {
			client["expiryTime"] = expiryTime
			updatedClient = client
			break
		}
	}
	if updatedClient == nil {
		return nil, fmt.Errorf("client not found in inbound: %s", email)
	}

	inboundID = intValue(inbound["id"])
	if clientID := stringValue(updatedClient["id"]); clientID != "" {
		clientSettings, _ := json.Marshal(map[string]any{"clients": []map[string]any{updatedClient}})
		if result, err := c.postJSON(ctx, "/panel/api/inbounds/updateClient/"+url.PathEscape(clientID), map[string]any{
			"id":       inboundID,
			"settings": string(clientSettings),
		}); err == nil {
			if err := c.restartXrayService(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"message": result.Msg, "email": email, "expiry_time": expiryTime, "restarted": true}, nil
		}
	}

	settings["clients"] = clients
	body, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal inbound settings: %w", err)
	}
	if settingsText {
		inbound["settings"] = string(body)
	} else {
		inbound["settings"] = settings
	}
	result, err := c.postJSON(ctx, fmt.Sprintf("/panel/api/inbounds/update/%d", inboundID), inbound)
	if err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"message": result.Msg, "email": email, "expiry_time": expiryTime, "restarted": true}, nil
}

func decodeInboundSettings(raw any) (map[string]any, bool, error) {
	switch value := raw.(type) {
	case string:
		var settings map[string]any
		if strings.TrimSpace(value) == "" {
			return map[string]any{"clients": []map[string]any{}}, true, nil
		}
		if err := json.Unmarshal([]byte(value), &settings); err != nil {
			return nil, true, fmt.Errorf("decode inbound settings: %w", err)
		}
		return settings, true, nil
	case map[string]any:
		return value, false, nil
	default:
		return objectMap(raw), false, nil
	}
}

func (c *XUIClient) collectLocal(ctx context.Context, snapshot *model.XUISnapshot) error {
	dbPath, explicit, err := c.resolveLocalDBPath()
	if err != nil {
		if explicit {
			return fmt.Errorf("x-ui local db: %w", err)
		}
		return err
	}

	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(dbPath))
	if err != nil {
		return fmt.Errorf("open x-ui local db %s: %w", dbPath, err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("open x-ui local db %s: %w", dbPath, err)
	}

	inbounds, err := readLocalInbounds(ctx, db)
	if err != nil {
		return fmt.Errorf("read x-ui local inbounds: %w", err)
	}
	configJSON, err := readLocalXrayConfig(ctx, db)
	if err != nil {
		return fmt.Errorf("read x-ui local xray config: %w", err)
	}
	configJSON = c.enrichLocalXrayConfig(ctx, configJSON)
	outboundTraffic, err := readLocalOutboundTraffic(ctx, db)
	if err != nil {
		return fmt.Errorf("read x-ui local outbound traffic: %w", err)
	}

	snapshot.ServerStatus = c.localServerStatus()
	snapshot.Inbounds = inbounds
	snapshot.RawConfig = configJSON
	snapshot.Outbounds = extractObjectList(configJSON["outbounds"])
	snapshot.RoutingRules = extractRoutingRules(configJSON["routing"])
	snapshot.OutboundTraffic = outboundTraffic
	return nil
}

func (c *XUIClient) enrichLocalXrayConfig(ctx context.Context, localConfig map[string]any) map[string]any {
	if !xrayConfigNeedsFallback(localConfig) || c.baseURL == "" || c.config.Username == "" || c.config.Password == "" {
		return localConfig
	}
	if err := c.ensureLogin(ctx); err != nil {
		return localConfig
	}
	remoteConfig, err := c.collectXrayConfig(ctx)
	if isXUIAuthError(err) {
		c.invalidateSession()
		if loginErr := c.login(ctx); loginErr == nil {
			remoteConfig, err = c.collectXrayConfig(ctx)
		}
	}
	if err != nil {
		return localConfig
	}
	return mergeRicherXrayConfig(localConfig, remoteConfig)
}

func (c *XUIClient) resolveLocalDBPath() (string, bool, error) {
	if path := strings.TrimSpace(c.config.DBPath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return path, true, err
		}
		return path, true, nil
	}
	if path := strings.TrimSpace(os.Getenv("XUI_DB_PATH")); path != "" {
		if _, err := os.Stat(path); err != nil {
			return path, true, err
		}
		return path, true, nil
	}
	if folder := strings.TrimSpace(os.Getenv("XUI_DB_FOLDER")); folder != "" {
		path := filepath.Join(folder, "x-ui.db")
		if _, err := os.Stat(path); err == nil {
			return path, false, nil
		}
	}
	for _, path := range defaultXUIDBPaths {
		if _, err := os.Stat(path); err == nil {
			return path, false, nil
		}
	}
	return "", false, errXUILocalDBNotFound
}

func sqliteReadOnlyDSN(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	return u.String()
}

func readLocalInbounds(ctx context.Context, db *sql.DB) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, up, down, total, all_time, remark, enable, expiry_time,
		       traffic_reset, last_traffic_reset_time, listen, port, protocol,
		       settings, stream_settings, tag, sniffing
		FROM inbounds
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inbounds []map[string]any
	for rows.Next() {
		var id, userID, port int
		var up, down, total, allTime, expiryTime, lastTrafficResetTime int64
		var enable bool
		var remark, trafficReset, listen, protocol, settings, streamSettings, tag, sniffing sql.NullString
		if err := rows.Scan(
			&id, &userID, &up, &down, &total, &allTime, &remark, &enable, &expiryTime,
			&trafficReset, &lastTrafficResetTime, &listen, &port, &protocol,
			&settings, &streamSettings, &tag, &sniffing,
		); err != nil {
			return nil, err
		}
		inbounds = append(inbounds, map[string]any{
			"id":                   id,
			"userId":               userID,
			"up":                   up,
			"down":                 down,
			"total":                total,
			"allTime":              allTime,
			"remark":               nullString(remark),
			"enable":               enable,
			"expiryTime":           expiryTime,
			"trafficReset":         nullString(trafficReset),
			"lastTrafficResetTime": lastTrafficResetTime,
			"listen":               nullString(listen),
			"port":                 port,
			"protocol":             nullString(protocol),
			"settings":             nullString(settings),
			"streamSettings":       nullString(streamSettings),
			"tag":                  nullString(tag),
			"sniffing":             nullString(sniffing),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stats, err := readLocalClientStats(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		id := intValue(inbound["id"])
		inbound["clientStats"] = stats[id]
	}
	return inbounds, nil
}

func readLocalClientStats(ctx context.Context, db *sql.DB) (map[int][]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, inbound_id, enable, email, up, down, all_time, expiry_time, total, reset, last_online
		FROM client_traffics
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int][]map[string]any)
	for rows.Next() {
		var id, inboundID, reset int
		var enable bool
		var email sql.NullString
		var up, down, allTime, expiryTime, total, lastOnline int64
		if err := rows.Scan(&id, &inboundID, &enable, &email, &up, &down, &allTime, &expiryTime, &total, &reset, &lastOnline); err != nil {
			return nil, err
		}
		result[inboundID] = append(result[inboundID], map[string]any{
			"id":         id,
			"inboundId":  inboundID,
			"enable":     enable,
			"email":      nullString(email),
			"up":         up,
			"down":       down,
			"allTime":    allTime,
			"expiryTime": expiryTime,
			"total":      total,
			"reset":      reset,
			"lastOnline": lastOnline,
		})
	}
	return result, rows.Err()
}

func readLocalXrayConfig(ctx context.Context, db *sql.DB) (map[string]any, error) {
	var raw sql.NullString
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'xrayTemplateConfig'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	configJSON, err := decodeLocalXrayTemplate(nullString(raw))
	if err != nil {
		return nil, err
	}
	return configJSON, nil
}

func writeLocalXrayConfig(ctx context.Context, db *sql.DB, configJSON map[string]any) error {
	body, err := json.Marshal(configJSON)
	if err != nil {
		return fmt.Errorf("marshal local xray template: %w", err)
	}
	result, err := db.ExecContext(ctx, `UPDATE settings SET value = ? WHERE key = 'xrayTemplateConfig'`, string(body))
	if err != nil {
		return fmt.Errorf("update local xray template: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read local xray template update count: %w", err)
	}
	if affected > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('xrayTemplateConfig', ?)`, string(body)); err != nil {
		return fmt.Errorf("insert local xray template: %w", err)
	}
	return nil
}

func decodeLocalXrayTemplate(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var current any
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return nil, err
	}
	for i := 0; i < 5; i++ {
		switch value := current.(type) {
		case string:
			if strings.TrimSpace(value) == "" {
				return map[string]any{}, nil
			}
			var next any
			if err := json.Unmarshal([]byte(value), &next); err != nil {
				return nil, err
			}
			current = next
		case map[string]any:
			if wrapped, ok := value["xraySetting"]; ok {
				current = wrapped
				continue
			}
			return value, nil
		default:
			return map[string]any{}, nil
		}
	}
	return nil, fmt.Errorf("xray template nested too deeply")
}

func readLocalOutboundTraffic(ctx context.Context, db *sql.DB) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, tag, up, down, total FROM outbound_traffics ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var id int
		var tag sql.NullString
		var up, down, total int64
		if err := rows.Scan(&id, &tag, &up, &down, &total); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"id":    id,
			"tag":   nullString(tag),
			"up":    up,
			"down":  down,
			"total": total,
		})
	}
	return result, rows.Err()
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func (c *XUIClient) localServerStatus() model.XUIServerStatus {
	var status model.XUIServerStatus
	status.CPU = c.localStatus.cpuPercent()
	status.Uptime = localUptime()
	if used, total, ok := localMemUsage(); ok {
		status.Mem.Current = used
		status.Mem.Total = total
	}
	if netCounters, ok := localNetUsage(); ok {
		now := time.Now()
		status.NetTraffic.Recv = netCounters.rx
		status.NetTraffic.Sent = netCounters.tx
		if c.localStatus.hasNet && netCounters.rx >= c.localStatus.lastNet.rx && netCounters.tx >= c.localStatus.lastNet.tx {
			elapsed := now.Sub(c.localStatus.lastNetTime).Seconds()
			if elapsed > 0 {
				status.NetIO.Down = uint64(float64(netCounters.rx-c.localStatus.lastNet.rx) / elapsed)
				status.NetIO.Up = uint64(float64(netCounters.tx-c.localStatus.lastNet.tx) / elapsed)
			}
		}
		c.localStatus.lastNet = netCounters
		c.localStatus.lastNetTime = now
		c.localStatus.hasNet = true
	}
	status.PublicIP = publicIPFromBaseURL(c.baseURL)
	if localXrayRunning() {
		status.Xray.State = "running"
	}
	return status
}

func (s *localStatusSampler) cpuPercent() float64 {
	cpu, ok := localCPUTotal()
	if !ok {
		return 0
	}
	defer func() {
		s.lastCPU = cpu
		s.hasCPU = true
	}()
	if !s.hasCPU || cpu.total <= s.lastCPU.total {
		return 0
	}
	totalDelta := cpu.total - s.lastCPU.total
	idleDelta := uint64(0)
	if cpu.idle > s.lastCPU.idle {
		idleDelta = cpu.idle - s.lastCPU.idle
	}
	if totalDelta == 0 || idleDelta > totalDelta {
		return 0
	}
	return (1 - float64(idleDelta)/float64(totalDelta)) * 100
}

func localCPUTotal() (localCPUCounters, bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return localCPUCounters{}, false
	}
	lines := strings.SplitN(string(data), "\n", 2)
	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return localCPUCounters{}, false
	}
	var total uint64
	var idle uint64
	for index, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return localCPUCounters{}, false
		}
		total += value
		if index == 3 || index == 4 {
			idle += value
		}
	}
	return localCPUCounters{idle: idle, total: total}, total > 0
}

func localUptime() uint64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return uint64(value)
}

func localMemUsage() (uint64, uint64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	var totalKB uint64
	var availableKB uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			totalKB = value
		case "MemAvailable":
			availableKB = value
		}
	}
	if totalKB == 0 {
		return 0, 0, false
	}
	if availableKB > totalKB {
		availableKB = totalKB
	}
	return (totalKB - availableKB) * 1024, totalKB * 1024, true
}

func localNetUsage() (localNetCounters, bool) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return localNetCounters{}, false
	}
	var total localNetCounters
	var fallback localNetCounters
	for lineNo, line := range strings.Split(string(data), "\n") {
		if lineNo < 2 {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		fallback.rx += rx
		fallback.tx += tx
		if name == "lo" || strings.HasPrefix(name, "lo:") {
			continue
		}
		total.rx += rx
		total.tx += tx
	}
	if total.rx == 0 && total.tx == 0 {
		total = fallback
	}
	return total, total.rx > 0 || total.tx > 0
}

func publicIPFromBaseURL(baseURL string) model.XUIPublicIP {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return model.XUIPublicIP{}
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil {
		return model.XUIPublicIP{}
	}
	if ip.To4() != nil {
		return model.XUIPublicIP{IPv4: ip.String()}
	}
	return model.XUIPublicIP{IPv6: ip.String()}
}

func localXrayRunning() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(string(comm)))
		if name == "xray" || strings.HasPrefix(name, "xray-") {
			return true
		}
	}
	return false
}

func (c *XUIClient) login(ctx context.Context) error {
	c.resetCookieJar()
	form := url.Values{}
	form.Set("username", c.config.Username)
	form.Set("password", c.config.Password)
	if c.config.TwoFactorCode != "" {
		form.Set("twoFactorCode", c.config.TwoFactorCode)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build x-ui login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var payload xuiEnvelope
	if err := c.doJSON(req, &payload); err != nil {
		return fmt.Errorf("x-ui login request failed: %w", err)
	}
	if !payload.Success {
		return fmt.Errorf("x-ui login failed: %s", payload.Msg)
	}
	c.authenticated = true
	return nil
}

func (c *XUIClient) ensureLogin(ctx context.Context) error {
	if c.hasAPIToken() {
		c.authenticated = true
		return nil
	}
	if c.authenticated {
		return nil
	}
	return c.login(ctx)
}

func (c *XUIClient) ensureActionSession(ctx context.Context) error {
	if c.hasAPIToken() {
		c.authenticated = true
		return nil
	}
	if !c.authenticated {
		return c.login(ctx)
	}
	if err := c.validateSession(ctx); err != nil {
		if !isXUIAuthError(err) {
			return err
		}
		c.invalidateSession()
		return c.login(ctx)
	}
	return nil
}

func (c *XUIClient) hasAPIToken() bool {
	return strings.TrimSpace(c.config.APIToken) != ""
}

func (c *XUIClient) validateSession(ctx context.Context) error {
	_, err := c.getStatus(ctx)
	return err
}

func (c *XUIClient) invalidateSession() {
	c.authenticated = false
	c.resetCookieJar()
}

func (c *XUIClient) resetCookieJar() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return
	}
	c.client.Jar = jar
}

func (c *XUIClient) getMutableXrayConfig(ctx context.Context) (mutableXrayConfig, error) {
	configJSON, err := c.getXrayTemplate(ctx)
	if err == nil {
		return mutableXrayConfig{config: configJSON, source: "api"}, nil
	}
	if !isXUIHTTPStatus(err, http.StatusNotFound) && !isXUIAuthError(err) {
		return mutableXrayConfig{}, err
	}
	localConfig, dbPath, localErr := c.readLocalMutableXrayConfig(ctx)
	if localErr != nil {
		return mutableXrayConfig{}, fmt.Errorf("%w (local db fallback failed: %v)", err, localErr)
	}
	return mutableXrayConfig{config: localConfig, source: "local_db", dbPath: dbPath}, nil
}

func (c *XUIClient) readLocalMutableXrayConfig(ctx context.Context) (map[string]any, string, error) {
	dbPath, _, err := c.resolveLocalDBPath()
	if err != nil {
		return nil, "", err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, "", fmt.Errorf("open x-ui db: %w", err)
	}
	defer db.Close()

	configJSON, err := readLocalXrayConfig(ctx, db)
	if err != nil {
		return nil, "", fmt.Errorf("read local xray template: %w", err)
	}
	return configJSON, dbPath, nil
}

func (c *XUIClient) updateMutableXrayConfig(ctx context.Context, mutableConfig mutableXrayConfig) error {
	switch mutableConfig.source {
	case "api":
		return c.updateXrayTemplate(ctx, mutableConfig.config)
	case "local_db":
		db, err := sql.Open("sqlite", mutableConfig.dbPath)
		if err != nil {
			return fmt.Errorf("open x-ui db: %w", err)
		}
		defer db.Close()
		return writeLocalXrayConfig(ctx, db, mutableConfig.config)
	default:
		return fmt.Errorf("unknown xray config source: %s", mutableConfig.source)
	}
}

func (c *XUIClient) getXrayTemplate(ctx context.Context) (map[string]any, error) {
	form := url.Values{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/panel/api/xray/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build x-ui template request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var payload xuiEnvelope
	if err := c.doJSON(req, &payload); err != nil {
		return nil, fmt.Errorf("x-ui template request failed: %w", err)
	}
	if !payload.Success {
		return nil, fmt.Errorf("x-ui template request failed: %s", payload.Msg)
	}

	var wrappedText string
	if err := json.Unmarshal(payload.Obj, &wrappedText); err != nil {
		return nil, fmt.Errorf("decode x-ui template wrapper text: %w", err)
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(wrappedText), &wrapper); err != nil {
		return nil, fmt.Errorf("decode x-ui template wrapper: %w", err)
	}

	raw, ok := wrapper["xraySetting"]
	if !ok {
		return nil, fmt.Errorf("x-ui template response missing xraySetting")
	}
	var rawText string
	if err := json.Unmarshal(raw, &rawText); err == nil {
		raw = json.RawMessage(rawText)
	}
	var configJSON map[string]any
	if err := json.Unmarshal(raw, &configJSON); err != nil {
		return nil, fmt.Errorf("decode x-ui template config: %w", err)
	}
	return configJSON, nil
}

func (c *XUIClient) updateXrayTemplate(ctx context.Context, configJSON map[string]any) error {
	body, err := json.Marshal(configJSON)
	if err != nil {
		return fmt.Errorf("marshal x-ui template config: %w", err)
	}
	form := url.Values{}
	form.Set("xraySetting", string(body))
	form.Set("outboundTestUrl", "https://www.google.com/generate_204")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/panel/api/xray/update", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build x-ui template update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var payload xuiEnvelope
	if err := c.doJSON(req, &payload); err != nil {
		return fmt.Errorf("x-ui template update failed: %w", err)
	}
	if !payload.Success {
		return fmt.Errorf("x-ui template update failed: %s", payload.Msg)
	}
	return nil
}

func (c *XUIClient) restartIfRequested(ctx context.Context, payload map[string]any) (bool, error) {
	restart, ok := payload["restart"].(bool)
	if !ok {
		restart = true
	}
	if !restart {
		return false, nil
	}
	if err := c.restartXrayService(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (c *XUIClient) restartXrayService(ctx context.Context) error {
	if err := c.postFormAction(ctx, "/panel/api/server/restartXrayService", url.Values{}); err != nil {
		if isXUIAuthError(err) {
			if localErr := restartLocalXUIService(ctx); localErr == nil {
				return nil
			} else {
				return fmt.Errorf("%w (local x-ui restart fallback failed: %v)", err, localErr)
			}
		}
		return err
	}
	return nil
}

func restartLocalXUIService(ctx context.Context) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("local x-ui restart is only supported on linux clients")
	}
	commands := [][]string{}
	if commandAvailable("systemctl") {
		commands = append(commands, []string{"systemctl", "restart", "x-ui"})
	}
	if commandAvailable("service") {
		commands = append(commands, []string{"service", "x-ui", "restart"})
	}
	if commandAvailable("x-ui") {
		commands = append(commands, []string{"x-ui", "restart"})
	}
	if len(commands) == 0 {
		return fmt.Errorf("systemctl/service/x-ui command not found")
	}
	var errs []string
	for _, command := range commands {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		output, err := exec.CommandContext(runCtx, command[0], command[1:]...).CombinedOutput()
		cancel()
		if err == nil && runCtx.Err() == nil {
			return nil
		}
		if runCtx.Err() != nil {
			err = runCtx.Err()
		}
		errs = append(errs, fmt.Sprintf("%s: %v: %s", strings.Join(command, " "), err, strings.TrimSpace(string(output))))
	}
	return errors.New(strings.Join(errs, "; "))
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (c *XUIClient) getStatus(ctx context.Context) (model.XUIServerStatus, error) {
	var status model.XUIServerStatus
	var payload xuiEnvelope
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/panel/api/server/status", nil)
	if err != nil {
		return status, fmt.Errorf("build x-ui status request: %w", err)
	}
	if err := c.doJSON(req, &payload); err != nil {
		return status, fmt.Errorf("x-ui status request failed: %w", err)
	}
	if !payload.Success {
		return status, fmt.Errorf("x-ui status failed: %s", payload.Msg)
	}
	if err := json.Unmarshal(payload.Obj, &status); err != nil {
		return status, fmt.Errorf("decode x-ui status: %w", err)
	}
	return status, nil
}

func (c *XUIClient) getJSONList(ctx context.Context, path string) ([]map[string]any, error) {
	var payload xuiEnvelope
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build x-ui request %s: %w", path, err)
	}
	if err := c.doJSON(req, &payload); err != nil {
		return nil, fmt.Errorf("x-ui request %s failed: %w", path, err)
	}
	if !payload.Success {
		return nil, fmt.Errorf("x-ui request %s failed: %s", path, payload.Msg)
	}
	var result []map[string]any
	if err := json.Unmarshal(payload.Obj, &result); err != nil {
		return nil, fmt.Errorf("decode x-ui response %s: %w", path, err)
	}
	return result, nil
}

func (c *XUIClient) postJSON(ctx context.Context, path string, body any) (xuiEnvelope, error) {
	var payload xuiEnvelope
	data, err := json.Marshal(body)
	if err != nil {
		return payload, fmt.Errorf("marshal x-ui request %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return payload, fmt.Errorf("build x-ui request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.doJSON(req, &payload); err != nil {
		return payload, fmt.Errorf("x-ui request %s failed: %w", path, err)
	}
	if !payload.Success {
		return payload, fmt.Errorf("x-ui request %s failed: %s", path, payload.Msg)
	}
	return payload, nil
}

func (c *XUIClient) postFormAction(ctx context.Context, path string, form url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build x-ui request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var payload xuiEnvelope
	if err := c.doJSON(req, &payload); err != nil {
		return fmt.Errorf("x-ui request %s failed: %w", path, err)
	}
	if !payload.Success {
		return fmt.Errorf("x-ui request %s failed: %s", path, payload.Msg)
	}
	return nil
}

func (c *XUIClient) getJSONObject(ctx context.Context, path string) (map[string]any, error) {
	var payload xuiEnvelope
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build x-ui request %s: %w", path, err)
	}
	if err := c.doJSON(req, &payload); err != nil {
		return nil, fmt.Errorf("x-ui request %s failed: %w", path, err)
	}
	if !payload.Success {
		return nil, fmt.Errorf("x-ui request %s failed: %s", path, payload.Msg)
	}
	var result map[string]any
	if err := json.Unmarshal(payload.Obj, &result); err != nil {
		return nil, fmt.Errorf("decode x-ui response %s: %w", path, err)
	}
	return result, nil
}

func (c *XUIClient) doJSON(req *http.Request, target any) error {
	if token := strings.TrimSpace(c.config.APIToken); token != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
			(resp.StatusCode == http.StatusNotFound && isXUISessionProbeRequest(req)) {
			return xuiHTTPError{StatusCode: resp.StatusCode, Body: string(body), AuthExpired: true}
		}
		return xuiHTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func isXUISessionProbeRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return strings.HasSuffix(req.URL.Path, "/panel/api/server/status")
}

func isXUIAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errXUIAuthExpired) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "forbidden") ||
		strings.Contains(text, "not login") ||
		strings.Contains(text, "not logged") ||
		strings.Contains(text, "session") ||
		strings.Contains(text, "登录")
}

func isXUIHTTPStatus(err error, statusCode int) bool {
	var httpErr xuiHTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == statusCode
}

func extractObjectList(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

func extractRoutingRules(raw any) []map[string]any {
	routing, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return extractObjectList(routing["rules"])
}

func payloadObject(payload map[string]any, key string) (map[string]any, error) {
	if payload == nil {
		return nil, fmt.Errorf("%s payload is required", key)
	}
	if raw, ok := payload[key]; ok {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		return obj, nil
	}
	if _, hasRestart := payload["restart"]; hasRestart {
		return nil, fmt.Errorf("%s is required", key)
	}
	return payload, nil
}

func objectMap(raw any) map[string]any {
	obj, ok := raw.(map[string]any)
	if !ok || obj == nil {
		return map[string]any{}
	}
	return obj
}

func objectSlice(raw any) []map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		return items
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if obj, ok := item.(map[string]any); ok {
				result = append(result, obj)
			}
		}
		return result
	default:
		return []map[string]any{}
	}
}

func stringFromMap(obj map[string]any, key string) string {
	return stringValue(obj[key])
}

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

func intValue(raw any) int {
	return int(int64Value(raw))
}

func int64Value(raw any) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}

func decodeEnvelopeObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func injectLocalCertificate(inbound map[string]any, payload map[string]any, localCertificates []model.XUILocalCertificate) (map[string]any, error) {
	streamSettings, encoded, err := jsonObjectField(inbound["streamSettings"])
	if err != nil {
		return nil, fmt.Errorf("decode inbound streamSettings: %w", err)
	}
	security := strings.ToLower(stringFromMap(streamSettings, "security"))
	if security != "tls" {
		writeJSONField(inbound, "streamSettings", streamSettings, encoded)
		return nil, nil
	}

	selector := objectMap(payload["tls_certificate"])
	if len(selector) == 0 {
		writeJSONField(inbound, "streamSettings", streamSettings, encoded)
		return nil, nil
	}

	certificateFile, keyFile, resolved, err := resolveLocalCertificate(selector, streamSettings, localCertificates)
	if err != nil {
		return nil, err
	}
	if certificateFile == "" || keyFile == "" {
		writeJSONField(inbound, "streamSettings", streamSettings, encoded)
		return nil, nil
	}

	tlsSettings := objectMap(streamSettings["tlsSettings"])
	tlsSettings["certificates"] = []map[string]any{
		{
			"certificateFile": certificateFile,
			"keyFile":         keyFile,
		},
	}
	streamSettings["tlsSettings"] = tlsSettings
	writeJSONField(inbound, "streamSettings", streamSettings, encoded)
	return resolved, nil
}

func jsonObjectField(raw any) (map[string]any, bool, error) {
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return map[string]any{}, true, nil
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return nil, true, err
		}
		return decoded, true, nil
	case map[string]any:
		return value, false, nil
	default:
		return objectMap(raw), false, nil
	}
}

func writeJSONField(target map[string]any, key string, value map[string]any, encoded bool) {
	if !encoded {
		target[key] = value
		return
	}
	body, err := json.Marshal(value)
	if err != nil {
		target[key] = value
		return
	}
	target[key] = string(body)
}

func validateOutboundConfig(outbound map[string]any) error {
	protocol := strings.ToLower(strings.TrimSpace(stringFromMap(outbound, "protocol")))
	if err := validateOutboundRealitySettings(outbound); err != nil {
		return err
	}
	switch protocol {
	case "vless":
		settings := objectMap(outbound["settings"])
		if validEndpoint(settings, "address", "port") {
			return nil
		}
		for _, item := range objectSlice(settings["vnext"]) {
			if validEndpoint(item, "address", "port") {
				return nil
			}
		}
		return fmt.Errorf("%s outbound requires a valid address and port", protocol)
	case "vmess":
		settings := objectMap(outbound["settings"])
		for _, item := range objectSlice(settings["vnext"]) {
			if validEndpoint(item, "address", "port") {
				return nil
			}
		}
		return fmt.Errorf("%s outbound requires a valid address and port", protocol)
	case "trojan", "shadowsocks", "http", "socks", "socks5":
		settings := objectMap(outbound["settings"])
		for _, item := range objectSlice(settings["servers"]) {
			if validEndpoint(item, "address", "port") {
				return nil
			}
		}
		return fmt.Errorf("%s outbound requires a valid address and port", protocol)
	default:
		return nil
	}
}

func normalizeOutboundForXUI(outbound map[string]any) {
	if strings.ToLower(strings.TrimSpace(stringFromMap(outbound, "protocol"))) != "vless" {
		return
	}
	settings := objectMap(outbound["settings"])
	if validEndpoint(settings, "address", "port") {
		if strings.TrimSpace(stringFromMap(settings, "encryption")) == "" {
			settings["encryption"] = "none"
		}
		outbound["settings"] = settings
		return
	}
	for _, item := range objectSlice(settings["vnext"]) {
		if !validEndpoint(item, "address", "port") {
			continue
		}
		settings["address"] = stringFromMap(item, "address")
		settings["port"] = intValue(item["port"])
		if users := objectSlice(item["users"]); len(users) > 0 {
			user := users[0]
			if id := strings.TrimSpace(stringFromMap(user, "id")); id != "" {
				settings["id"] = id
			}
			if flow := strings.TrimSpace(stringFromMap(user, "flow")); flow != "" {
				settings["flow"] = flow
			}
			if encryption := strings.TrimSpace(stringFromMap(user, "encryption")); encryption != "" {
				settings["encryption"] = encryption
			}
		}
		if strings.TrimSpace(stringFromMap(settings, "encryption")) == "" {
			settings["encryption"] = "none"
		}
		delete(settings, "vnext")
		outbound["settings"] = settings
		return
	}
}

func validateOutboundRealitySettings(outbound map[string]any) error {
	streamSettings := objectMap(outbound["streamSettings"])
	if strings.ToLower(strings.TrimSpace(stringFromMap(streamSettings, "security"))) != "reality" {
		return nil
	}
	realitySettings := objectMap(streamSettings["realitySettings"])
	if isPlaceholderValue(stringFromMap(realitySettings, "serverName")) || strings.TrimSpace(stringFromMap(realitySettings, "serverName")) == "" {
		return fmt.Errorf("reality outbound requires streamSettings.realitySettings.serverName")
	}
	if isPlaceholderValue(stringFromMap(realitySettings, "publicKey")) || strings.TrimSpace(stringFromMap(realitySettings, "publicKey")) == "" {
		return fmt.Errorf("reality outbound requires streamSettings.realitySettings.publicKey")
	}
	return nil
}

func validEndpoint(item map[string]any, addressKey, portKey string) bool {
	address := strings.TrimSpace(stringFromMap(item, addressKey))
	if address == "" || isPlaceholderValue(address) {
		return false
	}
	port := intValue(item[portKey])
	return port > 0 && port <= 65535
}

func isPlaceholderValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "undefined", "null", "nan":
		return true
	default:
		return false
	}
}

func resolveLocalCertificate(selector map[string]any, streamSettings map[string]any, inventory []model.XUILocalCertificate) (string, string, map[string]any, error) {
	mode := strings.ToLower(stringFromMap(selector, "mode"))
	switch mode {
	case "", "none":
		return "", "", nil, nil
	case "manual":
		certificateFile := stringFromMap(selector, "certificate_file")
		keyFile := stringFromMap(selector, "key_file")
		if certificateFile == "" || keyFile == "" {
			return "", "", nil, fmt.Errorf("manual tls certificate requires certificate_file and key_file")
		}
		return certificateFile, keyFile, map[string]any{
			"mode":             mode,
			"certificate_file": certificateFile,
			"key_file":         keyFile,
		}, nil
	case "inventory":
		inventoryID := stringFromMap(selector, "inventory_id")
		if inventoryID == "" {
			return "", "", nil, fmt.Errorf("inventory tls certificate requires inventory_id")
		}
		for _, cert := range inventory {
			if cert.ID == inventoryID {
				return cert.CertPath, cert.KeyPath, localCertificateResult(mode, cert), nil
			}
		}
		return "", "", nil, fmt.Errorf("local tls certificate not found: %s", inventoryID)
	case "domain_auto":
		serverName := strings.TrimSpace(stringFromMap(selector, "domain"))
		if serverName == "" {
			tlsSettings := objectMap(streamSettings["tlsSettings"])
			serverName = strings.TrimSpace(stringFromMap(tlsSettings, "serverName"))
		}
		if serverName == "" {
			return "", "", nil, fmt.Errorf("auto tls certificate matching requires a server name")
		}
		for _, cert := range inventory {
			if localCertificateMatchesDomain(cert, serverName) {
				return cert.CertPath, cert.KeyPath, localCertificateResult(mode, cert), nil
			}
		}
		return "", "", nil, fmt.Errorf("no local tls certificate matches domain %q", serverName)
	default:
		return "", "", nil, fmt.Errorf("unsupported tls certificate mode: %s", mode)
	}
}

func localCertificateResult(mode string, cert model.XUILocalCertificate) map[string]any {
	return map[string]any{
		"mode":      mode,
		"id":        cert.ID,
		"name":      cert.Name,
		"subject":   cert.Subject,
		"cert_path": cert.CertPath,
		"key_path":  cert.KeyPath,
	}
}

func localCertificateMatchesDomain(cert model.XUILocalCertificate, domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	if matchesCertificatePattern(strings.ToLower(cert.Subject), domain) {
		return true
	}
	for _, name := range cert.DNSNames {
		if matchesCertificatePattern(strings.ToLower(name), domain) {
			return true
		}
	}
	return false
}

func matchesCertificatePattern(pattern, domain string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == domain {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(domain, suffix)
	}
	return false
}
