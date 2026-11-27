package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"bridge-core/internal/model"
)

func TestAdminTagManagementRemovesGlobalReferencesAndRejectsStaleWrites(t *testing.T) {
	f := newCustomerOwnerFixture(t)
	legacyRequest := model.AgentRegisterRequest{AgentID: "legacy", SeedConfig: model.ManagedAgentConfig{Tags: []string{"old"}}}
	if _, err := testRegisterAgent(f.s, legacyRequest); err != nil {
		t.Fatal(err)
	}
	if _, err := testRegisterAgent(f.s, model.AgentRegisterRequest{AgentID: "managed"}); err != nil {
		t.Fatal(err)
	}
	config := model.ManagedAgentConfig{
		Tags: []string{"Old", model.AgentRoleLine, "keep"},
		Features: model.AgentFeatureConfig{
			Configured: true, Realm: true, RealmExplicitlyConfigured: true,
		},
	}
	if _, err := testUpdateAgentConfig(f.s, "managed", config); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.SaveAreaManagerAgentTags(f.manager.ID, "managed", []string{"Old"}); err != nil {
		t.Fatal(err)
	}

	request := func(token, method, path, body string, expected int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/admin/tags"+path, bytes.NewBufferString(body))
		r.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, r)
		if w.Code != expected {
			t.Fatalf("%s: got %d, want %d: %s", method, w.Code, expected, w.Body)
		}
		return w
	}
	request(f.root, http.MethodPost, "", "{\"tags\":[\"OLD\",\"unused\"]}", http.StatusOK)
	w := request(f.root, http.MethodGet, "?details=1", "", http.StatusOK)
	var items []model.AgentTagInventoryItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil ||
		!slices.Contains(items, model.AgentTagInventoryItem{Tag: "OLD", AgentCount: 2}) ||
		!slices.Contains(items, model.AgentTagInventoryItem{Tag: "unused"}) {
		t.Fatalf("inventory did not combine assigned and unused tags: %s (%v)", w.Body, err)
	}
	request(f.area, http.MethodDelete, "", "{\"tag\":\"OLD\"}", http.StatusForbidden)
	request(f.root, http.MethodDelete, "", "{\"tag\":\"线路机\"}", http.StatusConflict)
	w = request(f.root, http.MethodDelete, "", "{\"tag\":\"OLD\"}", http.StatusOK)
	if !bytes.Contains(w.Body.Bytes(), []byte("\"removed_clients\":2")) {
		t.Fatalf("expected to remove two references: %s", w.Body)
	}
	checkManagedConfig := func() {
		t.Helper()
		saved, found, err := f.s.GetAgentConfig("managed")
		if err != nil || !found || !saved.Features.Realm || !saved.Features.RealmExplicitlyConfigured ||
			!slices.Equal(saved.Tags, []string{"keep", model.AgentRoleLine}) {
			t.Fatalf("expected to keep the remaining tags and feature settings: %#v (%v)", saved, err)
		}
	}
	checkManagedConfig()
	request(f.root, http.MethodDelete, "", "{\"tag\":\"unused\"}", http.StatusOK)
	request(f.root, http.MethodPut, "", "{\"tags\":[\"old\",\"unused\"]}", http.StatusOK)
	if tags, _, err := f.s.GetTagSettings(); err != nil || len(tags) != 0 {
		t.Fatalf("stale tag settings appeared after deletion: %v (%v)", tags, err)
	}
	if _, err := testUpdateAgentConfig(f.s, "managed", config); err != nil {
		t.Fatal(err)
	}
	checkManagedConfig()
	agent, err := testRegisterAgent(f.s, legacyRequest)
	if err != nil || len(agent.Config.Tags) != 0 {
		t.Fatalf("register restored a deleted tag: %v (%v)", agent.Config.Tags, err)
	}
	privateTags, _, err := f.s.GetAreaManagerAgentTags(f.manager.ID, "managed")
	if err != nil || !slices.Equal(privateTags, []string{"Old"}) {
		t.Fatalf("global deletion changed regional private tags: %v (%v)", privateTags, err)
	}
	request(f.root, http.MethodPost, "", "{\"tags\":[\"Old\"]}", http.StatusOK)
	agent, err = testRegisterAgent(f.s, legacyRequest)
	if err != nil || !slices.Equal(agent.Config.Tags, []string{"old"}) {
		t.Fatalf("explicitly recreating the tag did not re-enable it: %v (%v)", agent.Config.Tags, err)
	}
}
