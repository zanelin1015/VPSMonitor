package server

import (
	"bridge-core/internal/model"
	"bridge-core/internal/store"
	"path/filepath"
	"testing"
	"time"
)

func TestAreaPermissionCandidatesAndSwitches(t *testing.T) {
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, id := range []string{"line", "landing", "plain", "entry"} {
		if err := db.SaveSnapshot(model.AgentSnapshot{AgentID: id, ReportedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	for id, role := range map[string]string{"line": model.AgentRoleLine, "landing": model.AgentRoleLanding} {
		cfg, _, _ := db.GetAgentConfig(id)
		cfg.Tags = []string{role}
		if _, err := db.UpdateAgentConfig(id, cfg); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{store: db}
	on, off := true, false
	manager, err := db.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area", RealmEnabled: &on, LineEnabled: &on, LandingEnabled: &on, FrontLinkEnabled: &on})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.validateAreaRoleIDs(model.AreaManagerAccountRequest{LineAgentIDs: []string{"plain"}, LineEnabled: &on}); err == nil {
		t.Fatal("unclassified Client accepted as line")
	}
	if err := app.validateAreaRoleIDs(model.AreaManagerAccountRequest{LineAgentIDs: []string{"line"}, LineEnabled: &off}); err == nil {
		t.Fatal("disabled line switch accepted selections")
	}
	assignment := func(id, tag string) []model.AreaManagerAssignmentRequest {
		return []model.AreaManagerAssignmentRequest{{AgentID: id, InboundID: 10000, InboundTag: tag}}
	}
	for _, id := range []string{"line", "landing"} {
		if err := app.validateAreaAssignments(manager.ID, assignment(id, "xui")); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.validateAreaAssignments(manager.ID, assignment("plain", "xui")); err == nil {
		t.Fatal("unclassified Client accepted")
	}
	if err := app.validateAreaAssignments(manager.ID, assignment("entry", "realm:10000")); err == nil {
		t.Fatal("stopped Realm accepted")
	}
	if err := db.SaveSnapshot(model.AgentSnapshot{AgentID: "entry", ReportedAt: time.Now().UTC(), Realm: &model.RealmSnapshot{Running: true}}); err != nil {
		t.Fatal(err)
	}
	if err := app.validateAreaAssignments(manager.ID, assignment("entry", "realm:10000")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateAreaManager(manager.ID, model.AreaManagerAccountRequest{RealmEnabled: &off, LineEnabled: &off}); err != nil {
		t.Fatal(err)
	}
	if err := app.validateAreaAssignments(manager.ID, assignment("line", "xui")); err == nil {
		t.Fatal("disabled line category accepted")
	}
	if err := app.validateAreaAssignments(manager.ID, assignment("entry", "realm:10000")); err == nil {
		t.Fatal("disabled entry category accepted")
	}
}
