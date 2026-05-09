package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/devdata"
	"bridge-core/internal/model"
)

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:8090", "bridge-server base URL")
	registrationToken := flag.String("registration-token", "preview-registration-token", "registration token configured on bridge-server")
	agentID := flag.String("agent-id", "local-dev-01", "demo agent ID")
	agentName := flag.String("agent-name", "Local Dev VPS", "demo agent name")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 15 * time.Second}
	agentToken, err := register(ctx, client, *serverURL, *registrationToken, *agentID, *agentName)
	if err != nil {
		log.Fatalf("register demo agent: %v", err)
	}

	snapshot := devdata.DemoSnapshot(*agentID, *agentName, time.Now().UTC())
	if err := pushSnapshot(ctx, client, *serverURL, *agentID, agentToken, snapshot); err != nil {
		log.Fatalf("push demo snapshot: %v", err)
	}
	log.Printf("seeded demo snapshot for %s into %s", *agentID, strings.TrimRight(*serverURL, "/"))
}

func register(ctx context.Context, client *http.Client, serverURL, registrationToken, agentID, agentName string) (string, error) {
	profile := devdata.ResolveDemoProfile(agentID, agentName)
	reqBody := model.AgentRegisterRequest{
		AgentID:    agentID,
		AgentName:  profile.AgentName,
		Hostname:   profile.Hostname,
		PublicIPv4: profile.PublicIPv4,
		SeedConfig: model.ManagedAgentConfig{
			AgentID:   agentID,
			AgentName: profile.AgentName,
			Tags:      profile.Tags,
			XUI: config.XUIConfig{
				Enabled:       true,
				BaseURL:       "http://127.0.0.1:19090",
				Username:      "admin",
				Password:      "password",
				SkipTLSVerify: true,
			},
		},
	}

	var resp model.AgentRegisterResponse
	if err := postJSON(ctx, client, strings.TrimRight(serverURL, "/")+"/api/v1/agents/register", registrationToken, "", reqBody, &resp); err != nil {
		return "", err
	}
	if resp.AgentToken == "" {
		return "", fmt.Errorf("server returned empty agent token")
	}
	return resp.AgentToken, nil
}

func pushSnapshot(ctx context.Context, client *http.Client, serverURL, agentID, agentToken string, snapshot model.AgentSnapshot) error {
	url := strings.TrimRight(serverURL, "/") + "/api/v1/agents/" + agentID + "/heartbeat"
	return postJSON(ctx, client, url, "", agentToken, snapshot, nil)
}

func postJSON(ctx context.Context, client *http.Client, url, registrationToken, agentToken string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if registrationToken != "" {
		req.Header.Set("X-Registration-Token", registrationToken)
	}
	if agentToken != "" {
		req.Header.Set("X-Agent-Token", agentToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
