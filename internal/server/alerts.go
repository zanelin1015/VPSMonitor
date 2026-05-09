package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

const (
	alertCooldown          = 6 * time.Hour
	alertSweepInterval     = 5 * time.Minute
	agentOfflineAfter      = 5 * time.Minute
	telegramAPITimeout     = 8 * time.Second
	dailyTrafficReportHour = 9
)

type alertService struct {
	store *store.SQLiteStore
	http  *http.Client
}

type alertMessage struct {
	key         string
	fingerprint string
	title       string
	severity    string
	detail      string
	agent       model.AgentRecord
}

func newAlertService(s *store.SQLiteStore) *alertService {
	return &alertService{
		store: s,
		http:  &http.Client{Timeout: telegramAPITimeout},
	}
}

func (s *alertService) Start() {
	if s == nil {
		return
	}
	go s.runAlertSweep()
	go s.runDailyTrafficReports()
}

func (s *alertService) runAlertSweep() {
	ticker := time.NewTicker(alertSweepInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.EvaluateAll()
	}
}

func (s *alertService) runDailyTrafficReports() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), dailyTrafficReportHour, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		timer := time.NewTimer(time.Until(next))
		<-timer.C
		s.SendDailyTrafficReport(time.Now().AddDate(0, 0, -1))
	}
}

func (s *alertService) EvaluateAgent(agentID string) {
	if s == nil || agentID == "" {
		return
	}
	agent, found, err := s.store.GetAgent(agentID)
	if err != nil || !found {
		if err != nil {
			log.Printf("alert evaluate agent: %v", err)
		}
		return
	}
	snapshot, _ := s.store.GetLatest(agentID)
	s.evaluateAgentRecord(agent, snapshot, false)
}

func (s *alertService) EvaluateAll() {
	if s == nil {
		return
	}
	agents, err := s.store.ListAgents()
	if err != nil {
		log.Printf("alert list agents: %v", err)
		return
	}
	latest := make(map[string]model.AgentSnapshot)
	for _, snapshot := range s.store.ListLatest() {
		latest[snapshot.AgentID] = snapshot
	}
	for _, agent := range agents {
		snapshot := latest[agent.AgentID]
		s.evaluateAgentRecord(agent, snapshot, true)
		s.evaluateXUIClientExpiryRenewals(agent, snapshot)
	}
}

func (s *alertService) evaluateAgentRecord(agent model.AgentRecord, snapshot model.AgentSnapshot, includeOffline bool) {
	alerts := buildAgentAlerts(agent, snapshot, includeOffline)
	for _, alert := range alerts {
		s.dispatch(alert)
	}
}

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

func (s *alertService) dispatch(alert alertMessage) {
	if strings.HasSuffix(alert.key, ":resolved") {
		_ = s.store.ResolveAlert(strings.TrimSuffix(alert.key, ":resolved"))
		return
	}
	if alert.key == "" {
		return
	}
	text := formatTelegramAlert(alert)
	bots, err := s.store.ListEnabledTelegramBotSecrets()
	if err != nil {
		log.Printf("alert telegram bots: %v", err)
		return
	}
	if len(bots) == 0 {
		return
	}
	shouldSend, err := s.store.ShouldSendAlert(alert.key, alert.fingerprint, text, alertCooldown)
	if err != nil {
		log.Printf("alert state: %v", err)
		return
	}
	if !shouldSend {
		return
	}
	for _, bot := range bots {
		if err := s.sendTelegramMessage(bot.BotToken, bot.ChatID, text); err != nil {
			log.Printf("send telegram alert to %s: %v", bot.Name, err)
		}
	}
}

func (s *alertService) sendTelegramMessage(botToken, chatID, text string) error {
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot token and chat id are required")
	}
	endpoint := "https://api.telegram.org/bot" + botToken + "/sendMessage"
	body, _ := json.Marshal(map[string]string{
		"chat_id": chatID,
		"text":    text,
	})
	resp, err := s.http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("telegram api http %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func (s *alertService) SendDailyTrafficReport(day time.Time) {
	if s == nil {
		return
	}
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	items, err := s.store.ListDailyTrafficUsage(day)
	if err != nil {
		log.Printf("daily traffic report: %v", err)
		return
	}
	bots, err := s.store.ListEnabledTelegramBotSecrets()
	if err != nil {
		log.Printf("daily traffic telegram bots: %v", err)
		return
	}
	if len(bots) == 0 {
		return
	}
	text := formatDailyTrafficReport(day, items)
	key := "daily_traffic:" + day.Format("2006-01-02")
	shouldSend, err := s.store.ShouldSendAlert(key, key, text, 48*time.Hour)
	if err != nil {
		log.Printf("daily traffic alert state: %v", err)
		return
	}
	if !shouldSend {
		return
	}
	for _, bot := range bots {
		if err := s.sendTelegramMessage(bot.BotToken, bot.ChatID, text); err != nil {
			log.Printf("send daily traffic report to %s: %v", bot.Name, err)
		}
	}
}

func formatDailyTrafficReport(day time.Time, items []model.DailyTrafficUsage) string {
	var upload, download uint64
	for _, item := range items {
		upload += item.Upload
		download += item.Download
	}
	lines := []string{
		fmt.Sprintf("📊 NanFengMonitor 昨日流量日报（%s）", day.Format("2006-01-02")),
		fmt.Sprintf("Client 数：%d", len(items)),
		fmt.Sprintf("总流量：%s", formatBytes(upload+download)),
		fmt.Sprintf("总上传：%s", formatBytes(upload)),
		fmt.Sprintf("总下载：%s", formatBytes(download)),
		"前三名：",
	}
	if len(items) == 0 {
		lines = append(lines, "暂无昨日快照数据")
	} else {
		limit := 3
		if len(items) < limit {
			limit = len(items)
		}
		for index := 0; index < limit; index++ {
			item := items[index]
			name := item.AgentName
			if name == "" {
				name = item.AgentID
			}
			lines = append(lines, fmt.Sprintf("%d. %s：%s（上传 %s / 下载 %s）", index+1, name, formatBytes(item.Total), formatBytes(item.Upload), formatBytes(item.Download)))
		}
	}
	lines = append(lines, "发送时间："+time.Now().Format("2006-01-02 15:04:05"))
	return strings.Join(lines, "\n")
}

func (s *alertService) evaluateXUIClientExpiryRenewals(agent model.AgentRecord, snapshot model.AgentSnapshot) {
	if snapshot.XUI == nil || len(agent.Config.Renewal.ClientBillings) == 0 {
		return
	}
	overview := dashboard.BuildXUIOverview(snapshot)
	if overview == nil {
		return
	}
	now := time.Now()
	for _, billing := range agent.Config.Renewal.ClientBillings {
		if !billing.ExpireAutoRenew {
			continue
		}
		client := findOverviewClient(overview.Clients, billing)
		expiryMillis := billing.ExpireTime
		if expiryMillis <= 0 && client != nil {
			expiryMillis = client.ExpiryTime
		}
		if expiryMillis <= 0 {
			continue
		}
		expiry := time.UnixMilli(expiryMillis)
		if expiry.After(now) {
			continue
		}
		cycle := normalizeClientExpireCycle(billing.ExpireCycle)
		next := expiry
		for !next.After(now) {
			next = addClientExpireCycle(next, cycle)
		}
		key := fmt.Sprintf("xui_client_expiry:%s:%d:%s:%s", agent.AgentID, billing.InboundID, billing.InboundTag, billing.Email)
		fingerprint := fmt.Sprintf("%d:%d", expiryMillis, next.UnixMilli())
		shouldCreate, err := s.store.ShouldSendAlert(key, fingerprint, "x-ui client expiry auto renew", 24*time.Hour)
		if err != nil {
			log.Printf("x-ui client expiry state: %v", err)
			continue
		}
		if !shouldCreate {
			continue
		}
		inboundTag := billing.InboundTag
		if inboundTag == "" && client != nil {
			inboundTag = client.InboundTag
		}
		_, err = s.store.CreateXUIAction(agent.AgentID, model.XUIActionRequest{
			Kind: model.XUIActionUpdateClientExpiry,
			Payload: map[string]any{
				"inbound_id":        billing.InboundID,
				"inbound_tag":       inboundTag,
				"email":             billing.Email,
				"expiry_time":       next.UnixMilli(),
				"expire_cycle":      cycle,
				"expire_auto_renew": true,
			},
		})
		if err != nil {
			log.Printf("create x-ui client expiry action: %v", err)
		}
	}
}

func findOverviewClient(clients []model.XUIClientView, billing model.XUIClientBillingConfig) *model.XUIClientView {
	for index := range clients {
		client := &clients[index]
		if client.InboundID == billing.InboundID && client.InboundTag == billing.InboundTag && client.Email == billing.Email {
			return client
		}
	}
	return nil
}

func normalizeClientExpireCycle(cycle string) string {
	switch strings.ToLower(strings.TrimSpace(cycle)) {
	case "quarter", "quarterly", "season":
		return "quarter"
	case "year", "yearly":
		return "year"
	default:
		return "month"
	}
}

func addClientExpireCycle(value time.Time, cycle string) time.Time {
	switch cycle {
	case "quarter":
		return value.AddDate(0, 3, 0)
	case "year":
		return value.AddDate(1, 0, 0)
	default:
		return value.AddDate(0, 1, 0)
	}
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
	used := periodTrafficUsed(currentTotal, cfg.TrafficBaselineBytes)
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
	detail := fmt.Sprintf("当前周期已用 %.1f%%（%s / %s），上传 %s，下载 %s。", percent, formatBytes(used), formatBytes(cfg.TrafficLimitBytes), formatBytes(periodTrafficUsed(summary.NetTrafficSent, cfg.TrafficSentBaselineBytes)), formatBytes(periodTrafficUsed(summary.NetTrafficRecv, cfg.TrafficRecvBaselineBytes)))
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
	return fmt.Sprintf("%s NanFengMonitor 告警\nClient：%s (%s)\n标签：%s\n级别：%s\n类型：%s\n详情：%s\n时间：%s", icon, name, alert.agent.AgentID, tags, alert.severity, alert.title, alert.detail, time.Now().Format("2006-01-02 15:04:05"))
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
