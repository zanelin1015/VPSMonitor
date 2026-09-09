package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestAgentMetricsWSRejectsTokenInQuery(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()
	registered, err := sqliteStore.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent-query-token"})
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	app := &App{store: sqliteStore, realtime: newRealtimeHub()}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-query-token/metrics/ws?agent_token="+url.QueryEscape(registered.AgentToken), nil)
	response := httptest.NewRecorder()
	app.handleAgentMetricsWS(response, request, "agent-query-token")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("query token must be rejected, got status %d body=%q", response.Code, response.Body.String())
	}
}
