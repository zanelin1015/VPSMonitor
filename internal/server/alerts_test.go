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
