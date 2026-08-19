package server

import (
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestBuildXUIClientExpiryAlerts(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	agent := model.AgentRecord{AgentID: "agent-1", AgentName: "Tokyo"}
	clients := []model.XUIClientView{
		{InboundID: 1, InboundTag: "vless", InboundRemark: "主入口", Email: "soon@example.com", ExpiryTime: now.AddDate(0, 0, 6).UnixMilli()},
		{InboundID: 1, InboundTag: "vless", Email: "urgent@example.com", ExpiryTime: now.AddDate(0, 0, 2).UnixMilli()},
		{InboundID: 2, InboundTag: "vmess", Email: "old@example.com", ExpiryTime: now.AddDate(0, 0, -1).UnixMilli()},
		{InboundID: 3, InboundTag: "safe", Email: "safe@example.com", ExpiryTime: now.AddDate(0, 0, 10).UnixMilli()},
		{InboundID: 4, InboundTag: "none", Email: "none@example.com"},
	}

	alerts := buildXUIClientExpiryAlerts(agent, clients, now)
	if len(alerts) != 4 {
		t.Fatalf("expected 4 alerts/resolutions, got %d", len(alerts))
	}
	if alerts[0].severity != "warning" || alerts[0].title != "X-UI Client 即将到期" {
		t.Fatalf("unexpected warning alert: %#v", alerts[0])
	}
	if !strings.Contains(alerts[0].detail, "到期时间：2026-05-16 20:00:00") {
		t.Fatalf("expected Beijing expiry time, got %q", alerts[0].detail)
	}
	if alerts[1].severity != "critical" || !strings.Contains(alerts[1].fingerprint, "critical") {
		t.Fatalf("unexpected urgent alert: %#v", alerts[1])
	}
	if alerts[2].severity != "critical" || alerts[2].title != "X-UI Client 已过期" || !strings.Contains(alerts[2].detail, "已过期 1 天") {
		t.Fatalf("unexpected expired alert: %#v", alerts[2])
	}
	if !strings.HasSuffix(alerts[3].key, ":resolved") {
		t.Fatalf("expected safe client to resolve alert, got %q", alerts[3].key)
	}
}

func TestFormatBeijingTime(t *testing.T) {
	value := time.Date(2026, 8, 5, 2, 51, 52, 0, time.UTC)
	if got := formatBeijingTime(value); got != "2026-08-05 10:51:52" {
		t.Fatalf("formatBeijingTime=%q", got)
	}
}

func TestAlertDeliveryPolicy(t *testing.T) {
	offline := alertMessage{key: resolveAgentAlertKey("agent-1", "offline")}
	xuiError := alertMessage{key: resolveAgentAlertKey("agent-1", "xui_error")}

	if !shouldDeliverAlert(offline, deliverImmediateAlerts) {
		t.Fatal("Client offline alert must be delivered immediately")
	}
	if shouldDeliverAlert(xuiError, deliverImmediateAlerts) {
		t.Fatal("non-offline alert must not be delivered immediately")
	}
	if shouldDeliverAlert(offline, deliverDailyAlerts) {
		t.Fatal("Client offline alert must not be repeated in the daily delivery")
	}
	if !shouldDeliverAlert(xuiError, deliverDailyAlerts) {
		t.Fatal("non-offline alert must be delivered in the daily window")
	}
}

func TestBuildAgentAlertsClassifiesHAProxyErrorSeparately(t *testing.T) {
	agent := model.AgentRecord{AgentID: "agent-1", AgentName: "Guangzhou"}
	snapshot := model.AgentSnapshot{
		AgentID: "agent-1",
		HAProxy: &model.HAProxySnapshot{Error: "runtime socket unavailable"},
	}
	alerts := buildAgentAlerts(agent, snapshot, false)

	var haProxyAlert alertMessage
	for _, alert := range alerts {
		if alert.key == resolveAgentAlertKey(agent.AgentID, "haproxy_error") {
			haProxyAlert = alert
		}
		if alert.key == resolveAgentAlertKey(agent.AgentID, "xui_error") {
			t.Fatalf("HAProxy error must not be reported as X-UI error: %#v", alert)
		}
	}
	if haProxyAlert.title != "HAProxy 状态采集异常" || !strings.Contains(haProxyAlert.detail, "runtime socket unavailable") {
		t.Fatalf("unexpected HAProxy alert: %#v", haProxyAlert)
	}
}

func TestBuildAgentAlertsClassifiesLegacyHAProxyCollectionError(t *testing.T) {
	agent := model.AgentRecord{AgentID: "agent-1", AgentName: "Guangzhou"}
	snapshot := model.AgentSnapshot{
		AgentID: "agent-1",
		Summary: model.VPSSummary{LastCollectionErr: "haproxy: runtime socket unavailable"},
	}
	alerts := buildAgentAlerts(agent, snapshot, false)

	xuiAlert, xuiActive := activeAlertByKey(alerts, resolveAgentAlertKey(agent.AgentID, "xui_error"))
	if xuiActive {
		t.Fatalf("legacy HAProxy collection error must not create an X-UI alert: %#v", xuiAlert)
	}
	haProxyAlert, haProxyActive := activeAlertByKey(alerts, resolveAgentAlertKey(agent.AgentID, "haproxy_error"))
	if !haProxyActive || !strings.Contains(haProxyAlert.detail, "runtime socket unavailable") {
		t.Fatalf("expected legacy HAProxy collection error to create an HAProxy alert: %#v", haProxyAlert)
	}
}

func TestBuildAgentAlertsSplitsMixedLegacyCollectionErrors(t *testing.T) {
	agent := model.AgentRecord{AgentID: "agent-1", AgentName: "Guangzhou"}
	snapshot := model.AgentSnapshot{
		AgentID: "agent-1",
		Summary: model.VPSSummary{LastCollectionErr: "x-ui: login failed; haproxy: runtime socket unavailable"},
	}
	alerts := buildAgentAlerts(agent, snapshot, false)

	xuiAlert, xuiActive := activeAlertByKey(alerts, resolveAgentAlertKey(agent.AgentID, "xui_error"))
	if !xuiActive || !strings.Contains(xuiAlert.detail, "x-ui: login failed") {
		t.Fatalf("expected mixed collection error to preserve X-UI alert: %#v", xuiAlert)
	}
	if strings.Contains(strings.ToLower(xuiAlert.detail), "haproxy") || strings.Contains(strings.ToLower(xuiAlert.fingerprint), "haproxy") {
		t.Fatalf("X-UI alert must not contain HAProxy error details: %#v", xuiAlert)
	}
	haProxyAlert, haProxyActive := activeAlertByKey(alerts, resolveAgentAlertKey(agent.AgentID, "haproxy_error"))
	if !haProxyActive || !strings.Contains(haProxyAlert.detail, "runtime socket unavailable") {
		t.Fatalf("expected mixed collection error to create an HAProxy alert: %#v", haProxyAlert)
	}
}

func activeAlertByKey(alerts []alertMessage, key string) (alertMessage, bool) {
	for _, alert := range alerts {
		if alert.key == key {
			return alert, true
		}
	}
	return alertMessage{}, false
}

func TestAccountedTrafficUsedModes(t *testing.T) {
	bidirectional := model.VPSRenewalConfig{TrafficAccountingMode: "bidirectional"}
	if got := accountedTrafficUsed(bidirectional, 90, 40, 50); got != 90 {
		t.Fatalf("bidirectional should sum upload and download, got %d", got)
	}

	singleDirection := model.VPSRenewalConfig{TrafficAccountingMode: "single_direction"}
	if got := accountedTrafficUsed(singleDirection, 90, 40, 50); got != 50 {
		t.Fatalf("single direction should use max(upload, download), got %d", got)
	}

	if got := accountedTrafficUsed(singleDirection, 90, 0, 0); got != 90 {
		t.Fatalf("single direction should fallback to total when directional data is unavailable, got %d", got)
	}
}
