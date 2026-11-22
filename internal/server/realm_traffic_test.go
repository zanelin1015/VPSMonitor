package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestRealmTrafficQueryRangeAndAreaPortIsolation(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	if _, err := testRegisterAgent(f.s, model.AgentRegisterRequest{AgentID: "realm-entry"}); err != nil {
		t.Fatal(err)
	}
	for _, grant := range []model.AreaManagerAssignmentRequest{
		{AgentID: "realm-entry", InboundID: 20002, InboundTag: "realm:20002"},
		{AgentID: "realm-entry", InboundID: 20003, InboundTag: "xui-20003", ClientEmail: "one-user"},
	} {
		if _, err := testCreateAreaManagerAssignment(f.s, f.manager.ID, grant); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 2; index++ {
		at := time.Date(2026, 11, 18, 0, index, 0, 0, time.UTC)
		if err := f.s.SaveSnapshot(model.AgentSnapshot{AgentID: "realm-entry", ReportedAt: at, Realm: &model.RealmSnapshot{Running: true,
			Traffic: &model.RealmTrafficSnapshot{Enabled: true, CollectedAt: at, Counters: []model.RealmTrafficCounter{
				{Key: "a", RuleID: "a", ListenAddress: "0.0.0.0", ListenPort: 20002, Network: "both", Epoch: "epoch", UploadBytes: uint64(index * 100), DownloadBytes: uint64(index * 200)},
				{Key: "b", RuleID: "b", ListenAddress: "0.0.0.0", ListenPort: 20003, Network: "tcp", Epoch: "epoch", UploadBytes: uint64(index * 900), DownloadBytes: uint64(index * 900)},
			}},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	query := func(token, query string, status int) model.RealmTrafficUsageResponse {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/realm-entry/realm/traffic"+query, nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		}
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("expected %d, got %d %s", status, w.Code, w.Body.String())
		}
		var response model.RealmTrafficUsageResponse
		if status == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}
		return response
	}
	valid := "?from=2026-11-18&to=2026-11-18"
	root := query(f.root, valid, http.StatusOK)
	if len(root.Items) != 2 || root.Timezone != "Asia/Shanghai" || !root.Enabled || !root.Reported {
		t.Fatalf("unexpected root result: %#v", root)
	}
	area := query(f.area, valid, http.StatusOK)
	if len(area.Items) != 1 || area.Items[0].ListenPort != 20002 || area.Items[0].TotalBytes != 300 {
		t.Fatalf("area must not see another port or derive port authority from a client grant: %#v", area)
	}
	query(f.other, valid, http.StatusForbidden)
	query("", valid, http.StatusUnauthorized)
	for _, invalid := range []string{"?from=bad", "?from=2026-11-19&to=2026-11-18", "?from=2024-01-01&to=2026-11-18"} {
		query(f.root, invalid, http.StatusBadRequest)
	}
	before := query(f.root, "?from=2026-11-17&to=2026-11-17", http.StatusOK)
	for _, item := range before.Items {
		if item.TotalBytes != 0 {
			t.Fatal("range included another day")
		}
	}
}
