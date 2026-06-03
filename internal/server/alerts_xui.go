package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (s *alertService) evaluateXUIClientExpiryAlerts(agent model.AgentRecord, snapshot model.AgentSnapshot) {
	if snapshot.XUI == nil {
		return
	}
	overview := dashboard.BuildXUIOverview(snapshot)
	if overview == nil {
		return
	}
	for _, alert := range buildXUIClientExpiryAlerts(agent, overview.Clients, time.Now()) {
		s.dispatch(alert)
	}
}

func buildXUIClientExpiryAlerts(agent model.AgentRecord, clients []model.XUIClientView, now time.Time) []alertMessage {
	alerts := make([]alertMessage, 0)
	for _, client := range clients {
		if client.ExpiryTime <= 0 {
			continue
		}
		kind := xuiClientExpiryAlertKind(client)
		expiry := time.UnixMilli(client.ExpiryTime)
		remaining := daysBetween(now, expiry)
		if remaining > xuiClientExpiryWarning {
			alerts = append(alerts, newResolvedAlert(agent.AgentID, kind))
			continue
		}

		severity := "warning"
		title := "X-UI Client 即将到期"
		state := "warning"
		remainingText := fmt.Sprintf("剩余 %d 天", remaining)
		if remaining <= xuiClientExpiryUrgent {
			severity = "critical"
			state = "critical"
		}
		if remaining < 0 {
			title = "X-UI Client 已过期"
			state = "expired"
			remainingText = fmt.Sprintf("已过期 %d 天", -remaining)
		}

		clientName := firstNonEmptyString(client.Email, client.Comment, "未命名")
		inboundName := firstNonEmptyString(client.InboundRemark, client.InboundTag, fmt.Sprintf("Inbound %d", client.InboundID))
		detail := fmt.Sprintf("入站：%s，Client：%s，到期时间：%s，%s。", inboundName, clientName, expiry.Local().Format("2006-01-02 15:04:05"), remainingText)
		alerts = append(alerts, newAgentAlert(agent, kind, severity, title, fmt.Sprintf("%s:%d", state, client.ExpiryTime), detail))
	}
	return alerts
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
		if !billing.ExpireAutoRenew || billing.StartTime <= 0 {
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

func xuiClientExpiryAlertKind(client model.XUIClientView) string {
	name := firstNonEmptyString(client.Email, client.Comment, "client")
	return fmt.Sprintf("xui_client_expiry:%d:%s:%s", client.InboundID, alertKeyPart(client.InboundTag), alertKeyPart(name))
}

func alertKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", "\n", "_", "\r", "_", "\t", "_", " ", "_")
	return replacer.Replace(value)
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
