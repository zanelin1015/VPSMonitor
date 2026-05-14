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
		INSERT INTO admin_accounts (id, username, password_hash, avatar_url, created_at, updated_at)
		VALUES (?, ?, ?, '', ?, ?)
	`, adminAccountID, username, hash, now, now)
	if err != nil {
		return fmt.Errorf("create admin account: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AuthenticateAdmin(username, password string) (model.AdminUser, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return model.AdminUser{}, false, nil
	}
	var (
		storedUsername string
		passwordHash   string
		avatarURL      string
		updatedAtText  string
	)
	err := s.db.QueryRow(`
		SELECT username, password_hash, avatar_url, updated_at
		FROM admin_accounts
		WHERE id = ?
	`, adminAccountID).Scan(&storedUsername, &passwordHash, &avatarURL, &updatedAtText)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, fmt.Errorf("load admin account: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(storedUsername)) != 1 {
		return s.authenticateAreaManager(username, password)
	}
	ok, err := verifyPassword(password, passwordHash)
	if err != nil {
		return model.AdminUser{}, false, err
	}
	if !ok {
		return model.AdminUser{}, false, nil
	}
	return model.AdminUser{
		ID:        adminAccountID,
		Username:  storedUsername,
		AvatarURL: avatarURL,
		Role:      model.AdminRoleRoot,
		UpdatedAt: parseTime(updatedAtText),
	}, true, nil
}

func (s *SQLiteStore) CreateAdminSession(user model.AdminUser, ttl time.Duration) (string, model.AdminSession, error) {
	token, err := randomTokenBytes(adminSessionTokenBytes)
	if err != nil {
		return "", model.AdminSession{}, err
	}
	role := normalizeAdminRole(user.Role)
	accountID := user.ID
	if role == model.AdminRoleRoot || accountID <= 0 {
		accountID = adminAccountID
	}
	now := time.Now().UTC()
	session := model.AdminSession{
		Username:  user.Username,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	_, err = s.db.Exec(`
		INSERT INTO admin_sessions (token_hash, username, role, account_id, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		sessionTokenHash(token),
		user.Username,
		role,
		accountID,
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
		sessionUsername  string
		sessionRole      string
		sessionAccountID int64
		createdAtText    string
		expiresAtText    string
	)
	tokenHash := sessionTokenHash(token)
	err := s.db.QueryRow(`
		SELECT username, role, account_id, created_at, expires_at
		FROM admin_sessions
		WHERE token_hash = ?
	`, tokenHash).Scan(
		&sessionUsername,
		&sessionRole,
		&sessionAccountID,
		&createdAtText,
		&expiresAtText,
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

	var user model.AdminUser
	var ok bool
	role := normalizeAdminRole(sessionRole)
	if role == model.AdminRoleAreaManager {
		user, ok, err = s.loadAreaManagerAdminUser(sessionAccountID)
		if err != nil {
			return model.AdminUser{}, model.AdminSession{}, false, err
		}
		if !ok {
			_ = s.DeleteAdminSession(token)
			return model.AdminUser{}, model.AdminSession{}, false, nil
		}
	} else {
		user, ok, err = s.loadRootAdminUser()
		if err != nil {
			return model.AdminUser{}, model.AdminSession{}, false, err
		}
		if !ok {
			_ = s.DeleteAdminSession(token)
			return model.AdminUser{}, model.AdminSession{}, false, nil
		}
	}

	_, _ = s.db.Exec(`UPDATE admin_sessions SET last_seen_at = ? WHERE token_hash = ?`, time.Now().UTC().Format(time.RFC3339Nano), tokenHash)
	session.Username = sessionUsername
	return user, session, true, nil
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
		currentAvatar   string
	)
	err := s.db.QueryRow(`
		SELECT username, password_hash, avatar_url
		FROM admin_accounts
		WHERE id = ?
	`, adminAccountID).Scan(&currentUsername, &currentHash, &currentAvatar)
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
	newAvatar := currentAvatar
	if req.AvatarURL != nil {
		newAvatar, err = normalizeAdminAvatarURL(*req.AvatarURL)
		if err != nil {
			return model.AdminUser{}, err
		}
	}
	if newUsername == currentUsername && newHash == currentHash && newAvatar == currentAvatar {
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
		SET username = ?, password_hash = ?, avatar_url = ?, updated_at = ?
		WHERE id = ?
	`, newUsername, newHash, newAvatar, now.Format(time.RFC3339Nano), adminAccountID)
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

	return model.AdminUser{ID: adminAccountID, Username: newUsername, AvatarURL: newAvatar, Role: model.AdminRoleRoot, UpdatedAt: now}, nil
}

func (s *SQLiteStore) authenticateAreaManager(username, password string) (model.AdminUser, bool, error) {
	var (
		user          model.AdminUser
		passwordHash  string
		enabled       int
		updatedAtText string
	)
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, display_name, enabled, updated_at
		FROM area_manager_accounts
		WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &passwordHash, &user.DisplayName, &enabled, &updatedAtText)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, fmt.Errorf("load area manager account: %w", err)
	}
	if enabled == 0 {
		return model.AdminUser{}, false, nil
	}
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(username)), []byte(strings.ToLower(user.Username))) != 1 {
		return model.AdminUser{}, false, nil
	}
	ok, err := verifyPassword(password, passwordHash)
	if err != nil {
		return model.AdminUser{}, false, err
	}
	if !ok {
		return model.AdminUser{}, false, nil
	}
	user.Role = model.AdminRoleAreaManager
	user.UpdatedAt = parseTime(updatedAtText)
	user.AgentIDs, err = s.ListAreaManagerAgentIDs(user.ID)
	if err != nil {
		return model.AdminUser{}, false, err
	}
	return user, true, nil
}

func (s *SQLiteStore) loadRootAdminUser() (model.AdminUser, bool, error) {
	var (
		username      string
		avatarURL     string
		updatedAtText string
	)
	err := s.db.QueryRow(`
		SELECT username, avatar_url, updated_at
		FROM admin_accounts
		WHERE id = ?
	`, adminAccountID).Scan(&username, &avatarURL, &updatedAtText)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, fmt.Errorf("load admin account: %w", err)
	}
	return model.AdminUser{ID: adminAccountID, Username: username, AvatarURL: avatarURL, Role: model.AdminRoleRoot, UpdatedAt: parseTime(updatedAtText)}, true, nil
}

func (s *SQLiteStore) loadAreaManagerAdminUser(id int64) (model.AdminUser, bool, error) {
	if id <= 0 {
		return model.AdminUser{}, false, nil
	}
	var (
		user          model.AdminUser
		enabled       int
		updatedAtText string
	)
	err := s.db.QueryRow(`
		SELECT id, username, display_name, enabled, updated_at
		FROM area_manager_accounts
		WHERE id = ?
	`, id).Scan(&user.ID, &user.Username, &user.DisplayName, &enabled, &updatedAtText)
	if err == sql.ErrNoRows {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, fmt.Errorf("load area manager account: %w", err)
	}
	if enabled == 0 {
		return model.AdminUser{}, false, nil
	}
	user.Role = model.AdminRoleAreaManager
	user.UpdatedAt = parseTime(updatedAtText)
	agentIDs, err := s.ListAreaManagerAgentIDs(user.ID)
	if err != nil {
		return model.AdminUser{}, false, err
	}
	user.AgentIDs = agentIDs
	return user, true, nil
}

func normalizeAdminRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case model.AdminRoleAreaManager:
		return model.AdminRoleAreaManager
	default:
		return model.AdminRoleRoot
	}
}

func normalizeAdminAvatarURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 1024*1024 {
		return "", fmt.Errorf("avatar image is too large")
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return value, nil
	}
	return "", fmt.Errorf("avatar must be an image data url or http(s) url")
}
