package panels

import (
	"bytes"
	"context"
	"crypto/rand"
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
	case model.XUIActionAddClient:
		return c.addClient(ctx, action.Payload)
	case model.XUIActionAddOutbound:
		return c.addOutbound(ctx, action.Payload)
	case model.XUIActionAddRoutingRule:
		return c.addRoutingRule(ctx, action.Payload)
	case model.XUIActionUpsertRoutingRule:
		return c.upsertRoutingRule(ctx, action.Payload)
	case model.XUIActionUpdateClientExpiry:
		return c.updateClientExpiry(ctx, action.Payload)
	case model.XUIActionSetClientEnabled:
		return c.setClientEnabled(ctx, action.Payload)
	case model.XUIActionDeleteClient:
		return c.deleteClient(ctx, action.Payload)
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

func (c *XUIClient) addClient(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	client, err := payloadObject(payload, "client")
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(stringFromMap(client, "email"))
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" {
		return nil, fmt.Errorf("client.email is required")
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
		return nil, fmt.Errorf("inbound not found for new client %s", email)
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	for _, existing := range clients {
		if strings.EqualFold(strings.TrimSpace(stringValue(existing["email"])), email) {
			return nil, fmt.Errorf("client already exists in inbound: %s", email)
		}
	}
	normalizeInboundClient(client, stringValue(inbound["protocol"]))
	inboundID = intValue(inbound["id"])

	clientSettings, _ := json.Marshal(map[string]any{"clients": []map[string]any{client}})
	if result, err := c.postJSON(ctx, "/panel/api/inbounds/addClient", map[string]any{
		"id":       inboundID,
		"settings": string(clientSettings),
	}); err == nil {
		if err := c.restartXrayService(ctx); err != nil {
			return nil, err
		}
		return map[string]any{"message": result.Msg, "email": email, "client_id": clientPrimaryID(client), "inbound_id": inboundID, "restarted": true}, nil
	}

	clients = append(clients, client)
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
	return map[string]any{"message": result.Msg, "email": email, "client_id": clientPrimaryID(client), "inbound_id": inboundID, "restarted": true}, nil
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
	previousTag := firstNonEmptyString(
		stringFromMap(payload, "previous_outbound_tag"),
		stringFromMap(payload, "old_outbound_tag"),
	)
	removedPrevious := false
	renamingOutbound := previousTag != "" && tag != "" && previousTag != tag
	if renamingOutbound {
		removedPrevious = removeOutboundByTag(configJSON, previousTag)
	}
	outboundResult, err := upsertOutboundInConfig(configJSON, outbound, !renamingOutbound)
	if err != nil {
		return nil, err
	}
	if previousTag != "" && outboundResult.Tag != "" && previousTag != outboundResult.Tag {
		outboundResult.RemovedPrevious = removedPrevious || removeOutboundByTag(configJSON, previousTag)
		outboundResult.RoutingRefsUpdated = replaceRoutingOutboundTag(configJSON, previousTag, outboundResult.Tag)
	}

	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"outbound_tag":     outboundResult.Tag,
		"outbound_added":   outboundResult.Added,
		"outbound_updated": outboundResult.Updated,
		"outbound_reused":  outboundResult.Reused,
		"outbound_removed": outboundResult.RemovedPrevious,
		"routing_refs":     outboundResult.RoutingRefsUpdated,
		"restarted":        true,
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
	updated := false
	if existingIndex := findEquivalentRoutingRule(rules, rule); existingIndex >= 0 {
		rules[existingIndex] = rule
		rules = moveRoutingRuleToFront(rules, existingIndex)
		updated = true
	} else {
		rules = prependRoutingRule(rules, rule)
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
		"rule_index": 1,
		"updated":    updated,
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

	outboundResult := outboundUpsertResult{}
	if rawOutbound, ok := payload["outbound"]; ok && rawOutbound != nil {
		outbound, ok := rawOutbound.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("outbound must be an object")
		}
		requestedTag := stringFromMap(outbound, "tag")
		previousTag := firstNonEmptyString(
			stringFromMap(payload, "previous_outbound_tag"),
			stringFromMap(payload, "old_outbound_tag"),
		)
		removedPrevious := false
		renamingOutbound := previousTag != "" && requestedTag != "" && previousTag != requestedTag
		if renamingOutbound {
			removedPrevious = removeOutboundByTag(configJSON, previousTag)
		}
		outboundResult, err = upsertOutboundInConfig(configJSON, outbound, !renamingOutbound)
		if err != nil {
			return nil, err
		}
		if requestedTag != "" && outboundResult.Tag != "" && stringFromMap(rule, "outboundTag") == requestedTag {
			rule["outboundTag"] = outboundResult.Tag
		}
		if previousTag != "" && outboundResult.Tag != "" && previousTag != outboundResult.Tag {
			outboundResult.RemovedPrevious = removedPrevious || removeOutboundByTag(configJSON, previousTag)
			outboundResult.RoutingRefsUpdated = replaceRoutingOutboundTag(configJSON, previousTag, outboundResult.Tag)
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
		rules = moveRoutingRuleToFront(rules, ruleIndex-1)
		ruleIndex = 1
		updated = true
	} else if existingIndex := findEquivalentRoutingRule(rules, rule); existingIndex >= 0 {
		rules[existingIndex] = rule
		rules = moveRoutingRuleToFront(rules, existingIndex)
		ruleIndex = 1
		updated = true
	} else {
		rules = prependRoutingRule(rules, rule)
		ruleIndex = 1
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
		"outbound_added":   outboundResult.Added,
		"outbound_updated": outboundResult.Updated,
		"outbound_reused":  outboundResult.Reused,
		"outbound_removed": outboundResult.RemovedPrevious,
		"routing_refs":     outboundResult.RoutingRefsUpdated,
		"restarted":        true,
	}, nil
}

type outboundUpsertResult struct {
	Tag                string
	Added              bool
	Updated            bool
	Reused             bool
	RoutingRefsUpdated int
	RemovedPrevious    bool
}

func upsertOutboundInConfig(configJSON map[string]any, outbound map[string]any, reuseEquivalent ...bool) (outboundUpsertResult, error) {
	normalizeOutboundForXUI(outbound)
	tag := stringFromMap(outbound, "tag")
	if tag == "" {
		return outboundUpsertResult{}, fmt.Errorf("outbound.tag is required")
	}
	if err := validateOutboundConfig(outbound); err != nil {
		return outboundUpsertResult{}, err
	}

	outbounds := objectSlice(configJSON["outbounds"])
	for index, existing := range outbounds {
		if stringFromMap(existing, "tag") == tag {
			outbounds[index] = outbound
			configJSON["outbounds"] = outbounds
			return outboundUpsertResult{Tag: tag, Updated: true}, nil
		}
	}
	allowEquivalentReuse := true
	if len(reuseEquivalent) > 0 {
		allowEquivalentReuse = reuseEquivalent[0]
	}
	if allowEquivalentReuse {
		if existingIndex := findEquivalentOutbound(outbounds, outbound); existingIndex >= 0 {
			existingTag := stringFromMap(outbounds[existingIndex], "tag")
			if existingTag == "" {
				existingTag = tag
			}
			return outboundUpsertResult{Tag: existingTag, Reused: true}, nil
		}
	}
	configJSON["outbounds"] = append(outbounds, outbound)
	return outboundUpsertResult{Tag: tag, Added: true}, nil
}

func removeOutboundByTag(configJSON map[string]any, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	outbounds := objectSlice(configJSON["outbounds"])
	filtered := make([]map[string]any, 0, len(outbounds))
	removed := false
	for _, outbound := range outbounds {
		if strings.TrimSpace(stringFromMap(outbound, "tag")) == tag {
			removed = true
			continue
		}
		filtered = append(filtered, outbound)
	}
	if removed {
		configJSON["outbounds"] = filtered
	}
	return removed
}

func findEquivalentOutbound(outbounds []map[string]any, outbound map[string]any) int {
	target := canonicalOutbound(outbound)
	if target == "" {
		return -1
	}
	for index, existing := range outbounds {
		if canonicalOutbound(existing) == target {
			return index
		}
	}
	return -1
}

func canonicalOutbound(outbound map[string]any) string {
	normalized := make(map[string]any, len(outbound))
	for key, value := range outbound {
		if key == "tag" {
			continue
		}
		normalized[key] = normalizeXUIConfigValue(value)
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(body)
}

func prependRoutingRule(rules []map[string]any, rule map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(rules)+1)
	result = append(result, rule)
	result = append(result, rules...)
	return result
}

func moveRoutingRuleToFront(rules []map[string]any, index int) []map[string]any {
	if index <= 0 || index >= len(rules) {
		return rules
	}
	rule := rules[index]
	copy(rules[1:index+1], rules[0:index])
	rules[0] = rule
	return rules
}

func findEquivalentRoutingRule(rules []map[string]any, rule map[string]any) int {
	target := canonicalRoutingRule(rule)
	if target == "" {
		return -1
	}
	for index, existing := range rules {
		if canonicalRoutingRule(existing) == target {
			return index
		}
	}
	return -1
}

func canonicalRoutingRule(rule map[string]any) string {
	normalized := normalizeRoutingRuleForCompare(rule)
	body, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(body)
}

func normalizeRoutingRuleForCompare(rule map[string]any) map[string]any {
	normalized := make(map[string]any, len(rule)+1)
	hasType := false
	for key, value := range rule {
		if key == "type" {
			hasType = true
			if strings.TrimSpace(stringValue(value)) == "" {
				normalized[key] = "field"
				continue
			}
		}
		normalized[key] = normalizeRoutingRuleValue(value)
	}
	if !hasType {
		normalized["type"] = "field"
	}
	return normalized
}

func normalizeRoutingRuleValue(value any) any {
	return normalizeXUIConfigValue(value)
}

func normalizeXUIConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = normalizeXUIConfigValue(item)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, normalizeXUIConfigValue(item))
		}
		return result
	case []map[string]any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, normalizeXUIConfigValue(item))
		}
		return result
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return typed
	}
}

func replaceRoutingOutboundTag(configJSON map[string]any, oldTag string, newTag string) int {
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if oldTag == "" || newTag == "" || oldTag == newTag {
		return 0
	}
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	updated := 0
	for _, rule := range rules {
		if strings.TrimSpace(stringFromMap(rule, "outboundTag")) == oldTag {
			rule["outboundTag"] = newTag
			updated++
		}
	}
	if updated > 0 {
		routing["rules"] = rules
		configJSON["routing"] = routing
	}
	return updated
}

func replaceRoutingUser(configJSON map[string]any, oldEmail string, newEmail string) int {
	oldEmail = strings.TrimSpace(oldEmail)
	newEmail = strings.TrimSpace(newEmail)
	if oldEmail == "" || newEmail == "" || oldEmail == newEmail {
		return 0
	}
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	updated := 0
	for _, rule := range rules {
		users := stringListValue(rule["user"])
		if len(users) == 0 {
			continue
		}
		changed := false
		for index, user := range users {
			if strings.TrimSpace(user) == oldEmail {
				users[index] = newEmail
				changed = true
			}
		}
		if changed {
			rule["user"] = users
			updated++
		}
	}
	if updated > 0 {
		routing["rules"] = rules
		configJSON["routing"] = routing
	}
	return updated
}

func removeRoutingUser(configJSON map[string]any, email string) int {
	email = strings.TrimSpace(email)
	if email == "" {
		return 0
	}
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	if len(rules) == 0 {
		return 0
	}
	filtered := make([]map[string]any, 0, len(rules))
	updated := 0
	for _, rule := range rules {
		users := stringListValue(rule["user"])
		if len(users) == 0 {
			filtered = append(filtered, rule)
			continue
		}
		remaining := make([]string, 0, len(users))
		removed := false
		for _, user := range users {
			if strings.TrimSpace(user) == email {
				removed = true
				continue
			}
			remaining = append(remaining, user)
		}
		if !removed {
			filtered = append(filtered, rule)
			continue
		}
		updated++
		if len(remaining) > 0 {
			rule["user"] = remaining
			filtered = append(filtered, rule)
			continue
		}
		delete(rule, "user")
		if routingRuleHasMatcher(rule) {
			filtered = append(filtered, rule)
		}
	}
	if updated > 0 {
		routing["rules"] = filtered
		configJSON["routing"] = routing
	}
	return updated
}

func routingRuleHasMatcher(rule map[string]any) bool {
	for _, key := range []string{"inboundTag", "domain", "ip", "port", "sourcePort", "sourceIP", "network", "protocol"} {
		if len(stringListValue(rule[key])) > 0 || strings.TrimSpace(stringValue(rule[key])) != "" {
			return true
		}
	}
	return false
}

func stringListValue(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	default:
		return nil
	}
}

func (c *XUIClient) updateClientExpiry(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	email := strings.TrimSpace(stringFromMap(payload, "email"))
	previousEmail := firstNonEmptyString(
		stringFromMap(payload, "previous_email"),
		stringFromMap(payload, "old_email"),
	)
	lookupEmail := firstNonEmptyString(previousEmail, email)
	expiryTime := int64Value(payload["expiry_time"])
	enabled, hasEnabled := boolPayloadValue(payload, "enabled")
	if value, ok := boolPayloadValue(payload, "enable"); ok {
		enabled = value
		hasEnabled = true
	}
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
		if strings.TrimSpace(stringValue(client["email"])) == lookupEmail {
			client["expiryTime"] = expiryTime
			if hasEnabled {
				client["enable"] = enabled
			}
			if previousEmail != "" && previousEmail != email {
				client["email"] = email
			}
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
			routingRefsUpdated, err := c.replaceRoutingUserReferences(ctx, previousEmail, email)
			if err != nil {
				return nil, err
			}
			if err := c.restartXrayService(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"message": result.Msg, "email": email, "expiry_time": expiryTime, "enabled": updatedClient["enable"], "routing_refs": routingRefsUpdated, "restarted": true}, nil
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
	routingRefsUpdated, err := c.replaceRoutingUserReferences(ctx, previousEmail, email)
	if err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"message": result.Msg, "email": email, "expiry_time": expiryTime, "enabled": updatedClient["enable"], "routing_refs": routingRefsUpdated, "restarted": true}, nil
}

func (c *XUIClient) setClientEnabled(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	email := strings.TrimSpace(stringFromMap(payload, "email"))
	clientID := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(payload, "client_id"),
		stringFromMap(payload, "client_uuid"),
		stringFromMap(payload, "auth_uuid"),
	))
	enabled, hasEnabled := boolPayloadValue(payload, "enabled")
	if value, ok := boolPayloadValue(payload, "enable"); ok {
		enabled = value
		hasEnabled = true
	}
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" && clientID == "" {
		return nil, fmt.Errorf("email or client_id is required")
	}
	if !hasEnabled {
		return nil, fmt.Errorf("enabled is required")
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
		return nil, fmt.Errorf("inbound not found for client %s", firstNonEmptyString(email, clientID))
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	var updatedClient map[string]any
	for _, client := range clients {
		if clientMatchesDeletePayload(client, email, clientID) {
			client["enable"] = enabled
			if email == "" {
				email = strings.TrimSpace(stringValue(client["email"]))
			}
			if clientID == "" {
				clientID = clientPrimaryID(client)
			}
			updatedClient = client
			break
		}
	}
	if updatedClient == nil {
		return nil, fmt.Errorf("client not found in inbound: %s", firstNonEmptyString(email, clientID))
	}

	inboundID = intValue(inbound["id"])
	if updateID := clientPrimaryID(updatedClient); updateID != "" {
		clientSettings, _ := json.Marshal(map[string]any{"clients": []map[string]any{updatedClient}})
		if result, err := c.postJSON(ctx, "/panel/api/inbounds/updateClient/"+url.PathEscape(updateID), map[string]any{
			"id":       inboundID,
			"settings": string(clientSettings),
		}); err == nil {
			if err := c.restartXrayService(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"message": result.Msg, "email": email, "client_id": updateID, "inbound_id": inboundID, "enabled": updatedClient["enable"], "restarted": true}, nil
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
	return map[string]any{"message": result.Msg, "email": email, "client_id": clientID, "inbound_id": inboundID, "enabled": updatedClient["enable"], "restarted": true}, nil
}

func boolPayloadValue(payload map[string]any, key string) (bool, bool) {
	value, ok := payload[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on", "enable", "enabled":
			return true, true
		case "0", "false", "no", "off", "disable", "disabled":
			return false, true
		default:
			return false, false
		}
	case int:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case float64:
		return typed != 0, true
	default:
		return false, false
	}
}

func (c *XUIClient) deleteClient(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	email := strings.TrimSpace(stringFromMap(payload, "email"))
	clientID := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(payload, "client_id"),
		stringFromMap(payload, "client_uuid"),
		stringFromMap(payload, "auth_uuid"),
	))
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" && clientID == "" {
		return nil, fmt.Errorf("email or client_id is required")
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
		return nil, fmt.Errorf("inbound not found for client %s", firstNonEmptyString(email, clientID))
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	removedIndex := -1
	removedClientID := clientID
	for index, client := range clients {
		if clientMatchesDeletePayload(client, email, clientID) {
			removedIndex = index
			if email == "" {
				email = strings.TrimSpace(stringValue(client["email"]))
			}
			if removedClientID == "" {
				removedClientID = firstNonEmptyString(
					stringValue(client["id"]),
					stringValue(client["password"]),
					stringValue(client["email"]),
				)
			}
			break
		}
	}
	if removedIndex < 0 {
		return nil, fmt.Errorf("client not found in inbound: %s", firstNonEmptyString(email, clientID))
	}

	inboundID = intValue(inbound["id"])
	routingRefsUpdated, err := c.removeRoutingUserReferences(ctx, email)
	if err != nil {
		return nil, err
	}
	if removedClientID != "" {
		path := fmt.Sprintf("/panel/api/inbounds/%d/delClient/%s", inboundID, url.PathEscape(removedClientID))
		if result, err := c.postJSON(ctx, path, map[string]any{}); err == nil {
			if err := c.restartXrayService(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"message": result.Msg, "email": email, "client_id": removedClientID, "inbound_id": inboundID, "routing_refs": routingRefsUpdated, "restarted": true}, nil
		}
	}

	clients = append(clients[:removedIndex], clients[removedIndex+1:]...)
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
	return map[string]any{"message": result.Msg, "email": email, "client_id": removedClientID, "inbound_id": inboundID, "routing_refs": routingRefsUpdated, "restarted": true}, nil
}

func (c *XUIClient) removeRoutingUserReferences(ctx context.Context, email string) (int, error) {
	if strings.TrimSpace(email) == "" {
		return 0, nil
	}
	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return 0, err
	}
	updated := removeRoutingUser(mutableConfig.config, email)
	if updated == 0 {
		return 0, nil
	}
	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return 0, err
	}
	return updated, nil
}

func (c *XUIClient) replaceRoutingUserReferences(ctx context.Context, oldEmail string, newEmail string) (int, error) {
	if strings.TrimSpace(oldEmail) == "" || strings.TrimSpace(newEmail) == "" || strings.TrimSpace(oldEmail) == strings.TrimSpace(newEmail) {
		return 0, nil
	}
	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return 0, err
	}
	updated := replaceRoutingUser(mutableConfig.config, oldEmail, newEmail)
	if updated == 0 {
		return 0, nil
	}
	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return 0, err
	}
	return updated, nil
}

func clientMatchesDeletePayload(client map[string]any, email string, clientID string) bool {
	if email != "" && strings.TrimSpace(stringValue(client["email"])) == email {
		return true
	}
	if clientID == "" {
		return false
	}
	for _, key := range []string{"id", "password", "email"} {
		if strings.TrimSpace(stringValue(client[key])) == clientID {
			return true
		}
	}
	return false
}

func normalizeInboundClient(client map[string]any, protocol string) {
	if _, ok := client["enable"]; !ok {
		client["enable"] = true
	}
	if strings.TrimSpace(stringValue(client["subId"])) == "" {
		client["subId"] = randomHexString(8)
	}
	if strings.EqualFold(protocol, "trojan") {
		if strings.TrimSpace(stringValue(client["password"])) == "" {
			client["password"] = randomHexString(16)
		}
		return
	}
	if id := strings.TrimSpace(stringValue(client["id"])); id == "" || id == "00000000-0000-0000-0000-000000000001" || id == "00000000-0000-0000-0000-000000000000" {
		client["id"] = randomUUIDString()
	}
}

func clientPrimaryID(client map[string]any) string {
	return firstNonEmptyString(
		stringValue(client["id"]),
		stringValue(client["password"]),
		stringValue(client["email"]),
	)
}

func randomUUIDString() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

func randomHexString(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buf)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	columns, err := sqliteTableColumns(ctx, db, "inbounds")
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s,
		       %s, %s, %s, %s, %s,
		       %s, %s, %s, %s
		FROM inbounds
		ORDER BY id`,
		sqliteColumnExpr(columns, "id", "0", "id"),
		sqliteColumnExpr(columns, "user_id", "0", "user_id"),
		sqliteColumnExpr(columns, "up", "0", "up"),
		sqliteColumnExpr(columns, "down", "0", "down"),
		sqliteColumnExpr(columns, "total", "0", "total"),
		sqliteColumnExpr(columns, "all_time", "0", "all_time"),
		sqliteColumnExpr(columns, "remark", "''", "remark"),
		sqliteColumnExpr(columns, "enable", "1", "enable"),
		sqliteColumnExpr(columns, "expiry_time", "0", "expiry_time"),
		sqliteColumnExpr(columns, "traffic_reset", "''", "traffic_reset"),
		sqliteColumnExpr(columns, "last_traffic_reset_time", "0", "last_traffic_reset_time"),
		sqliteColumnExpr(columns, "listen", "''", "listen"),
		sqliteColumnExpr(columns, "port", "0", "port"),
		sqliteColumnExpr(columns, "protocol", "''", "protocol"),
		sqliteColumnExpr(columns, "settings", "'{}'", "settings"),
		sqliteColumnExpr(columns, "stream_settings", "'{}'", "stream_settings"),
		sqliteColumnExpr(columns, "tag", "''", "tag"),
		sqliteColumnExpr(columns, "sniffing", "'{}'", "sniffing"),
	)
	rows, err := db.QueryContext(ctx, query)
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
	columns, err := sqliteTableColumns(ctx, db, "client_traffics")
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s
		FROM client_traffics
		ORDER BY id`,
		sqliteColumnExpr(columns, "id", "0", "id"),
		sqliteColumnExpr(columns, "inbound_id", "0", "inbound_id"),
		sqliteColumnExpr(columns, "enable", "1", "enable"),
		sqliteColumnExpr(columns, "email", "''", "email"),
		sqliteColumnExpr(columns, "up", "0", "up"),
		sqliteColumnExpr(columns, "down", "0", "down"),
		sqliteColumnExpr(columns, "all_time", "0", "all_time"),
		sqliteColumnExpr(columns, "expiry_time", "0", "expiry_time"),
		sqliteColumnExpr(columns, "total", "0", "total"),
		sqliteColumnExpr(columns, "reset", "0", "reset"),
		sqliteColumnExpr(columns, "last_online", "0", "last_online"),
	)
	rows, err := db.QueryContext(ctx, query)
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

func sqliteTableColumns(ctx context.Context, db *sql.DB, table string) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		if name != "" {
			columns[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s has no columns", table)
	}
	return columns, nil
}

func sqliteColumnExpr(columns map[string]struct{}, column, fallback, alias string) string {
	if _, ok := columns[column]; ok {
		return column
	}
	return fallback + " AS " + alias
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
	var lastErr error
	for _, path := range []string{"/panel/xray/", "/panel/api/xray/"} {
		configJSON, err := c.getXrayTemplateAt(ctx, path)
		if err == nil {
			return configJSON, nil
		}
		lastErr = err
		if !isXUIHTTPStatus(err, http.StatusNotFound) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *XUIClient) getXrayTemplateAt(ctx context.Context, path string) (map[string]any, error) {
	form := url.Values{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
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

	var wrapper map[string]json.RawMessage
	var wrappedText string
	if err := json.Unmarshal(payload.Obj, &wrappedText); err == nil {
		if err := json.Unmarshal([]byte(wrappedText), &wrapper); err != nil {
			return nil, fmt.Errorf("decode x-ui template wrapper: %w", err)
		}
	} else if err := json.Unmarshal(payload.Obj, &wrapper); err != nil {
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
	var lastErr error
	for _, path := range []string{"/panel/xray/update", "/panel/api/xray/update"} {
		err := c.updateXrayTemplateAt(ctx, path, configJSON)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isXUIHTTPStatus(err, http.StatusNotFound) {
			return err
		}
	}
	return lastErr
}

func (c *XUIClient) updateXrayTemplateAt(ctx context.Context, path string, configJSON map[string]any) error {
	body, err := json.Marshal(configJSON)
	if err != nil {
		return fmt.Errorf("marshal x-ui template config: %w", err)
	}
	form := url.Values{}
	form.Set("xraySetting", string(body))
	form.Set("outboundTestUrl", "https://www.google.com/generate_204")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
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
		localErr := restartLocalXUIService(ctx)
		if localErr == nil {
			return nil
		}
		return fmt.Errorf("%w (local x-ui restart fallback failed: %v)", err, localErr)
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
		outputText := string(output)
		if err == nil && runCtx.Err() == nil {
			return nil
		}
		if isLocalXUIRestartSuccessOutput(outputText) {
			return nil
		}
		if runCtx.Err() != nil {
			err = runCtx.Err()
		}
		errs = append(errs, fmt.Sprintf("%s: %v: %s", strings.Join(command, " "), err, strings.TrimSpace(outputText)))
	}
	return errors.New(strings.Join(errs, "; "))
}

func isLocalXUIRestartSuccessOutput(output string) bool {
	normalized := strings.ToLower(stripANSIEscape(output))
	return strings.Contains(normalized, "restarted successfully") ||
		strings.Contains(normalized, "restart successfully") ||
		strings.Contains(normalized, "restart successful")
}

func stripANSIEscape(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	inEscape := false
	for i := 0; i < len(value); i++ {
		char := value[i]
		if inEscape {
			if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') {
				inEscape = false
			}
			continue
		}
		if char == 0x1b {
			inEscape = true
			continue
		}
		builder.WriteByte(char)
	}
	return builder.String()
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func detectLocal3XUIVersion(ctx context.Context) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	candidates := [][]string{}
	if commandAvailable("x-ui") {
		candidates = append(candidates, []string{"x-ui", "version"})
	}
	for _, path := range []string{"/usr/local/x-ui/x-ui", "/usr/bin/x-ui"} {
		if _, err := os.Stat(path); err == nil {
			candidates = append(candidates, []string{path, "version"})
		}
	}
	for _, command := range candidates {
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		output, err := exec.CommandContext(runCtx, command[0], command[1:]...).CombinedOutput()
		cancel()
		if err == nil && runCtx.Err() == nil {
			if version := extractLocalSemver(string(output)); version != "" {
				return version
			}
		}
	}
	return ""
}

func extractLocalSemver(value string) string {
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
		if dots == 2 {
			return value[start:end]
		}
	}
	return ""
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
