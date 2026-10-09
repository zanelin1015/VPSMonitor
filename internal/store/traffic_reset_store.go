package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"bridge-core/internal/model"
)

func TrafficResetID(parts ...any) string {
	data, _ := json.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (s *SQLiteStore) SaveTrafficResetPolicy(policy model.TrafficResetPolicy) (model.TrafficResetPolicy, error) {
	if policy.AgentID == "" || policy.ClientID == "" || policy.Email == "" || policy.InboundID <= 0 {
		return policy, fmt.Errorf("agent, inbound and stable client identity are required")
	}
	if _, err := model.TrafficCycleMonths(policy.Cycle); err != nil {
		return policy, err
	}
	if policy.StartTime <= 0 {
		return policy, fmt.Errorf("start_time is required")
	}
	policy.ID = TrafficResetID(policy.AgentID, policy.ClientID)
	policy.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(policy)
	if err != nil {
		return policy, err
	}
	_, err = s.db.Exec(`INSERT INTO traffic_reset_policies(id,agent_id,data_json) VALUES(?,?,?)
	 ON CONFLICT(id) DO UPDATE SET data_json=excluded.data_json`, policy.ID, policy.AgentID, string(raw))
	return policy, err
}

func (s *SQLiteStore) ListTrafficResetPolicies() ([]model.TrafficResetPolicy, error) {
	rows, err := s.db.Query(`SELECT data_json FROM traffic_reset_policies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.TrafficResetPolicy{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value model.TrafficResetPolicy
		if err = json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) GetTrafficResetPolicy(id string) (model.TrafficResetPolicy, error) {
	var raw string
	var value model.TrafficResetPolicy
	err := s.db.QueryRow(`SELECT data_json FROM traffic_reset_policies WHERE id=?`, id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &value)
	}
	return value, err
}

func (s *SQLiteStore) PrepareTrafficResetJob(job model.TrafficResetJob) error {
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO traffic_reset_jobs(id,agent_id,status,data_json,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, job.ID, job.AgentID, job.Status, string(raw), job.UpdatedAt)
	return err
}

func (s *SQLiteStore) ListTrafficResetJobs(limit int) ([]model.TrafficResetJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT data_json FROM traffic_reset_jobs ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.TrafficResetJob{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value model.TrafficResetJob
		if err = json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) GetTrafficResetJob(id string) (model.TrafficResetJob, error) {
	var raw string
	var value model.TrafficResetJob
	err := s.db.QueryRow(`SELECT data_json FROM traffic_reset_jobs WHERE id=?`, id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &value)
	}
	return value, err
}

func (s *SQLiteStore) UpdateTrafficResetJob(job model.TrafficResetJob, expected string) error {
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE traffic_reset_jobs SET status=?,data_json=?,updated_at=? WHERE id=? AND status=?`, job.Status, string(raw), job.UpdatedAt, job.ID, expected)
	return err
}

// Commit the final job state and immutable audit entry together. A repeated
// scheduler sweep/receipt cannot append duplicates or overwrite earlier logs.
func (s *SQLiteStore) FinishTrafficResetJob(job model.TrafficResetJob, expected string, entry model.TrafficResetLog) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	job.UpdatedAt = now
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE traffic_reset_jobs SET status=?,data_json=?,updated_at=? WHERE id=? AND status=?`, job.Status, string(raw), now, job.ID, expected)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	entry.RecordedAt = now
	raw, err = json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO traffic_reset_logs(job_id,action_id,operation,data_json,recorded_at) VALUES(?,?,?,?,?) ON CONFLICT(job_id,action_id,operation) DO NOTHING`, job.ID, entry.ActionID, entry.Operation, string(raw), now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListTrafficResetLogs(limit int) ([]model.TrafficResetLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id,data_json FROM traffic_reset_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []model.TrafficResetLog{}
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var entry model.TrafficResetLog
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, err
		}
		entry.ID = id
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// Creating the executable action and attaching it to the period is atomic.
func (s *SQLiteStore) QueueTrafficResetJob(job model.TrafficResetJob, payload map[string]any) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow(`SELECT status FROM traffic_reset_jobs WHERE id=?`, job.ID).Scan(&status); err != nil {
		return err
	}
	if status != "prepared" && !(status == "needs_enable" && payload["resume_only"] == true) {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`INSERT INTO xui_actions(agent_id,kind,status,created_by_role,created_by_account_id,created_by_username,payload_json,result_json,error,created_at,updated_at,claimed_at,completed_at)
	 VALUES(?,?,?,'system',0,'traffic-reset',?,'{}','',?,?,'','')`, job.AgentID, model.XUIActionResetClientTraffic, model.XUIActionStatusPending, string(rawPayload), now, now)
	if err != nil {
		return err
	}
	job.ActionID, err = res.LastInsertId()
	if err != nil {
		return err
	}
	job.Status = "dispatched"
	job.UpdatedAt = now
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	res, err = tx.Exec(`UPDATE traffic_reset_jobs SET status=?,action_id=?,data_json=?,updated_at=? WHERE id=? AND status=?`, job.Status, job.ActionID, string(raw), now, job.ID, status)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("traffic reset period already claimed")
	}
	return tx.Commit()
}

func (s *SQLiteStore) CancelPendingTrafficResetAction(job model.TrafficResetJob, reason string) error {
	if job.ActionID == 0 {
		return nil
	}
	_, err := s.db.Exec(`UPDATE xui_actions SET status=?,error=?,completed_at=?,updated_at=? WHERE id=? AND agent_id=? AND status=?`, model.XUIActionStatusFailed, reason, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), job.ActionID, job.AgentID, model.XUIActionStatusPending)
	return err
}

func (s *SQLiteStore) ListActiveTrafficResetJobs() ([]model.TrafficResetJob, error) {
	rows, err := s.db.Query(`SELECT data_json FROM traffic_reset_jobs WHERE status IN ('prepared','dispatched') ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []model.TrafficResetJob{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var job model.TrafficResetJob
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
