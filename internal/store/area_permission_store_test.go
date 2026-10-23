package store

import (
	"bridge-core/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestAreaPermissionsPersistAndRevokeByCategory(t *testing.T) {
	db, err := NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, id := range []string{"line", "landing", "entry"} {
		if err := db.SaveSnapshot(model.AgentSnapshot{AgentID: id, ReportedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	on, off := true, false
	manager, err := db.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area", RealmEnabled: &on, LineEnabled: &on, LandingEnabled: &on, FrontLinkEnabled: &on, LineAgentIDs: []string{"line"}, LandingAgentIDs: []string{"landing"}})
	if err != nil {
		t.Fatal(err)
	}
	if !manager.OutboundCreateEnabled {
		t.Fatal("landing permission must enable node landing on create")
	}
	if !manager.RealmEnabled || len(manager.LineAgentIDs) != 1 || len(manager.LandingAgentIDs) != 1 {
		t.Fatalf("permissions lost: %+v", manager)
	}
	_, err = db.CreateAreaManagerAssignments(manager.ID, []model.AreaManagerAssignmentRequest{{AgentID: "line", InboundID: 1}, {AgentID: "landing", InboundID: 2}, {AgentID: "entry", InboundID: 3, InboundTag: "realm:3"}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := db.UpdateAreaManager(manager.ID, model.AreaManagerAccountRequest{LineEnabled: &off, LineAgentIDs: []string{}, RealmEnabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	if updated.LineEnabled || updated.RealmEnabled || !updated.LandingEnabled || len(updated.Assignments) != 1 || updated.Assignments[0].AgentID != "landing" {
		t.Fatalf("revocation failed: %+v", updated)
	}
	if !updated.OutboundCreateEnabled {
		t.Fatal("landing permission was lost when another category changed")
	}
	// Password-only updates preserve permission switches and the remaining category.
	updated, err = db.UpdateAreaManager(manager.ID, model.AreaManagerAccountRequest{Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.LineEnabled || !updated.LandingEnabled || len(updated.Assignments) != 1 {
		t.Fatalf("partial update changed permissions: %+v", updated)
	}
	updated, err = db.UpdateAreaManager(manager.ID, model.AreaManagerAccountRequest{LandingEnabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	if updated.OutboundCreateEnabled {
		t.Fatal("disabled landing permission still allows node landing")
	}
	updated, err = db.UpdateAreaManager(manager.ID, model.AreaManagerAccountRequest{LandingEnabled: &on, OutboundCreateEnabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.OutboundCreateEnabled {
		t.Fatal("landing permission must enable node landing without a second switch")
	}
}
