package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestReplaceAgentReferencesMigratesPermissionsAndForwarding(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	for _, agentID := range []string{"old", "new", "realm-source", "haproxy-source", "final"} {
		if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: agentID, AgentName: agentID}); err != nil {
			t.Fatalf("RegisterAgent %s: %v", agentID, err)
		}
	}
	if _, err := store.UpdateAgentConfig("new", model.ManagedAgentConfig{
		AgentID:             "new",
		CustomerDisplayName: "New public name",
		Renewal:             model.VPSRenewalConfig{Enabled: true, CostAmount: 99},
		Entry: model.AgentEntryConfig{
			ImportDomain: "new.example.com",
			PortForwarding: model.RealmForwardConfig{
				Enabled: true,
				Backend: "realm",
				Rules: []model.RealmForwardRule{{
					ID:            "new-20001",
					Enabled:       true,
					ListenAddress: "0.0.0.0",
					ListenPort:    20001,
					TargetAddress: "final.example.com",
					TargetPort:    20001,
					Network:       "tcp",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig new: %v", err)
	}
	if _, err := store.UpdateAgentConfig("realm-source", model.ManagedAgentConfig{
		AgentID: "realm-source",
		Entry: model.AgentEntryConfig{PortForwarding: model.RealmForwardConfig{
			Enabled: true,
			Backend: "realm",
			Rules: []model.RealmForwardRule{{
				ID:            "entry-20001",
				Enabled:       true,
				ListenPort:    20001,
				TargetAgentID: "old",
				TargetAddress: "old.example.com",
				TargetPort:    20001,
				Network:       "tcp",
			}},
		}},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig realm-source: %v", err)
	}
	if _, err := store.UpdateAgentConfig("haproxy-source", model.ManagedAgentConfig{
		AgentID: "haproxy-source",
		Entry: model.AgentEntryConfig{HAProxy: model.HAProxyConfig{
			Enabled: true,
			Rules: []model.HAProxyRule{{
				ID:         "ha-20001",
				Enabled:    true,
				ListenPort: 20001,
				Primary: model.HAProxyRealmTarget{
					AgentID:     "old",
					RealmRuleID: "old-20001",
					Address:     "old.example.com",
					Port:        20001,
				},
				Backups: []model.HAProxyRealmTarget{{
					AgentID:     "final",
					RealmRuleID: "final-20001",
					Address:     "final.example.com",
					Port:        20001,
				}},
			}},
		}},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig haproxy-source: %v", err)
	}

	enabled := true
	manager, err := store.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"old", "new", "final"},
		OutboundGrants: []model.AreaManagerOutboundGrantRequest{
			{AgentID: "old", OutboundTag: "relay-shared"},
			{AgentID: "old", OutboundTag: "relay-old-only"},
			{AgentID: "new", OutboundTag: "relay-shared"},
			{AgentID: "final", OutboundTag: "relay-final"},
		},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	oldTags, _ := json.Marshal([]string{"seasonal", "gz"})
	newTags, _ := json.Marshal([]string{"replacement", "gz"})
	if _, err := store.db.Exec(`
		INSERT INTO area_manager_agent_tags (manager_id, agent_id, tags_json, updated_at)
		VALUES (?, 'old', ?, ?), (?, 'new', ?, ?)
	`, manager.ID, string(oldTags), now, manager.ID, string(newTags), now); err != nil {
		t.Fatalf("insert area manager tags: %v", err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO area_manager_assignments (
			manager_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
			enabled, created_at, updated_at
		) VALUES
			(?, 'old', 20001, 'realm:20001', '', 'Guangzhou 20001', 1, ?, ?),
			(?, 'new', 20001, 'realm:20001', '', '', 0, ?, ?),
			(?, 'old', 2, 'node-two', 'client@example.com', 'Old client', 1, ?, ?)
	`, manager.ID, now, now, manager.ID, now, now, manager.ID, now, now); err != nil {
		t.Fatalf("insert area manager assignments: %v", err)
	}
	customer, err := store.CreateCustomerForOwner(model.CustomerAccountRequest{
		Username: "sub-user",
		Password: "password123",
		Enabled:  &enabled,
	}, model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO customer_assignments (
			customer_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
			customer_remark, enabled, created_at, updated_at
		) VALUES
			(?, 'old', 20001, 'realm:20001', '', 'Old public', 'Old remark', 1, ?, ?),
			(?, 'new', 20001, 'realm:20001', '', '', '', 0, ?, ?),
			(?, 'final', 9, 'dmit', 'final@example.com', 'Final', '', 1, ?, ?)
	`, customer.ID, now, now, customer.ID, now, now, customer.ID, now, now); err != nil {
		t.Fatalf("insert customer assignments: %v", err)
	}

	result, err := store.ReplaceAgentReferences("old", "new", "admin")
	if err != nil {
		t.Fatalf("ReplaceAgentReferences: %v", err)
	}
	if result.AreaManagerAgentsMigrated != 1 || result.AreaManagerTagsMigrated != 1 || result.AreaAssignmentsMigrated != 2 || result.CustomerAssignmentsMigrated != 1 || result.OutboundGrantsMigrated != 2 {
		t.Fatalf("unexpected migration counts: %#v", result)
	}
	if result.RealmReferencesUpdated != 1 || result.HAProxyReferencesUpdated != 1 {
		t.Fatalf("unexpected forwarding update counts: %#v", result)
	}
	if _, found, err := store.GetAgent("old"); err != nil || !found {
		t.Fatalf("source agent must remain after replacement, found=%v err=%v", found, err)
	}

	newConfig, found, err := store.GetAgentConfig("new")
	if err != nil || !found {
		t.Fatalf("GetAgentConfig new found=%v err=%v", found, err)
	}
	if newConfig.CustomerDisplayName != "New public name" || newConfig.Renewal.CostAmount != 99 || newConfig.Entry.ImportDomain != "new.example.com" {
		t.Fatalf("replacement agent config was overwritten: %#v", newConfig)
	}
	realmSource, _, _ := store.GetAgentConfig("realm-source")
	realmRule := realmSource.Entry.PortForwarding.Rules[0]
	if realmRule.TargetAgentID != "new" || realmRule.TargetAddress != "new.example.com" {
		t.Fatalf("Realm reference was not replaced: %#v", realmRule)
	}
	haProxySource, _, _ := store.GetAgentConfig("haproxy-source")
	primary := haProxySource.Entry.HAProxy.Rules[0].Primary
	if primary.AgentID != "new" || primary.RealmRuleID != "new-20001" || primary.Address != "new.example.com" || primary.Port != 20001 {
		t.Fatalf("HAProxy reference was not hydrated from replacement Realm: %#v", primary)
	}

	managerAfter, found, err := store.GetAreaManager(manager.ID)
	if err != nil || !found {
		t.Fatalf("GetAreaManager found=%v err=%v", found, err)
	}
	if containsString(managerAfter.AgentIDs, "old") || !containsString(managerAfter.AgentIDs, "new") || !containsString(managerAfter.AgentIDs, "final") {
		t.Fatalf("unexpected manager agents after replacement: %#v", managerAfter.AgentIDs)
	}
	if len(managerAfter.OutboundGrants) != 3 {
		t.Fatalf("expected merged outbound grants without duplicates: %#v", managerAfter.OutboundGrants)
	}
	for _, grant := range managerAfter.OutboundGrants {
		if grant.AgentID == "old" {
			t.Fatalf("source outbound grant remains: %#v", managerAfter.OutboundGrants)
		}
	}
	assignments, err := store.ListAreaManagerAssignments(manager.ID)
	if err != nil {
		t.Fatalf("ListAreaManagerAssignments: %v", err)
	}
	if len(assignments) != 2 {
		t.Fatalf("expected merged area assignments: %#v", assignments)
	}
	for _, assignment := range assignments {
		if assignment.AgentID != "new" || !assignment.Enabled {
			t.Fatalf("unexpected migrated area assignment: %#v", assignment)
		}
	}
	var mergedTagsJSON string
	if err := store.db.QueryRow(`SELECT tags_json FROM area_manager_agent_tags WHERE manager_id = ? AND agent_id = 'new'`, manager.ID).Scan(&mergedTagsJSON); err != nil {
		t.Fatalf("load merged tags: %v", err)
	}
	if !strings.Contains(mergedTagsJSON, "seasonal") || !strings.Contains(mergedTagsJSON, "replacement") || strings.Count(mergedTagsJSON, "gz") != 1 {
		t.Fatalf("unexpected merged area tags: %s", mergedTagsJSON)
	}
	customerAssignments, err := store.ListCustomerAssignments(customer.ID)
	if err != nil {
		t.Fatalf("ListCustomerAssignments: %v", err)
	}
	if len(customerAssignments) != 2 {
		t.Fatalf("expected merged customer assignment plus final assignment: %#v", customerAssignments)
	}
	for _, assignment := range customerAssignments {
		if assignment.AgentID == "old" {
			t.Fatalf("source customer assignment remains: %#v", assignment)
		}
		if assignment.AgentID == "new" && (!assignment.Enabled || assignment.PublicClientName != "Old public" || assignment.Remark != "Old remark") {
			t.Fatalf("customer assignment fields were not merged: %#v", assignment)
		}
	}
}

func TestReplaceAgentReferencesRollsBackWhenReplacementRealmPortIsMissing(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()
	for _, agentID := range []string{"old", "new", "haproxy-source"} {
		if _, err := store.RegisterAgent(model.AgentRegisterRequest{AgentID: agentID}); err != nil {
			t.Fatalf("RegisterAgent %s: %v", agentID, err)
		}
	}
	if _, err := store.UpdateAgentConfig("new", model.ManagedAgentConfig{
		AgentID: "new",
		Entry: model.AgentEntryConfig{
			ImportDomain: "new.example.com",
			PortForwarding: model.RealmForwardConfig{
				Enabled: true,
				Backend: "realm",
				Rules:   []model.RealmForwardRule{{ID: "new-20002", Enabled: true, ListenPort: 20002, TargetAddress: "final.example.com", TargetPort: 20002}},
			},
		},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig new: %v", err)
	}
	if _, err := store.UpdateAgentConfig("haproxy-source", model.ManagedAgentConfig{
		AgentID: "haproxy-source",
		Entry: model.AgentEntryConfig{HAProxy: model.HAProxyConfig{Enabled: true, Rules: []model.HAProxyRule{{
			ID:         "ha-20001",
			Enabled:    true,
			ListenPort: 20001,
			Primary:    model.HAProxyRealmTarget{AgentID: "old", RealmRuleID: "old-20001", Address: "old.example.com", Port: 20001},
		}}}},
	}); err != nil {
		t.Fatalf("UpdateAgentConfig haproxy-source: %v", err)
	}
	enabled := true
	manager, err := store.CreateAreaManager(model.AreaManagerAccountRequest{
		Username: "area",
		Password: "password123",
		Enabled:  &enabled,
		AgentIDs: []string{"old"},
	})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}

	if _, err := store.ReplaceAgentReferences("old", "new", "admin"); err == nil || !strings.Contains(err.Error(), "port 20001") {
		t.Fatalf("expected missing Realm port error, got %v", err)
	}
	agentIDs, err := store.ListAreaManagerAgentIDs(manager.ID)
	if err != nil {
		t.Fatalf("ListAreaManagerAgentIDs: %v", err)
	}
	if len(agentIDs) != 1 || agentIDs[0] != "old" {
		t.Fatalf("permissions should roll back after config conflict: %#v", agentIDs)
	}
	cfg, _, _ := store.GetAgentConfig("haproxy-source")
	if cfg.Entry.HAProxy.Rules[0].Primary.AgentID != "old" {
		t.Fatalf("HAProxy reference should roll back after conflict: %#v", cfg.Entry.HAProxy.Rules[0].Primary)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
