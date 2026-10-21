package store

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) ListOperators() ([]model.OperatorAdminView, error) {
	rows, err := s.db.Query(`
		SELECT id, username, display_name, enabled, created_at, updated_at
		FROM operator_accounts
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list operators: %w", err)
	}
	defer rows.Close()
	items := make([]model.OperatorAdminView, 0)
	for rows.Next() {
		var item model.OperatorAdminView
		var enabled int
		var createdAt, updatedAt string
		if err := rows.Scan(&item.ID, &item.Username, &item.DisplayName, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan operator: %w", err)
		}
		item.Enabled = enabled != 0
		item.CreatedAt = parseTime(createdAt)
		item.UpdatedAt = parseTime(updatedAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operators: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) CreateOperator(req model.OperatorAccountRequest) (model.OperatorAdminView, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return model.OperatorAdminView{}, fmt.Errorf("username is required")
	}
	if len(username) > 120 {
		return model.OperatorAdminView{}, fmt.Errorf("username is too long")
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 160 {
		return model.OperatorAdminView{}, fmt.Errorf("display name is too long")
	}
	if err := s.ensureOperatorUsernameAvailable(username, 0); err != nil {
		return model.OperatorAdminView{}, err
	}
	password := req.Password
	if password == "" {
		password = model.DefaultAccountPassword
	}
	if len(password) < adminPasswordMinLength {
		return model.OperatorAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
	}
	hash, err := hashPassword(password)
	if err != nil {
		return model.OperatorAdminView{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`
		INSERT INTO operator_accounts (username, password_hash, display_name, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, username, hash, displayName, boolInt(enabled), now, now)
	if err != nil {
		return model.OperatorAdminView{}, fmt.Errorf("create operator: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.OperatorAdminView{}, fmt.Errorf("read operator id: %w", err)
	}
	return s.GetOperator(id)
}

func (s *SQLiteStore) GetOperator(id int64) (model.OperatorAdminView, error) {
	if id <= 0 {
		return model.OperatorAdminView{}, fmt.Errorf("invalid operator id")
	}
	var item model.OperatorAdminView
	var enabled int
	var createdAt, updatedAt string
	err := s.db.QueryRow(`
		SELECT id, username, display_name, enabled, created_at, updated_at
		FROM operator_accounts
		WHERE id = ?
	`, id).Scan(&item.ID, &item.Username, &item.DisplayName, &enabled, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return model.OperatorAdminView{}, fmt.Errorf("operator not found")
	}
	if err != nil {
		return model.OperatorAdminView{}, fmt.Errorf("load operator: %w", err)
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func (s *SQLiteStore) UpdateOperator(id int64, req model.OperatorAccountRequest) (model.OperatorAdminView, error) {
	if id <= 0 {
		return model.OperatorAdminView{}, fmt.Errorf("invalid operator id")
	}
	var currentUsername, currentHash, currentDisplayName string
	var currentEnabled int
	if err := s.db.QueryRow(`
		SELECT username, password_hash, display_name, enabled
		FROM operator_accounts
		WHERE id = ?
	`, id).Scan(&currentUsername, &currentHash, &currentDisplayName, &currentEnabled); err == sql.ErrNoRows {
		return model.OperatorAdminView{}, fmt.Errorf("operator not found")
	} else if err != nil {
		return model.OperatorAdminView{}, fmt.Errorf("load operator: %w", err)
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = currentUsername
	}
	if len(username) > 120 {
		return model.OperatorAdminView{}, fmt.Errorf("username is too long")
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = currentDisplayName
	}
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 160 {
		return model.OperatorAdminView{}, fmt.Errorf("display name is too long")
	}
	if err := s.ensureOperatorUsernameAvailable(username, id); err != nil {
		return model.OperatorAdminView{}, err
	}
	enabled := currentEnabled != 0
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	hash := currentHash
	passwordChanged := false
	if req.Password != "" {
		if len(req.Password) < adminPasswordMinLength {
			return model.OperatorAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
		}
		var err error
		hash, err = hashPassword(req.Password)
		if err != nil {
			return model.OperatorAdminView{}, err
		}
		passwordChanged = true
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`
		UPDATE operator_accounts
		SET username = ?, password_hash = ?, display_name = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, username, hash, displayName, boolInt(enabled), now, id); err != nil {
		return model.OperatorAdminView{}, fmt.Errorf("update operator: %w", err)
	}
	if !enabled || passwordChanged {
		if _, err := s.db.Exec(`DELETE FROM admin_sessions WHERE role = ? AND account_id = ?`, model.AdminRoleOperator, id); err != nil {
			return model.OperatorAdminView{}, fmt.Errorf("clear operator sessions: %w", err)
		}
	}
	return s.GetOperator(id)
}

func (s *SQLiteStore) DeleteOperator(id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid operator id")
	}
	result, err := s.db.Exec(`DELETE FROM operator_accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete operator: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("operator not found")
	}
	_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE role = ? AND account_id = ?`, model.AdminRoleOperator, id)
	return nil
}

func (s *SQLiteStore) authenticateOperator(username, password string) (model.AdminUser, bool, error) {
	var user model.AdminUser
	var passwordHash string
	var enabled int
	var updatedAt string
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, display_name, enabled, updated_at
		FROM operator_accounts
		WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &passwordHash, &user.DisplayName, &enabled, &updatedAt)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, fmt.Errorf("load operator account: %w", err)
	}
	if enabled == 0 || subtle.ConstantTimeCompare([]byte(strings.ToLower(username)), []byte(strings.ToLower(user.Username))) != 1 {
		return model.AdminUser{}, false, nil
	}
	ok, err := verifyPassword(password, passwordHash)
	if err != nil || !ok {
		return model.AdminUser{}, false, err
	}
	user.Role = model.AdminRoleOperator
	user.UpdatedAt = parseTime(updatedAt)
	return user, true, nil
}

func (s *SQLiteStore) loadOperatorAdminUser(id int64) (model.AdminUser, bool, error) {
	if id <= 0 {
		return model.AdminUser{}, false, nil
	}
	var user model.AdminUser
	var enabled int
	var updatedAt string
	err := s.db.QueryRow(`
		SELECT id, username, display_name, enabled, updated_at
		FROM operator_accounts
		WHERE id = ?
	`, id).Scan(&user.ID, &user.Username, &user.DisplayName, &enabled, &updatedAt)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, fmt.Errorf("load operator account: %w", err)
	}
	if enabled == 0 {
		return model.AdminUser{}, false, nil
	}
	user.Role = model.AdminRoleOperator
	user.UpdatedAt = parseTime(updatedAt)
	return user, true, nil
}

func (s *SQLiteStore) ensureOperatorUsernameAvailable(username string, exceptID int64) error {
	username = strings.TrimSpace(username)
	var rootUsername string
	if err := s.db.QueryRow(`SELECT username FROM admin_accounts WHERE id = ?`, adminAccountID).Scan(&rootUsername); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load admin account: %w", err)
	}
	if strings.EqualFold(username, strings.TrimSpace(rootUsername)) {
		return fmt.Errorf("username already exists")
	}
	var existingID int64
	if err := s.db.QueryRow(`SELECT id FROM area_manager_accounts WHERE username = ?`, username).Scan(&existingID); err == nil {
		return fmt.Errorf("username already exists")
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("check area manager username: %w", err)
	}
	if err := s.db.QueryRow(`SELECT id FROM operator_accounts WHERE username = ?`, username).Scan(&existingID); err == nil {
		if existingID != exceptID {
			return fmt.Errorf("username already exists")
		}
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("check operator username: %w", err)
	}
	return nil
}
