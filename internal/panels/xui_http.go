package panels

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"bridge-core/internal/model"
)

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
	return decodeXUIListPayload(payload.Obj, path)
}

func decodeXUIListPayload(raw json.RawMessage, path string) ([]map[string]any, error) {
	var result []map[string]any
	if err := json.Unmarshal(raw, &result); err == nil {
		return result, nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil && strings.TrimSpace(asString) != "" {
		return decodeXUIListPayload(json.RawMessage(asString), path)
	}

	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("decode x-ui response %s: %w", path, err)
	}
	for _, key := range []string{"data", "list", "items", "rows", "traffic", "traffics", "outboundTraffic", "outboundsTraffic", "outbounds"} {
		if value, ok := wrapper[key]; ok {
			items, err := decodeXUIListPayload(value, path)
			if err == nil {
				return items, nil
			}
		}
	}
	return nil, fmt.Errorf("decode x-ui response %s: response object does not contain a list", path)
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
