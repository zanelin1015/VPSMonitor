package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

type trafficResetCandidate struct {
	AgentID       string `json:"agent_id"`
	AgentName     string `json:"agent_name"`
	ClientID      string `json:"client_id"`
	ClientName    string `json:"client_name"`
	Email         string `json:"email"`
	InboundID     int    `json:"inbound_id"`
	StartTime     int64  `json:"start_time"`
	Cycle         string `json:"cycle"`
	ExpiryTime    int64  `json:"expiry_time"`
	TotalBytes    int64  `json:"total_bytes"`
	ConfigEnabled bool   `json:"config_enabled"`
	Supported     bool   `json:"supported"`
	Fresh         bool   `json:"fresh"`
	Protocol      string `json:"protocol"`
}

func (a *App) trafficResetCandidates(now time.Time) ([]trafficResetCandidate, error) {
	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return nil, err
	}
	byID := map[string]model.AgentRecord{}
	for _, agent := range agents {
		byID[agent.AgentID] = agent
	}
	result := []trafficResetCandidate{}
	for _, snapshot := range snapshots {
		agent := byID[snapshot.AgentID]
		overview := dashboard.BuildXUIOverview(snapshot)
		if overview == nil {
			continue
		}
		counts := map[string]int{}
		idCounts := map[string]int{}
		for _, client := range overview.Clients {
			counts[client.Email]++
			idCounts[model.TrafficResetClientIdentity(client.Protocol, client.ClientID, client.AuthPassword)]++
		}
		for _, client := range overview.Clients {
			identity := model.TrafficResetClientIdentity(client.Protocol, client.ClientID, client.AuthPassword)
			if identity == "" || client.Email == "" {
				continue
			}
			value := trafficResetCandidate{AgentID: agent.AgentID, AgentName: agent.AgentName, ClientID: identity, ClientName: client.Comment, Email: client.Email, InboundID: client.InboundID,
				ExpiryTime: client.ExpiryTime, TotalBytes: client.TotalGB, Protocol: client.Protocol, Cycle: "month",
				Supported: snapshot.Capabilities.TrafficReset && counts[client.Email] == 1 && idCounts[identity] == 1 && agent.Config.XUI.Enabled,
				Fresh:     !snapshot.ReportedAt.After(now.Add(time.Minute)) && now.Sub(snapshot.ReportedAt) <= 5*time.Minute}
			if client.ConfigEnabled != nil {
				value.ConfigEnabled = *client.ConfigEnabled
			}
			for _, billing := range agent.Config.Renewal.ClientBillings {
				if (billing.ClientID != "" && billing.ClientID == client.ClientID) || (billing.ClientID == "" && billing.InboundID == client.InboundID && billing.Email == client.Email) {
					value.StartTime = billing.StartTime
					break
				}
			}
			result = append(result, value)
		}
	}
	return result, nil
}

func resolveTrafficResetPolicy(policy model.TrafficResetPolicy, candidates []trafficResetCandidate) (model.TrafficResetPolicy, trafficResetCandidate, error) {
	for _, candidate := range candidates {
		if candidate.AgentID != policy.AgentID || candidate.ClientID != policy.ClientID {
			continue
		}
		if candidate.Email != policy.Email || candidate.InboundID != policy.InboundID {
			return policy, candidate, fmt.Errorf("客户端身份或所在入站已改变")
		}
		if policy.FollowBilling {
			policy.StartTime = candidate.StartTime
		}
		// Billing can be monthly, quarterly, semiannual, or annual, but traffic
		// quota resets always run monthly on the billing start day.
		policy.Cycle = "month"
		if policy.StartTime <= 0 {
			return policy, candidate, fmt.Errorf("请先设置客户端开始日期")
		}
		start := time.UnixMilli(policy.StartTime).In(model.TrafficResetLocation)
		if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 || start.Nanosecond() != 0 {
			return policy, candidate, fmt.Errorf("重置基准必须为北京时间 00:00:00")
		}
		if _, err := model.TrafficCycleMonths(policy.Cycle); err != nil {
			return policy, candidate, err
		}
		return policy, candidate, nil
	}
	return policy, trafficResetCandidate{}, fmt.Errorf("最新上报中未找到实际客户端")
}

func trafficResetSignature(policy model.TrafficResetPolicy, candidate trafficResetCandidate) string {
	return store.TrafficResetID(policy.UpdatedAt, policy.StartTime, policy.Cycle, candidate.AgentID, candidate.ClientID, candidate.InboundID, candidate.Email, candidate.ExpiryTime, candidate.TotalBytes, candidate.ConfigEnabled, candidate.Protocol)
}

func plannedTrafficResetJobs(policy model.TrafficResetPolicy, candidate trafficResetCandidate, settings model.TrafficResetSettings, now time.Time, preview bool) []model.TrafficResetJob {
	last, next, err := model.TrafficResetBoundaries(policy.StartTime, policy.Cycle, now)
	if err != nil {
		return nil
	}
	result := []model.TrafficResetJob{}
	for _, boundary := range []time.Time{last, next} {
		if boundary.IsZero() {
			continue
		}
		runAt := boundary.Add(time.Minute)
		deadline := runAt.Add(time.Duration(settings.CatchUpMinutes) * time.Minute)
		if !preview && (now.Before(boundary.Add(-5*time.Minute)) || now.After(deadline)) {
			continue
		}
		if preview && boundary.Before(now.Add(-time.Duration(settings.CatchUpMinutes)*time.Minute)) {
			continue
		}
		result = append(result, model.TrafficResetJob{ID: store.TrafficResetID(policy.AgentID, policy.ClientID, boundary.UnixMilli()), PolicyID: policy.ID, AgentID: policy.AgentID, AgentName: candidate.AgentName, ClientName: candidate.ClientName, ClientID: policy.ClientID, Email: policy.Email, InboundID: policy.InboundID, Boundary: boundary.UnixMilli(), RunAt: runAt.UnixMilli(), Deadline: deadline.UnixMilli(), Signature: trafficResetSignature(policy, candidate), Status: "prepared"})
	}
	return result
}

func (a *App) runTrafficResetSweep(now time.Time) error {
	a.trafficResetMu.Lock()
	defer a.trafficResetMu.Unlock()
	settings, _, err := a.store.GetScheduledTaskSettings()
	if err != nil {
		return err
	}
	candidates, err := a.trafficResetCandidates(now)
	if err != nil {
		return err
	}
	policies, err := a.store.ListTrafficResetPolicies()
	if err != nil {
		return err
	}
	if settings.TrafficReset.Enabled {
		for _, policy := range policies {
			if !policy.Enabled {
				continue
			}
			resolved, candidate, e := resolveTrafficResetPolicy(policy, candidates)
			if e != nil {
				continue
			}
			for _, job := range plannedTrafficResetJobs(resolved, candidate, settings.TrafficReset, now, false) {
				if err = a.store.PrepareTrafficResetJob(job); err != nil {
					return err
				}
			}
		}
	}
	jobs, err := a.store.ListActiveTrafficResetJobs()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Status != "prepared" && job.Status != "dispatched" {
			continue
		}
		if job.Status == "dispatched" {
			if _, err := a.store.ExpireStaleXUIActions(job.AgentID, 0); err != nil {
				return err
			}
			action, found, e := a.store.GetXUIAction(job.AgentID, job.ActionID)
			if e != nil {
				return e
			}
			if !found {
				continue
			}
			if action.Status == model.XUIActionStatusSucceeded || action.Status == model.XUIActionStatusFailed {
				job.Result = action.Result
				entry := trafficResetActionLog(job, action)
				job.Message = entry.Message
				job.Status = "succeeded"
				if action.Status == model.XUIActionStatusFailed {
					job.Status = "failed"
					if entry.Outcome == "uncertain" {
						job.Status = "uncertain"
					}
					if action.Result["reset_done"] == true {
						job.Status = "needs_enable"
					}
				}
				if err = a.store.FinishTrafficResetJob(job, "dispatched", entry); err != nil {
					return err
				}
				a.clearCustomerOverviewCache()
				continue
			}
			if action.Status == model.XUIActionStatusRunning {
				continue
			}
		}
		policy, e := a.store.GetTrafficResetPolicy(job.PolicyID)
		resolved, candidate, resolveErr := resolveTrafficResetPolicy(policy, candidates)
		reason := ""
		switch {
		case !settings.TrafficReset.Enabled:
			reason = "全局流量重置已关闭"
		case e != nil || !policy.Enabled:
			reason = "该客户端自动重置已关闭"
		case resolveErr != nil:
			reason = "客户端不存在或重置基准无效"
		case trafficResetSignature(resolved, candidate) != job.Signature:
			reason = "计划生成后客户端设置已改变，请核对新计划"
		case now.UnixMilli() > job.Deadline:
			reason = "已超出补执行窗口，未清零"
		case !candidate.ConfigEnabled:
			reason = "客户端配置已禁用，不自动恢复"
		}
		if reason != "" {
			old := job.Status
			job.Status = "skipped"
			job.Message = reason
			if err = a.store.CancelPendingTrafficResetAction(job, reason); err != nil {
				return err
			}
			entry := trafficResetLogBase(job)
			entry.Operation = "skip"
			entry.Outcome = "skipped"
			entry.Message = reason
			entry.CompletedAt = now.UTC().Format(time.RFC3339Nano)
			entry.TimeSource = "not_executed"
			entry.CompletedTimeSource = "server"
			if err = a.store.FinishTrafficResetJob(job, old, entry); err != nil {
				return err
			}
			continue
		}
		if job.Status != "prepared" || now.UnixMilli() < job.RunAt {
			continue
		}
		if !candidate.Supported || !candidate.Fresh {
			continue
		}
		payload := map[string]any{"job_id": job.ID, "inbound_id": job.InboundID, "client_id": job.ClientID, "email": job.Email, "not_before": job.RunAt, "deadline": job.Deadline, "expected_expiry": candidate.ExpiryTime, "expected_total": candidate.TotalBytes, "expected_protocol": candidate.Protocol}
		if err = a.store.QueueTrafficResetJob(job, payload); err != nil {
			return err
		}
	}
	return nil
}

func trafficResetLogBase(job model.TrafficResetJob) model.TrafficResetLog {
	entry := model.TrafficResetLog{JobID: job.ID, ActionID: job.ActionID, AgentID: job.AgentID, AgentName: job.AgentName, ClientID: job.ClientID, ClientName: job.ClientName, Email: job.Email, InboundID: job.InboundID, PlannedAt: job.RunAt, Operation: "reset"}
	if entry.AgentName == "" {
		entry.AgentName = job.AgentID
	}
	if entry.ClientName == "" {
		entry.ClientName = job.Email
	}
	return entry
}

func trafficResetActionLog(job model.TrafficResetJob, action model.XUIAction) model.TrafficResetLog {
	entry := trafficResetLogBase(job)
	if action.Payload["resume_only"] == true {
		entry.Operation = "verify"
	}
	entry.StartedAt = trafficResetResultTime(action.Result, "execution_started_at")
	entry.CompletedAt = trafficResetResultTime(action.Result, "execution_finished_at")
	entry.TimeSource = "unknown"
	entry.CompletedTimeSource = "unknown"
	if entry.StartedAt != "" {
		entry.TimeSource = "client"
	}
	if entry.CompletedAt != "" {
		entry.CompletedTimeSource = "client"
	}
	// Claim/completion timestamps are explicitly fallback times for older
	// receipts; never substitute the scheduled 00:01 for an actual event.
	if entry.StartedAt == "" && action.ClaimedAt != nil {
		entry.StartedAt = action.ClaimedAt.UTC().Format(time.RFC3339Nano)
		entry.TimeSource = "server_claim"
	}
	if entry.CompletedAt == "" && action.CompletedAt != nil {
		entry.CompletedAt = action.CompletedAt.UTC().Format(time.RFC3339Nano)
		entry.CompletedTimeSource = "server"
	}
	entry.Outcome = "succeeded"
	entry.Message = action.Error
	if action.Status == model.XUIActionStatusFailed {
		entry.Outcome = "failed"
		if action.Result["uncertain"] == true || strings.Contains(action.Error, "execution lease expired") {
			entry.Outcome = "uncertain"
		}
		if action.Result["reset_done"] == true {
			entry.Outcome = "unverified"
		}
	}
	if entry.Message == "" {
		entry.Message, _ = action.Result["message"].(string)
	}
	if entry.Message == "" {
		entry.Message = map[string]string{"succeeded": "执行成功", "failed": "执行失败", "uncertain": "结果无法确认，请人工核查", "unverified": "清零接口已确认，但核验未通过"}[entry.Outcome]
	}
	return entry
}

func trafficResetResultTime(result map[string]any, key string) string {
	value, _ := result[key].(string)
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

func (a *App) startTrafficResetScheduler() {
	if a.demoDataSource != nil {
		return
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := a.runTrafficResetSweep(time.Now()); err != nil {
				log.Printf("traffic reset scheduler: %v", err)
			}
		}
	}()
}

func (a *App) handleAdminTrafficReset(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		now := time.Now()
		candidates, err := a.trafficResetCandidates(now)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		policies, err := a.store.ListTrafficResetPolicies()
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		// Older records may retain the former billing-cycle value. Present the
		// effective traffic-reset cadence, which is always monthly.
		for i := range policies {
			policies[i].Cycle = "month"
		}
		jobs, err := a.store.ListTrafficResetJobs(100)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		logs, err := a.store.ListTrafficResetLogs(100)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		settings, _, err := a.store.GetScheduledTaskSettings()
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		preview := []model.TrafficResetJob{}
		for _, policy := range policies {
			resolved, c, e := resolveTrafficResetPolicy(policy, candidates)
			if e == nil && policy.Enabled {
				for _, job := range plannedTrafficResetJobs(resolved, c, settings.TrafficReset, now, true) {
					existing, err := a.store.GetTrafficResetJob(job.ID)
					if err == nil && existing.Status != "prepared" && existing.Status != "dispatched" {
						continue
					}
					preview = append(preview, job)
				}
			}
		}
		writeJSON(w, 200, map[string]any{"policies": policies, "jobs": jobs, "logs": logs, "preview": preview, "clients": candidates})
	case http.MethodPut:
		var policy model.TrafficResetPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		candidates, err := a.trafficResetCandidates(time.Now())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		if !policy.Enabled {
			previous, e := a.store.GetTrafficResetPolicy(store.TrafficResetID(policy.AgentID, policy.ClientID))
			if e == nil {
				previous.Enabled = false
				saved, e := a.store.SaveTrafficResetPolicy(previous)
				if e != nil {
					writeError(w, 500, e.Error())
					return
				}
				writeJSON(w, 200, saved)
				return
			}
		}
		resolved, _, err := resolveTrafficResetPolicy(policy, candidates)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		policy.StartTime = resolved.StartTime
		policy.Cycle = resolved.Cycle
		saved, err := a.store.SaveTrafficResetPolicy(policy)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, saved)
	case http.MethodPost:
		if r.URL.Query().Get("verify_job") != "" {
			a.trafficResetMu.Lock()
			defer a.trafficResetMu.Unlock()
			job, err := a.store.GetTrafficResetJob(r.URL.Query().Get("verify_job"))
			if err != nil || job.Status != "needs_enable" {
				writeError(w, 409, "仅允许核验已确认重置的任务，不会再次清零")
				return
			}
			now := time.Now()
			settings, _, err := a.store.GetScheduledTaskSettings()
			if err != nil || !settings.TrafficReset.Enabled || now.UnixMilli() > job.Deadline {
				writeError(w, 409, "任务已关闭或超过执行窗口，请人工核查")
				return
			}
			policy, err := a.store.GetTrafficResetPolicy(job.PolicyID)
			if err != nil || !policy.Enabled {
				writeError(w, 409, "重置策略已关闭")
				return
			}
			candidates, err := a.trafficResetCandidates(now)
			if err != nil {
				writeError(w, 500, err.Error())
				return
			}
			resolved, c, err := resolveTrafficResetPolicy(policy, candidates)
			if err != nil || trafficResetSignature(resolved, c) != job.Signature || !c.Supported || !c.Fresh {
				writeError(w, 409, "配置发生变化或 Client 不在线，请人工核查")
				return
			}
			payload := map[string]any{"job_id": job.ID, "inbound_id": job.InboundID, "client_id": job.ClientID, "email": job.Email, "not_before": job.RunAt, "deadline": job.Deadline, "expected_expiry": c.ExpiryTime, "expected_total": c.TotalBytes, "expected_protocol": c.Protocol, "resume_only": true}
			if err := a.store.QueueTrafficResetJob(job, payload); err != nil {
				writeError(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"message": "已下发只读核验，不会再次重置流量"})
			return
		}
		// Scan only. Never move the execution window forward for a button click.
		if err := a.runTrafficResetSweep(time.Now()); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"message": "扫描完成；未到执行时间的任务不会下发"})
	default:
		writeError(w, 405, "method not allowed")
	}
}

func (a *App) handleTrafficResetValidation(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	if !a.isAuthorized(agentID, r.Header.Get("X-Agent-Token")) {
		writeError(w, 401, "invalid agent token")
		return
	}
	actionID, err := strconv.ParseInt(r.URL.Query().Get("action_id"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid action id")
		return
	}
	action, found, err := a.store.GetXUIAction(agentID, actionID)
	if err != nil || !found || action.Kind != model.XUIActionResetClientTraffic || action.Status != model.XUIActionStatusRunning {
		writeError(w, 409, "reset action is not executable")
		return
	}
	jobID, _ := action.Payload["job_id"].(string)
	job, err := a.store.GetTrafficResetJob(jobID)
	if err != nil || job.AgentID != agentID || job.ActionID != actionID || job.Status != "dispatched" {
		writeError(w, 409, "reset period does not match action")
		return
	}
	settings, _, err := a.store.GetScheduledTaskSettings()
	if err != nil || !settings.TrafficReset.Enabled {
		writeError(w, 409, "traffic reset is disabled")
		return
	}
	now := time.Now()
	if now.UnixMilli() < job.RunAt || now.UnixMilli() > job.Deadline {
		writeError(w, 409, "outside traffic reset execution window")
		return
	}
	policy, err := a.store.GetTrafficResetPolicy(job.PolicyID)
	if err != nil || !policy.Enabled {
		writeError(w, 409, "traffic reset policy is disabled")
		return
	}
	candidates, err := a.trafficResetCandidates(now)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	resolved, c, err := resolveTrafficResetPolicy(policy, candidates)
	if err != nil || trafficResetSignature(resolved, c) != job.Signature || !c.Supported || !c.ConfigEnabled {
		writeError(w, 409, "client or policy changed since planning")
		return
	}
	writeJSON(w, 200, map[string]bool{"allowed": true})
}
