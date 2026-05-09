package panels

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

type NezhaClient struct {
	baseURL string
	config  config.NezhaConfig
	client  *http.Client
	token   string
}

type nezhaResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error"`
}

type nezhaLoginResponse struct {
	Token  string `json:"token"`
	Expire string `json:"expire"`
}

func NewNezhaClient(cfg config.NezhaConfig, timeout time.Duration) *NezhaClient {
	return &NezhaClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		config:  cfg,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.SkipTLSVerify},
			},
		},
	}
}

func (c *NezhaClient) Collect(ctx context.Context) *model.NezhaSnapshot {
	snapshot := &model.NezhaSnapshot{
		BaseURL:     c.baseURL,
		CollectedAt: time.Now().UTC(),
	}

	if err := c.login(ctx); err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}

	server, err := c.getSelectedServer(ctx)
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}

	snapshot.RawServer = server
	snapshot.ServerID = toUint64(server["id"])
	snapshot.ServerUUID = toString(server["uuid"])
	snapshot.ServerName = toString(server["name"])
	return snapshot
}

func (c *NezhaClient) login(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{
		"username": c.config.Username,
		"password": c.config.Password,
	})
	if err != nil {
		return fmt.Errorf("marshal nezha login: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build nezha login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var payload nezhaResponse[nezhaLoginResponse]
	if err := c.doJSON(req, &payload); err != nil {
		return fmt.Errorf("nezha login request failed: %w", err)
	}
	if !payload.Success || payload.Data.Token == "" {
		if payload.Error != "" {
			return fmt.Errorf("nezha login failed: %s", payload.Error)
		}
		return fmt.Errorf("nezha login failed")
	}
	c.token = payload.Data.Token
	return nil
}

func (c *NezhaClient) getSelectedServer(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/server", nil)
	if err != nil {
		return nil, fmt.Errorf("build nezha server request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	var payload nezhaResponse[[]map[string]any]
	if err := c.doJSON(req, &payload); err != nil {
		return nil, fmt.Errorf("nezha server request failed: %w", err)
	}
	if !payload.Success {
		return nil, fmt.Errorf("nezha server request failed: %s", payload.Error)
	}

	for _, server := range payload.Data {
		if c.matchesServer(server) {
			return server, nil
		}
	}
	return nil, fmt.Errorf("nezha server not found with configured selector")
}

func (c *NezhaClient) matchesServer(server map[string]any) bool {
	if c.config.ServerID != 0 {
		return toUint64(server["id"]) == c.config.ServerID
	}
	if c.config.ServerUUID != "" {
		return toString(server["uuid"]) == c.config.ServerUUID
	}
	if c.config.ServerName != "" {
		return toString(server["name"]) == c.config.ServerName
	}
	return false
}

func (c *NezhaClient) doJSON(req *http.Request, target any) error {
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
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func toUint64(v any) uint64 {
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
