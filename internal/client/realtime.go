package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"bridge-core/internal/model"
	"bridge-core/internal/version"
)

const defaultRealtimeMetricsInterval = 2 * time.Second

func (a *App) RunRealtimeMetrics(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultRealtimeMetricsInterval
	}
	retryDelay := 5 * time.Second
	for ctx.Err() == nil {
		startedAt := time.Now()
		if err := a.runRealtimeMetricsSession(ctx, interval); err != nil && ctx.Err() == nil {
			log.Printf("realtime metrics disconnected: %v; retry in %s", err, retryDelay)
		}
		if !sleepContext(ctx, retryDelay) {
			return
		}
		if time.Since(startedAt) > 30*time.Second {
			retryDelay = 5 * time.Second
		} else if retryDelay < time.Minute {
			retryDelay *= 2
			if retryDelay > time.Minute {
				retryDelay = time.Minute
			}
		}
	}
}

func (a *App) runRealtimeMetricsSession(ctx context.Context, interval time.Duration) error {
	effectiveConfig, err := a.loadEffectiveConfig(ctx)
	if err != nil {
		return err
	}
	token := firstNonEmpty(a.currentAgentToken(), a.config.AgentToken)
	if token == "" {
		return fmt.Errorf("agent token is required")
	}

	endpoint, err := a.websocketEndpoint("/api/v1/agents/" + url.PathEscape(a.config.AgentID) + "/metrics/ws")
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: a.requestTimeout,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: a.config.ServerSkipTLSVerify},
	}
	header := http.Header{}
	header.Set("X-Agent-Token", token)

	conn, resp, err := dialer.DialContext(ctx, endpoint, header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			fallbackEndpoint, fallbackErr := endpointWithAgentToken(endpoint, token)
			if fallbackErr == nil {
				conn, resp, err = dialer.DialContext(ctx, fallbackEndpoint, nil)
				if err == nil {
					goto connected
				}
			}
		}
		return fmt.Errorf("connect realtime metrics ws: %s", formatWebSocketDialError(err, resp))
	}
connected:
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	sampler := newSystemMetricsSampler()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		metric := model.AgentRealtimeMetrics{
			AgentID:       a.config.AgentID,
			AgentName:     firstNonEmpty(effectiveConfig.AgentName, a.config.AgentName, a.config.AgentID),
			ClientVersion: version.Version,
			ClientOS:      runtime.GOOS,
			ClientArch:    runtime.GOARCH,
			SystemVersion: currentSystemVersion(),
			ReportedAt:    time.Now().UTC(),
			Summary:       sampler.sample(),
		}
		if err := conn.SetWriteDeadline(time.Now().Add(a.requestTimeout)); err != nil {
			return err
		}
		if err := conn.WriteJSON(metric); err != nil {
			return fmt.Errorf("send realtime metrics: %w", err)
		}

		select {
		case <-ticker.C:
		case <-done:
			return fmt.Errorf("server closed realtime metrics ws")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (a *App) websocketEndpoint(path string) (string, error) {
	base, err := url.Parse(strings.TrimRight(a.config.ServerURL, "/"))
	if err != nil {
		return "", fmt.Errorf("parse server_url: %w", err)
	}
	switch base.Scheme {
	case "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported server_url scheme: %s", base.Scheme)
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

func endpointWithAgentToken(endpoint string, token string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	values.Set("agent_token", token)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func formatWebSocketDialError(err error, resp *http.Response) string {
	if resp == nil {
		return err.Error()
	}
	status := resp.Status
	if status == "" {
		status = fmt.Sprintf("http %d", resp.StatusCode)
	}
	bodyText := ""
	if resp.Body != nil {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		if readErr == nil {
			bodyText = strings.TrimSpace(string(body))
		}
	}
	if bodyText == "" {
		return fmt.Sprintf("%v (%s)", err, status)
	}
	return fmt.Sprintf("%v (%s: %s)", err, status, bodyText)
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
