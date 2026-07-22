package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) ListAreaManagers() ([]model.AreaManagerAdminView, error) {
	rows, err := s.db.Query(`
		SELECT id, username, display_name, enabled, billing_enabled, revenue_amount, revenue_currency, revenue_cycle,
			outbound_create_enabled, created_at, updated_at
		FROM area_manager_accounts
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list area managers: %w", err)
	}
	defer rows.Close()

	items := make([]model.AreaManagerAdminView, 0)
	for rows.Next() {
		item, err := scanAreaManager(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate area managers: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close area manager rows: %w", err)
	}
	for i := range items {
		items[i].AgentIDs, err = s.ListAreaManagerAgentIDs(items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].Assignments, err = s.ListAreaManagerAssignments(items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].OutboundGrants, err = s.ListAreaManagerOutboundGrants(items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].Customers, err = s.ListCustomersForOwner(model.AdminRoleAreaManager, items[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *SQLiteStore) GetAreaManager(id int64) (model.AreaManagerAdminView, bool, error) {
	if id <= 0 {
		return model.AreaManagerAdminView{}, false, nil
	}
	item, found, err := s.getAreaManagerBase(id)
	if err != nil || !found {
		return item, found, err
	}
	item.AgentIDs, err = s.ListAreaManagerAgentIDs(id)
	if err != nil {
		return model.AreaManagerAdminView{}, false, err
	}
	item.Assignments, err = s.ListAreaManagerAssignments(id)
	if err != nil {
		return model.AreaManagerAdminView{}, false, err
	}
	item.OutboundGrants, err = s.ListAreaManagerOutboundGrants(id)
	if err != nil {
		return model.AreaManagerAdminView{}, false, err
	}
	item.Customers, err = s.ListCustomersForOwner(model.AdminRoleAreaManager, id)
	if err != nil {
		return model.AreaManagerAdminView{}, false, err
	}
	return item, true, nil
}

func (s *SQLiteStore) CreateAreaManager(req model.AreaManagerAccountRequest) (model.AreaManagerAdminView, error) {
	normalized, err := s.normalizeAreaManagerRequest(req, true)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if req.Password == "" {
		req.Password = model.DefaultAccountPassword
	}
	if len(req.Password) < adminPasswordMinLength {
		return model.AreaManagerAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
	}
	if err := s.ensureAreaManagerUsernameAvailable(normalized.username, 0); err != nil {
		return model.AreaManagerAdminView{}, err
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("begin area manager create: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.Exec(`
		INSERT INTO area_manager_accounts (
			username, password_hash, display_name, enabled, billing_enabled, revenue_amount, revenue_currency, revenue_cycle,
			outbound_create_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, normalized.username, hash, normalized.displayName, boolInt(normalized.enabled), boolInt(normalized.billingEnabled), normalized.revenueAmount, normalized.revenueCurrency, normalized.revenueCycle, boolInt(normalized.outboundCreateEnabled), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("create area manager: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("read area manager id: %w", err)
	}
	if err = s.replaceAreaManagerAgentsTx(tx, id, normalized.agentIDs); err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if err = s.replaceAreaManagerOutboundGrantsTx(tx, id, normalized.outboundGrants); err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("commit area manager create: %w", err)
	}
	item, found, err := s.GetAreaManager(id)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if !found {
		return model.AreaManagerAdminView{}, fmt.Errorf("created area manager not found")
	}
	return item, nil
}

func (s *SQLiteStore) UpdateAreaManager(id int64, req model.AreaManagerAccountRequest) (model.AreaManagerAdminView, error) {
	if id <= 0 {
		return model.AreaManagerAdminView{}, fmt.Errorf("invalid area manager id")
	}
	current, found, err := s.getAreaManagerWithHash(id)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if !found {
		return model.AreaManagerAdminView{}, fmt.Errorf("area manager not found")
	}

	normalized, err := s.normalizeAreaManagerRequest(req, false)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if normalized.username == "" {
		normalized.username = current.view.Username
	}
	if normalized.displayName == "" {
		normalized.displayName = firstNonEmpty(current.view.DisplayName, normalized.username)
	}
	if req.Enabled == nil {
		normalized.enabled = current.view.Enabled
	}
	if req.BillingEnabled == nil {
		normalized.billingEnabled = current.view.BillingEnabled
	}
	if req.RevenueAmount == nil {
		normalized.revenueAmount = current.view.RevenueAmount
	}
	if strings.TrimSpace(req.RevenueCurrency) == "" {
		normalized.revenueCurrency = current.view.RevenueCurrency
	}
	if strings.TrimSpace(req.RevenueCycle) == "" {
		normalized.revenueCycle = current.view.RevenueCycle
	}
	if req.OutboundCreateEnabled == nil {
		normalized.outboundCreateEnabled = current.view.OutboundCreateEnabled
	}
	if normalized.agentIDs == nil {
		normalized.agentIDs = current.view.AgentIDs
	}
	normalized.agentIDs = appendAreaManagerOutboundGrantAgents(normalized.agentIDs, normalized.outboundGrants)
	if err := s.ensureAreaManagerUsernameAvailable(normalized.username, id); err != nil {
		return model.AreaManagerAdminView{}, err
	}

	passwordHash := current.passwordHash
	passwordChanged := false
	if req.Password != "" {
		if len(req.Password) < adminPasswordMinLength {
			return model.AreaManagerAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
		}
		passwordHash, err = hashPassword(req.Password)
		if err != nil {
			return model.AreaManagerAdminView{}, err
		}
		passwordChanged = true
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("begin area manager update: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	_, err = tx.Exec(`
		UPDATE area_manager_accounts
		SET username = ?, password_hash = ?, display_name = ?, enabled = ?, billing_enabled = ?, revenue_amount = ?, revenue_currency = ?, revenue_cycle = ?,
			outbound_create_enabled = ?, updated_at = ?
		WHERE id = ?
	`, normalized.username, passwordHash, normalized.displayName, boolInt(normalized.enabled), boolInt(normalized.billingEnabled), normalized.revenueAmount, normalized.revenueCurrency, normalized.revenueCycle, boolInt(normalized.outboundCreateEnabled), now.Format(time.RFC3339Nano), id)
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("update area manager: %w", err)
	}
	if err = s.replaceAreaManagerAgentsTx(tx, id, normalized.agentIDs); err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if normalized.outboundGrants != nil {
		if err = s.replaceAreaManagerOutboundGrantsTx(tx, id, normalized.outboundGrants); err != nil {
			return model.AreaManagerAdminView{}, err
		}
	}
	if !normalized.enabled || passwordChanged {
		_, err = tx.Exec(`DELETE FROM admin_sessions WHERE role = ? AND account_id = ?`, model.AdminRoleAreaManager, id)
		if err != nil {
			return model.AreaManagerAdminView{}, fmt.Errorf("clear area manager sessions: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("commit area manager update: %w", err)
	}
	item, found, err := s.GetAreaManager(id)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if !found {
		return model.AreaManagerAdminView{}, fmt.Errorf("area manager not found")
	}
	return item, nil
}

func (s *SQLiteStore) DeleteAreaManager(id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid area manager id")
	}
	result, err := s.db.Exec(`DELETE FROM area_manager_accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete area manager: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("area manager not found")
	}
	_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE role = ? AND account_id = ?`, model.AdminRoleAreaManager, id)
	return nil
}

func (s *SQLiteStore) ListAreaManagerAgentIDs(managerID int64) ([]string, error) {
	if managerID <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT agent_id
		FROM area_manager_agents
		WHERE manager_id = ?
		ORDER BY agent_id ASC
	`, managerID)
	if err != nil {
		return nil, fmt.Errorf("list area manager agents: %w", err)
	}
	defer rows.Close()

	agentIDs := make([]string, 0)
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return nil, fmt.Errorf("scan area manager agent: %w", err)
		}
		agentIDs = append(agentIDs, agentID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate area manager agents: %w", err)
	}
	return agentIDs, nil
}

func (s *SQLiteStore) AreaManagerCanAccessAgent(managerID int64, agentID string) (bool, error) {
	agentID = strings.TrimSpace(agentID)
	if managerID <= 0 || agentID == "" {
		return false, nil
	}
	var exists int
	err := s.db.QueryRow(`
		SELECT 1
		FROM area_manager_agents
		WHERE manager_id = ? AND agent_id = ?
		LIMIT 1
	`, managerID, agentID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check area manager agent access: %w", err)
	}
	return exists == 1, nil
}

func (s *SQLiteStore) AddAreaManagerAgents(managerID int64, agentIDs []string) error {
	if managerID <= 0 {
		return fmt.Errorf("invalid area manager id")
	}
	agentIDs, err := s.normalizeAreaManagerAgentIDs(agentIDs)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, agentID := range agentIDs {
		if _, err := s.db.Exec(`
			INSERT OR IGNORE INTO area_manager_agents (manager_id, agent_id, created_at)
			VALUES (?, ?, ?)
		`, managerID, agentID, now); err != nil {
			return fmt.Errorf("assign agent %s to area manager: %w", agentID, err)
		}
	}
	return nil
}

func (s *SQLiteStore) ListAreaManagerAssignments(managerID int64) ([]model.AreaManagerAssignment, error) {
	if managerID <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT id, manager_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
			enabled, created_at, updated_at
		FROM area_manager_assignments
		WHERE manager_id = ?
		ORDER BY agent_id ASC, inbound_id ASC, client_email ASC, id ASC
	`, managerID)
	if err != nil {
		return nil, fmt.Errorf("list area manager assignments: %w", err)
	}
	defer rows.Close()

	items := make([]model.AreaManagerAssignment, 0)
	for rows.Next() {
		item, err := scanAreaManagerAssignment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate area manager assignments: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) CreateAreaManagerAssignment(managerID int64, req model.AreaManagerAssignmentRequest) (model.AreaManagerAssignment, error) {
	items, err := s.CreateAreaManagerAssignments(managerID, []model.AreaManagerAssignmentRequest{req})
	if err != nil {
		return model.AreaManagerAssignment{}, err
	}
	if len(items) == 0 {
		return model.AreaManagerAssignment{}, fmt.Errorf("created area manager assignment not found")
	}
	return items[0], nil
}

func (s *SQLiteStore) CreateAreaManagerAssignments(managerID int64, requests []model.AreaManagerAssignmentRequest) ([]model.AreaManagerAssignment, error) {
	if managerID <= 0 {
		return nil, fmt.Errorf("invalid area manager id")
	}
	if len(requests) == 0 {
		return []model.AreaManagerAssignment{}, nil
	}
	normalized := make([]model.AreaManagerAssignmentRequest, 0, len(requests))
	agentSet := make(map[string]struct{})
	for _, req := range requests {
		item, err := s.normalizeAreaManagerAssignmentRequest(req)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, item)
		agentSet[item.AgentID] = struct{}{}
	}
	agentIDs := make([]string, 0, len(agentSet))
	for agentID := range agentSet {
		agentIDs = append(agentIDs, agentID)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin area manager assignments: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, agentID := range agentIDs {
		if _, err = tx.Exec(`
			INSERT OR IGNORE INTO area_manager_agents (manager_id, agent_id, created_at)
			VALUES (?, ?, ?)
		`, managerID, agentID, now); err != nil {
			return nil, fmt.Errorf("assign agent %s to area manager: %w", agentID, err)
		}
	}

	for _, req := range normalized {
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		if _, err = tx.Exec(`
			INSERT INTO area_manager_assignments (
				manager_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
				enabled, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(manager_id, agent_id, inbound_id, inbound_tag, client_email) DO UPDATE SET
				public_client_name = excluded.public_client_name,
				enabled = excluded.enabled,
				updated_at = excluded.updated_at
		`, managerID, req.AgentID, req.InboundID, req.InboundTag, req.ClientEmail, req.PublicClientName, boolInt(enabled), now, now); err != nil {
			return nil, fmt.Errorf("create area manager assignment: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit area manager assignments: %w", err)
	}
	return s.ListAreaManagerAssignments(managerID)
}

func (s *SQLiteStore) DeleteAreaManagerAssignment(managerID, assignmentID int64) error {
	if managerID <= 0 || assignmentID <= 0 {
		return fmt.Errorf("invalid area manager assignment id")
	}
	result, err := s.db.Exec(`DELETE FROM area_manager_assignments WHERE manager_id = ? AND id = ?`, managerID, assignmentID)
	if err != nil {
		return fmt.Errorf("delete area manager assignment: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("area manager assignment not found")
	}
	return nil
}

func (s *SQLiteStore) ListAreaManagerAgentTags(managerID int64) (map[string][]string, error) {
	result := make(map[string][]string)
	if managerID <= 0 {
		return result, nil
	}
	rows, err := s.db.Query(`
		SELECT agent_id, tags_json
		FROM area_manager_agent_tags
		WHERE manager_id = ?
	`, managerID)
	if err != nil {
		return nil, fmt.Errorf("list area manager agent tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agentID, raw string
		if err := rows.Scan(&agentID, &raw); err != nil {
			return nil, fmt.Errorf("scan area manager agent tags: %w", err)
		}
		var tags []string
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &tags)
		}
		result[agentID] = normalizeTags(tags)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate area manager agent tags: %w", err)
	}
	return result, nil
}

func (s *SQLiteStore) GetAreaManagerAgentTags(managerID int64, agentID string) ([]string, bool, error) {
	agentID = strings.TrimSpace(agentID)
	if managerID <= 0 || agentID == "" {
		return nil, false, nil
	}
	var raw string
	err := s.db.QueryRow(`
		SELECT tags_json
		FROM area_manager_agent_tags
		WHERE manager_id = ? AND agent_id = ?
	`, managerID, agentID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load area manager agent tags: %w", err)
	}
	var tags []string
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &tags)
	}
	return normalizeTags(tags), true, nil
}

func (s *SQLiteStore) SaveAreaManagerAgentTags(managerID int64, agentID string, tags []string) ([]string, error) {
	agentID = strings.TrimSpace(agentID)
	if managerID <= 0 {
		return nil, fmt.Errorf("invalid area manager id")
	}
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	normalized := normalizeTags(tags)
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode area manager agent tags: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO area_manager_agent_tags (manager_id, agent_id, tags_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(manager_id, agent_id) DO UPDATE SET tags_json = excluded.tags_json, updated_at = excluded.updated_at
	`, managerID, agentID, string(data), now)
	if err != nil {
		return nil, fmt.Errorf("save area manager agent tags: %w", err)
	}
	return normalized, nil
}

type normalizedAreaManagerRequest struct {
	username              string
	displayName           string
	enabled               bool
	agentIDs              []string
	billingEnabled        bool
	revenueAmount         float64
	revenueCurrency       string
	revenueCycle          string
	outboundCreateEnabled bool
	outboundGrants        []model.AreaManagerOutboundGrantRequest
}

func (s *SQLiteStore) normalizeAreaManagerRequest(req model.AreaManagerAccountRequest, creating bool) (normalizedAreaManagerRequest, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" && creating {
		return normalizedAreaManagerRequest{}, fmt.Errorf("username is required")
	}
	if len(username) > 120 {
		return normalizedAreaManagerRequest{}, fmt.Errorf("username is too long")
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" && creating {
		displayName = username
	}
	if len(displayName) > 160 {
		return normalizedAreaManagerRequest{}, fmt.Errorf("display name is too long")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	billingEnabled := false
	if req.BillingEnabled != nil {
		billingEnabled = *req.BillingEnabled
	}
	outboundCreateEnabled := false
	if req.OutboundCreateEnabled != nil {
		outboundCreateEnabled = *req.OutboundCreateEnabled
	}
	revenueAmount := 0.0
	if req.RevenueAmount != nil {
		revenueAmount = *req.RevenueAmount
	}
	if revenueAmount < 0 {
		revenueAmount = 0
	}
	revenueCurrency := strings.ToUpper(strings.TrimSpace(req.RevenueCurrency))
	if revenueCurrency != "USDT" {
		revenueCurrency = "CNY"
	}
	revenueCycle := strings.ToLower(strings.TrimSpace(req.RevenueCycle))
	switch revenueCycle {
	case "quarter", "year":
	default:
		revenueCycle = "month"
	}
	var agentIDs []string
	if creating || req.AgentIDs != nil {
		var err error
		agentIDs, err = s.normalizeAreaManagerAgentIDs(req.AgentIDs)
		if err != nil {
			return normalizedAreaManagerRequest{}, err
		}
	}
	var outboundGrants []model.AreaManagerOutboundGrantRequest
	if creating || req.OutboundGrants != nil {
		var err error
		outboundGrants, err = s.normalizeAreaManagerOutboundGrants(req.OutboundGrants)
		if err != nil {
			return normalizedAreaManagerRequest{}, err
		}
		agentIDs = appendAreaManagerOutboundGrantAgents(agentIDs, outboundGrants)
	}
	return normalizedAreaManagerRequest{
		username:              username,
		displayName:           displayName,
		enabled:               enabled,
		agentIDs:              agentIDs,
		billingEnabled:        billingEnabled,
		revenueAmount:         revenueAmount,
		revenueCurrency:       revenueCurrency,
		revenueCycle:          revenueCycle,
		outboundCreateEnabled: outboundCreateEnabled,
		outboundGrants:        outboundGrants,
	}, nil
}

func appendAreaManagerOutboundGrantAgents(agentIDs []string, grants []model.AreaManagerOutboundGrantRequest) []string {
	seen := make(map[string]struct{}, len(agentIDs)+len(grants))
	result := make([]string, 0, len(agentIDs)+len(grants))
	for _, agentID := range agentIDs {
		if agentID = strings.TrimSpace(agentID); agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		result = append(result, agentID)
	}
	for _, grant := range grants {
		if _, ok := seen[grant.AgentID]; ok {
			continue
		}
		seen[grant.AgentID] = struct{}{}
		result = append(result, grant.AgentID)
	}
	return result
}

func (s *SQLiteStore) normalizeAreaManagerAgentIDs(raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	agentIDs := make([]string, 0, len(raw))
	for _, agentID := range raw {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		if _, found, err := s.GetAgent(agentID); err != nil {
			return nil, err
		} else if !found {
			return nil, fmt.Errorf("agent %s not found", agentID)
		}
		seen[agentID] = struct{}{}
		agentIDs = append(agentIDs, agentID)
	}
	return agentIDs, nil
}

func (s *SQLiteStore) normalizeAreaManagerAssignmentRequest(req model.AreaManagerAssignmentRequest) (model.AreaManagerAssignmentRequest, error) {
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
	return req, nil
}

func (s *SQLiteStore) ensureAreaManagerUsernameAvailable(username string, exceptID int64) error {
	var rootUsername string
	err := s.db.QueryRow(`SELECT username FROM admin_accounts WHERE id = ?`, adminAccountID).Scan(&rootUsername)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load admin account: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(username), strings.TrimSpace(rootUsername)) {
		return fmt.Errorf("username already exists")
	}

	var existingID int64
	err = s.db.QueryRow(`SELECT id FROM area_manager_accounts WHERE username = ?`, username).Scan(&existingID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check area manager username: %w", err)
	}
	if existingID != exceptID {
		return fmt.Errorf("username already exists")
	}
	return nil
}

func (s *SQLiteStore) getAreaManagerBase(id int64) (model.AreaManagerAdminView, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, username, display_name, enabled, billing_enabled, revenue_amount, revenue_currency, revenue_cycle,
			outbound_create_enabled, created_at, updated_at
		FROM area_manager_accounts
		WHERE id = ?
	`, id)
	item, err := scanAreaManager(row)
	if err == sql.ErrNoRows {
		return model.AreaManagerAdminView{}, false, nil
	}
	if err != nil {
		return model.AreaManagerAdminView{}, false, err
	}
	return item, true, nil
}

type areaManagerWithHash struct {
	view         model.AreaManagerAdminView
	passwordHash string
}

func (s *SQLiteStore) getAreaManagerWithHash(id int64) (areaManagerWithHash, bool, error) {
	var item areaManagerWithHash
	var enabled, billingEnabled, outboundCreateEnabled int
	var createdAtText, updatedAtText string
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, display_name, enabled, billing_enabled, revenue_amount, revenue_currency, revenue_cycle,
			outbound_create_enabled, created_at, updated_at
		FROM area_manager_accounts
		WHERE id = ?
	`, id).Scan(&item.view.ID, &item.view.Username, &item.passwordHash, &item.view.DisplayName, &enabled, &billingEnabled, &item.view.RevenueAmount, &item.view.RevenueCurrency, &item.view.RevenueCycle, &outboundCreateEnabled, &createdAtText, &updatedAtText)
	if err == sql.ErrNoRows {
		return areaManagerWithHash{}, false, nil
	}
	if err != nil {
		return areaManagerWithHash{}, false, fmt.Errorf("load area manager: %w", err)
	}
	item.view.Enabled = enabled != 0
	item.view.BillingEnabled = billingEnabled != 0
	item.view.OutboundCreateEnabled = outboundCreateEnabled != 0
	item.view.RevenueCurrency = normalizeAreaManagerRevenueCurrency(item.view.RevenueCurrency)
	item.view.RevenueCycle = normalizeAreaManagerRevenueCycle(item.view.RevenueCycle)
	item.view.CreatedAt = parseTime(createdAtText)
	item.view.UpdatedAt = parseTime(updatedAtText)
	item.view.AgentIDs, err = s.ListAreaManagerAgentIDs(id)
	if err != nil {
		return areaManagerWithHash{}, false, err
	}
	return item, true, nil
}

func (s *SQLiteStore) replaceAreaManagerAgentsTx(tx *sql.Tx, managerID int64, agentIDs []string) error {
	if _, err := tx.Exec(`DELETE FROM area_manager_agents WHERE manager_id = ?`, managerID); err != nil {
		return fmt.Errorf("clear area manager agents: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, agentID := range agentIDs {
		if _, err := tx.Exec(`
			INSERT INTO area_manager_agents (manager_id, agent_id, created_at)
			VALUES (?, ?, ?)
		`, managerID, agentID, now); err != nil {
			return fmt.Errorf("assign agent %s to area manager: %w", agentID, err)
		}
	}
	return nil
}

func scanAreaManager(scanner rowScanner) (model.AreaManagerAdminView, error) {
	var item model.AreaManagerAdminView
	var enabled, billingEnabled, outboundCreateEnabled int
	var createdAtText, updatedAtText string
	if err := scanner.Scan(&item.ID, &item.Username, &item.DisplayName, &enabled, &billingEnabled, &item.RevenueAmount, &item.RevenueCurrency, &item.RevenueCycle, &outboundCreateEnabled, &createdAtText, &updatedAtText); err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("scan area manager: %w", err)
	}
	item.Enabled = enabled != 0
	item.BillingEnabled = billingEnabled != 0
	item.OutboundCreateEnabled = outboundCreateEnabled != 0
	item.RevenueCurrency = normalizeAreaManagerRevenueCurrency(item.RevenueCurrency)
	item.RevenueCycle = normalizeAreaManagerRevenueCycle(item.RevenueCycle)
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	return item, nil
}

func normalizeAreaManagerRevenueCurrency(value string) string {
	if strings.ToUpper(strings.TrimSpace(value)) == "USDT" {
		return "USDT"
	}
	return "CNY"
}

func normalizeAreaManagerRevenueCycle(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "quarter", "year":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "month"
	}
}

func scanAreaManagerAssignment(scanner rowScanner) (model.AreaManagerAssignment, error) {
	var (
		item          model.AreaManagerAssignment
		enabled       int
		createdAtText string
		updatedAtText string
	)
	if err := scanner.Scan(&item.ID, &item.ManagerID, &item.AgentID, &item.InboundID, &item.InboundTag, &item.ClientEmail, &item.PublicClientName, &enabled, &createdAtText, &updatedAtText); err != nil {
		return model.AreaManagerAssignment{}, fmt.Errorf("scan area manager assignment: %w", err)
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	return item, nil
}
