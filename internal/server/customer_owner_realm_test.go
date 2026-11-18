package server

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestCustomerOwnerTransferChecksEntryAndFinalClientAcrossRealmHops(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	forwardEntry := func(listen int, target string, port int) model.AgentEntryConfig {
		return model.AgentEntryConfig{ImportDomain: fmt.Sprintf("entry-%d.example.com", listen), PortForwarding: model.RealmForwardConfig{
			Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{ID: "rule", Name: "CNIX-HK", Enabled: true, ListenPort: listen, TargetAgentID: target, TargetAddress: target + ".example.com", TargetPort: port, Network: "tcp+udp"}},
		}}
	}
	for id, entry := range map[string]model.AgentEntryConfig{
		"gz": forwardEntry(20002, "ix", 15601), "ix": forwardEntry(15601, "dmit", 20002), "dmit": {ImportDomain: "dmit.example.com"},
	} {
		if _, err := testRegisterAgent(f.s, model.AgentRegisterRequest{AgentID: id}); err != nil {
			t.Fatal(err)
		}
		if _, err := testUpdateAgentConfig(f.s, id, model.ManagedAgentConfig{AgentID: id, Entry: entry}); err != nil {
			t.Fatal(err)
		}
		snapshot := model.AgentSnapshot{AgentID: id, ReportedAt: time.Now().UTC()}
		if id == "dmit" {
			snapshot.XUI = &model.XUISnapshot{CollectedAt: snapshot.ReportedAt, Inbounds: []map[string]any{{
				"id": 2, "tag": "in-20002-tcp", "remark": "HK", "enable": true, "protocol": "vless", "port": 20002,
				"settings": `{"clients":[{"id":"stable-alice","email":"alice","enable":true}]}`,
			}}}
		}
		if err := f.s.SaveSnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := testCreateAreaManagerAssignment(f.s, f.manager.ID, model.AreaManagerAssignmentRequest{AgentID: "gz", InboundID: 20002, InboundTag: "realm:20002"}); err != nil {
		t.Fatal(err)
	}
	customer := f.customer(f.root, "realm-transfer", nil)
	overview := f.app.xuiOverviewForOutboundAuthorization("gz")
	if overview == nil || len(overview.Clients) != 1 {
		t.Fatalf("expected a client mapped through the full chain: %#v", overview)
	}
	client := overview.Clients[0]
	assignment, err := f.s.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{AgentID: "gz", InboundID: 20002, InboundTag: client.InboundTag, ClientID: client.ClientID, ClientEmail: client.Email})
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/%d", customer.ID)
	// The entry listener alone must not grant the final DMIT client.
	f.request(f.root, http.MethodPut, path, map[string]any{"owner_area_manager_id": f.manager.ID}, http.StatusBadRequest)
	if _, err := testCreateAreaManagerAssignment(f.s, f.manager.ID, model.AreaManagerAssignmentRequest{AgentID: "dmit", InboundID: 2, InboundTag: "in-20002-tcp", ClientEmail: "alice"}); err != nil {
		t.Fatal(err)
	}
	f.request(f.root, http.MethodPut, path, map[string]any{"owner_area_manager_id": f.manager.ID}, http.StatusOK)
	stored, _, err := f.s.GetCustomer(customer.ID)
	if err != nil || len(stored.Assignments) != 1 || !reflect.DeepEqual(stored.Assignments[0], assignment) {
		t.Fatalf("transfer changed the entry binding: %#v err=%v", stored, err)
	}
}
