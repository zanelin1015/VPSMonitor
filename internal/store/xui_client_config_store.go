package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"bridge-core/internal/model"
)

var ErrClientConfigConflict = errors.New("客户端配置已变化，请刷新后重新编辑")
var ErrClientConfigPending = errors.New("该客户端已有待下发或执行中的配置，请等待完成后再编辑")

// SaveXUIClientConfig updates only this client's billing, and queues its remote
// patch in the same transaction. Other VPS configuration and drafts stay intact.
func (s *SQLiteStore) SaveXUIClientConfig(agentID string, target model.XUIClientConfigTarget, req model.XUIClientConfigRequest, actor model.XUIActionActor) (model.XUIClientConfigSave, error) {
	if err := req.Validate(); err != nil {
		return model.XUIClientConfigSave{}, err
	}
	requestJSON, err := json.Marshal(req)
	if err != nil {
		return model.XUIClientConfigSave{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return model.XUIClientConfigSave{}, err
	}
	defer tx.Rollback()
	var savedAgent, savedRole, savedRequest string
	var savedActor int64
	err = tx.QueryRow(`SELECT agent_id, actor_role, actor_id, request_json FROM xui_client_config_saves WHERE request_id = ?`, req.RequestID).Scan(&savedAgent, &savedRole, &savedActor, &savedRequest)
	if err == nil {
		if savedAgent != agentID || savedRole != actor.Role || savedActor != actor.AccountID || savedRequest != string(requestJSON) {
			return model.XUIClientConfigSave{}, ErrClientConfigConflict
		}
		save, _, err := readClientConfigSave(tx, agentID, req.RequestID)
		return save, err
	}
	if err != sql.ErrNoRows {
		return model.XUIClientConfigSave{}, err
	}
	key := fmt.Sprintf("%d\x00%s\x00%s", req.InboundID, req.InboundTag, req.Email)
	if req.ClientID != "" {
		key = "id:" + req.ClientID
	}
	if _, err := expireStaleXUIActionsTx(tx, target.AgentID, XUIActionExecutionLease); err != nil {
		return model.XUIClientConfigSave{}, err
	}
	var pending int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM xui_client_config_saves s JOIN xui_actions a ON a.id = s.action_id AND a.agent_id = s.action_agent_id WHERE s.agent_id = ? AND s.client_key = ? AND a.status IN (?, ?)`, agentID, key, model.XUIActionStatusPending, model.XUIActionStatusRunning).Scan(&pending); err != nil {
		return model.XUIClientConfigSave{}, err
	}
	if pending > 0 {
		return model.XUIClientConfigSave{}, ErrClientConfigPending
	}
	var renewalJSON string
	if err := tx.QueryRow(`SELECT renewal_config_json FROM agents WHERE agent_id = ?`, agentID).Scan(&renewalJSON); err != nil {
		return model.XUIClientConfigSave{}, fmt.Errorf("load billing agent: %w", err)
	}
	var renewal model.VPSRenewalConfig
	if renewalJSON != "" {
		if err := json.Unmarshal([]byte(renewalJSON), &renewal); err != nil {
			return model.XUIClientConfigSave{}, err
		}
	}
	// Preserve all other settings; normalize just the client billing records.
	renewal.ClientBillings = normalizeClientBillings(renewal.ClientBillings)
	index := -1
	for i, b := range renewal.ClientBillings {
		if (req.ClientID != "" && b.ClientID == req.ClientID) || (b.InboundID == req.InboundID && b.InboundTag == req.InboundTag && b.Email == req.Email) {
			index = i
			break
		}
	}
	var before *model.XUIClientBillingConfig
	if index >= 0 {
		old := renewal.ClientBillings[index]
		before = &old
	}
	if (before == nil) != (req.PreviousBilling == nil) {
		return model.XUIClientConfigSave{}, ErrClientConfigConflict
	}
	if before != nil {
		previous := normalizeClientBillings([]model.XUIClientBillingConfig{*req.PreviousBilling})
		if len(previous) != 1 || previous[0] != *before {
			return model.XUIClientConfigSave{}, ErrClientConfigConflict
		}
	}
	billing := req.Billing
	billing.ClientID, billing.InboundID, billing.InboundTag, billing.Email = req.ClientID, req.InboundID, req.InboundTag, req.Email
	billing.ExpireCycle, billing.ExpireAutoRenew = billing.RevenueCycle, false
	// A fee-only edit must not silently change a legacy renewal flag.
	if before != nil && billing.StartTime == before.StartTime && billing.ExpireTime == before.ExpireTime && billing.RevenueCycle == before.RevenueCycle {
		billing.ExpireAutoRenew = before.ExpireAutoRenew
	}
	if index >= 0 {
		renewal.ClientBillings[index] = billing
	} else {
		renewal.ClientBillings = append(renewal.ClientBillings, billing)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE agents SET renewal_config_json = ?, updated_at = ? WHERE agent_id = ?`, mustJSON(renewal), now, agentID); err != nil {
		return model.XUIClientConfigSave{}, err
	}
	if _, err := tx.Exec(`INSERT INTO config_audit_logs (agent_id, actor, before_json, after_json, created_at) VALUES (?, ?, ?, ?, ?)`, agentID, actor.Username, mustJSON(map[string]any{"client_billing": before}), mustJSON(map[string]any{"client_billing": billing}), now); err != nil {
		return model.XUIClientConfigSave{}, err
	}
	var actionID int64
	if !req.Changes.Empty() {
		payload := map[string]any{"inbound_id": target.InboundID, "inbound_tag": target.InboundTag, "email": target.Email, "client_id": target.ClientID, "changes": req.Changes, "expected": req.Expected, "request_id": req.RequestID, "billing_agent_id": agentID}
		// Forwarding is resolved on the Server, never accepted from the browser.
		var targetExists int
		if err := tx.QueryRow(`SELECT 1 FROM agents WHERE agent_id = ?`, target.AgentID).Scan(&targetExists); err != nil {
			return model.XUIClientConfigSave{}, fmt.Errorf("load target agent: %w", err)
		}
		result, err := tx.Exec(`INSERT INTO xui_actions (agent_id, kind, status, created_by_role, created_by_account_id, created_by_username, payload_json, result_json, error, created_at, updated_at, claimed_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, '{}', '', ?, ?, '', '')`, target.AgentID, model.XUIActionUpdateClientConfig, model.XUIActionStatusPending, actor.Role, actor.AccountID, actor.Username, mustJSON(payload), now, now)
		if err != nil {
			return model.XUIClientConfigSave{}, err
		}
		actionID, err = result.LastInsertId()
		if err != nil {
			return model.XUIClientConfigSave{}, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO xui_client_config_saves (request_id, agent_id, client_key, actor_role, actor_id, request_json, billing_json, action_agent_id, action_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, req.RequestID, agentID, key, actor.Role, actor.AccountID, string(requestJSON), mustJSON(billing), target.AgentID, actionID, now); err != nil {
		return model.XUIClientConfigSave{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.XUIClientConfigSave{}, err
	}
	save, _, err := s.GetXUIClientConfigSave(agentID, req.RequestID)
	return save, err
}

func (s *SQLiteStore) GetXUIClientConfigSave(agentID, requestID string) (model.XUIClientConfigSave, bool, error) {
	return readClientConfigSave(s.db, agentID, requestID)
}

func readClientConfigSave(q queryer, agentID, requestID string) (model.XUIClientConfigSave, bool, error) {
	var save model.XUIClientConfigSave
	var requestJSON, billingJSON, actionAgent string
	var actionID int64
	err := q.QueryRow(`SELECT request_id, request_json, billing_json, action_agent_id, action_id FROM xui_client_config_saves WHERE agent_id = ? AND request_id = ?`, agentID, requestID).Scan(&save.RequestID, &requestJSON, &billingJSON, &actionAgent, &actionID)
	if err == sql.ErrNoRows {
		return save, false, nil
	}
	if err != nil {
		return save, false, err
	}
	var req model.XUIClientConfigRequest
	if err := json.Unmarshal([]byte(requestJSON), &req); err != nil {
		return save, false, err
	}
	if err := json.Unmarshal([]byte(billingJSON), &save.Billing); err != nil {
		return save, false, err
	}
	save.ClientID, save.InboundID, save.InboundTag, save.Email, save.BillingSaved = req.ClientID, req.InboundID, req.InboundTag, req.Email, true
	if actionID > 0 {
		action, err := scanXUIAction(q.QueryRow(`SELECT id, agent_id, kind, status, created_by_role, created_by_account_id, created_by_username, payload_json, result_json, error, created_at, updated_at, claimed_at, completed_at FROM xui_actions WHERE agent_id = ? AND id = ?`, actionAgent, actionID))
		if err == sql.ErrNoRows {
			action = model.XUIAction{ID: actionID, AgentID: actionAgent, Kind: model.XUIActionUpdateClientConfig, Status: model.XUIActionStatusFailed, Error: "目标 Client 或执行记录已删除"}
		} else if err != nil {
			return save, false, err
		}
		save.Action = &action
	}
	return save, true, nil
}

func (s *SQLiteStore) LatestXUIClientConfigSaves(agentID string) ([]model.XUIClientConfigSave, error) {
	rows, err := s.db.Query(`SELECT request_id, action_agent_id FROM xui_client_config_saves WHERE agent_id = ? AND rowid IN (SELECT MAX(rowid) FROM xui_client_config_saves WHERE agent_id = ? GROUP BY client_key)`, agentID, agentID)
	if err != nil {
		return nil, err
	}
	var ids []string
	targets := map[string]bool{}
	for rows.Next() {
		var id string
		var target string
		if err := rows.Scan(&id, &target); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
		if target != "" {
			targets[target] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for target := range targets {
		if _, err := s.ExpireStaleXUIActions(target, XUIActionExecutionLease); err != nil {
			return nil, err
		}
	}
	saves := make([]model.XUIClientConfigSave, 0, len(ids))
	for _, id := range ids {
		save, found, err := s.GetXUIClientConfigSave(agentID, id)
		if err != nil {
			return nil, err
		}
		if found {
			saves = append(saves, save)
		}
	}
	return saves, nil
}
