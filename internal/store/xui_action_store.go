package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) CreateXUIAction(agentID string, req model.XUIActionRequest) (model.XUIAction, error) {
	return s.CreateXUIActionWithActor(agentID, req, model.XUIActionActor{})
}

func (s *SQLiteStore) CreateXUIActionWithActor(agentID string, req model.XUIActionRequest, actor model.XUIActionActor) (model.XUIAction, error) {
	if agentID == "" {
		return model.XUIAction{}, fmt.Errorf("agent_id is required")
	}
	if !isValidXUIActionKind(req.Kind) {
		return model.XUIAction{}, fmt.Errorf("unsupported x-ui action kind: %s", req.Kind)
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}
	if _, found, err := s.GetAgent(agentID); err != nil {
		return model.XUIAction{}, err
	} else if !found {
		return model.XUIAction{}, fmt.Errorf("agent not found")
	}

	now := time.Now().UTC()
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		return model.XUIAction{}, fmt.Errorf("marshal action payload: %w", err)
	}
	result, err := s.db.Exec(`
		INSERT INTO xui_actions (
			agent_id, kind, status, created_by_role, created_by_account_id, created_by_username,
			payload_json, result_json, error, created_at, updated_at, claimed_at, completed_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, '{}', '', ?, ?, '', '')
	`, agentID, req.Kind, model.XUIActionStatusPending, strings.TrimSpace(actor.Role), actor.AccountID,
		strings.TrimSpace(actor.Username), string(payloadJSON), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return model.XUIAction{}, fmt.Errorf("create x-ui action: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.XUIAction{}, fmt.Errorf("read x-ui action id: %w", err)
	}
	action, found, err := s.GetXUIAction(agentID, id)
	if err != nil {
		return model.XUIAction{}, err
	}
	if !found {
		return model.XUIAction{}, fmt.Errorf("created x-ui action not found")
	}
	return action, nil
}

func (s *SQLiteStore) GetXUIAction(agentID string, id int64) (model.XUIAction, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, agent_id, kind, status, created_by_role, created_by_account_id, created_by_username,
		       payload_json, result_json, error, created_at, updated_at, claimed_at, completed_at
		FROM xui_actions
		WHERE agent_id = ? AND id = ?
	`, agentID, id)
	action, err := scanXUIAction(row)
	if err == sql.ErrNoRows {
		return model.XUIAction{}, false, nil
	}
	if err != nil {
		return model.XUIAction{}, false, err
	}
	return action, true, nil
}

func (s *SQLiteStore) ListXUIActions(agentID string, limit int) ([]model.XUIAction, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.Query(`
		SELECT id, agent_id, kind, status, created_by_role, created_by_account_id, created_by_username,
		       payload_json, result_json, error, created_at, updated_at, claimed_at, completed_at
		FROM xui_actions
		WHERE agent_id = ?
		ORDER BY id DESC
		LIMIT ?
	`, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list x-ui actions: %w", err)
	}
	defer rows.Close()
	return scanXUIActions(rows)
}

func (s *SQLiteStore) ListXUIActionsByActor(agentID, role string, accountID int64, limit int) ([]model.XUIAction, error) {
	if strings.TrimSpace(role) == "" || accountID <= 0 {
		return []model.XUIAction{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.Query(`
		SELECT id, agent_id, kind, status, created_by_role, created_by_account_id, created_by_username,
		       payload_json, result_json, error, created_at, updated_at, claimed_at, completed_at
		FROM xui_actions
		WHERE agent_id = ? AND created_by_role = ? AND created_by_account_id = ?
		ORDER BY id DESC
		LIMIT ?
	`, agentID, strings.TrimSpace(role), accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("list x-ui actions by actor: %w", err)
	}
	defer rows.Close()
	return scanXUIActions(rows)
}

func (s *SQLiteStore) ClaimPendingXUIActions(agentID string, limit int) ([]model.XUIAction, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin x-ui action claim: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.Query(`
		SELECT id
		FROM xui_actions
		WHERE agent_id = ? AND status = ?
		ORDER BY id ASC
		LIMIT ?
	`, agentID, model.XUIActionStatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("select pending x-ui actions: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan pending x-ui action id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close pending x-ui actions: %w", closeErr)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending x-ui actions: %w", err)
	}
	if len(ids) == 0 {
		if err = tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit empty x-ui action claim: %w", err)
		}
		return []model.XUIAction{}, nil
	}

	for _, id := range ids {
		if _, err = tx.Exec(`
			UPDATE xui_actions
			SET status = ?, updated_at = ?, claimed_at = ?
			WHERE agent_id = ? AND id = ? AND status = ?
		`, model.XUIActionStatusRunning, now, now, agentID, id, model.XUIActionStatusPending); err != nil {
			return nil, fmt.Errorf("claim x-ui action %d: %w", id, err)
		}
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := []any{agentID}
	for _, id := range ids {
		args = append(args, id)
	}
	claimedRows, err := tx.Query(`
		SELECT id, agent_id, kind, status, created_by_role, created_by_account_id, created_by_username,
		       payload_json, result_json, error, created_at, updated_at, claimed_at, completed_at
		FROM xui_actions
		WHERE agent_id = ? AND id IN (`+placeholders+`)
		ORDER BY id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load claimed x-ui actions: %w", err)
	}
	actions, err := scanXUIActions(claimedRows)
	_ = claimedRows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit x-ui action claim: %w", err)
	}
	return actions, nil
}

func (s *SQLiteStore) MarkXUIActionRunning(agentID string, id int64) (model.XUIAction, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`
		UPDATE xui_actions
		SET status = ?, updated_at = ?, claimed_at = ?
		WHERE agent_id = ? AND id = ? AND status = ?
	`, model.XUIActionStatusRunning, now, now, agentID, id, model.XUIActionStatusPending); err != nil {
		return model.XUIAction{}, fmt.Errorf("mark x-ui action running: %w", err)
	}
	action, found, err := s.GetXUIAction(agentID, id)
	if err != nil {
		return model.XUIAction{}, err
	}
	if !found {
		return model.XUIAction{}, fmt.Errorf("x-ui action not found")
	}
	return action, nil
}

func (s *SQLiteStore) CompleteXUIAction(agentID string, id int64, req model.XUIActionResultRequest) (model.XUIAction, error) {
	status := req.Status
	if status == "" {
		if req.Error == "" {
			status = model.XUIActionStatusSucceeded
		} else {
			status = model.XUIActionStatusFailed
		}
	}
	if status != model.XUIActionStatusSucceeded && status != model.XUIActionStatusFailed {
		return model.XUIAction{}, fmt.Errorf("invalid x-ui action result status: %s", status)
	}
	if req.Result == nil {
		req.Result = map[string]any{}
	}
	resultJSON, err := json.Marshal(req.Result)
	if err != nil {
		return model.XUIAction{}, fmt.Errorf("marshal x-ui action result: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`
		UPDATE xui_actions
		SET status = ?, result_json = ?, error = ?, updated_at = ?, completed_at = ?
		WHERE agent_id = ? AND id = ?
	`, status, string(resultJSON), req.Error, now, now, agentID, id)
	if err != nil {
		return model.XUIAction{}, fmt.Errorf("complete x-ui action: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return model.XUIAction{}, fmt.Errorf("read x-ui action completion count: %w", err)
	}
	if affected == 0 {
		return model.XUIAction{}, fmt.Errorf("x-ui action not found")
	}
	action, found, err := s.GetXUIAction(agentID, id)
	if err != nil {
		return model.XUIAction{}, err
	}
	if !found {
		return model.XUIAction{}, fmt.Errorf("completed x-ui action not found")
	}
	if status == model.XUIActionStatusSucceeded && action.Kind == model.XUIActionUpdateClientExpiry && shouldPersistXUIClientExpiry(action.Payload) {
		if err := s.applyXUIClientExpiryConfig(agentID, action.Payload); err != nil {
			return model.XUIAction{}, err
		}
	}
	if status == model.XUIActionStatusSucceeded && action.Kind == model.XUIActionDeleteClient {
		if err := s.applyXUIClientDeleteConfig(agentID, action.Payload); err != nil {
			return model.XUIAction{}, err
		}
	}
	return action, nil
}

func shouldPersistXUIClientExpiry(payload map[string]any) bool {
	persist, specified := payload["persist_billing"].(bool)
	return !specified || persist
}

func (s *SQLiteStore) applyXUIClientDeleteConfig(agentID string, payload map[string]any) error {
	record, found, err := s.GetAgent(agentID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("agent not found")
	}
	inboundID := int(numberFromPayload(payload["inbound_id"]))
	inboundTag := stringFromPayload(payload["inbound_tag"])
	email := stringFromPayload(payload["email"])
	if email == "" {
		return nil
	}
	next := record.Config.Renewal.ClientBillings[:0]
	removed := false
	for _, billing := range record.Config.Renewal.ClientBillings {
		if billing.InboundID == inboundID && billing.InboundTag == inboundTag && billing.Email == email {
			removed = true
			continue
		}
		next = append(next, billing)
	}
	if !removed {
		return nil
	}
	record.Config.Renewal.ClientBillings = next
	_, err = s.UpdateAgentConfigWithActor(agentID, record.Config, "system:xui-client-delete")
	return err
}

func (s *SQLiteStore) applyXUIClientExpiryConfig(agentID string, payload map[string]any) error {
	record, found, err := s.GetAgent(agentID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("agent not found")
	}
	inboundID := int(numberFromPayload(payload["inbound_id"]))
	inboundTag, _ := payload["inbound_tag"].(string)
	email, _ := payload["email"].(string)
	expiryTime := numberFromPayload(payload["expiry_time"])
	if email == "" || expiryTime <= 0 {
		return nil
	}
	foundBilling := false
	for index := range record.Config.Renewal.ClientBillings {
		billing := &record.Config.Renewal.ClientBillings[index]
		if billing.InboundID == inboundID && billing.InboundTag == inboundTag && billing.Email == email {
			if startTime := numberFromPayload(payload["start_time"]); startTime > 0 {
				billing.StartTime = startTime
			}
			billing.ExpireTime = expiryTime
			if cycle, _ := payload["expire_cycle"].(string); cycle != "" {
				billing.ExpireCycle = cycle
			}
			if autoRenew, ok := payload["expire_auto_renew"].(bool); ok {
				billing.ExpireAutoRenew = autoRenew
			}
			foundBilling = true
			break
		}
	}
	if !foundBilling {
		record.Config.Renewal.ClientBillings = append(record.Config.Renewal.ClientBillings, model.XUIClientBillingConfig{
			InboundID:       inboundID,
			InboundTag:      inboundTag,
			Email:           email,
			StartTime:       numberFromPayload(payload["start_time"]),
			ExpireTime:      expiryTime,
			ExpireCycle:     stringFromPayload(payload["expire_cycle"]),
			ExpireAutoRenew: boolFromPayload(payload["expire_auto_renew"]),
		})
	}
	_, err = s.UpdateAgentConfigWithActor(agentID, record.Config, "system:xui-client-expiry")
	return err
}

func numberFromPayload(raw any) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}

func stringFromPayload(raw any) string {
	value, _ := raw.(string)
	return value
}

func boolFromPayload(raw any) bool {
	value, _ := raw.(bool)
	return value
}

func scanXUIActions(rows rowsScanner) ([]model.XUIAction, error) {
	defer rows.Close()
	actions := make([]model.XUIAction, 0)
	for rows.Next() {
		action, err := scanXUIAction(rows)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate x-ui actions: %w", err)
	}
	return actions, nil
}

func scanXUIAction(row rowScanner) (model.XUIAction, error) {
	var (
		action        model.XUIAction
		payloadJSON   string
		resultJSON    string
		createdAtText string
		updatedAtText string
		claimedAtText string
		completedText string
	)
	if err := row.Scan(
		&action.ID,
		&action.AgentID,
		&action.Kind,
		&action.Status,
		&action.CreatedByRole,
		&action.CreatedByAccountID,
		&action.CreatedByUsername,
		&payloadJSON,
		&resultJSON,
		&action.Error,
		&createdAtText,
		&updatedAtText,
		&claimedAtText,
		&completedText,
	); err != nil {
		return model.XUIAction{}, err
	}
	action.CreatedAt = parseTime(createdAtText)
	action.UpdatedAt = parseTime(updatedAtText)
	if claimedAtText != "" {
		claimed := parseTime(claimedAtText)
		action.ClaimedAt = &claimed
	}
	if completedText != "" {
		completed := parseTime(completedText)
		action.CompletedAt = &completed
	}
	if payloadJSON != "" {
		_ = json.Unmarshal([]byte(payloadJSON), &action.Payload)
	}
	if action.Payload == nil {
		action.Payload = map[string]any{}
	}
	if resultJSON != "" {
		_ = json.Unmarshal([]byte(resultJSON), &action.Result)
	}
	if action.Result == nil {
		action.Result = map[string]any{}
	}
	return action, nil
}
