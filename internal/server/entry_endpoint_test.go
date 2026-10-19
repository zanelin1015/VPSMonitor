package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestEntryConnectionConfigAPI(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "entry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	registered, err := s.RegisterAgent(model.AgentRegisterRequest{AgentID: "nat-node"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	user, ok, err := s.AuthenticateAdmin("admin", "password123")
	if err != nil || !ok {
		t.Fatal("test login failed")
	}
	token, _, err := s.CreateAdminSession(user, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s, realtime: newRealtimeHub()}
	cfg := registered.Config
	cfg.Entry.Connection = model.EntryConnectionConfig{Deployment: "docker", DockerNetwork: "bridge", PublicAccess: "nat", PublicAddress: "entry.example.com"}
	cfg.Entry.Mappings = []model.AgentEntryMapping{{UseForExport: true, TargetType: "xui", TargetID: "7", InternalPort: 20001, HostPort: 30001, ExternalPort: 333, Transport: "tcp", DefaultExport: true}}
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		if status == http.StatusBadRequest {
			cfg.Entry.Mappings[0].ExternalPort = 70000
		}
		body, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/nat-node/config", bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		response := httptest.NewRecorder()
		app.handleAgentConfig(response, req, registered.AgentID)
		if response.Code != status {
			t.Fatalf("want %d, got %d: %s", status, response.Code, response.Body.String())
		}
	}
	saved, found, err := s.GetAgentConfig(registered.AgentID)
	if err != nil || !found {
		t.Fatal("saved config missing")
	}
	endpoint := model.ResolveEntryEndpoint(saved.Entry, model.EntryTarget{Type: "xui", ID: "7", Port: 20001, Transport: "tcp"}, "")
	if endpoint.Error != "" || endpoint.Port != 333 || endpoint.HostPort != 30001 {
		t.Fatalf("saved mapping corrupted or invalid update persisted: %+v", endpoint)
	}
}

func TestNATCustomerSubscriptionAndForwarding(t *testing.T) {
	assignment := model.CustomerAssignment{AgentID: "xui", InboundID: 7, ClientEmail: "alice"}
	key := customerAssignmentKey("xui", 7, "alice")
	chain := model.ClientChainView{RootAgentID: "xui", RootInboundID: 7, RootClientEmail: "alice", Steps: []model.ClientChainStep{{StepType: "inbound", AgentID: "xui", Port: 20001}}}
	sourceURL := "vless://uuid@node.example.com:333?security=reality&pbk=key&sni=tls.example.com#test"
	clients := map[string]customerClientRef{key: {Client: model.XUIClientView{InboundID: 7, Email: "alice", ImportURL: sourceURL}, TemplateImportURL: sourceURL}}
	agents := map[string]model.DashboardAgentView{"xui": {AgentID: "xui", Entry: model.AgentEntryConfig{Connection: model.EntryConnectionConfig{PublicAccess: "nat", PublicAddress: "node.example.com"}, Mappings: []model.AgentEntryMapping{{UseForExport: true, TargetType: "xui", TargetID: "7", InternalPort: 20001, ExternalPort: 333, Transport: "tcp"}}}}}
	link := buildCustomerLinkView(assignment, map[string]model.ClientChainView{key: chain}, clients, agents)
	subscription := buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{link})
	if !strings.Contains(subscription, "port: 333") || !strings.Contains(subscription, `server: "node.example.com"`) {
		t.Fatal("subscription did not contain the mapped endpoint")
	}
	// The relay uses the mapped destination port; its outer NAT is applied once.
	agents["relay"] = model.DashboardAgentView{AgentID: "relay", Entry: model.AgentEntryConfig{Connection: model.EntryConnectionConfig{PublicAccess: "nat", PublicAddress: "outer.example.com"}, Mappings: []model.AgentEntryMapping{{UseForExport: true, TargetType: "realm", TargetID: "r1", InternalPort: 30001, ExternalPort: 444, Transport: "tcp"}}, PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{ID: "r1", Enabled: true, ListenPort: 30001, TargetAgentID: "xui", TargetPort: 333}}}}}
	link = buildCustomerLinkView(assignment, map[string]model.ClientChainView{key: chain}, clients, agents)
	if !strings.Contains(link.ImportURL, "@outer.example.com:444?") || !strings.Contains(link.ImportURL, "sni=tls.example.com") {
		t.Fatalf("outer endpoint not applied: %+v", link)
	}
	relay := agents["relay"]
	relay.Entry.Mappings = nil
	agents["relay"] = relay
	link = buildCustomerLinkView(assignment, map[string]model.ClientChainView{key: chain}, clients, agents)
	if link.Resolved || link.ImportURL != "" || link.ShadowrocketImportURL != "" || link.UnresolvedReason == "" {
		t.Fatalf("invalid outer mapping fell back to inner/template: %+v", link)
	}
}

func TestForwardedOverviewUsesEntryMapping(t *testing.T) {
	for _, kind := range []string{"realm", "haproxy"} {
		t.Run(kind, func(t *testing.T) {
			target := model.XUINodeView{ID: 7, Port: 20001, Protocol: "vless"}
			raw := "vless://uuid@placeholder.example.com:20001?security=reality&sni=tls.example.com#test"
			context := forwardedOverviewContext{targetOverviewByAgent: map[string]*model.XUIOverview{"xui": {Nodes: []model.XUINodeView{target}, Clients: []model.XUIClientView{{InboundID: 7, Email: "alice", Protocol: "vless", ExportEndpoint: &model.EntryEndpoint{Error: "internal listener has no public mapping"}}}}}, templateURLs: map[string]map[string]string{"xui": {customerAssignmentKey("xui", 7, "alice"): raw}}}
			entry := forwardedXUIEntry{RuleID: "r1", ForwardType: kind, ListenPort: 30001, Network: "tcp", Resolution: realmForwardResolution{Resolved: true, FinalAgentID: "xui", FinalNode: target}, SourceAgent: model.DashboardAgentView{AgentID: "relay", Entry: model.AgentEntryConfig{Connection: model.EntryConnectionConfig{PublicAccess: "nat", PublicAddress: "outer.example.com"}, Mappings: []model.AgentEntryMapping{{UseForExport: true, TargetType: kind, TargetID: "r1", InternalPort: 30001, ExternalPort: 444, Transport: "tcp"}}}}, InboundTag: func(model.XUIClientView) string { return "relay" }, InboundName: func(model.XUIClientView) string { return "Relay" }}
			overview := &model.XUIOverview{}
			appendResolvedForwardedClients(overview, context, map[string]int{}, entry)
			if len(overview.Clients) != 1 || !strings.Contains(overview.Clients[0].ImportURL, "@outer.example.com:444?") || overview.Nodes[0].Port != 30001 || overview.Nodes[0].ExportEndpoint.Port != 444 {
				t.Fatalf("bad forwarded view: %+v", overview)
			}
			entry.SourceAgent.Entry.Mappings = nil
			overview = &model.XUIOverview{}
			appendResolvedForwardedClients(overview, context, map[string]int{}, entry)
			if len(overview.Clients) != 1 || overview.Clients[0].ImportURL != "" || overview.Clients[0].ExportEndpoint.Error == "" {
				t.Fatal("unconfigured entry should remain visible with blocked export")
			}
		})
	}
}

func TestRealmSharedNATAddressUsesPort(t *testing.T) {
	agents := map[string]model.DashboardAgentView{}
	for index, id := range []string{"first", "second"} {
		agents[id] = model.DashboardAgentView{AgentID: id, Entry: model.AgentEntryConfig{Connection: model.EntryConnectionConfig{PublicAddress: "shared.example.com", PublicAccess: "nat"}, Mappings: []model.AgentEntryMapping{{UseForExport: true, TargetType: "xui", TargetID: "7", InternalPort: 20001, ExternalPort: 333 + index, Transport: "tcp"}}}}
	}
	if id := findRealmTargetAgentID(model.RealmForwardRule{TargetAddress: "shared.example.com", TargetPort: 334}, agents); id != "second" {
		t.Fatalf("matched wrong NAT server: %q", id)
	}
	if id := findRealmTargetAgentID(model.RealmForwardRule{TargetAddress: "shared.example.com", TargetPort: 999}, agents); id != "" {
		t.Fatalf("ambiguous host selected: %q", id)
	}
	if id := findRealmTargetAgentID(model.RealmForwardRule{TargetAgentID: "first", TargetAddress: "shared.example.com", TargetPort: 333}, agents); id != "first" {
		t.Fatal("explicit target lost")
	}
	entry := agents["first"].Entry
	entry.Mappings[0].DefaultExport = true
	additional := entry.Mappings[0]
	additional.DefaultExport = false
	additional.ExternalPort = 555
	entry.Mappings = append(entry.Mappings, additional)
	if node, ok := realmTargetNode([]model.XUINodeView{{ID: 7, Port: 20001, Protocol: "vless"}}, 555, entry); !ok || node.ID != 7 {
		t.Fatal("non-default entrance must remain usable for existing forwarding")
	}
	if !customerTargetPortMatches(entry, 20001, 555) {
		t.Fatal("non-default entrance lost from customer forwarding discovery")
	}
}

func TestHAProxyTargetUsesMappedRealmEndpoint(t *testing.T) {
	entry := model.AgentEntryConfig{
		Connection:     model.EntryConnectionConfig{PublicAccess: "nat", PublicAddress: "realm.example.com"},
		PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{ID: "r1", Enabled: true, ListenPort: 20001, Network: "both"}}},
		Mappings:       []model.AgentEntryMapping{{UseForExport: true, TargetType: "realm", TargetID: "r1", InternalPort: 20001, ExternalPort: 333, Transport: "both"}},
	}
	agents := map[string]model.AgentRecord{"relay": {AgentID: "relay", Config: model.ManagedAgentConfig{Entry: entry}}}
	target, err := hydrateHAProxyRealmTarget("outer", "rule", "primary", model.HAProxyRealmTarget{AgentID: "relay", RealmRuleID: "r1"}, agents)
	if err != nil || target.Address != "realm.example.com" || target.Port != 333 {
		t.Fatalf("HAProxy retained the internal endpoint: %+v %v", target, err)
	}
}
