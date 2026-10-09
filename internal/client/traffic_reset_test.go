package client

import (
	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/panels"
	"bridge-core/internal/store"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestTrafficResetJournalPreventsDestructiveReplay(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	cfgPath := filepath.Join(t.TempDir(), "client.json")
	a := &App{config: config.ClientConfig{ConfigPath: cfgPath, ServerURL: server.URL, AgentID: "agent"}, httpClient: server.Client()}
	xui, err := panels.NewXUIClient(config.XUIConfig{Enabled: true, BaseURL: server.URL, APIToken: "token"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	jobID := store.TrafficResetID("journal")
	path := filepath.Join(filepath.Dir(cfgPath), "traffic-reset-journal", jobID+".json")
	action := model.XUIAction{ID: 1, Kind: model.XUIActionResetClientTraffic, Payload: map[string]any{"job_id": jobID}}
	if err := writeTrafficResetJournal(path, trafficResetJournal{Phase: "intent"}); err != nil {
		t.Fatal(err)
	}
	result := a.executeTrafficReset(context.Background(), xui, action)
	if result.Status != model.XUIActionStatusFailed || result.Result["uncertain"] != true || calls != 0 {
		t.Fatalf("uncertain operation replayed: %#v calls=%d", result, calls)
	}
	receipt := model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: map[string]any{"reset_done": true}}
	if err := writeTrafficResetJournal(path, trafficResetJournal{Phase: "done", Result: receipt}); err != nil {
		t.Fatal(err)
	}
	result = a.executeTrafficReset(context.Background(), xui, action)
	if result.Status != receipt.Status || calls != 0 {
		t.Fatal("completed period invoked network again")
	}
	action.Payload["job_id"] = "../../unsafe"
	result = a.executeTrafficReset(context.Background(), xui, action)
	if result.Status != model.XUIActionStatusFailed || calls != 0 {
		t.Fatal("unsafe journal path accepted")
	}
}

func TestTrafficResetReportsActualExecutionTimesWithoutChangingReplay(t *testing.T) {
	started := time.Now().UTC().Format(time.RFC3339Nano)
	result := trafficResetTimedResult(model.XUIActionResultRequest{Status: model.XUIActionStatusFailed}, started)
	if result.Result["execution_started_at"] != started {
		t.Fatal("failed execution did not record start time")
	}
	finished, ok := result.Result["execution_finished_at"].(string)
	if !ok {
		t.Fatal("completion time missing")
	}
	if _, err := time.Parse(time.RFC3339Nano, finished); err != nil {
		t.Fatal(err)
	}
	replayed := trafficResetTimedResult(result, time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano))
	if replayed.Result["execution_started_at"] != started || replayed.Result["execution_finished_at"] != finished {
		t.Fatal("replayed receipt rewrote original execution time")
	}
}
