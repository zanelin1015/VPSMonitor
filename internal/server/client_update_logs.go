package server

import (
	"time"

	"bridge-core/internal/model"
)

// Keep normal preview and non-force dispatch eligibility identical; force mode
// only relaxes the equal-version check after all safety checks still pass.
func clientUpdateEligibility(agent model.AgentRecord, snapshot model.AgentSnapshot, latest *model.UpdateLatestInfo, prefix string, force bool) (model.UpdateAgentStatus, string) {
	status := buildUpdateAgentStatus(agent, latest.LatestClientVersion, prefix, latest.ClientAssets)
	rawReason := status.Reason
	code := "eligible"
	switch rawReason {
	case "client has not reported os/arch yet":
		code, status.Reason = "unknown_platform", "Client 尚未上报系统或架构"
	case "unsupported client platform":
		code, status.Reason = "unsupported_platform", "不支持该 Client 系统或架构的在线升级"
	case "release asset not found":
		code, status.Reason = "missing_package", "Release 缺少该系统/架构的 Client 安装包"
	case "client is already up to date":
		if _, ok := parseSemver(agent.Version); !ok {
			code, status.Reason = "unknown_version", "Client 尚未上报有效版本号，无法安全判断升级"
		} else if force && normalizeVersion(agent.Version) == normalizeVersion(latest.LatestClientVersion) {
			status.UpdateAvailable = true
			code, status.Reason = "forced", "强制升级：当前版本已是目标版本，仍重新下发升级任务"
		} else if force && isVersionNewer(agent.Version, latest.LatestClientVersion) {
			code, status.Reason = "newer_version", "当前 Client 版本高于目标版本，不执行降级"
		} else {
			code, status.Reason = "up_to_date", "当前版本已达到或高于目标版本，无需升级"
		}
	}
	if !status.UpdateAvailable {
		return status, code
	}
	if !supportsVerifiedAutomaticUpdate(snapshot) {
		status.UpdateAvailable = false
		status.Reason = "旧 Client 未上报安全在线升级能力，需先手动升级一次"
		return status, "manual_upgrade_required"
	}
	if _, err := requiredReleaseAssetDigest(latest.ClientAssetDigests, status.PackageName); err != nil {
		status.UpdateAvailable = false
		status.Reason = "Release 安装包缺少有效 SHA-256 校验值，拒绝不安全升级"
		return status, "missing_digest"
	}
	if code == "forced" {
		status.Reason = "强制升级：当前版本已是目标版本，仍重新下发升级任务"
	} else {
		status.Reason = "可安全在线升级"
	}
	return status, code
}

func (a *App) clientUpdateLogs() ([]model.ClientUpdateLog, error) {
	logs, err := a.store.ListClientUpdateLogs(200)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, log := range logs {
		if log.Decision == "dispatched" && (log.TaskStatus == "running" || log.TaskStatus == "pending") && !seen[log.AgentID] {
			if err := a.store.ExpireStaleClientUpdateActions(log.AgentID); err != nil {
				return nil, err
			}
			seen[log.AgentID] = true
		}
	}
	logs, err = a.store.ListClientUpdateLogs(200)
	if err != nil {
		return nil, err
	}
	agents, _, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return nil, err
	}
	byID := map[string]model.AgentRecord{}
	for _, agent := range agents {
		byID[agent.AgentID] = agent
	}
	for i := range logs {
		log := &logs[i]
		if log.TaskStatus != model.XUIActionStatusSucceeded || log.ConfirmedAt != "" {
			continue
		}
		agent, found := byID[log.AgentID]
		completed, err := time.Parse(time.RFC3339Nano, log.CompletedAt)
		created, createdErr := time.Parse(time.RFC3339Nano, log.CreatedAt)
		if err != nil || createdErr != nil || !found || agent.RegisteredAt.After(created) || agent.ReportedAt == nil || !agent.ReportedAt.After(completed) || normalizeVersion(agent.Version) != normalizeVersion(log.TargetVersion) {
			continue
		}
		if err := a.store.ConfirmClientUpdateLog(log.ID, *agent.ReportedAt); err != nil {
			return nil, err
		}
		log.ConfirmedAt = agent.ReportedAt.UTC().Format(time.RFC3339Nano)
	}
	return logs, nil
}
