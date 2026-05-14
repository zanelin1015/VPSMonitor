package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) ListAreaManagers() ([]model.AreaManagerAdminView, error) {
	rows, err := s.db.Query(`
		SELECT id, username, display_name, enabled, created_at, updated_at
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
		item.AgentIDs, err = s.ListAreaManagerAgentIDs(item.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate area managers: %w", err)
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
	return item, true, nil
}

func (s *SQLiteStore) CreateAreaManager(req model.AreaManagerAccountRequest) (model.AreaManagerAdminView, error) {
	username, displayName, enabled, agentIDs, err := s.normalizeAreaManagerRequest(req, true)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if len(req.Password) < adminPasswordMinLength {
		return model.AreaManagerAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
	}
	if err := s.ensureAreaManagerUsernameAvailable(username, 0); err != nil {
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
		INSERT INTO area_manager_accounts (username, password_hash, display_name, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, username, hash, displayName, boolInt(enabled), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("create area manager: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("read area manager id: %w", err)
	}
	if err = s.replaceAreaManagerAgentsTx(tx, id, agentIDs); err != nil {
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

	username, displayName, enabled, agentIDs, err := s.normalizeAreaManagerRequest(req, false)
	if err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if username == "" {
		username = current.view.Username
	}
	if displayName == "" {
		displayName = firstNonEmpty(current.view.DisplayName, username)
	}
	if req.Enabled == nil {
		enabled = current.view.Enabled
	}
	if agentIDs == nil {
		agentIDs = current.view.AgentIDs
	}
	if err := s.ensureAreaManagerUsernameAvailable(username, id); err != nil {
		return model.AreaManagerAdminView{}, err
	}

	passwordHash := current.passwordHash
	if req.Password != "" {
		if len(req.Password) < adminPasswordMinLength {
			return model.AreaManagerAdminView{}, fmt.Errorf("password must be at least %d characters", adminPasswordMinLength)
		}
		passwordHash, err = hashPassword(req.Password)
		if err != nil {
			return model.AreaManagerAdminView{}, err
		}
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
		SET username = ?, password_hash = ?, display_name = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, username, passwordHash, displayName, boolInt(enabled), now.Format(time.RFC3339Nano), id)
	if err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("update area manager: %w", err)
	}
	if err = s.replaceAreaManagerAgentsTx(tx, id, agentIDs); err != nil {
		return model.AreaManagerAdminView{}, err
	}
	if !enabled {
		_, err = tx.Exec(`DELETE FROM admin_sessions WHERE role = ? AND account_id = ?`, model.AdminRoleAreaManager, id)
		if err != nil {
			return model.AreaManagerAdminView{}, fmt.Errorf("clear disabled area manager sessions: %w", err)
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

func (s *SQLiteStore) normalizeAreaManagerRequest(req model.AreaManagerAccountRequest, creating bool) (string, string, bool, []string, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" && creating {
		return "", "", false, nil, fmt.Errorf("username is required")
	}
	if len(username) > 120 {
		return "", "", false, nil, fmt.Errorf("username is too long")
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" && creating {
		displayName = username
	}
	if len(displayName) > 160 {
		return "", "", false, nil, fmt.Errorf("display name is too long")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	var agentIDs []string
	if creating || req.AgentIDs != nil {
		var err error
		agentIDs, err = s.normalizeAreaManagerAgentIDs(req.AgentIDs)
		if err != nil {
			return "", "", false, nil, err
		}
	}
	return username, displayName, enabled, agentIDs, nil
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
		SELECT id, username, display_name, enabled, created_at, updated_at
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
	var enabled int
	var createdAtText, updatedAtText string
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, display_name, enabled, created_at, updated_at
		FROM area_manager_accounts
		WHERE id = ?
	`, id).Scan(&item.view.ID, &item.view.Username, &item.passwordHash, &item.view.DisplayName, &enabled, &createdAtText, &updatedAtText)
	if err == sql.ErrNoRows {
		return areaManagerWithHash{}, false, nil
	}
	if err != nil {
		return areaManagerWithHash{}, false, fmt.Errorf("load area manager: %w", err)
	}
	item.view.Enabled = enabled != 0
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
	var enabled int
	var createdAtText, updatedAtText string
	if err := scanner.Scan(&item.ID, &item.Username, &item.DisplayName, &enabled, &createdAtText, &updatedAtText); err != nil {
		return model.AreaManagerAdminView{}, fmt.Errorf("scan area manager: %w", err)
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	return item, nil
}
