package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestCustomerAccountsIgnoreRetiredStyleColumn(t *testing.T) {
	s, customer := customerSessionFixture(t)
	if sqliteColumnExists(t, s.db, "customer_accounts", "style_code") {
		t.Fatal("fresh databases should not create the retired style column")
	}
	// Existing databases retain this unused column without a destructive migration.
	if _, err := s.db.Exec(`ALTER TABLE customer_accounts ADD COLUMN style_code TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	const oldStyle = "<script>legacyStyle()</script>"
	if _, err := s.db.Exec(`UPDATE customer_accounts SET style_code = ? WHERE id = ?`, oldStyle, customer.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "new-customer", Password: "password123"}); err != nil {
		t.Fatalf("create with legacy schema: %v", err)
	}
	user, ok, err := s.AuthenticateCustomer(customer.Username, "password123")
	if err != nil || !ok {
		t.Fatalf("authenticate with legacy schema: ok=%v err=%v", ok, err)
	}
	encoded, err := json.Marshal(user)
	if err != nil || strings.Contains(string(encoded), "style_code") || strings.Contains(string(encoded), "legacyStyle") {
		t.Fatalf("retired style must not appear in API data: %s, %v", encoded, err)
	}
	pair := mustCustomerPair(t, s, customer.ID)
	assertCustomerAccess(t, s, pair.AccessToken, true)
	if _, _, ok, err := s.RefreshCustomerSession(pair.RefreshToken, time.Hour); err != nil || !ok {
		t.Fatalf("refresh with legacy schema: ok=%v err=%v", ok, err)
	}
	if _, err := s.UpdateCustomerPassword(customer.ID, model.CustomerPasswordUpdateRequest{CurrentPassword: "password123", NewPassword: "newpassword123"}, ""); err != nil {
		t.Fatalf("password update with legacy schema: %v", err)
	}
	var retained string
	if err := s.db.QueryRow(`SELECT style_code FROM customer_accounts WHERE id = ?`, customer.ID).Scan(&retained); err != nil || retained != oldStyle {
		t.Fatalf("legacy column should remain untouched: %q, %v", retained, err)
	}
}
