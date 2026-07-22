package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) ListAreaManagerOutboundGrants(managerID int64) ([]model.AreaManagerOutboundGrant, error) {
	if managerID <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT manager_id, agent_id, outbound_tag, created_at
		FROM area_manager_outbound_grants
		WHERE manager_id = ?
		ORDER BY agent_id ASC, outbound_tag ASC
	`, managerID)
	if err != nil {
		return nil, fmt.Errorf("list area manager outbound grants: %w", err)
	}
	defer rows.Close()

	items := make([]model.AreaManagerOutboundGrant, 0)
	for rows.Next() {
		var item model.AreaManagerOutboundGrant
		var createdAtText string
		if err := rows.Scan(&item.ManagerID, &item.AgentID, &item.OutboundTag, &createdAtText); err != nil {
			return nil, fmt.Errorf("scan area manager outbound grant: %w", err)
		}
		item.CreatedAt = parseTime(createdAtText)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate area manager outbound grants: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) UpsertAreaManagerOutboundGrant(managerID int64, req model.AreaManagerOutboundGrantRequest) error {
	if managerID <= 0 {
		return fmt.Errorf("invalid area manager id")
	}
	items, err := s.normalizeAreaManagerOutboundGrants([]model.AreaManagerOutboundGrantRequest{req})
	if err != nil {
		return err
	}
	if len(items) != 1 {
		return fmt.Errorf("outbound grant is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin area manager outbound grant: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM area_manager_accounts WHERE id = ?`, managerID).Scan(&exists); err != nil {
		return fmt.Errorf("load area manager: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("area manager not found")
	}
	var canAccess int
	if err := tx.QueryRow(`
		SELECT 1 FROM area_manager_agents
		WHERE manager_id = ? AND agent_id = ?
		LIMIT 1
	`, managerID, items[0].AgentID).Scan(&canAccess); err == sql.ErrNoRows {
		return fmt.Errorf("outbound grant agent is not assigned to area manager")
	} else if err != nil {
		return fmt.Errorf("check outbound grant agent access: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO area_manager_outbound_grants (manager_id, agent_id, outbound_tag, created_at)
		VALUES (?, ?, ?, ?)
	`, managerID, items[0].AgentID, items[0].OutboundTag, now); err != nil {
		return fmt.Errorf("save area manager outbound grant: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit area manager outbound grant: %w", err)
	}
	return nil
}

func (s *SQLiteStore) replaceAreaManagerOutboundGrantsTx(tx *sql.Tx, managerID int64, grants []model.AreaManagerOutboundGrantRequest) error {
	if _, err := tx.Exec(`DELETE FROM area_manager_outbound_grants WHERE manager_id = ?`, managerID); err != nil {
		return fmt.Errorf("clear area manager outbound grants: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, grant := range grants {
		if _, err := tx.Exec(`
			INSERT INTO area_manager_outbound_grants (manager_id, agent_id, outbound_tag, created_at)
			VALUES (?, ?, ?, ?)
		`, managerID, grant.AgentID, grant.OutboundTag, now); err != nil {
			return fmt.Errorf("save area manager outbound grant %s/%s: %w", grant.AgentID, grant.OutboundTag, err)
		}
	}
	return nil
}

func (s *SQLiteStore) normalizeAreaManagerOutboundGrants(raw []model.AreaManagerOutboundGrantRequest) ([]model.AreaManagerOutboundGrantRequest, error) {
	seen := make(map[string]struct{}, len(raw))
	items := make([]model.AreaManagerOutboundGrantRequest, 0, len(raw))
	for _, item := range raw {
		item.AgentID = strings.TrimSpace(item.AgentID)
		item.OutboundTag = strings.TrimSpace(item.OutboundTag)
		if item.AgentID == "" || item.OutboundTag == "" {
			return nil, fmt.Errorf("outbound grant agent_id and outbound_tag are required")
		}
		if len(item.OutboundTag) > 160 {
			return nil, fmt.Errorf("outbound tag is too long")
		}
		if _, found, err := s.GetAgent(item.AgentID); err != nil {
			return nil, err
		} else if !found {
			return nil, fmt.Errorf("agent %s not found", item.AgentID)
		}
		key := item.AgentID + "\x00" + item.OutboundTag
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	return items, nil
}

func validateAreaManagerOutboundGrantAgents(agentIDs []string, grants []model.AreaManagerOutboundGrantRequest) error {
	assigned := make(map[string]struct{}, len(agentIDs))
	for _, agentID := range agentIDs {
		if agentID = strings.TrimSpace(agentID); agentID != "" {
			assigned[agentID] = struct{}{}
		}
	}
	for _, grant := range grants {
		if _, ok := assigned[strings.TrimSpace(grant.AgentID)]; !ok {
			return fmt.Errorf("outbound grant agent %s is not assigned to area manager", grant.AgentID)
		}
	}
	return nil
}
