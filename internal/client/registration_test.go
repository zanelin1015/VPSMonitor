package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func TestLoadEffectiveConfigRegistersOnceAndSurvivesRestart(t *testing.T) {
	for _, persistenceFails := range []bool{false, true} {
		name := "concurrent-bootstrap"
		if persistenceFails {
			name = "retry-persistence"
		}
		t.Run(name, func(t *testing.T) {
			const agentID = "HK_Node.01"
			var registrations, configFetches atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/agents/register":
					if registrations.Add(1) != 1 || r.Header.Get("X-Registration-Token") != "bootstrap" {
						http.Error(w, "bootstrap token rotated", http.StatusUnauthorized)
						return
					}
					_ = json.NewEncoder(w).Encode(model.AgentRegisterResponse{AgentID: agentID, AgentToken: "issued-token", Config: model.ManagedAgentConfig{AgentID: agentID}})
				case "/api/v1/agents/" + agentID + "/config":
					configFetches.Add(1)
					if r.Header.Get("X-Agent-Token") != "issued-token" {
						http.Error(w, "invalid agent token", http.StatusUnauthorized)
						return
					}
					_ = json.NewEncoder(w).Encode(model.ManagedAgentConfig{AgentID: agentID})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "client.json")
			writeConfig := func() {
				t.Helper()
				data, err := json.Marshal(config.ClientConfig{AgentID: agentID, RegistrationToken: "bootstrap", ServerURL: server.URL})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if !persistenceFails {
				writeConfig()
			}
			app := &App{
				config:     config.ClientConfig{AgentID: agentID, RegistrationToken: "bootstrap", ServerURL: server.URL, ConfigPath: path},
				httpClient: server.Client(), requestTimeout: time.Second,
			}
			if persistenceFails {
				if _, err := app.loadEffectiveConfig(context.Background()); err == nil {
					t.Fatal("expected missing config file to prevent credential persistence")
				}
				writeConfig()
			}
			results := make(chan error, 3)
			for i := 0; i < cap(results); i++ {
				go func() {
					_, err := app.loadEffectiveConfig(context.Background())
					results <- err
				}()
			}
			for i := 0; i < cap(results); i++ {
				if err := <-results; err != nil {
					t.Fatal(err)
				}
			}
			cfg, _, err := config.LoadClientConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AgentID != agentID || cfg.RegistrationToken != "" || cfg.AgentToken != "issued-token" {
				t.Fatalf("unexpected saved credentials: %+v", cfg)
			}
			restarted := &App{config: cfg, httpClient: server.Client(), requestTimeout: time.Second}
			if _, err := restarted.loadEffectiveConfig(context.Background()); err != nil {
				t.Fatalf("restart with persisted credentials: %v", err)
			}
			if registrations.Load() != 1 || configFetches.Load() < 3 {
				t.Fatalf("expected one registration followed by authenticated config fetches; registrations=%d fetches=%d", registrations.Load(), configFetches.Load())
			}
		})
	}
}
