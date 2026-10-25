package server

import (
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (s *alertService) evaluateXUIClientExpiryAlerts(agent model.AgentRecord, snapshot model.AgentSnapshot, mode alertDeliveryMode) {
	if snapshot.XUI == nil {
		return
	}
	overview := dashboard.BuildXUIOverview(snapshot)
	if overview == nil {
		return
	}
	for _, alert := range buildXUIClientExpiryAlerts(agent, overview.Clients, time.Now()) {
		s.dispatch(alert, mode)
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
		detail := fmt.Sprintf("入站：%s，Client：%s，到期时间：%s，%s。", inboundName, clientName, formatBeijingTime(expiry), remainingText)
		alerts = append(alerts, newAgentAlert(agent, kind, severity, title, fmt.Sprintf("%s:%d", state, client.ExpiryTime), detail))
	}
	return alerts
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
