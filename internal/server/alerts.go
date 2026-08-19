package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

const (
	alertCooldown          = 6 * time.Hour
	agentOfflineAfter      = 5 * time.Minute
	telegramAPITimeout     = 8 * time.Second
	xuiClientExpiryWarning = 7
	xuiClientExpiryUrgent  = 3
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

type alertDeliveryMode int

const (
	deliverImmediateAlerts alertDeliveryMode = iota
	deliverDailyAlerts
)

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
	go s.runDailyNotifications()
}

func (s *alertService) runAlertSweep() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	lastRun := time.Now()
	for range ticker.C {
		settings := s.scheduledTaskSettings()
		if !settings.AlertSweep.Enabled {
			continue
		}
		interval := time.Duration(settings.AlertSweep.IntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = 5 * time.Minute
		}
		if time.Since(lastRun) < interval {
			continue
		}
		s.EvaluateAll()
		lastRun = time.Now()
	}
}

func (s *alertService) runDailyNotifications() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var lastRunDate string
	for range ticker.C {
		now := time.Now().In(beijingTimeLocation)
		if now.Hour() != 9 || now.Minute() != 0 {
			continue
		}
		runDate := now.Format("2006-01-02")
		if lastRunDate == runDate {
			continue
		}

		// Non-urgent alerts are deliberately evaluated only in the daily window.
		// Customer support messages use their own immediate path, while Client
		// offline alerts continue to be handled by the frequent alert sweep.
		s.EvaluateAllDaily()
		lastRunDate = runDate

		settings := s.scheduledTaskSettings()
		if !settings.DailyTrafficReport.Enabled {
			continue
		}
		days := settings.DailyTrafficReport.IntervalDays
		if days <= 0 {
			days = 1
		}
		if days > 1 && now.Day()%days != 0 {
			continue
		}
		s.SendDailyTrafficReport(now.AddDate(0, 0, -1))
	}
}

func (s *alertService) scheduledTaskSettings() model.ScheduledTaskSettings {
	settings, _, err := s.store.GetScheduledTaskSettings()
	if err != nil {
		log.Printf("load scheduled task settings: %v", err)
		return model.ScheduledTaskSettings{
			AlertSweep:         model.ScheduledTaskConfig{Enabled: true, IntervalMinutes: 5},
			DailyTrafficReport: model.ScheduledTaskConfig{Enabled: true, TimeOfDay: "09:00", IntervalDays: 1},
		}
	}
	return settings
}

func parseTaskTimeOfDay(value string, fallbackHour int, fallbackMinute int) (int, int) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return fallbackHour, fallbackMinute
	}
	return parsed.Hour(), parsed.Minute()
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
	s.evaluateAgentRecord(agent, snapshot, false, deliverImmediateAlerts)
	s.evaluateXUIClientExpiryAlerts(agent, snapshot, deliverImmediateAlerts)
}

func (s *alertService) EvaluateAll() {
	s.evaluateAll(deliverImmediateAlerts)
}

func (s *alertService) EvaluateAllDaily() {
	s.evaluateAll(deliverDailyAlerts)
}

func (s *alertService) evaluateAll(mode alertDeliveryMode) {
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
		s.evaluateAgentRecord(agent, snapshot, true, mode)
		s.evaluateXUIClientExpiryAlerts(agent, snapshot, mode)
	}
}

func (s *alertService) evaluateAgentRecord(agent model.AgentRecord, snapshot model.AgentSnapshot, includeOffline bool, mode alertDeliveryMode) {
	alerts := buildAgentAlerts(agent, snapshot, includeOffline)
	for _, alert := range alerts {
		s.dispatch(alert, mode)
	}
}

func (s *alertService) dispatch(alert alertMessage, mode alertDeliveryMode) {
	if strings.HasSuffix(alert.key, ":resolved") {
		_ = s.store.ResolveAlert(strings.TrimSuffix(alert.key, ":resolved"))
		return
	}
	if alert.key == "" {
		return
	}
	if !shouldDeliverAlert(alert, mode) {
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

func shouldDeliverAlert(alert alertMessage, mode alertDeliveryMode) bool {
	isOffline := strings.HasSuffix(alert.key, ":offline")
	if mode == deliverImmediateAlerts {
		return isOffline
	}
	return !isOffline
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

func (s *alertService) sendMessageToEnabledTelegramBots(text string) error {
	if s == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	bots, err := s.store.ListEnabledTelegramBotSecrets()
	if err != nil {
		return err
	}
	var sendErrors []error
	for _, bot := range bots {
		if err := s.sendTelegramMessage(bot.BotToken, bot.ChatID, text); err != nil {
			sendErrors = append(sendErrors, fmt.Errorf("%s: %w", bot.Name, err))
		}
	}
	return errors.Join(sendErrors...)
}
