package panels

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
}

var errXUIAuthExpired = errors.New("x-ui authentication expired")

type xuiEnvelope struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func NewXUIClient(cfg config.XUIConfig, timeout time.Duration) (*XUIClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
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

func (c *XUIClient) Collect(ctx context.Context) *model.XUISnapshot {
	snapshot := &model.XUISnapshot{
		BaseURL:     c.baseURL,
		CollectedAt: time.Now().UTC(),
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

	configJSON, err := c.getJSONObject(ctx, "/panel/api/server/getConfigJson")
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

func (c *XUIClient) ExecuteAction(ctx context.Context, action model.XUIAction) (map[string]any, error) {
	if err := c.ensureLogin(ctx); err != nil {
		return nil, err
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

func (c *XUIClient) executeActionAuthenticated(ctx context.Context, action model.XUIAction) (map[string]any, error) {
	switch action.Kind {
	case model.XUIActionAddOutbound:
		return c.addOutbound(ctx, action.Payload)
	case model.XUIActionAddRoutingRule:
		return c.addRoutingRule(ctx, action.Payload)
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

	configJSON, err := c.getXrayTemplate(ctx)
	if err != nil {
		return nil, err
	}
	outbounds := objectSlice(configJSON["outbounds"])
	for _, existing := range outbounds {
		if stringFromMap(existing, "tag") == tag {
			return nil, fmt.Errorf("outbound tag already exists: %s", tag)
		}
	}
	configJSON["outbounds"] = append(outbounds, outbound)

	if err := c.updateXrayTemplate(ctx, configJSON); err != nil {
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

	configJSON, err := c.getXrayTemplate(ctx)
	if err != nil {
		return nil, err
	}
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	rules = append(rules, rule)
	routing["rules"] = rules
	configJSON["routing"] = routing

	if err := c.updateXrayTemplate(ctx, configJSON); err != nil {
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
	if c.authenticated {
		return nil
	}
	return c.login(ctx)
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
	return c.postFormAction(ctx, "/panel/api/server/restartXrayService", url.Values{})
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
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%w: http %d: %s", errXUIAuthExpired, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
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
	value, _ := obj[key].(string)
	return value
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
