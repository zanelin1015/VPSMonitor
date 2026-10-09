package store

import (
	"encoding/json"
	"fmt"
	"time"

	"bridge-core/internal/model"
)

// The dispatch decision and executable task are committed together. No task
// can run without an audit record, and a log never claims an uncommitted task.
func (s *SQLiteStore) RecordClientUpdate(log model.ClientUpdateLog, payload map[string]any) (model.ClientUpdateLog, model.XUIAction, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return log, model.XUIAction{}, err
	}
	defer tx.Rollback()
	log.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	var action model.XUIAction
	if log.Decision == "dispatched" {
		var pending int
		if err = tx.QueryRow(`SELECT count(*) FROM xui_actions WHERE agent_id=? AND kind=? AND status IN ('pending','running')`, log.AgentID, model.XUIActionUpdateClient).Scan(&pending); err != nil {
			return log, action, err
		}
		if pending > 0 {
			log.Decision, log.ReasonCode, log.Reason = "skipped", "already_pending", "已有待执行或执行中的 Client 升级任务，避免重复下发"
		} else {
			if payload == nil {
				return log, action, fmt.Errorf("update payload required")
			}
			raw, e := json.Marshal(payload)
			if e != nil {
				return log, action, e
			}
			res, e := tx.Exec(`INSERT INTO xui_actions(agent_id,kind,status,created_by_role,created_by_account_id,created_by_username,payload_json,result_json,error,created_at,updated_at,claimed_at,completed_at)
			 VALUES(?,?,'pending','admin',1,'',?,'{}','',?,?,'','')`, log.AgentID, model.XUIActionUpdateClient, string(raw), log.CreatedAt, log.CreatedAt)
			if e != nil {
				return log, action, e
			}
			log.ActionID, err = res.LastInsertId()
			if err != nil {
				return log, action, err
			}
			log.TaskStatus = model.XUIActionStatusPending
			now, _ := time.Parse(time.RFC3339Nano, log.CreatedAt)
			action = model.XUIAction{ID: log.ActionID, AgentID: log.AgentID, Kind: model.XUIActionUpdateClient, Status: model.XUIActionStatusPending, Payload: payload, CreatedAt: now, UpdatedAt: now}
		}
	}
	raw, err := json.Marshal(log)
	if err != nil {
		return log, action, err
	}
	res, err := tx.Exec(`INSERT INTO client_update_logs(action_id,data_json,task_status) VALUES(?,?,?)`, log.ActionID, string(raw), log.TaskStatus)
	if err != nil {
		return log, action, err
	}
	log.ID, err = res.LastInsertId()
	if err != nil {
		return log, action, err
	}
	if err = tx.Commit(); err != nil {
		return log, action, err
	}
	return log, action, nil
}

func (s *SQLiteStore) ListClientUpdateLogs(limit int) ([]model.ClientUpdateLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id,data_json,task_status,task_error,claimed_at,completed_at,confirmed_at FROM client_update_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []model.ClientUpdateLog{}
	for rows.Next() {
		var log model.ClientUpdateLog
		var raw string
		if err = rows.Scan(&log.ID, &raw, &log.TaskStatus, &log.TaskError, &log.ClaimedAt, &log.CompletedAt, &log.ConfirmedAt); err != nil {
			return nil, err
		}
		var original model.ClientUpdateLog
		if err = json.Unmarshal([]byte(raw), &original); err != nil {
			return nil, err
		}
		original.ID, original.TaskStatus, original.TaskError = log.ID, log.TaskStatus, log.TaskError
		original.ClaimedAt, original.CompletedAt, original.ConfirmedAt = log.ClaimedAt, log.CompletedAt, log.ConfirmedAt
		logs = append(logs, original)
	}
	return logs, rows.Err()
}

func (s *SQLiteStore) ConfirmClientUpdateLog(id int64, reportedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE client_update_logs SET confirmed_at=? WHERE id=? AND task_status='succeeded' AND confirmed_at=''`, reportedAt.UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *SQLiteStore) ExpireStaleClientUpdateActions(agentID string) error {
	now := time.Now().UTC()
	cutoff := now.Add(-XUIActionExecutionLease).Format(time.RFC3339Nano)
	_, err := s.db.Exec(`UPDATE xui_actions SET status='failed',error='execution lease expired; retry manually if appropriate',updated_at=?,completed_at=?
	 WHERE agent_id=? AND kind=? AND status='running'
	 AND ((claimed_at<>'' AND claimed_at<?) OR (claimed_at='' AND updated_at<?))`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), agentID, model.XUIActionUpdateClient, cutoff, cutoff)
	return err
}
