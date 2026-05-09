package store

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) EnsureAdminAccount(username, password string) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_accounts`).Scan(&count); err != nil {
		return fmt.Errorf("check admin account: %w", err)
	}
	if count > 0 {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	if password == "" {
		return fmt.Errorf("admin_password is required when no admin account exists")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO admin_accounts (id, username, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, adminAccountID, username, hash, now, now)
	if err != nil {
		return fmt.Errorf("create admin account: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AuthenticateAdmin(username, password string) (model.AdminUser, bool, error) {
	var (
		storedUsername string
		passwordHash   string
		updatedAtText  string
	)
	err := s.db.QueryRow(`
		SELECT username, password_hash, updated_at
		FROM admin_accounts
		WHERE id = ?
	`, adminAccountID).Scan(&storedUsername, &passwordHash, &updatedAtText)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, fmt.Errorf("load admin account: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(storedUsername)) != 1 {
		return model.AdminUser{}, false, nil
	}
	ok, err := verifyPassword(password, passwordHash)
	if err != nil {
		return model.AdminUser{}, false, err
	}
	if !ok {
		return model.AdminUser{}, false, nil
	}
	return model.AdminUser{
		Username:  storedUsername,
		UpdatedAt: parseTime(updatedAtText),
	}, true, nil
}

func (s *SQLiteStore) CreateAdminSession(username string, ttl time.Duration) (string, model.AdminSession, error) {
	token, err := randomTokenBytes(adminSessionTokenBytes)
	if err != nil {
		return "", model.AdminSession{}, err
	}
	now := time.Now().UTC()
	session := model.AdminSession{
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	_, err = s.db.Exec(`
		INSERT INTO admin_sessions (token_hash, username, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
	`,
		sessionTokenHash(token),
		username,
		session.CreatedAt.Format(time.RFC3339Nano),
		session.ExpiresAt.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return "", model.AdminSession{}, fmt.Errorf("create admin session: %w", err)
	}
	return token, session, nil
}

func (s *SQLiteStore) ValidateAdminSession(token string) (model.AdminUser, model.AdminSession, bool, error) {
	if token == "" {
		return model.AdminUser{}, model.AdminSession{}, false, nil
	}

	var (
		sessionUsername string
		createdAtText   string
		expiresAtText   string
		accountUsername string
		updatedAtText   string
	)
	tokenHash := sessionTokenHash(token)
	err := s.db.QueryRow(`
		SELECT s.username, s.created_at, s.expires_at, a.username, a.updated_at
		FROM admin_sessions s
		JOIN admin_accounts a ON a.id = ?
		WHERE s.token_hash = ?
	`, adminAccountID, tokenHash).Scan(
		&sessionUsername,
		&createdAtText,
		&expiresAtText,
		&accountUsername,
		&updatedAtText,
	)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, model.AdminSession{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, model.AdminSession{}, false, fmt.Errorf("load admin session: %w", err)
	}

	session := model.AdminSession{
		Username:  sessionUsername,
		CreatedAt: parseTime(createdAtText),
		ExpiresAt: parseTime(expiresAtText),
	}
	if session.ExpiresAt.IsZero() || time.Now().UTC().After(session.ExpiresAt) {
		_ = s.DeleteAdminSession(token)
		return model.AdminUser{}, model.AdminSession{}, false, nil
	}

	_, _ = s.db.Exec(`UPDATE admin_sessions SET last_seen_at = ? WHERE token_hash = ?`, time.Now().UTC().Format(time.RFC3339Nano), tokenHash)
	return model.AdminUser{
		Username:  accountUsername,
		UpdatedAt: parseTime(updatedAtText),
	}, session, true, nil
}

func (s *SQLiteStore) DeleteAdminSession(token string) error {
	if token == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, sessionTokenHash(token))
	if err != nil {
		return fmt.Errorf("delete admin session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateAdminAccount(req model.AdminAccountUpdateRequest, keepSessionToken string) (model.AdminUser, error) {
	var (
		currentUsername string
		currentHash     string
	)
	err := s.db.QueryRow(`
		SELECT username, password_hash
		FROM admin_accounts
		WHERE id = ?
	`, adminAccountID).Scan(&currentUsername, &currentHash)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, fmt.Errorf("admin account not initialized")
	}
	if err != nil {
		return model.AdminUser{}, fmt.Errorf("load admin account: %w", err)
	}

	ok, err := verifyPassword(req.CurrentPassword, currentHash)
	if err != nil {
		return model.AdminUser{}, err
	}
	if !ok {
		return model.AdminUser{}, fmt.Errorf("current password is invalid")
	}

	newUsername := strings.TrimSpace(req.NewUsername)
	if newUsername == "" {
		newUsername = currentUsername
	}
	newHash := currentHash
	if req.NewPassword != "" {
		if len(req.NewPassword) < adminPasswordMinLength {
			return model.AdminUser{}, fmt.Errorf("new password must be at least %d characters", adminPasswordMinLength)
		}
		newHash, err = hashPassword(req.NewPassword)
		if err != nil {
			return model.AdminUser{}, err
		}
	}
	if newUsername == currentUsername && newHash == currentHash {
		return model.AdminUser{}, fmt.Errorf("no account changes provided")
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return model.AdminUser{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.Exec(`
		UPDATE admin_accounts
		SET username = ?, password_hash = ?, updated_at = ?
		WHERE id = ?
	`, newUsername, newHash, now.Format(time.RFC3339Nano), adminAccountID)
	if err != nil {
		return model.AdminUser{}, fmt.Errorf("update admin account: %w", err)
	}

	if keepSessionToken != "" {
		keepSessionHash := sessionTokenHash(keepSessionToken)
		_, err = tx.Exec(`DELETE FROM admin_sessions WHERE token_hash <> ?`, keepSessionHash)
		if err == nil {
			_, err = tx.Exec(`UPDATE admin_sessions SET username = ? WHERE token_hash = ?`, newUsername, keepSessionHash)
		}
	} else {
		_, err = tx.Exec(`DELETE FROM admin_sessions`)
	}
	if err != nil {
		return model.AdminUser{}, fmt.Errorf("clear stale admin sessions: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return model.AdminUser{}, fmt.Errorf("commit admin account update: %w", err)
	}

	return model.AdminUser{Username: newUsername, UpdatedAt: now}, nil
}
