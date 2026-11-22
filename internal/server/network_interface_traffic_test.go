package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestNetworkInterfaceTrafficIsRootOnlyAndUsesSelectedPeriod(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	f.grant(f.manager.ID, "one-user")
	for index := 0; index < 2; index++ {
		at := time.Date(2026, 11, 18, 0, index, 0, 0, time.UTC)
		if err := f.s.SaveSnapshot(model.AgentSnapshot{AgentID: "owner-node", ReportedAt: at, NetworkInterfaces: &model.NetworkInterfacesSnapshot{CollectedAt: at, Interfaces: []model.NetworkInterfaceCounter{
			{Key: "physical", Name: "ens3", Index: 2, Kind: "physical", Up: true, Primary: true, Epoch: "boot:2", ReceivedBytes: uint64(index * 100), SentBytes: uint64(index * 200)},
			{Key: "virtual", Name: "docker0", Index: 3, Kind: "bridge", Up: true, Epoch: "boot:3", ReceivedBytes: uint64(index * 100), SentBytes: uint64(index * 200)},
		}}}); err != nil {
			t.Fatal(err)
		}
	}
	query := func(token, query string, status int) model.NetworkInterfaceTrafficResponse {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/owner-node/interfaces/traffic"+query, nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		}
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("expected %d, got %d %s", status, w.Code, w.Body.String())
		}
		var response model.NetworkInterfaceTrafficResponse
		if status == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}
		return response
	}
	period := "?from=2026-11-18&to=2026-11-18"
	response := query(f.root, period, http.StatusOK)
	if !response.Reported || len(response.Items) != 2 || response.Items[0].Name != "ens3" || !response.Items[0].Primary || response.Items[0].TotalBytes != 300 || response.Timezone != "Asia/Shanghai" {
		t.Fatalf("unexpected interface result: %#v", response)
	}
	query(f.area, period, http.StatusForbidden)
	query("", period, http.StatusUnauthorized)
	if _, err := f.s.CreateOperator(model.OperatorAccountRequest{Username: "nic-operator", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	query(f.login("nic-operator"), period, http.StatusForbidden)
	for _, bad := range []string{"?from=wrong", "?from=2026-11-19&to=2026-11-18", "?from=2024-01-01&to=2026-11-18"} {
		query(f.root, bad, http.StatusBadRequest)
	}
	before := query(f.root, "?from=2026-11-17&to=2026-11-17", http.StatusOK)
	for _, item := range before.Items {
		if item.TotalBytes != 0 {
			t.Fatal("included another period's traffic")
		}
	}
	user, ok, err := f.s.AuthenticateAdmin("area-b", "password123")
	if err != nil || !ok {
		t.Fatal(err)
	}
	raw := []model.AgentSnapshot{{AgentID: "owner-node", NetworkInterfaces: &model.NetworkInterfacesSnapshot{}}}
	filtered := f.app.filterSnapshotsForAdmin(user, raw)
	if len(filtered) != 1 || filtered[0].NetworkInterfaces != nil || raw[0].NetworkInterfaces == nil {
		t.Fatal("area snapshot leaked NIC metadata or mutated root snapshot")
	}
}
