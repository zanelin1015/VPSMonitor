package store

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
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
		SELECT id, username, display_name, owner_type, owner_id, enabled, created_at, updated_at
		FROM customer_accounts
		WHERE guest_ip = ''
	`
	args := make([]any, 0, 2)
	if ownerType != "" && ownerID > 0 {
		query += ` AND owner_type = ? AND owner_id = ?`
		args = append(args, ownerType, ownerID)
	}
	query += ` ORDER BY updated_at DESC, id DESC`
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
	customerIDs := make([]int64, 0, len(customers))
	for _, customer := range customers {
		customerIDs = append(customerIDs, customer.ID)
	}
	assignmentsByCustomer, err := s.listCustomerAssignmentsForCustomers(customerIDs)
	if err != nil {
		return nil, err
	}
	frontProxiesByCustomer, err := s.ListFrontProxyNodesForGrantees(model.FrontProxyGranteeCustomer, customerIDs)
	if err != nil {
		return nil, err
	}
	for i := range customers {
		customers[i].Assignments = assignmentsByCustomer[customers[i].ID]
		customers[i].FrontProxies = frontProxyNodeViews(frontProxiesByCustomer[customers[i].ID])
	}
	return customers, nil
}

// GetOrCreateGuestCustomer creates a short-lived, non-login customer account
// keyed by the observed client IP and support owner scope. It lets guest
// support reuse the existing conversation and admin inbox without exposing
// guest accounts in customer management.
func (s *SQLiteStore) GetOrCreateGuestCustomer(ip string, ttl time.Duration, ownerType string, ownerID int64) (model.CustomerUser, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	if ttl <= 0 {
		return model.CustomerUser{}, fmt.Errorf("invalid guest customer lifetime")
	}
	ownerType = strings.TrimSpace(ownerType)
	if ownerType == "" {
		ownerType = model.CustomerOwnerGuest
	}
	if ownerType == model.CustomerOwnerGuest {
		ownerID = 0
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", ownerType, ownerID, ip)))
	username := "guest-" + hex.EncodeToString(digest[:])[:20]
	displayName := "访客 · " + ip
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	var existingUsername string
	err := s.db.QueryRow(`SELECT username FROM customer_accounts WHERE guest_ip = ? AND owner_type = ? AND owner_id = ?`, ip, ownerType, ownerID).Scan(&existingUsername)
	if err == sql.ErrNoRows {
		hash, hashErr := hashPassword(fmt.Sprintf("guest:%s:%d:%s", ownerType, ownerID, ip))
		if hashErr != nil {
			return model.CustomerUser{}, fmt.Errorf("hash guest customer password: %w", hashErr)
		}
		_, err = s.db.Exec(`
			INSERT OR IGNORE INTO customer_accounts
			(username, password_hash, display_name, subscription_token, owner_type, owner_id, enabled, guest_ip, guest_expires_at, created_at, updated_at)
			VALUES (?, ?, ?, '', ?, ?, 1, ?, ?, ?, ?)
		`, username, hash, displayName, ownerType, ownerID, ip, expiresAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil {
			return model.CustomerUser{}, fmt.Errorf("create guest customer: %w", err)
		}
	} else if err != nil {
		return model.CustomerUser{}, fmt.Errorf("find guest customer: %w", err)
	} else if existingUsername != "" {
		username = existingUsername
	}
	_, err = s.db.Exec(`
		UPDATE customer_accounts
		SET display_name = ?, enabled = 1, guest_ip = ?, guest_expires_at = ?, updated_at = ?
		WHERE username = ? AND owner_type = ? AND owner_id = ?
	`, displayName, ip, expiresAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), username, ownerType, ownerID)
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("refresh guest customer: %w", err)
	}
	user, err := scanCustomerUser(s.db.QueryRow(`
		SELECT id, username, display_name, owner_type, owner_id, enabled, created_at, updated_at
		FROM customer_accounts WHERE username = ? AND owner_type = ? AND owner_id = ?
	`, username, ownerType, ownerID))
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("load guest customer: %w", err)
	}
	return user, nil
}

func (s *SQLiteStore) GuestCustomerMatchesIP(customerID int64, ip, ownerType string, ownerID int64) (bool, error) {
	if customerID <= 0 {
		return false, nil
	}
	var storedIP string
	err := s.db.QueryRow(`SELECT guest_ip FROM customer_accounts WHERE id = ? AND owner_type = ? AND owner_id = ?`, customerID, ownerType, ownerID).Scan(&storedIP)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load guest customer ip: %w", err)
	}
	return strings.TrimSpace(storedIP) == strings.TrimSpace(ip), nil
}

func (s *SQLiteStore) listCustomerAssignmentsForCustomers(customerIDs []int64) (map[int64][]model.CustomerAssignment, error) {
	result := make(map[int64][]model.CustomerAssignment)
	ids := uniquePositiveInt64s(customerIDs)
	if len(ids) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT id, customer_id, agent_id, inbound_id, inbound_tag, client_id, client_email, public_client_name,
		       customer_remark, price_mode, revenue_amount, revenue_currency, revenue_cycle, enabled, created_at, updated_at
		FROM customer_assignments
		WHERE customer_id IN (%s)
		ORDER BY customer_id ASC, created_at DESC, id DESC
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("list customer assignments: %w", err)
	}
	defer rows.Close()
	var all []model.CustomerAssignment
	for rows.Next() {
		item, scanErr := scanCustomerAssignment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan customer assignment: %w", scanErr)
		}
		result[item.CustomerID] = append(result[item.CustomerID], item)
		all = append(all, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customer assignments: %w", err)
	}
	assignmentIDs := make([]int64, 0, len(all))
	for _, item := range all {
		assignmentIDs = append(assignmentIDs, item.ID)
	}
	frontProxies, err := s.ListFrontProxyNodesForAssignments(assignmentIDs)
	if err != nil {
		return nil, err
	}
	for customerID, assignments := range result {
		for index := range assignments {
			assignments[index].FrontProxies = frontProxyNodeViews(frontProxies[assignments[index].ID])
		}
		result[customerID] = assignments
	}
	return result, nil
}

func (s *SQLiteStore) GetCustomer(id int64) (model.CustomerAdminView, bool, error) {
	if id <= 0 {
		return model.CustomerAdminView{}, false, nil
	}
	row := s.db.QueryRow(`
		SELECT id, username, display_name, owner_type, owner_id, enabled, created_at, updated_at
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
	frontProxies, err := s.ListFrontProxyNodeViewsForGrantee(model.FrontProxyGranteeCustomer, id)
	if err != nil {
		return model.CustomerAdminView{}, false, err
	}
	return model.CustomerAdminView{CustomerUser: user, Assignments: assignments, FrontProxies: frontProxies}, true, nil
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
	subscriptionToken, err := randomTokenBytes(32)
	if err != nil {
		return model.CustomerAdminView{}, err
	}
	result, err := s.db.Exec(`
		INSERT INTO customer_accounts (username, password_hash, display_name, subscription_token, owner_type, owner_id, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, username, hash, displayName, subscriptionToken, ownerType, ownerID, boolInt(enabled), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
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
	if len(req.FrontProxyNodeIDs) > 0 {
		if err := s.ReplaceFrontProxyGrants(model.FrontProxyGranteeCustomer, id, req.FrontProxyNodeIDs); err != nil {
			return model.CustomerAdminView{}, err
		}
		customer, found, err = s.GetCustomer(id)
		if err != nil {
			return model.CustomerAdminView{}, err
		}
		if !found {
			return model.CustomerAdminView{}, fmt.Errorf("created customer not found")
		}
	}
	return customer, nil
}

func (s *SQLiteStore) EnsureCustomerSubscriptionToken(customerID int64) (string, error) {
	if customerID <= 0 {
		return "", fmt.Errorf("invalid customer id")
	}
	var current string
	err := s.db.QueryRow(`
		SELECT subscription_token
		FROM customer_accounts
		WHERE id = ? AND enabled = 1
	`, customerID).Scan(&current)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("customer not found")
	}
	if err != nil {
		return "", fmt.Errorf("load customer subscription token: %w", err)
	}
	current = strings.TrimSpace(current)
	if current != "" {
		return current, nil
	}
	for attempt := 0; attempt < 5; attempt++ {
		token, err := randomTokenBytes(32)
		if err != nil {
			return "", err
		}
		result, err := s.db.Exec(`
			UPDATE customer_accounts
			SET subscription_token = ?, updated_at = ?
			WHERE id = ? AND enabled = 1 AND subscription_token = ''
		`, token, time.Now().UTC().Format(time.RFC3339Nano), customerID)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return "", fmt.Errorf("update customer subscription token: %w", err)
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			return token, nil
		}
		err = s.db.QueryRow(`
			SELECT subscription_token
			FROM customer_accounts
			WHERE id = ? AND enabled = 1
		`, customerID).Scan(&current)
		if err != nil {
			return "", fmt.Errorf("reload customer subscription token: %w", err)
		}
		current = strings.TrimSpace(current)
		if current != "" {
			return current, nil
		}
	}
	return "", fmt.Errorf("generate customer subscription token: exhausted retries")
}

func (s *SQLiteStore) GetCustomerBySubscriptionToken(token string) (model.CustomerUser, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return model.CustomerUser{}, false, nil
	}
	row := s.db.QueryRow(`
		SELECT id, username, display_name, owner_type, owner_id, enabled, created_at, updated_at
		FROM customer_accounts
		WHERE subscription_token = ? AND enabled = 1
	`, token)
	user, err := scanCustomerUser(row)
	if err == sql.ErrNoRows {
		return model.CustomerUser{}, false, nil
	}
	if err != nil {
		return model.CustomerUser{}, false, fmt.Errorf("load customer by subscription token: %w", err)
	}
	return user, true, nil
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
		displayName = firstNonEmpty(currentDisplay, username)
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
	tx, err := s.db.Begin()
	if err != nil {
		return model.CustomerAdminView{}, fmt.Errorf("begin customer update: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		UPDATE customer_accounts
		SET username = ?, password_hash = ?, display_name = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, username, passwordHash, displayName, boolInt(enabled), now.Format(time.RFC3339Nano), id)
	if err != nil {
		return model.CustomerAdminView{}, fmt.Errorf("update customer: %w", err)
	}
	if !enabled || passwordChanged {
		if err := revokeCustomerSessions(tx, id, ""); err != nil {
			return model.CustomerAdminView{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.CustomerAdminView{}, fmt.Errorf("commit customer update: %w", err)
	}
	if req.FrontProxyNodeIDs != nil {
		if err := s.ReplaceFrontProxyGrants(model.FrontProxyGranteeCustomer, id, req.FrontProxyNodeIDs); err != nil {
			return model.CustomerAdminView{}, err
		}
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
		SELECT id, username, password_hash, display_name, enabled, created_at, updated_at
		FROM customer_accounts
		WHERE id = ?
	`, customerID).Scan(&user.ID, &user.Username, &currentHash, &user.DisplayName, &enabled, &createdAtText, &updatedAtText)
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
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE customer_accounts
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, newHash, now.Format(time.RFC3339Nano), customerID)
	if err != nil {
		return model.CustomerUser{}, fmt.Errorf("update customer password: %w", err)
	}

	keepSessionHash := ""
	if keepSessionToken != "" {
		keepSessionHash = sessionTokenHash(keepSessionToken)
	}
	if err := revokeCustomerSessions(tx, customerID, keepSessionHash); err != nil {
		return model.CustomerUser{}, err
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
		SELECT id, username, password_hash, display_name, enabled, created_at, updated_at
		FROM customer_accounts
		WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &passwordHash, &user.DisplayName, &enabled, &createdAtText, &updatedAtText)
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
		refreshHash, refreshExpiresAt  string
	)
	tokenHash := sessionTokenHash(token)
	err := s.db.QueryRow(`
		SELECT c.id, c.username, c.display_name, c.owner_type, c.owner_id, c.enabled, c.created_at, c.updated_at,
		       s.created_at, s.expires_at, s.refresh_token_hash, COALESCE(f.expires_at, '')
		FROM customer_sessions s
		JOIN customer_accounts c ON c.id = s.customer_id
		LEFT JOIN customer_refresh_sessions f ON f.token_hash = s.refresh_token_hash AND f.customer_id = s.customer_id
		WHERE s.token_hash = ?
	`, tokenHash).Scan(&user.ID, &user.Username, &user.DisplayName, &user.OwnerType, &user.OwnerID, &enabled, &createdAtText, &updatedAtText, &sCreatedAtText, &sExpiresAtText, &refreshHash, &refreshExpiresAt)
	if err == sql.ErrNoRows {
		return model.CustomerUser{}, model.CustomerSession{}, false, nil
	}
	if err != nil {
		return model.CustomerUser{}, model.CustomerSession{}, false, fmt.Errorf("load customer session: %w", err)
	}
	session := model.CustomerSession{CustomerID: user.ID, CreatedAt: parseTime(sCreatedAtText), ExpiresAt: parseTime(sExpiresAtText)}
	now := time.Now().UTC()
	if enabled == 0 || !now.Before(session.ExpiresAt) || (refreshHash != "" && !now.Before(parseTime(refreshExpiresAt))) {
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
		SELECT id, customer_id, agent_id, inbound_id, inbound_tag, client_id, client_email, public_client_name,
		       customer_remark, price_mode, revenue_amount, revenue_currency, revenue_cycle, enabled, created_at, updated_at
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
	if err := s.attachFrontProxyViewsToCustomerAssignments(items); err != nil {
		return nil, err
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
			customer_id, agent_id, inbound_id, inbound_tag, client_id, client_email, public_client_name,
			customer_remark, price_mode, revenue_amount, revenue_currency, revenue_cycle, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)
	`, customerID, normalized.AgentID, normalized.InboundID, normalized.InboundTag, normalized.ClientID, normalized.ClientEmail, normalized.PublicClientName, normalized.PriceMode, normalized.RevenueAmount, normalized.RevenueCurrency, normalized.RevenueCycle, boolInt(*normalized.Enabled), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
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
	if len(normalized.FrontProxyNodeIDs) > 0 {
		if err := s.ReplaceCustomerAssignmentFrontProxyNodes(assignment.ID, normalized.FrontProxyNodeIDs, model.AdminRoleRoot, adminAccountID); err != nil {
			return model.CustomerAssignment{}, err
		}
		assignment, found, err = s.GetCustomerAssignment(customerID, id)
		if err != nil {
			return model.CustomerAssignment{}, err
		}
		if !found {
			return model.CustomerAssignment{}, fmt.Errorf("created customer assignment not found")
		}
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
		SET agent_id = ?, inbound_id = ?, inbound_tag = ?, client_id = ?, client_email = ?, public_client_name = ?,
		    price_mode = ?, revenue_amount = ?, revenue_currency = ?, revenue_cycle = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND customer_id = ?
	`, normalized.AgentID, normalized.InboundID, normalized.InboundTag, normalized.ClientID, normalized.ClientEmail, normalized.PublicClientName,
		normalized.PriceMode, normalized.RevenueAmount, normalized.RevenueCurrency, normalized.RevenueCycle, boolInt(*normalized.Enabled), now.Format(time.RFC3339Nano), assignmentID, customerID)
	if err != nil {
		return model.CustomerAssignment{}, fmt.Errorf("update customer assignment: %w", err)
	}
	if req.FrontProxyNodeIDs != nil {
		if err := s.ReplaceCustomerAssignmentFrontProxyNodes(assignmentID, normalized.FrontProxyNodeIDs, model.AdminRoleRoot, adminAccountID); err != nil {
			return model.CustomerAssignment{}, err
		}
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
		SELECT id, customer_id, agent_id, inbound_id, inbound_tag, client_id, client_email, public_client_name,
		       customer_remark, price_mode, revenue_amount, revenue_currency, revenue_cycle, enabled, created_at, updated_at
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
	byAssignment, err := s.ListFrontProxyNodesForAssignments([]int64{item.ID})
	if err != nil {
		return model.CustomerAssignment{}, false, err
	}
	item.FrontProxies = frontProxyNodeViews(byAssignment[item.ID])
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
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientEmail = strings.TrimSpace(req.ClientEmail)
	req.PublicClientName = strings.TrimSpace(req.PublicClientName)
	req.PriceMode = strings.ToLower(strings.TrimSpace(req.PriceMode))
	if req.PriceMode != "override" {
		req.PriceMode = "inherit"
		req.RevenueAmount = nil
		req.RevenueCurrency = "CNY"
		req.RevenueCycle = "month"
	} else {
		if req.RevenueAmount == nil {
			return req, fmt.Errorf("revenue_amount is required when price_mode is override")
		}
		req.RevenueCurrency = strings.ToUpper(strings.TrimSpace(req.RevenueCurrency))
		if req.RevenueCurrency != "USDT" {
			req.RevenueCurrency = "CNY"
		}
		switch strings.ToLower(strings.TrimSpace(req.RevenueCycle)) {
		case "quarter":
			req.RevenueCycle = "quarter"
		case "semiannual", "halfyear", "half-year", "half_year", "half_yearly":
			req.RevenueCycle = "semiannual"
		case "year":
			req.RevenueCycle = "year"
		default:
			req.RevenueCycle = "month"
		}
		amount := max(*req.RevenueAmount, 0)
		req.RevenueAmount = &amount
	}
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
	if err := scanner.Scan(&user.ID, &user.Username, &user.DisplayName, &user.OwnerType, &user.OwnerID, &enabled, &createdAtText, &updatedAtText); err != nil {
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
		revenueAmount *float64
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
		&item.ClientID,
		&item.ClientEmail,
		&item.PublicClientName,
		&item.Remark,
		&item.PriceMode,
		&revenueAmount,
		&item.RevenueCurrency,
		&item.RevenueCycle,
		&enabled,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return model.CustomerAssignment{}, err
	}
	item.Enabled = enabled != 0
	if strings.TrimSpace(item.PriceMode) != "override" {
		item.PriceMode = "inherit"
	} else {
		item.RevenueAmount = revenueAmount
	}
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	return item, nil
}
