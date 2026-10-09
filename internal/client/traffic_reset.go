package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/panels"
)

type trafficResetJournal struct {
	Phase  string                       `json:"phase"`
	Result model.XUIActionResultRequest `json:"result"`
}

var resetJobIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func writeTrafficResetJournal(path string, value trafficResetJournal) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".reset-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	// Linux must persist the rename too before any destructive API call.
	if runtime.GOOS != "windows" {
		dir, e := os.Open(filepath.Dir(path))
		if e != nil {
			return e
		}
		e = dir.Sync()
		closeErr := dir.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (a *App) executeTrafficReset(ctx context.Context, xui *panels.XUIClient, action model.XUIAction) (result model.XUIActionResultRequest) {
	started := time.Now().UTC().Format(time.RFC3339Nano)
	// Every exit, including validation failures, reports an actual execution
	// interval. A replayed completed receipt keeps its original timestamps.
	defer func() { result = trafficResetTimedResult(result, started) }()
	failed := func(err error, result map[string]any) model.XUIActionResultRequest {
		return model.XUIActionResultRequest{Status: model.XUIActionStatusFailed, Error: err.Error(), Result: result}
	}
	jobID, _ := action.Payload["job_id"].(string)
	if !resetJobIDPattern.MatchString(jobID) || a.config.ConfigPath == "" || xui == nil {
		return failed(fmt.Errorf("traffic reset requires stable job ID, persisted Client configuration and x-ui connection"), nil)
	}
	path := filepath.Join(filepath.Dir(a.config.ConfigPath), "traffic-reset-journal", jobID+".json")
	var journal trafficResetJournal
	raw, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(raw, &journal); err != nil {
			return failed(fmt.Errorf("invalid reset journal; manual investigation required"), nil)
		}
		if journal.Phase != "intent" && journal.Phase != "done" && journal.Phase != "reset_done" {
			return failed(fmt.Errorf("unknown reset journal phase; refusing destructive retry"), nil)
		}
		if journal.Phase == "reset_done" && journal.Result.Result["reset_done"] != true {
			return failed(fmt.Errorf("reset receipt is not confirmed; refusing retry"), nil)
		}
		if journal.Phase == "intent" {
			return failed(fmt.Errorf("previous reset outcome is uncertain; will not clear traffic again"), map[string]any{"uncertain": true})
		}
		if journal.Phase == "done" {
			return journal.Result
		}
	} else if !os.IsNotExist(err) {
		return failed(err, nil)
	}
	url := fmt.Sprintf("%s/api/v1/agents/%s/traffic-reset-validation?action_id=%d", strings.TrimRight(a.config.ServerURL, "/"), a.config.AgentID, action.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return failed(err, nil)
	}
	req.Header.Set("X-Agent-Token", firstNonEmpty(a.currentAgentToken(), a.config.AgentToken))
	var validation struct {
		Allowed bool `json:"allowed"`
	}
	if err = a.doJSON(req, &validation); err != nil || !validation.Allowed {
		if err == nil {
			err = fmt.Errorf("server denied reset")
		}
		return failed(err, nil)
	}
	resume, _ := action.Payload["resume_only"].(bool)
	if resume && journal.Phase != "reset_done" {
		return failed(fmt.Errorf("no confirmed reset to resume; refusing destructive retry"), nil)
	}
	actionCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var output map[string]any
	if journal.Phase == "reset_done" {
		output, err = xui.VerifyClientPeriodTraffic(actionCtx, action.Payload, journal.Result.Result)
	} else {
		output, err = xui.ResetClientPeriodTraffic(actionCtx, action.Payload, func() error { return writeTrafficResetJournal(path, trafficResetJournal{Phase: "intent"}) })
	}
	result = model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded, Result: output}
	if err != nil {
		result = failed(err, output)
	}
	if resume {
		delete(result.Result, "execution_started_at")
		delete(result.Result, "execution_finished_at")
	}
	result = trafficResetTimedResult(result, started)
	phase := "done"
	if output["uncertain"] == true {
		phase = "intent"
	} else if err != nil && output["reset_done"] == true {
		phase = "reset_done"
	}
	if e := writeTrafficResetJournal(path, trafficResetJournal{Phase: phase, Result: result}); e != nil {
		return failed(fmt.Errorf("save reset receipt: %w", e), output)
	}
	return result
}

func trafficResetTimedResult(result model.XUIActionResultRequest, started string) model.XUIActionResultRequest {
	if result.Result == nil {
		result.Result = map[string]any{}
	}
	if _, exists := result.Result["execution_started_at"]; !exists {
		result.Result["execution_started_at"] = started
	}
	if _, exists := result.Result["execution_finished_at"]; !exists {
		result.Result["execution_finished_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return result
}
