package store

import (
	"database/sql"
	"fmt"
	"time"

	"bridge-core/internal/model"
)

// CustomerSessionPair contains opaque credentials returned only to the HTTP
// cookie writer. Only their hashes are stored in the database.
type CustomerSessionPair struct {
	AccessToken      string `json:"-"`
	RefreshToken     string `json:"-"`
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

func (s *SQLiteStore) CreateCustomerSessionPair(customerID int64, accessTTL, refreshTTL time.Duration) (CustomerSessionPair, error) {
	if customerID <= 0 || accessTTL <= 0 || refreshTTL < accessTTL {
		return CustomerSessionPair{}, fmt.Errorf("invalid customer session lifetime or customer id")
	}
	accessToken, err := randomTokenBytes(adminSessionTokenBytes)
	if err != nil {
		return CustomerSessionPair{}, err
	}
	refreshToken, err := randomTokenBytes(adminSessionTokenBytes)
	if err != nil {
		return CustomerSessionPair{}, err
	}
	now := time.Now().UTC()
	pair := CustomerSessionPair{AccessToken: accessToken, RefreshToken: refreshToken, AccessExpiresAt: now.Add(accessTTL), RefreshExpiresAt: now.Add(refreshTTL)}
	tx, err := s.db.Begin()
	if err != nil {
		return CustomerSessionPair{}, err
	}
	defer tx.Rollback()
	var enabled int
	if err := tx.QueryRow(`SELECT enabled FROM customer_accounts WHERE id = ?`, customerID).Scan(&enabled); err != nil {
		return CustomerSessionPair{}, fmt.Errorf("load customer for login: %w", err)
	}
	if enabled == 0 {
		return CustomerSessionPair{}, fmt.Errorf("customer is disabled")
	}
	if err := pruneCustomerSessions(tx, now); err != nil {
		return CustomerSessionPair{}, err
	}
	refreshHash := sessionTokenHash(refreshToken)
	if _, err := tx.Exec(`INSERT INTO customer_refresh_sessions (token_hash, customer_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		refreshHash, customerID, now.Format(time.RFC3339Nano), pair.RefreshExpiresAt.Format(time.RFC3339Nano)); err != nil {
		return CustomerSessionPair{}, fmt.Errorf("create customer refresh session: %w", err)
	}
	if err := insertCustomerAccessSession(tx, customerID, accessToken, refreshHash, now, pair.AccessExpiresAt); err != nil {
		return CustomerSessionPair{}, err
	}
	if err := tx.Commit(); err != nil {
		return CustomerSessionPair{}, err
	}
	return pair, nil
}

// RefreshCustomerSession issues a new access credential without extending the
// refresh deadline. Concurrent browser tabs may each receive a valid access
// credential; revoking the refresh session invalidates all of them together.
func (s *SQLiteStore) RefreshCustomerSession(refreshToken string, accessTTL time.Duration) (model.CustomerUser, CustomerSessionPair, bool, error) {
	if refreshToken == "" || accessTTL <= 0 {
		return model.CustomerUser{}, CustomerSessionPair{}, false, nil
	}
	accessToken, err := randomTokenBytes(adminSessionTokenBytes)
	if err != nil {
		return model.CustomerUser{}, CustomerSessionPair{}, false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return model.CustomerUser{}, CustomerSessionPair{}, false, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if err := pruneCustomerSessions(tx, now); err != nil {
		return model.CustomerUser{}, CustomerSessionPair{}, false, err
	}
	refreshHash := sessionTokenHash(refreshToken)
	var customerID int64
	var expiresAtText string
	err = tx.QueryRow(`SELECT customer_id, expires_at FROM customer_refresh_sessions WHERE token_hash = ?`, refreshHash).Scan(&customerID, &expiresAtText)
	if err == sql.ErrNoRows {
		return model.CustomerUser{}, CustomerSessionPair{}, false, tx.Commit()
	}
	if err != nil {
		return model.CustomerUser{}, CustomerSessionPair{}, false, fmt.Errorf("load customer refresh session: %w", err)
	}
	refreshExpiresAt := parseTime(expiresAtText)
	user, err := scanCustomerUser(tx.QueryRow(`SELECT id, username, display_name, style_code, owner_type, owner_id, enabled, created_at, updated_at FROM customer_accounts WHERE id = ?`, customerID))
	if err != nil {
		return model.CustomerUser{}, CustomerSessionPair{}, false, fmt.Errorf("load refresh customer: %w", err)
	}
	if !user.Enabled || !now.Before(refreshExpiresAt) {
		if err := revokeCustomerSessions(tx, customerID, ""); err != nil {
			return model.CustomerUser{}, CustomerSessionPair{}, false, err
		}
		return model.CustomerUser{}, CustomerSessionPair{}, false, tx.Commit()
	}
	accessExpiresAt := now.Add(accessTTL)
	if accessExpiresAt.After(refreshExpiresAt) {
		accessExpiresAt = refreshExpiresAt
	}
	if err := insertCustomerAccessSession(tx, customerID, accessToken, refreshHash, now, accessExpiresAt); err != nil {
		return model.CustomerUser{}, CustomerSessionPair{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return model.CustomerUser{}, CustomerSessionPair{}, false, err
	}
	return user, CustomerSessionPair{AccessToken: accessToken, RefreshToken: refreshToken, AccessExpiresAt: accessExpiresAt, RefreshExpiresAt: refreshExpiresAt}, true, nil
}

// RevokeCustomerSessionPair also works after the access credential expires.
func (s *SQLiteStore) RevokeCustomerSessionPair(accessToken, refreshToken string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	accessHash := sessionTokenHash(accessToken)
	var linkedRefreshHash string
	err = tx.QueryRow(`SELECT refresh_token_hash FROM customer_sessions WHERE token_hash = ?`, accessHash).Scan(&linkedRefreshHash)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load logout session: %w", err)
	}
	refreshHash := sessionTokenHash(refreshToken)
	if _, err := tx.Exec(`DELETE FROM customer_sessions WHERE token_hash = ? OR (refresh_token_hash <> '' AND refresh_token_hash IN (?, ?))`, accessHash, refreshHash, linkedRefreshHash); err != nil {
		return fmt.Errorf("revoke customer access sessions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM customer_refresh_sessions WHERE token_hash IN (?, ?)`, refreshHash, linkedRefreshHash); err != nil {
		return fmt.Errorf("revoke customer refresh session: %w", err)
	}
	return tx.Commit()
}

func insertCustomerAccessSession(tx *sql.Tx, customerID int64, token, refreshHash string, now, expiresAt time.Time) error {
	_, err := tx.Exec(`INSERT INTO customer_sessions (token_hash, customer_id, refresh_token_hash, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?)`,
		sessionTokenHash(token), customerID, refreshHash, now.Format(time.RFC3339Nano), expiresAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create customer access session: %w", err)
	}
	return nil
}

func pruneCustomerSessions(tx *sql.Tx, now time.Time) error {
	deadline := now.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`DELETE FROM customer_sessions WHERE julianday(expires_at) <= julianday(?) OR refresh_token_hash IN (SELECT token_hash FROM customer_refresh_sessions WHERE julianday(expires_at) <= julianday(?))`, deadline, deadline); err != nil {
		return fmt.Errorf("prune customer access sessions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM customer_refresh_sessions WHERE julianday(expires_at) <= julianday(?)`, deadline); err != nil {
		return fmt.Errorf("prune customer refresh sessions: %w", err)
	}
	return nil
}

// Keep the current device's refresh credential when a customer changes their
// own password. Admin resets and account disabling pass an empty keep hash.
func revokeCustomerSessions(tx *sql.Tx, customerID int64, keepAccessHash string) error {
	if _, err := tx.Exec(`DELETE FROM customer_refresh_sessions WHERE customer_id = ? AND token_hash <> COALESCE((SELECT refresh_token_hash FROM customer_sessions WHERE customer_id = ? AND token_hash = ?), '')`, customerID, customerID, keepAccessHash); err != nil {
		return fmt.Errorf("revoke customer refresh sessions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM customer_sessions WHERE customer_id = ? AND token_hash <> ?`, customerID, keepAccessHash); err != nil {
		return fmt.Errorf("revoke customer access sessions: %w", err)
	}
	return nil
}
