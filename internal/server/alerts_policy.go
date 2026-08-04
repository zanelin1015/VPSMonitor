package server

import (
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func buildAgentAlerts(agent model.AgentRecord, snapshot model.AgentSnapshot, includeOffline bool) []alertMessage {
	alerts := make([]alertMessage, 0)
	now := time.Now().UTC()
	summary := agent.Summary
	if snapshot.AgentID != "" {
		summary = snapshot.Summary
	}
	if includeOffline && agent.LastSeenAt != nil && now.Sub(agent.LastSeenAt.UTC()) > agentOfflineAfter {
		detail := fmt.Sprintf("最后上报：%s，已超过 %s 未上报。", agent.LastSeenAt.UTC().Format(time.RFC3339), agentOfflineAfter)
		alerts = append(alerts, newAgentAlert(agent, "offline", "critical", "Client 离线", "offline", detail))
	} else if includeOffline {
		alerts = append(alerts, newResolvedAlert(agent.AgentID, "offline"))
	}

	xuiError := strings.TrimSpace(summary.LastCollectionErr)
	if xuiError == "" && snapshot.XUI != nil {
		xuiError = strings.TrimSpace(snapshot.XUI.Error)
	}
	if xuiError != "" {
		detail := xuiError + "\n日志：VPSMonitor 后台 → 选择 Client → 日志"
		alerts = append(alerts, newAgentAlert(agent, "xui_error", "warning", "X-UI 采集异常", xuiError, detail))
	} else {
		alerts = append(alerts, newResolvedAlert(agent.AgentID, "xui_error"))
	}

	xrayState := strings.TrimSpace(strings.ToLower(summary.XrayState))
	if xrayState != "" && xrayState != "running" {
		alerts = append(alerts, newAgentAlert(agent, "xray_state", "critical", "Xray 状态异常", xrayState, "当前状态："+summary.XrayState))
	} else {
		alerts = append(alerts, newResolvedAlert(agent.AgentID, "xray_state"))
	}

	if renewalAlert := buildRenewalAlert(agent); renewalAlert.key != "" {
		alerts = append(alerts, renewalAlert)
	} else {
		alerts = append(alerts, newResolvedAlert(agent.AgentID, "renewal"))
	}

	if trafficAlert := buildTrafficAlert(agent, summary); trafficAlert.key != "" {
		alerts = append(alerts, trafficAlert)
	} else {
		alerts = append(alerts, newResolvedAlert(agent.AgentID, "traffic"))
	}

	return alerts
}

func newAgentAlert(agent model.AgentRecord, kind, severity, title, fingerprint, detail string) alertMessage {
	return alertMessage{
		key:         resolveAgentAlertKey(agent.AgentID, kind),
		fingerprint: fingerprint,
		title:       title,
		severity:    severity,
		detail:      detail,
		agent:       agent,
	}
}

func newResolvedAlert(agentID, kind string) alertMessage {
	return alertMessage{key: resolveAgentAlertKey(agentID, kind) + ":resolved"}
}

func resolveAgentAlertKey(agentID, kind string) string {
	return fmt.Sprintf("agent:%s:%s", agentID, kind)
}

func buildRenewalAlert(agent model.AgentRecord) alertMessage {
	cfg := normalizeAlertRenewal(agent.Config.Renewal)
	if !cfg.Enabled || (cfg.StartDate == "" && cfg.ExpireDate == "") {
		return alertMessage{}
	}
	period, ok := renewalPeriod(cfg, time.Now())
	if !ok {
		return alertMessage{}
	}
	remaining := daysBetween(time.Now(), period.end)
	if remaining > 7 {
		return alertMessage{}
	}
	severity := "warning"
	title := "VPS 周期即将到期"
	fingerprint := "renewal-warning"
	if remaining <= 3 {
		severity = "critical"
		fingerprint = "renewal-critical"
	}
	if remaining < 0 {
		title = "VPS 周期已过期"
		fingerprint = "renewal-expired"
	}
	detail := fmt.Sprintf("到期日：%s，剩余 %d 天，周期：%s，自动刷新：%t。", period.end.Format("2006-01-02"), remaining, cfg.Cycle, cfg.AutoRenew)
	return newAgentAlert(agent, "renewal", severity, title, fingerprint, detail)
}

func buildTrafficAlert(agent model.AgentRecord, summary model.VPSSummary) alertMessage {
	cfg := normalizeAlertRenewal(agent.Config.Renewal)
	if cfg.TrafficLimitBytes == 0 {
		return alertMessage{}
	}
	currentTotal := summary.NetTrafficTotal
	if currentTotal == 0 {
		currentTotal = summary.NetTrafficSent + summary.NetTrafficRecv
	}
	uploadUsed := periodTrafficUsed(summary.NetTrafficSent, cfg.TrafficSentBaselineBytes)
	downloadUsed := periodTrafficUsed(summary.NetTrafficRecv, cfg.TrafficRecvBaselineBytes)
	used := accountedTrafficUsed(cfg, periodTrafficUsed(currentTotal, cfg.TrafficBaselineBytes), uploadUsed, downloadUsed)
	percent := float64(used) / float64(cfg.TrafficLimitBytes) * 100
	if percent < 75 {
		return alertMessage{}
	}
	severity := "warning"
	fingerprint := "traffic-warning"
	title := "周期流量使用较高"
	if percent >= 90 {
		severity = "critical"
		fingerprint = "traffic-critical"
		title = "周期流量即将用尽"
	}
	detail := fmt.Sprintf("当前周期已用 %.1f%%（%s / %s），计算方式：%s，上传 %s，下载 %s。", percent, formatBytes(used), formatBytes(cfg.TrafficLimitBytes), trafficAccountingModeLabel(cfg), formatBytes(uploadUsed), formatBytes(downloadUsed))
	return newAgentAlert(agent, "traffic", severity, title, fingerprint, detail)
}

func formatTelegramAlert(alert alertMessage) string {
	name := alert.agent.AgentName
	if name == "" {
		name = alert.agent.AgentID
	}
	tags := "未分组"
	if len(alert.agent.Tags) > 0 {
		tags = strings.Join(alert.agent.Tags, ", ")
	}
	icon := "⚠️"
	if alert.severity == "critical" {
		icon = "🚨"
	}
	return fmt.Sprintf("%s ZaneLin 告警\nClient：%s (%s)\n标签：%s\n级别：%s\n类型：%s\n详情：%s\n时间：%s", icon, name, alert.agent.AgentID, tags, alert.severity, alert.title, alert.detail, time.Now().Format("2006-01-02 15:04:05"))
}

func normalizeAlertRenewal(cfg model.VPSRenewalConfig) model.VPSRenewalConfig {
	cfg.StartDate = strings.TrimSpace(cfg.StartDate)
	cfg.ExpireDate = strings.TrimSpace(cfg.ExpireDate)
	switch strings.ToLower(strings.TrimSpace(cfg.Cycle)) {
	case "week", "weekly":
		cfg.Cycle = "week"
	case "month", "monthly":
		cfg.Cycle = "month"
	case "quarter", "quarterly", "season":
		cfg.Cycle = "quarter"
	case "semiannual", "halfyear", "half-year", "half_year", "half_yearly":
		cfg.Cycle = "semiannual"
	case "year", "yearly":
		cfg.Cycle = "year"
	default:
		cfg.Cycle = ""
	}
	return cfg
}

type renewalPeriodRange struct {
	start time.Time
	end   time.Time
}

func renewalPeriod(cfg model.VPSRenewalConfig, now time.Time) (renewalPeriodRange, bool) {
	now = startOfDay(now)
	if cfg.ExpireDate != "" {
		end, ok := parseDate(cfg.ExpireDate)
		if !ok {
			return renewalPeriodRange{}, false
		}
		if cfg.AutoRenew && cfg.Cycle != "" {
			for !end.After(now) {
				end = addRenewalCycle(end, cfg.Cycle)
			}
		}
		start := end
		if cfg.Cycle != "" {
			start = addRenewalCycle(end, "-"+cfg.Cycle)
		}
		return renewalPeriodRange{start: start, end: end}, true
	}
	start, ok := parseDate(cfg.StartDate)
	if !ok || cfg.Cycle == "" {
		return renewalPeriodRange{}, false
	}
	end := addRenewalCycle(start, cfg.Cycle)
	if cfg.AutoRenew {
		for !end.After(now) {
			start = end
			end = addRenewalCycle(start, cfg.Cycle)
		}
	}
	return renewalPeriodRange{start: start, end: end}, true
}

func parseDate(value string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return startOfDay(t), true
}

func startOfDay(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func addRenewalCycle(t time.Time, cycle string) time.Time {
	switch cycle {
	case "week":
		return t.AddDate(0, 0, 7)
	case "-week":
		return t.AddDate(0, 0, -7)
	case "quarter":
		return t.AddDate(0, 3, 0)
	case "-quarter":
		return t.AddDate(0, -3, 0)
	case "semiannual":
		return t.AddDate(0, 6, 0)
	case "-semiannual":
		return t.AddDate(0, -6, 0)
	case "year":
		return t.AddDate(1, 0, 0)
	case "-year":
		return t.AddDate(-1, 0, 0)
	case "-month":
		return t.AddDate(0, -1, 0)
	default:
		return t.AddDate(0, 1, 0)
	}
}

func daysBetween(start, end time.Time) int {
	duration := startOfDay(end).Sub(startOfDay(start))
	return int(duration.Hours() / 24)
}

func periodTrafficUsed(current, baseline uint64) uint64 {
	if current >= baseline {
		return current - baseline
	}
	return current
}

func accountedTrafficUsed(cfg model.VPSRenewalConfig, totalUsed, uploadUsed, downloadUsed uint64) uint64 {
	if normalizeTrafficAccountingMode(cfg) == "single_direction" {
		if uploadUsed == 0 && downloadUsed == 0 {
			return totalUsed
		}
		if uploadUsed >= downloadUsed {
			return uploadUsed
		}
		return downloadUsed
	}
	directionalTotal := uploadUsed + downloadUsed
	if directionalTotal > 0 {
		return directionalTotal
	}
	return totalUsed
}

func normalizeTrafficAccountingMode(cfg model.VPSRenewalConfig) string {
	if strings.EqualFold(strings.TrimSpace(cfg.TrafficAccountingMode), "single_direction") {
		return "single_direction"
	}
	return "bidirectional"
}

func trafficAccountingModeLabel(cfg model.VPSRenewalConfig) string {
	if normalizeTrafficAccountingMode(cfg) == "single_direction" {
		return "单向（取上传/下载较大值）"
	}
	return "双向（上传+下载）"
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(value)/float64(div), "KMGTPE"[exp])
}
