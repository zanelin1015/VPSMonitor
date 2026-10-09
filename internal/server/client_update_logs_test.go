package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestClientUpdateEligibilityExplainsEverySkip(t *testing.T) {
	packageName := "VPSMonitor-client-linux-amd64.tar.gz"
	latest := &model.UpdateLatestInfo{LatestClientVersion: "0.3.36", ClientAssets: []string{packageName}, ClientAssetDigests: map[string]string{packageName: strings.Repeat("a", 64)}}
	base := model.AgentRecord{AgentID: "agent", Version: "0.3.35", OS: "linux", Arch: "amd64"}
	for _, code := range []string{"eligible", "unknown_platform", "unsupported_platform", "missing_package", "unknown_version", "up_to_date", "manual_upgrade_required", "missing_digest"} {
		t.Run(code, func(t *testing.T) {
			agent, info, snapshot := base, *latest, model.AgentSnapshot{VerifiedSelfUpdate: true}
			switch code {
			case "unknown_platform":
				agent.OS = ""
			case "unsupported_platform":
				agent.OS = "darwin"
			case "missing_package":
				info.ClientAssets = []string{"other.zip"}
			case "unknown_version":
				agent.Version = ""
			case "up_to_date":
				agent.Version = "0.3.36"
			case "manual_upgrade_required":
				snapshot.VerifiedSelfUpdate = false
			case "missing_digest":
				info.ClientAssetDigests = nil
			}
			status, got := clientUpdateEligibility(agent, snapshot, &info, "VPSMonitor", false)
			if got != code || status.Reason == "" || status.UpdateAvailable != (code == "eligible") {
				t.Fatalf("%s: %#v (%s)", code, status, got)
			}
		})
	}
}

func TestClientUpdateEligibilityForceReinstallsExactTargetWithoutDowngrade(t *testing.T) {
	packageName := "VPSMonitor-client-linux-amd64.tar.gz"
	info := model.UpdateLatestInfo{LatestClientVersion: "0.3.36", ClientAssets: []string{packageName}, ClientAssetDigests: map[string]string{packageName: strings.Repeat("a", 64)}}
	snapshot := model.AgentSnapshot{VerifiedSelfUpdate: true}
	base := model.AgentRecord{AgentID: "agent", Version: "0.3.36", OS: "linux", Arch: "amd64"}
	status, code := clientUpdateEligibility(base, snapshot, &info, "VPSMonitor", false)
	if code != "up_to_date" || status.UpdateAvailable {
		t.Fatalf("normal same-version check: %#v (%s)", status, code)
	}
	status, code = clientUpdateEligibility(base, snapshot, &info, "VPSMonitor", true)
	if code != "forced" || !status.UpdateAvailable || !strings.Contains(status.Reason, "强制升级") {
		t.Fatalf("force same-version check: %#v (%s)", status, code)
	}
	newer := base
	newer.Version = "0.3.37"
	status, code = clientUpdateEligibility(newer, snapshot, &info, "VPSMonitor", true)
	if code != "newer_version" || status.UpdateAvailable {
		t.Fatalf("force downgrade guard: %#v (%s)", status, code)
	}
}

type clientUpdateTestTransport struct{}

func (clientUpdateTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `[{"tag_name":"v0.3.36","assets":[{"name":"VPSMonitor-client-linux-amd64.tar.gz","digest":"sha256:` + strings.Repeat("a", 64) + `"}]}]`
	if !strings.Contains(r.URL.Path, "/zanelin1015/VPSMonitor/") {
		body = `{"tag_name":"v3.0.2"}`
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func TestClientUpdateBatchPersistsSkippedClientsAndMatchesPreview(t *testing.T) {
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: clientUpdateTestTransport{}}
	t.Cleanup(func() { http.DefaultClient = previous })
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "updates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"new-client", "old-client", "latest-client"} {
		if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: id, AgentName: id}); err != nil {
			t.Fatal(err)
		}
		version := "0.3.35"
		if id == "latest-client" {
			version = "0.3.36"
		}
		if err := s.SaveSnapshot(model.AgentSnapshot{AgentID: id, Version: version, OS: "linux", Arch: "amd64", ReportedAt: time.Now(), VerifiedSelfUpdate: id != "old-client"}); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{store: s, realtime: newRealtimeHub()}
	latest, err := a.fetchUpdateLatestInfo(officialVPSMonitorRepository, officialVPSMonitorPrefix)
	if err != nil || latest.ClientUpdateAvailableCount != 1 {
		t.Fatalf("preview count: %#v %v", latest, err)
	}
	response, err := a.createClientUpdateActions(model.UpdateRequest{})
	if err != nil || response.Count != 1 || response.Skipped != 2 || len(response.Logs) != 3 || response.BatchID == "" {
		t.Fatalf("batch: %#v %v", response, err)
	}
	logs, err := a.clientUpdateLogs()
	if err != nil || len(logs) != 3 {
		t.Fatalf("logs: %#v %v", logs, err)
	}
	for _, log := range logs {
		if log.AgentID == "old-client" && (log.ReasonCode != "manual_upgrade_required" || log.Decision != "skipped") {
			t.Fatalf("missing old client reason: %#v", log)
		}
	}
	forced, err := a.createClientUpdateActions(model.UpdateRequest{AgentIDs: []string{"latest-client"}, Force: true})
	if err != nil || forced.Count != 1 || len(forced.Logs) != 1 || !forced.Logs[0].Force || forced.Logs[0].ReasonCode != "forced_queued" {
		t.Fatalf("force batch: %#v %v", forced, err)
	}
	response, err = a.createClientUpdateActions(model.UpdateRequest{AgentIDs: []string{"new-client"}})
	if err != nil || response.Count != 0 || response.Skipped != 1 || response.Logs[0].ReasonCode != "already_pending" {
		t.Fatalf("duplicate batch: %#v %v", response, err)
	}
	all, _ := s.ListClientUpdateLogs(200)
	if len(all) != 5 {
		t.Fatal("lost batch history")
	}
}

func TestClientUpdateLogRequiresNewVersionReportBeforeSuccess(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "updates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "agent"}); err != nil {
		t.Fatal(err)
	}
	_, action, err := s.RecordClientUpdate(model.ClientUpdateLog{AgentID: "agent", Decision: "dispatched", TargetVersion: "0.3.36"}, map[string]any{"version": "v0.3.36"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteXUIAction("agent", action.ID, model.XUIActionResultRequest{Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	a := &App{store: s}
	for _, report := range []struct {
		version   string
		reported  time.Time
		confirmed bool
	}{{"0.3.36", time.Now().Add(-time.Hour), false}, {"0.3.35", time.Now(), false}, {"0.3.36", time.Now().Add(time.Second), true}} {
		if err := s.SaveSnapshot(model.AgentSnapshot{AgentID: "agent", Version: report.version, ReportedAt: report.reported}); err != nil {
			t.Fatal(err)
		}
		logs, err := a.clientUpdateLogs()
		if err != nil || len(logs) != 1 || (logs[0].ConfirmedAt != "") != report.confirmed {
			t.Fatalf("version confirmation: %#v %v", logs, err)
		}
	}
}

func TestClientUpdateLogsRequireRootAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	(&App{}).handleAdminUpdates(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/updates/client-logs", nil), []string{"client-logs"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous access: %d", w.Code)
	}
}
