package store

import (
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestSQLiteStoreOperatorAccountLifecycle(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()
	if err := store.EnsureAdminAccount("owner", "owner-password"); err != nil {
		t.Fatalf("EnsureAdminAccount: %v", err)
	}

	operator, err := store.CreateOperator(model.OperatorAccountRequest{Username: "ops", Password: "ops-password", DisplayName: "运营"})
	if err != nil {
		t.Fatalf("CreateOperator: %v", err)
	}
	if operator.ID == 0 || operator.Username != "ops" || !operator.Enabled {
		t.Fatalf("unexpected operator: %#v", operator)
	}
	user, ok, err := store.AuthenticateAdmin("ops", "ops-password")
	if err != nil || !ok || user.Role != model.AdminRoleOperator || user.ID != operator.ID {
		t.Fatalf("expected operator authentication, ok=%v user=%#v err=%v", ok, user, err)
	}
	token, _, err := store.CreateAdminSession(user, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession: %v", err)
	}
	if sessionUser, _, ok, err := store.ValidateAdminSession(token); err != nil || !ok || sessionUser.Role != model.AdminRoleOperator || sessionUser.Username != "ops" {
		t.Fatalf("unexpected operator session: ok=%v user=%#v err=%v", ok, sessionUser, err)
	}

	disabled := false
	if _, err := store.UpdateOperator(operator.ID, model.OperatorAccountRequest{Enabled: &disabled, Password: "new-ops-password"}); err != nil {
		t.Fatalf("UpdateOperator: %v", err)
	}
	if _, ok, err := store.AuthenticateAdmin("ops", "new-ops-password"); err != nil || ok {
		t.Fatalf("disabled operator should not authenticate, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.AuthenticateAdmin("ops", "ops-password"); err != nil || ok {
		t.Fatalf("old operator password should not authenticate, ok=%v err=%v", ok, err)
	}
	if err := store.DeleteOperator(operator.ID); err != nil {
		t.Fatalf("DeleteOperator: %v", err)
	}
}
