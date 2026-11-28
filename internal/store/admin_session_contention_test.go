package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestDeleteMissingAgentReleasesSQLiteConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	s, err := NewSQLiteStore(dbPath, WithMaxOpenConns(1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.DeleteAgent("missing"); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("expected missing Client error, got %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	probe, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if _, err := probe.ExecContext(ctx, "UPDATE agents SET agent_name = agent_name WHERE agent_id = 'missing'"); err != nil {
		t.Fatalf("failed delete left SQLite write-locked: %v", err)
	}
	if err := s.db.PingContext(ctx); err != nil {
		t.Fatalf("failed delete left the only SQLite connection in a transaction: %v", err)
	}
}

func TestCreateAdminSessionRetriesTemporarySQLiteWriteLock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	s, err := NewSQLiteStore(dbPath, WithMaxOpenConns(1))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureAdminAccount("admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA busy_timeout = 20"); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	lock := func() *sql.Tx {
		t.Helper()
		tx, err := writer.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec("UPDATE admin_accounts SET updated_at = updated_at WHERE id = 1"); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		return tx
	}

	held := lock()
	released := make(chan error, 1)
	time.AfterFunc(60*time.Millisecond, func() { released <- held.Rollback() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	user := model.AdminUser{ID: 1, Username: "admin", Role: model.AdminRoleRoot}
	token, _, err := s.CreateAdminSessionContext(ctx, user, time.Hour)
	if releaseErr := <-released; releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err != nil || token == "" {
		t.Fatalf("expected login to recover after the lock was released: %v", err)
	}

	held = lock()
	defer held.Rollback()
	if _, _, ok, err := s.ValidateAdminSession(token); err != nil || !ok {
		t.Fatalf("valid session should be usable while another connection holds the write lock: %v", err)
	}
	if got, _, err := s.CreateAdminSessionContext(ctx, user, time.Hour); got != "" || !IsSQLiteBusy(err) {
		t.Fatalf("expected a persistent lock to return SQLITE_BUSY without creating a session: %v", err)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM admin_sessions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected exactly one saved session, got %d (%v)", count, err)
	}
}
