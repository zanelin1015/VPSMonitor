package store

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func customerSessionFixture(t *testing.T) (*SQLiteStore, model.CustomerAdminView) {
	t.Helper()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	customer, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "session-customer", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	return s, customer
}

func mustCustomerPair(t *testing.T, s *SQLiteStore, id int64) CustomerSessionPair {
	t.Helper()
	pair, err := s.CreateCustomerSessionPair(id, 2*time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

func assertCustomerAccess(t *testing.T, s *SQLiteStore, token string, want bool) {
	t.Helper()
	if _, _, ok, err := s.ValidateCustomerSession(token); err != nil || ok != want {
		t.Fatalf("access valid=%v want=%v err=%v", ok, want, err)
	}
}

func TestCustomerSessionPairExpiryAndRefresh(t *testing.T) {
	s, customer := customerSessionFixture(t)
	pair := mustCustomerPair(t, s, customer.ID)
	if pair.AccessToken == pair.RefreshToken || time.Until(pair.AccessExpiresAt) > 2*time.Hour || time.Until(pair.RefreshExpiresAt) < 30*24*time.Hour-time.Second {
		t.Fatal("credentials must be distinct with independent deadlines")
	}
	var accessHash, refreshHash string
	if err := s.db.QueryRow(`SELECT token_hash, refresh_token_hash FROM customer_sessions WHERE customer_id = ?`, customer.ID).Scan(&accessHash, &refreshHash); err != nil {
		t.Fatal(err)
	}
	if accessHash != sessionTokenHash(pair.AccessToken) || refreshHash != sessionTokenHash(pair.RefreshToken) {
		t.Fatal("only credential hashes should be stored")
	}
	assertCustomerAccess(t, s, pair.AccessToken, true)
	assertCustomerAccess(t, s, pair.RefreshToken, false)
	if _, _, ok, err := s.RefreshCustomerSession(pair.AccessToken, 2*time.Hour); err != nil || ok {
		t.Fatalf("access credential must not refresh: ok=%v err=%v", ok, err)
	}
	if _, err := s.db.Exec(`UPDATE customer_sessions SET expires_at = ? WHERE token_hash = ?`, time.Now().Add(-time.Second).Format(time.RFC3339Nano), accessHash); err != nil {
		t.Fatal(err)
	}
	assertCustomerAccess(t, s, pair.AccessToken, false)
	user, renewed, ok, err := s.RefreshCustomerSession(pair.RefreshToken, 2*time.Hour)
	if err != nil || !ok || user.ID != customer.ID {
		t.Fatalf("refresh expired access: ok=%v err=%v", ok, err)
	}
	if renewed.AccessToken == pair.AccessToken || !renewed.RefreshExpiresAt.Equal(pair.RefreshExpiresAt) {
		t.Fatal("refresh must issue a new access credential and retain the absolute deadline")
	}
	assertCustomerAccess(t, s, renewed.AccessToken, true)
	// An access credential issued near day 30 must expire with its refresh.
	deadline := time.Now().UTC().Add(10 * time.Minute)
	if _, err := s.db.Exec(`UPDATE customer_refresh_sessions SET expires_at = ? WHERE token_hash = ?`, deadline.Format(time.RFC3339Nano), refreshHash); err != nil {
		t.Fatal(err)
	}
	_, capped, ok, err := s.RefreshCustomerSession(pair.RefreshToken, 2*time.Hour)
	if err != nil || !ok || !capped.AccessExpiresAt.Equal(deadline) {
		t.Fatalf("refresh deadline must cap access: ok=%v expires=%v err=%v", ok, capped.AccessExpiresAt, err)
	}
	if _, err := s.db.Exec(`UPDATE customer_refresh_sessions SET expires_at = ? WHERE token_hash = ?`, time.Now().Add(-time.Second).Format(time.RFC3339Nano), refreshHash); err != nil {
		t.Fatal(err)
	}
	assertCustomerAccess(t, s, capped.AccessToken, false)
	if _, _, ok, err := s.RefreshCustomerSession(pair.RefreshToken, 2*time.Hour); err != nil || ok {
		t.Fatalf("expired refresh must fail: ok=%v err=%v", ok, err)
	}
}

func TestCustomerConcurrentRefreshAndDeviceLogout(t *testing.T) {
	s, customer := customerSessionFixture(t)
	pair := mustCustomerPair(t, s, customer.ID)
	otherDevice := mustCustomerPair(t, s, customer.ID)
	const count = 8
	tokens := make([]string, count)
	var wg sync.WaitGroup
	for i := range tokens {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, renewed, ok, err := s.RefreshCustomerSession(pair.RefreshToken, 2*time.Hour)
			if err != nil || !ok {
				t.Errorf("concurrent refresh ok=%v err=%v", ok, err)
			}
			tokens[i] = renewed.AccessToken
		}(i)
	}
	wg.Wait()
	unique := map[string]bool{}
	for _, token := range tokens {
		assertCustomerAccess(t, s, token, true)
		unique[token] = true
	}
	if len(unique) != count {
		t.Fatal("each refresh must issue a fresh access credential")
	}
	// Browser no longer has an expired access cookie; refresh alone must logout.
	if err := s.RevokeCustomerSessionPair("", pair.RefreshToken); err != nil {
		t.Fatal(err)
	}
	for _, token := range append(tokens, pair.AccessToken) {
		assertCustomerAccess(t, s, token, false)
	}
	if _, _, ok, err := s.RefreshCustomerSession(pair.RefreshToken, 2*time.Hour); err != nil || ok {
		t.Fatalf("logout refresh ok=%v err=%v", ok, err)
	}
	assertCustomerAccess(t, s, otherDevice.AccessToken, true)
	// A logout request without a refresh cookie can revoke it through access.
	if err := s.RevokeCustomerSessionPair(otherDevice.AccessToken, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.RefreshCustomerSession(otherDevice.RefreshToken, 2*time.Hour); err != nil || ok {
		t.Fatalf("access-only logout refresh ok=%v err=%v", ok, err)
	}
}

func TestCustomerSessionPairAccountRevocation(t *testing.T) {
	for _, action := range []string{"disable", "admin-reset", "delete", "self-password"} {
		t.Run(action, func(t *testing.T) {
			s, customer := customerSessionFixture(t)
			current := mustCustomerPair(t, s, customer.ID)
			other := mustCustomerPair(t, s, customer.ID)
			var err error
			switch action {
			case "disable":
				disabled := false
				_, err = s.UpdateCustomer(customer.ID, model.CustomerAccountRequest{Enabled: &disabled})
			case "admin-reset":
				_, err = s.UpdateCustomer(customer.ID, model.CustomerAccountRequest{Password: "new-password123"})
			case "delete":
				err = s.DeleteCustomer(customer.ID)
			case "self-password":
				_, err = s.UpdateCustomerPassword(customer.ID, model.CustomerPasswordUpdateRequest{CurrentPassword: "password123", NewPassword: "new-password123"}, current.AccessToken)
			}
			if err != nil {
				t.Fatal(err)
			}
			keepCurrent := action == "self-password"
			assertCustomerAccess(t, s, current.AccessToken, keepCurrent)
			assertCustomerAccess(t, s, other.AccessToken, false)
			if _, _, ok, err := s.RefreshCustomerSession(current.RefreshToken, 2*time.Hour); err != nil || ok != keepCurrent {
				t.Fatalf("current refresh valid=%v want=%v err=%v", ok, keepCurrent, err)
			}
			if _, _, ok, err := s.RefreshCustomerSession(other.RefreshToken, 2*time.Hour); err != nil || ok {
				t.Fatalf("other device refresh valid=%v err=%v", ok, err)
			}
			if action == "disable" {
				enabled := true
				if _, err := s.UpdateCustomer(customer.ID, model.CustomerAccountRequest{Enabled: &enabled}); err != nil {
					t.Fatal(err)
				}
				if _, _, ok, err := s.RefreshCustomerSession(current.RefreshToken, 2*time.Hour); err != nil || ok {
					t.Fatalf("re-enable must not revive credentials: ok=%v err=%v", ok, err)
				}
			}
		})
	}
}

func TestCustomerLegacySessionMigrationAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	customer, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "legacy-customer", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	legacyToken, _, err := s.CreateCustomerSession(customer.ID, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{`DROP INDEX idx_customer_sessions_refresh`, `ALTER TABLE customer_sessions DROP COLUMN refresh_token_hash`, `DROP TABLE customer_refresh_sessions`} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err = NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	assertCustomerAccess(t, s, legacyToken, true)
	if _, _, ok, err := s.RefreshCustomerSession(legacyToken, 2*time.Hour); err != nil || ok {
		t.Fatalf("legacy credential is not a refresh credential: ok=%v err=%v", ok, err)
	}
	pair := mustCustomerPair(t, s, customer.ID)
	s.Close()
	s, err = NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertCustomerAccess(t, s, pair.AccessToken, true)
	if _, _, ok, err := s.RefreshCustomerSession(pair.RefreshToken, 2*time.Hour); err != nil || !ok {
		t.Fatalf("refresh survives server restart: ok=%v err=%v", ok, err)
	}
}
