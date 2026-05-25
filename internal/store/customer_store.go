package store

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) ListCustomers() ([]model.CustomerAdminView, error) {
	return s.listCustomers("", 0)
}

func (s *SQLiteStore) ListCustomersForOwner(ownerType string, ownerID int64) ([]model.CustomerAdminView, error) {
	ownerType = normalizeCustomerOwnerType(ownerType)
	if ownerID <= 0 {
		return nil, nil
	}
	return s.listCustomers(ownerType, ownerID)
}

func (s *SQLiteStore) listCustomers(ownerType string, ownerID int64) ([]model.CustomerAdminView, error) {
	query := `
		SELECT id, username, display_name, style_code, owner_type, owner_id, enabled, created_at, updated_at
		FROM customer_accounts
	`
	args := []any{}
	if ownerType != "" && ownerID > 0 {
		query += ` WHERE owner_type = ? AND owner_id = ?`
		args = append(args, ownerType, ownerID)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()

	customers := make([]model.CustomerAdminView, 0)
	for rows.Next() {
		user, err := scanCustomerUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan customer: %w", err)
		}
		customers = append(customers, model.CustomerAdminView{CustomerUser: user})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customers: %w", err)
	}
	for i := range customers {
		assignments, err := s.ListCustomerAssignments(customers[i].ID)
		if err != nil {
			return nil, err
		}
		customers[i].Assignments = assignments
	}
	return customers, nil
}

func (s *SQLiteStore) GetCustomer(id int64) (model.CustomerAdminView, bool, error) {
	if id <= 0 {
		return model.CustomerAdminView{}, false, nil
	}
	row := s.db.QueryRow(`
		SELECT id, username, display_name, style_code, owner_type, owner_id, enabled, created_at, updated_at
		FROM customer_accounts
		WHERE id = ?
	`, id)
	user, err := scanCustomerUser(row)
	if err == sql.ErrNoRows {
		return model.CustomerAdminView{}, false, nil
	}
	if err != nil {
		return model.CustomerAdminView{}, false, fmt.Errorf("load customer: %w", err)
	}
	assignments, err := s.ListCustomerAssignments(id)
	if err != nil {
		return model.CustomerAdminView{}, false, err
	}
	return model.CustomerAdminView{CustomerUser: user, Assignments: assignments}, true, nil
}

func (s *SQLiteStore) CreateCustomer(req model.CustomerAccountRequest) (model.CustomerAdminView, error) {
	return s.CreateCustomerForOwner(req, model.AdminRoleRoot, adminAccountID)
}

func (s *SQLiteStore) CreateCustomerForOwner(req model.CustomerAccountRequest, ownerType string, ownerID int64) (model.CustomerAdminView, error) {
	username, displayName, enabled, err := normalizeCustomerAccountRequest(req, true)
	if err != nil {
		return model.CustomerAdminView{}, err
	}
	if req.Password == "" {
		req.Password = model.DefaultAccountPassword
	}
	if len(req.Password) < adminPasswordMinLength {
		return model.CustomerAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return model.CustomerAdminView{}, err
	}
	ownerType = normalizeCustomerOwnerType(ownerType)
	if ownerID <= 0 {
		ownerID = adminAccountID
	}
	now := time.Now().UTC()
	result, err := s.db.Exec(`
		INSERT INTO customer_accounts (username, password_hash, display_name, style_code, owner_type, owner_id, enabled, created_at, updated_at)
		VALUES (?, ?, ?, '', ?, ?, ?, ?, ?)
	`, username, hash, displayName, ownerType, ownerID, boolInt(enabled), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return model.CustomerAdminView{}, fmt.Errorf("create customer: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.CustomerAdminView{}, fmt.Errorf("read customer id: %w", err)
	}
	customer, found, err := s.GetCustomer(id)
	if err != nil {
		return model.CustomerAdminView{}, err
	}
	if !found {
		return model.CustomerAdminView{}, fmt.Errorf("created customer not found")
	}
	return customer, nil
}

func (s *SQLiteStore) CustomerOwnedBy(customerID int64, ownerType string, ownerID int64) (bool, error) {
	if customerID <= 0 || ownerID <= 0 {
		return false, nil
	}
	ownerType = normalizeCustomerOwnerType(ownerType)
	var exists int
	err := s.db.QueryRow(`
		SELECT 1
		FROM customer_accounts
		WHERE id = ? AND owner_type = ? AND owner_id = ?
		LIMIT 1
	`, customerID, ownerType, ownerID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check customer owner: %w", err)
	}
	return exists == 1, nil
}

func (s *SQLiteStore) UpdateCustomer(id int64, req model.CustomerAccountRequest) (model.CustomerAdminView, error) {
	if id <= 0 {
		return model.CustomerAdminView{}, fmt.Errorf("invalid customer id")
	}
	var (
		currentUsername string
		currentHash     string
		currentDisplay  string
		currentEnabled  int
	)
	err := s.db.QueryRow(`
		SELECT username, password_hash, display_name, enabled
		FROM customer_accounts
		WHERE id = ?
	`, id).Scan(&currentUsername, &currentHash, &currentDisplay, &currentEnabled)
	if err == sql.ErrNoRows {
		return model.CustomerAdminView{}, fmt.Errorf("customer not found")
	}
	if err != nil {
		return model.CustomerAdminView{}, fmt.Errorf("load customer: %w", err)
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = currentUsername
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	enabled := currentEnabled != 0
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	passwordHash := currentHash
	passwordChanged := false
	if req.Password != "" {
		if len(req.Password) < adminPasswordMinLength {
			return model.CustomerAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
		}
		passwordHash, err = hashPassword(req.Password)
		if err != nil {
			return model.CustomerAdminView{}, err
		}
		passwordChanged = true
	}

	now := time.Now().UTC()
	_, err = s.db.Exec(`
		UPDATE customer_accounts
		SET username = ?, password_hash = ?, display_name = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, username, passwordHash, displayName, boolInt(enabled), now.Format(time.RFC3339Nano), id)
	if err != nil {
		return model.CustomerAdminView{}, fmt.Errorf("update customer: %w", err)
	}
	if !enabled || passwordChanged {
		_, _ = s.db.Exec(`DELETE FROM customer_sessions WHERE customer_id = ?`, id)
	}
	customer, found, err := s.GetCustomer(id)
	if err != nil {
		return model.CustomerAdminView{}, err
	}
	if !found {
		return model.CustomerAdminView{}, fmt.Errorf("customer not found")
	}
	return customer, nil
}

func (s *SQLiteStore) UpdateCustomerPassword(customerID int64, req model.CustomerPasswordUpdateRequest, keepSessionToken string) (model.CustomerUser, error) {
	if customerID <= 0 {
		return model.CustomerUser{}, fmt.Errorf("invalid customer id")
	}
	if req.CurrentPassword == "" {
		return model.CustomerUser{}, fmt.Errorf("current password is required")
	}
	if len(req.NewPassword) < adminPasswordMinLength {
		return model.CustomerUser{}, fmt.Errorf("new password must be at least %d characters", adminPasswordMinLength)
	}

	var (
		user                         model.CustomerUser
		currentHash                  string
		enabled                      int
		createdAtText, updatedAtText string
	)
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, display_name, style_code, enabled, created_at, updated_at
		FROM customer_accounts
		WHERE id = ?
	`, customerID).Scan(&user.ID, &user.Username, &currentHash, &user.DisplayName, &user.StyleCode, &enabled, &createdAtText, &updatedAtText)
	if err == sql.ErrNoRows {
		return model.CustomerUser{}, fmt.Errorf("customer not found")
	}
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("load customer: %w", err)
	}
	if enabled == 0 {
		return model.CustomerUser{}, fmt.Errorf("customer is disabled")
	}
	ok, err := verifyPassword(req.CurrentPassword, currentHash)
	if err != nil {
		return model.CustomerUser{}, err
	}
	if !ok {
		return model.CustomerUser{}, fmt.Errorf("current password is invalid")
	}

	newHash, err := hashPassword(req.NewPassword)
	if err != nil {
		return model.CustomerUser{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("begin customer password update: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.Exec(`
		UPDATE customer_accounts
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, newHash, now.Format(time.RFC3339Nano), customerID)
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("update customer password: %w", err)
	}

	if keepSessionToken != "" {
		keepSessionHash := sessionTokenHash(keepSessionToken)
		_, err = tx.Exec(`DELETE FROM customer_sessions WHERE customer_id = ? AND token_hash <> ?`, customerID, keepSessionHash)
	} else {
		_, err = tx.Exec(`DELETE FROM customer_sessions WHERE customer_id = ?`, customerID)
	}
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("clear stale customer sessions: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return model.CustomerUser{}, fmt.Errorf("commit customer password update: %w", err)
	}

	user.Enabled = true
	user.CreatedAt = parseTime(createdAtText)
	user.UpdatedAt = now
	return user, nil
}

func (s *SQLiteStore) DeleteCustomer(id int64) error {
	result, err := s.db.Exec(`DELETE FROM customer_accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("customer not found")
	}
	return nil
}

func (s *SQLiteStore) AuthenticateCustomer(username, password string) (model.CustomerUser, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return model.CustomerUser{}, false, nil
	}
	var passwordHash string
	var user model.CustomerUser
	var enabled int
	var createdAtText, updatedAtText string
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, display_name, style_code, enabled, created_at, updated_at
		FROM customer_accounts
		WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &passwordHash, &user.DisplayName, &user.StyleCode, &enabled, &createdAtText, &updatedAtText)
	if err == sql.ErrNoRows {
		return model.CustomerUser{}, false, nil
	}
	if err != nil {
		return model.CustomerUser{}, false, fmt.Errorf("load customer: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(username)), []byte(strings.ToLower(user.Username))) != 1 {
		return model.CustomerUser{}, false, nil
	}
	if enabled == 0 {
		return model.CustomerUser{}, false, nil
	}
	ok, err := verifyPassword(password, passwordHash)
	if err != nil {
		return model.CustomerUser{}, false, err
	}
	if !ok {
		return model.CustomerUser{}, false, nil
	}
	user.Enabled = true
	user.CreatedAt = parseTime(createdAtText)
	user.UpdatedAt = parseTime(updatedAtText)
	return user, true, nil
}

func (s *SQLiteStore) CreateCustomerSession(customerID int64, ttl time.Duration) (string, model.CustomerSession, error) {
	if customerID <= 0 {
		return "", model.CustomerSession{}, fmt.Errorf("invalid customer id")
	}
	token, err := randomTokenBytes(adminSessionTokenBytes)
	if err != nil {
		return "", model.CustomerSession{}, err
	}
	now := time.Now().UTC()
	session := model.CustomerSession{CustomerID: customerID, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	_, err = s.db.Exec(`
		INSERT INTO customer_sessions (token_hash, customer_id, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
	`, sessionTokenHash(token), customerID, session.CreatedAt.Format(time.RFC3339Nano), session.ExpiresAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return "", model.CustomerSession{}, fmt.Errorf("create customer session: %w", err)
	}
	return token, session, nil
}

func (s *SQLiteStore) ValidateCustomerSession(token string) (model.CustomerUser, model.CustomerSession, bool, error) {
	if token == "" {
		return model.CustomerUser{}, model.CustomerSession{}, false, nil
	}
	var (
		user                           model.CustomerUser
		enabled                        int
		createdAtText, updatedAtText   string
		sCreatedAtText, sExpiresAtText string
	)
	tokenHash := sessionTokenHash(token)
	err := s.db.QueryRow(`
		SELECT c.id, c.username, c.display_name, c.style_code, c.enabled, c.created_at, c.updated_at,
		       s.created_at, s.expires_at
		FROM customer_sessions s
		JOIN customer_accounts c ON c.id = s.customer_id
		WHERE s.token_hash = ?
	`, tokenHash).Scan(&user.ID, &user.Username, &user.DisplayName, &user.StyleCode, &enabled, &createdAtText, &updatedAtText, &sCreatedAtText, &sExpiresAtText)
	if err == sql.ErrNoRows {
		return model.CustomerUser{}, model.CustomerSession{}, false, nil
	}
	if err != nil {
		return model.CustomerUser{}, model.CustomerSession{}, false, fmt.Errorf("load customer session: %w", err)
	}
	session := model.CustomerSession{CustomerID: user.ID, CreatedAt: parseTime(sCreatedAtText), ExpiresAt: parseTime(sExpiresAtText)}
	if enabled == 0 || session.ExpiresAt.IsZero() || time.Now().UTC().After(session.ExpiresAt) {
		_ = s.DeleteCustomerSession(token)
		return model.CustomerUser{}, model.CustomerSession{}, false, nil
	}
	_, _ = s.db.Exec(`UPDATE customer_sessions SET last_seen_at = ? WHERE token_hash = ?`, time.Now().UTC().Format(time.RFC3339Nano), tokenHash)
	user.Enabled = true
	user.CreatedAt = parseTime(createdAtText)
	user.UpdatedAt = parseTime(updatedAtText)
	return user, session, true, nil
}

func (s *SQLiteStore) DeleteCustomerSession(token string) error {
	if token == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM customer_sessions WHERE token_hash = ?`, sessionTokenHash(token))
	if err != nil {
		return fmt.Errorf("delete customer session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListCustomerAssignments(customerID int64) ([]model.CustomerAssignment, error) {
	return s.listCustomerAssignments(customerID, false)
}

func (s *SQLiteStore) ListEnabledCustomerAssignments(customerID int64) ([]model.CustomerAssignment, error) {
	return s.listCustomerAssignments(customerID, true)
}

func (s *SQLiteStore) listCustomerAssignments(customerID int64, enabledOnly bool) ([]model.CustomerAssignment, error) {
	if customerID <= 0 {
		return nil, nil
	}
	query := `
		SELECT id, customer_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
		       customer_remark, enabled, created_at, updated_at
		FROM customer_assignments
		WHERE customer_id = ?`
	args := []any{customerID}
	if enabledOnly {
		query += ` AND enabled = 1`
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list customer assignments: %w", err)
	}
	defer rows.Close()
	items := make([]model.CustomerAssignment, 0)
	for rows.Next() {
		item, err := scanCustomerAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan customer assignment: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customer assignments: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) CreateCustomerAssignment(customerID int64, req model.CustomerAssignmentRequest) (model.CustomerAssignment, error) {
	if customerID <= 0 {
		return model.CustomerAssignment{}, fmt.Errorf("invalid customer id")
	}
	normalized, err := s.normalizeCustomerAssignmentRequest(req, true)
	if err != nil {
		return model.CustomerAssignment{}, err
	}
	now := time.Now().UTC()
	result, err := s.db.Exec(`
		INSERT INTO customer_assignments (
			customer_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
			customer_remark, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, ?)
	`, customerID, normalized.AgentID, normalized.InboundID, normalized.InboundTag, normalized.ClientEmail, normalized.PublicClientName, boolInt(*normalized.Enabled), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return model.CustomerAssignment{}, fmt.Errorf("create customer assignment: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.CustomerAssignment{}, fmt.Errorf("read customer assignment id: %w", err)
	}
	assignment, found, err := s.GetCustomerAssignment(customerID, id)
	if err != nil {
		return model.CustomerAssignment{}, err
	}
	if !found {
		return model.CustomerAssignment{}, fmt.Errorf("created customer assignment not found")
	}
	return assignment, nil
}

func (s *SQLiteStore) UpdateCustomerAssignment(customerID, assignmentID int64, req model.CustomerAssignmentRequest) (model.CustomerAssignment, error) {
	if customerID <= 0 || assignmentID <= 0 {
		return model.CustomerAssignment{}, fmt.Errorf("invalid assignment id")
	}
	current, found, err := s.GetCustomerAssignment(customerID, assignmentID)
	if err != nil {
		return model.CustomerAssignment{}, err
	}
	if !found {
		return model.CustomerAssignment{}, fmt.Errorf("assignment not found")
	}
	normalized, err := s.normalizeCustomerAssignmentRequest(req, false)
	if err != nil {
		return model.CustomerAssignment{}, err
	}
	if normalized.Enabled == nil {
		normalized.Enabled = &current.Enabled
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(`
		UPDATE customer_assignments
		SET agent_id = ?, inbound_id = ?, inbound_tag = ?, client_email = ?, public_client_name = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND customer_id = ?
	`, normalized.AgentID, normalized.InboundID, normalized.InboundTag, normalized.ClientEmail, normalized.PublicClientName, boolInt(*normalized.Enabled), now.Format(time.RFC3339Nano), assignmentID, customerID)
	if err != nil {
		return model.CustomerAssignment{}, fmt.Errorf("update customer assignment: %w", err)
	}
	assignment, found, err := s.GetCustomerAssignment(customerID, assignmentID)
	if err != nil {
		return model.CustomerAssignment{}, err
	}
	if !found {
		return model.CustomerAssignment{}, fmt.Errorf("assignment not found")
	}
	return assignment, nil
}

func (s *SQLiteStore) DeleteCustomerAssignment(customerID, assignmentID int64) error {
	result, err := s.db.Exec(`DELETE FROM customer_assignments WHERE id = ? AND customer_id = ?`, assignmentID, customerID)
	if err != nil {
		return fmt.Errorf("delete customer assignment: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("assignment not found")
	}
	return nil
}

func (s *SQLiteStore) GetCustomerAssignment(customerID, assignmentID int64) (model.CustomerAssignment, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, customer_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
		       customer_remark, enabled, created_at, updated_at
		FROM customer_assignments
		WHERE id = ? AND customer_id = ?
	`, assignmentID, customerID)
	item, err := scanCustomerAssignment(row)
	if err == sql.ErrNoRows {
		return model.CustomerAssignment{}, false, nil
	}
	if err != nil {
		return model.CustomerAssignment{}, false, fmt.Errorf("load customer assignment: %w", err)
	}
	return item, true, nil
}

func (s *SQLiteStore) UpdateCustomerAssignmentRemark(customerID, assignmentID int64, remark string) (model.CustomerAssignment, error) {
	remark = strings.TrimSpace(remark)
	if len(remark) > 2000 {
		return model.CustomerAssignment{}, fmt.Errorf("remark is too long")
	}
	now := time.Now().UTC()
	result, err := s.db.Exec(`
		UPDATE customer_assignments
		SET customer_remark = ?, updated_at = ?
		WHERE id = ? AND customer_id = ? AND enabled = 1
	`, remark, now.Format(time.RFC3339Nano), assignmentID, customerID)
	if err != nil {
		return model.CustomerAssignment{}, fmt.Errorf("update customer assignment remark: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return model.CustomerAssignment{}, fmt.Errorf("assignment not found")
	}
	assignment, found, err := s.GetCustomerAssignment(customerID, assignmentID)
	if err != nil {
		return model.CustomerAssignment{}, err
	}
	if !found {
		return model.CustomerAssignment{}, fmt.Errorf("assignment not found")
	}
	return assignment, nil
}

func (s *SQLiteStore) UpdateCustomerStyle(customerID int64, styleCode string) (model.CustomerUser, error) {
	styleCode = strings.TrimSpace(styleCode)
	if len(styleCode) > 64*1024 {
		return model.CustomerUser{}, fmt.Errorf("style code is too long")
	}
	now := time.Now().UTC()
	result, err := s.db.Exec(`
		UPDATE customer_accounts
		SET style_code = ?, updated_at = ?
		WHERE id = ? AND enabled = 1
	`, styleCode, now.Format(time.RFC3339Nano), customerID)
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("update customer style: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return model.CustomerUser{}, fmt.Errorf("customer not found")
	}
	customer, found, err := s.GetCustomer(customerID)
	if err != nil {
		return model.CustomerUser{}, err
	}
	if !found {
		return model.CustomerUser{}, fmt.Errorf("customer not found")
	}
	return customer.CustomerUser, nil
}

func normalizeCustomerAccountRequest(req model.CustomerAccountRequest, creating bool) (string, string, bool, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return "", "", false, fmt.Errorf("username is required")
	}
	if len(username) > 120 {
		return "", "", false, fmt.Errorf("username is too long")
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 160 {
		return "", "", false, fmt.Errorf("display name is too long")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	} else if !creating {
		enabled = true
	}
	return username, displayName, enabled, nil
}

func (s *SQLiteStore) normalizeCustomerAssignmentRequest(req model.CustomerAssignmentRequest, creating bool) (model.CustomerAssignmentRequest, error) {
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.InboundTag = strings.TrimSpace(req.InboundTag)
	req.ClientEmail = strings.TrimSpace(req.ClientEmail)
	req.PublicClientName = strings.TrimSpace(req.PublicClientName)
	if req.AgentID == "" {
		return req, fmt.Errorf("agent_id is required")
	}
	if req.InboundID <= 0 {
		return req, fmt.Errorf("inbound_id is required")
	}
	if _, found, err := s.GetAgent(req.AgentID); err != nil {
		return req, err
	} else if !found {
		return req, fmt.Errorf("agent not found")
	}
	if req.PublicClientName == "" {
		req.PublicClientName = firstNonEmpty(req.ClientEmail, req.InboundTag, req.AgentID)
	}
	if len(req.PublicClientName) > 160 {
		return req, fmt.Errorf("public client name is too long")
	}
	if len(req.ClientEmail) > 320 {
		return req, fmt.Errorf("client email is too long")
	}
	if req.Enabled == nil && creating {
		enabled := true
		req.Enabled = &enabled
	}
	return req, nil
}

func scanCustomerUser(scanner rowScanner) (model.CustomerUser, error) {
	var (
		user          model.CustomerUser
		enabled       int
		createdAtText string
		updatedAtText string
	)
	if err := scanner.Scan(&user.ID, &user.Username, &user.DisplayName, &user.StyleCode, &user.OwnerType, &user.OwnerID, &enabled, &createdAtText, &updatedAtText); err != nil {
		return model.CustomerUser{}, err
	}
	user.Enabled = enabled != 0
	user.CreatedAt = parseTime(createdAtText)
	user.UpdatedAt = parseTime(updatedAtText)
	return user, nil
}

func normalizeCustomerOwnerType(ownerType string) string {
	switch strings.ToLower(strings.TrimSpace(ownerType)) {
	case model.AdminRoleAreaManager:
		return model.AdminRoleAreaManager
	default:
		return model.AdminRoleRoot
	}
}

func scanCustomerAssignment(scanner rowScanner) (model.CustomerAssignment, error) {
	var (
		item          model.CustomerAssignment
		enabled       int
		createdAtText string
		updatedAtText string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.CustomerID,
		&item.AgentID,
		&item.InboundID,
		&item.InboundTag,
		&item.ClientEmail,
		&item.PublicClientName,
		&item.Remark,
		&enabled,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return model.CustomerAssignment{}, err
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	return item, nil
}
