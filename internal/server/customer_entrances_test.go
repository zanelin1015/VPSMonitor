package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestCustomerMultiEntranceSharesXrayQuotaHistoryAndExpiry(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	f.app.realtime = newRealtimeHub()
	customer := f.customer(f.root, "multi-entrance", nil)
	now := time.Now().UTC()
	expiry := now.Add(24 * time.Hour).UnixMilli()
	for _, item := range []struct {
		id, target, name, purpose, routeType string
		port, targetPort                     int
	}{
		{"gz", "ix", "Guangzhou entrance", "domestic_entry", "optimized", 20001, 15601},
		{"sh", "ix", "Shanghai entrance", "domestic_entry", "", 20002, 15601},
		{"ix", "xray", "private relay", "ix_relay", "", 15601, 443},
		{"xray", "", "HK relay", "xray_line", "", 0, 0},
		{"cox", "", "COX", "residential_exit", "", 0, 0},
	} {
		if _, err := testRegisterAgent(f.s, model.AgentRegisterRequest{AgentID: item.id}); err != nil {
			t.Fatal(err)
		}
		entry := model.AgentEntryConfig{ImportDomain: item.id + ".example.com"}
		if item.target != "" {
			entry.PortForwarding = model.RealmForwardConfig{Enabled: true, Backend: "realm", Rules: []model.RealmForwardRule{{
				ID: "listener", Enabled: true, ListenPort: item.port, TargetAgentID: item.target, TargetPort: item.targetPort,
				CustomerRoute: model.CustomerRouteConfig{Name: item.name, Type: item.routeType},
			}}}
		}
		if _, err := testUpdateAgentConfig(f.s, item.id, model.ManagedAgentConfig{AgentID: item.id, Entry: entry, CustomerDisplayName: item.name}); err != nil {
			t.Fatal(err)
		}
		if item.id != "ix" {
			purposes := []string{item.purpose}
			if err := f.s.UpdateAgentClassifications([]string{item.id}, model.AgentClassificationPatch{Purposes: &purposes}, "admin"); err != nil {
				t.Fatal(err)
			}
		}
		if item.target != "" {
			if err := f.s.SaveSnapshot(model.AgentSnapshot{AgentID: item.id, ReportedAt: now}); err != nil {
				t.Fatal(err)
			}
		}
	}
	country := "US"
	if err := f.s.UpdateAgentClassifications([]string{"cox"}, model.AgentClassificationPatch{CountryCode: &country}, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SaveSnapshot(model.AgentSnapshot{AgentID: "cox", ReportedAt: now, XUI: &model.XUISnapshot{
		Inbounds:  []map[string]any{{"id": 8, "tag": "socks-in", "enable": true, "protocol": "socks", "port": 1080, "settings": `{"accounts":[]}`}},
		Outbounds: []map[string]any{{"tag": "direct", "protocol": "freedom"}},
	}}); err != nil {
		t.Fatal(err)
	}
	clientID := "78119341-e995-4216-8f88-1d0c7a92f668"
	legacy, err := f.s.CreateCustomerAssignment(customer.ID, model.CustomerAssignmentRequest{AgentID: "ix", InboundID: 15601, ClientID: clientID, ClientEmail: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	classifyIX := func(query string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agent-classification"+query,
			bytes.NewBufferString("{\"agent_ids\":[\"ix\"],\"patch\":{\"purposes\":[\"ix_relay\"]}}"))
		request.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: f.root})
		result := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(result, request)
		if result.Code != http.StatusOK {
			t.Fatalf("classification failed: %d %s", result.Code, result.Body)
		}
		return result
	}
	var impact []agentClassificationImpact
	if err := json.Unmarshal(classifyIX("?preview=1").Body.Bytes(), &impact); err != nil || len(impact) != 1 || impact[0].AssignmentID != legacy.ID {
		t.Fatalf("expected a preview of the direct IX grant: %#v (%v)", impact, err)
	}
	classifyIX("")
	for _, sample := range []struct {
		at       time.Time
		up, down int
	}{{now.Add(-2 * time.Minute), 100, 100}, {now.Add(-time.Minute), 200, 400}} {
		inbound := map[string]any{
			"id": 7, "tag": "vless-in", "enable": true, "protocol": "vless", "port": 443,
			"settings":    fmt.Sprintf("{\"clients\":[{\"id\":%q,\"email\":\"alice\",\"enable\":true,\"expiryTime\":%d,\"totalGB\":1000}]}", clientID, expiry),
			"clientStats": []map[string]any{{"id": 91, "email": "alice", "enable": true, "up": sample.up, "down": sample.down}},
		}
		snapshot := model.AgentSnapshot{AgentID: "xray", ReportedAt: sample.at, XUI: &model.XUISnapshot{
			Inbounds: []map[string]any{inbound},
			Outbounds: []map[string]any{{"tag": "COX-1-COMMON", "protocol": "socks", "settings": map[string]any{
				"servers": []map[string]any{{"address": "cox.example.com", "port": 1080}},
			}}},
			RoutingRules: []map[string]any{{"type": "field", "user": []string{"alice"}, "outboundTag": "COX-1-COMMON"}},
		}}
		if err := f.s.SaveSnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	path := fmt.Sprintf("/%d/entrances", customer.ID)
	optionsResponse := f.request(f.root, http.MethodPost, path, customerEntranceSelection{XrayAgentID: "xray", InboundID: 7, ClientID: clientID, Preview: true}, http.StatusOK)
	var options []customerEntranceOption
	if err := json.Unmarshal(optionsResponse.Body.Bytes(), &options); err != nil || len(options) != 2 {
		t.Fatalf("expected two public entrances and no private IX: %s (%v)", optionsResponse.Body, err)
	}
	for _, option := range options {
		if option.RouteType != model.CustomerRouteTypeIEPL {
			t.Fatalf("expected IX to determine the entrance type: %#v", option)
		}
	}
	selection := customerEntranceSelection{XrayAgentID: "xray", InboundID: 7, ClientID: clientID, Entries: []string{options[0].Key, options[1].Key}}
	f.request(f.root, http.MethodPost, path, selection, http.StatusOK)
	f.request(f.root, http.MethodPost, path, selection, http.StatusOK)
	overview, err := f.app.customerOverview(customer.CustomerUser)
	if err != nil || len(overview.Links) != 1 || len(overview.SubscriptionLinks) != 2 {
		t.Fatalf("expected one grouped card and two subscription nodes: %#v (%v)", overview, err)
	}
	link := overview.Links[0]
	if len(link.Entrances) != 2 || link.TrafficUsedBytes != 600 || link.TrafficLimitBytes != 1000 || link.ExpireTime != expiry {
		t.Fatalf("incorrect shared usage/expiry: %#v", link)
	}
	if link.RouteType != model.CustomerRouteTypeIEPL || link.Entrances[0].RouteType != model.CustomerRouteTypeIEPL || link.Entrances[1].RouteType != model.CustomerRouteTypeIEPL {
		t.Fatalf("expected both entrances and the grouped card to show IEPL: %#v", link)
	}
	if len(link.Steps) != 2 || link.Steps[0].Label != "HK relay" || link.Steps[1].Role != "exit" || link.Steps[1].Label != "COX" ||
		link.Steps[1].CountryCode != "US" || link.ExitType != "residential" {
		t.Fatalf("expected the grouped topology to show HK and the named COX exit: %#v", link)
	}
	subscription := buildMihomoSubscription(customer.CustomerUser, overview.SubscriptionLinks)
	if !strings.Contains(subscription, "gz.example.com") || !strings.Contains(subscription, "sh.example.com") || strings.Contains(subscription, "ix.example.com") {
		t.Fatalf("wrong subscription endpoints: %s", subscription)
	}
	token, _, err := f.s.CreateCustomerSession(customer.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customer/usage?range=7d", nil)
	request.AddCookie(&http.Cookie{Name: customerSessionCookieName, Value: token})
	response := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(response, request)
	var usage model.CustomerUsageResponse
	if err := json.Unmarshal(response.Body.Bytes(), &usage); err != nil || response.Code != http.StatusOK ||
		usage.CountedTotalBytes != 600 || usage.QuotaBytes != 1000 || len(usage.Daily) != 1 || usage.Daily[0].CountedBytes != 400 ||
		len(usage.Daily[0].Links) != 1 || len(usage.Records) != 1 || usage.Records[0].TotalBytes != 400 {
		t.Fatalf("expected history and current usage to count Xray once: %d %s (%v)", response.Code, response.Body, err)
	}

	sh, found, err := f.s.GetAgentConfig("sh")
	if err != nil || !found {
		t.Fatalf("Shanghai config not found: %v", err)
	}
	sh.Entry.PortForwarding.Rules[0].Enabled = false
	if _, err := testUpdateAgentConfig(f.s, "sh", sh); err != nil {
		t.Fatal(err)
	}
	f.app.clearCustomerOverviewCache()
	overview, err = f.app.customerOverview(customer.CustomerUser)
	if err != nil || len(overview.Links) != 1 || len(overview.Links[0].Entrances) != 2 {
		t.Fatalf("disabled listener should not split the grouped card: %#v (%v)", overview, err)
	}
	steps := overview.Links[0].Steps
	if len(steps) != 2 || steps[1].Label != "COX" || overview.Links[0].RouteType != model.CustomerRouteTypeIEPL ||
		overview.Links[0].Entrances[0].Status != "disabled" {
		t.Fatalf("expected the disabled entrance to keep the shared exit and route type: %#v", overview.Links[0])
	}
	subscription = buildMihomoSubscription(customer.CustomerUser, overview.SubscriptionLinks)
	if !strings.Contains(subscription, "gz.example.com") || strings.Contains(subscription, "sh.example.com") {
		t.Fatalf("disabled listener should not be in the subscription: %s", subscription)
	}
}

func TestCustomerHAProxyEntranceUsesIEPLForIXBackup(t *testing.T) {
	realm := model.RealmForwardConfig{Enabled: true, Rules: []model.RealmForwardRule{{
		ID: "relay", Enabled: true, ListenPort: 15601, TargetAgentID: "xray", TargetPort: 443,
	}}}
	context := forwardedOverviewContext{
		agentMap: map[string]model.DashboardAgentView{
			"plain": {AgentID: "plain", Entry: model.AgentEntryConfig{PortForwarding: realm}},
			"ix": {AgentID: "ix", Entry: model.AgentEntryConfig{
				PortForwarding: realm,
				Classification: &model.AgentClassification{Purposes: []string{model.AgentPurposeIXRelay}},
			}},
			"xray": {AgentID: "xray"},
		},
		targetOverviewByAgent: map[string]*model.XUIOverview{
			"xray": {Nodes: []model.XUINodeView{{ID: 7, Port: 443, Protocol: "vless"}}},
		},
	}
	agent := model.DashboardAgentView{AgentID: "entry"}
	rule := customerEntranceRule{kind: "haproxy", haproxy: model.HAProxyRule{
		Primary: model.HAProxyRealmTarget{AgentID: "plain", RealmRuleID: "relay", Port: 15601},
	}}
	if result := rule.resolve(agent, context); !result.Resolved || result.routeType("optimized") != "optimized" {
		t.Fatalf("ordinary path should retain its configured route type: %#v", result)
	}
	rule.haproxy.Backups = []model.HAProxyRealmTarget{{AgentID: "ix", RealmRuleID: "relay", Port: 15601}}
	if result := rule.resolve(agent, context); !result.Resolved || result.routeType("optimized") != model.CustomerRouteTypeIEPL {
		t.Fatalf("IX backup path should mark the entrance as IEPL: %#v", result)
	}
}
