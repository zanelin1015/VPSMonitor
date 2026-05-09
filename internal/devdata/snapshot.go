package devdata

import (
	"strings"
	"time"

	"bridge-core/internal/model"
)

type DemoProfile struct {
	AgentID    string
	AgentName  string
	Hostname   string
	PublicIPv4 string
	PublicIPv6 string
	BaseURL    string
	Tags       []string
}

func DemoProfiles() []DemoProfile {
	return []DemoProfile{
		{
			AgentID:    "local-dev-01",
			AgentName:  "HK Client",
			Hostname:   "hk-client-vps",
			PublicIPv4: "203.0.113.11",
			PublicIPv6: "2001:db8:1::11",
			BaseURL:    "http://127.0.0.1:19091",
			Tags:       []string{"HK", "线路机"},
		},
		{
			AgentID:    "local-dev-02",
			AgentName:  "US Client",
			Hostname:   "us-client-vps",
			PublicIPv4: "198.51.100.22",
			PublicIPv6: "2001:db8:2::22",
			BaseURL:    "http://127.0.0.1:19092",
			Tags:       []string{"US", "线路机"},
		},
		{
			AgentID:    "local-dev-03",
			AgentName:  "COX Client",
			Hostname:   "cox-us-vps",
			PublicIPv4: "203.0.113.33",
			PublicIPv6: "2001:db8:3::33",
			BaseURL:    "http://127.0.0.1:19093",
			Tags:       []string{"家宽", "US"},
		},
	}
}

func ResolveDemoProfile(agentID, agentName string) DemoProfile {
	normalizedID := strings.ToLower(strings.TrimSpace(agentID))
	for _, profile := range DemoProfiles() {
		if profile.AgentID == normalizedID {
			if agentID != "" {
				profile.AgentID = agentID
			}
			if agentName != "" {
				profile.AgentName = agentName
			}
			return profile
		}
	}

	profile := DemoProfiles()[0]
	if agentID != "" {
		profile.AgentID = agentID
	}
	if agentName != "" {
		profile.AgentName = agentName
	}
	if profile.AgentName == "" {
		profile.AgentName = "Demo VPS"
	}
	return profile
}

func DemoSnapshot(agentID, agentName string, now time.Time) model.AgentSnapshot {
	profile := ResolveDemoProfile(agentID, agentName)
	switch profile.AgentID {
	case "local-dev-02":
		return buildUSSnapshot(profile, now)
	case "local-dev-03":
		return buildCOXSnapshot(profile, now)
	default:
		return buildHKSnapshot(profile, now)
	}
}

func DemoOutbounds() []map[string]any {
	return buildHKSnapshot(ResolveDemoProfile("local-dev-01", ""), time.Now().UTC()).XUI.Outbounds
}

func DemoRoutingRules() []map[string]any {
	return buildHKSnapshot(ResolveDemoProfile("local-dev-01", ""), time.Now().UTC()).XUI.RoutingRules
}

func buildHKSnapshot(profile DemoProfile, now time.Time) model.AgentSnapshot {
	lastOnline := now.Add(-2 * time.Minute).UnixMilli()
	recentOnline := now.Add(-4 * time.Minute).UnixMilli()
	olderOnline := now.Add(-18 * time.Minute).UnixMilli()

	return model.AgentSnapshot{
		AgentID:    profile.AgentID,
		AgentName:  profile.AgentName,
		ReportedAt: now,
		Summary: model.VPSSummary{
			Hostname:         profile.Hostname,
			PublicIPv4:       profile.PublicIPv4,
			PublicIPv6:       profile.PublicIPv6,
			CPU:              24.6,
			MemUsed:          640 * 1024 * 1024,
			MemTotal:         2 * 1024 * 1024 * 1024,
			NetTrafficSent:   42 * 1024 * 1024 * 1024,
			NetTrafficRecv:   116 * 1024 * 1024 * 1024,
			NetTrafficTotal:  158 * 1024 * 1024 * 1024,
			NetIOUp:          580 * 1024,
			NetIODown:        4 * 1024 * 1024,
			XrayState:        "running",
			InboundCount:     1,
			OutboundCount:    3,
			RoutingRuleCount: 2,
		},
		XUI: &model.XUISnapshot{
			BaseURL:     profile.BaseURL,
			CollectedAt: now,
			Certificates: []model.XUILocalCertificate{
				{
					ID:        "hk-client-cert",
					Name:      "hk-client.demo.test",
					Subject:   "hk-client.demo.test",
					Issuer:    "Let's Encrypt",
					DNSNames:  []string{"hk-client.demo.test"},
					CertPath:  "/etc/letsencrypt/live/hk-client.demo.test/fullchain.pem",
					KeyPath:   "/etc/letsencrypt/live/hk-client.demo.test/privkey.pem",
					SourceDir: "/etc/letsencrypt/live/hk-client.demo.test",
					NotAfter:  ptrTime(now.Add(45 * 24 * time.Hour)),
				},
			},
			ServerStatus: demoServerStatus(profile, 24.6, 640*1024*1024, 2*1024*1024*1024, 15, 196, 21, 72*1024*1024, 5400),
			Inbounds: []map[string]any{
				{
					"id":             1,
					"tag":            "hk-vless-entry",
					"remark":         "HK Client VLESS Entry",
					"protocol":       "vless",
					"listen":         "",
					"port":           443,
					"enable":         true,
					"up":             int64(4 * 1024 * 1024 * 1024),
					"down":           int64(18 * 1024 * 1024 * 1024),
					"total":          int64(120 * 1024 * 1024 * 1024),
					"allTime":        int64(22 * 1024 * 1024 * 1024),
					"expiryTime":     now.Add(60 * 24 * time.Hour).UnixMilli(),
					"settings":       `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"hk-client-1@example.com","enable":true,"comment":"HK client 1 / direct HK","subId":"sub-hk-1","limitIp":2,"totalGB":214748364800,"flow":"xtls-rprx-vision"},{"id":"22222222-2222-2222-2222-222222222222","email":"hk-client-2@example.com","enable":true,"comment":"HK client 2 / direct HK","subId":"sub-hk-2","limitIp":1},{"id":"33333333-3333-3333-3333-333333333333","email":"hk-client-3@example.com","enable":true,"comment":"HK client 3 / forward to COX HTTP","subId":"sub-hk-3","limitIp":1}]}`,
					"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"hk-client.demo.test"}}`,
					"clientStats": []map[string]any{
						{"email": "hk-client-1@example.com", "enable": true, "up": int64(1 * 1024 * 1024 * 1024), "down": int64(5 * 1024 * 1024 * 1024), "allTime": int64(6 * 1024 * 1024 * 1024), "lastOnline": lastOnline},
						{"email": "hk-client-2@example.com", "enable": true, "up": int64(512 * 1024 * 1024), "down": int64(1600 * 1024 * 1024), "allTime": int64(2112 * 1024 * 1024), "lastOnline": olderOnline},
						{"email": "hk-client-3@example.com", "enable": true, "up": int64(780 * 1024 * 1024), "down": int64(3 * 1024 * 1024 * 1024), "allTime": int64(3852 * 1024 * 1024), "lastOnline": recentOnline},
					},
				},
			},
			Outbounds: []map[string]any{
				{"tag": "direct", "protocol": "freedom"},
				{
					"tag":      "to-cox-http",
					"protocol": "http",
					"settings": map[string]any{
						"servers": []map[string]any{
							{"address": "cox-us.demo.test", "port": 8080},
						},
					},
				},
				{"tag": "blocked", "protocol": "blackhole"},
			},
			OutboundTraffic: []map[string]any{
				{"tag": "direct", "up": int64(1500 * 1024 * 1024), "down": int64(7 * 1024 * 1024 * 1024), "total": int64(8668 * 1024 * 1024)},
				{"tag": "to-cox-http", "up": int64(780 * 1024 * 1024), "down": int64(3 * 1024 * 1024 * 1024), "total": int64(3852 * 1024 * 1024)},
			},
			RoutingRules: []map[string]any{
				{"type": "field", "user": []string{"hk-client-3@example.com"}, "outboundTag": "to-cox-http"},
				{"type": "field", "inboundTag": []string{"hk-vless-entry"}, "outboundTag": "direct"},
			},
		},
	}
}

func buildUSSnapshot(profile DemoProfile, now time.Time) model.AgentSnapshot {
	lastOnline := now.Add(-3 * time.Minute).UnixMilli()
	recentOnline := now.Add(-5 * time.Minute).UnixMilli()
	olderOnline := now.Add(-16 * time.Minute).UnixMilli()

	return model.AgentSnapshot{
		AgentID:    profile.AgentID,
		AgentName:  profile.AgentName,
		ReportedAt: now,
		Summary: model.VPSSummary{
			Hostname:         profile.Hostname,
			PublicIPv4:       profile.PublicIPv4,
			PublicIPv6:       profile.PublicIPv6,
			CPU:              18.9,
			MemUsed:          512 * 1024 * 1024,
			MemTotal:         1536 * 1024 * 1024,
			NetTrafficSent:   28 * 1024 * 1024 * 1024,
			NetTrafficRecv:   78 * 1024 * 1024 * 1024,
			NetTrafficTotal:  106 * 1024 * 1024 * 1024,
			NetIOUp:          460 * 1024,
			NetIODown:        3 * 1024 * 1024,
			XrayState:        "running",
			InboundCount:     1,
			OutboundCount:    3,
			RoutingRuleCount: 2,
		},
		XUI: &model.XUISnapshot{
			BaseURL:     profile.BaseURL,
			CollectedAt: now,
			Certificates: []model.XUILocalCertificate{
				{
					ID:        "us-client-cert",
					Name:      "us-client.demo.test",
					Subject:   "us-client.demo.test",
					Issuer:    "Google Trust Services",
					DNSNames:  []string{"us-client.demo.test"},
					CertPath:  "/etc/letsencrypt/live/us-client.demo.test/fullchain.pem",
					KeyPath:   "/etc/letsencrypt/live/us-client.demo.test/privkey.pem",
					SourceDir: "/etc/letsencrypt/live/us-client.demo.test",
					NotAfter:  ptrTime(now.Add(36 * 24 * time.Hour)),
				},
			},
			ServerStatus: demoServerStatus(profile, 18.9, 512*1024*1024, 1536*1024*1024, 11, 148, 16, 58*1024*1024, 4200),
			Inbounds: []map[string]any{
				{
					"id":             1,
					"tag":            "us-vless-entry",
					"remark":         "US Client VLESS Entry",
					"protocol":       "vless",
					"listen":         "",
					"port":           8443,
					"enable":         true,
					"settings":       `{"clients":[{"id":"44444444-4444-4444-4444-444444444444","email":"us-client-1@example.com","enable":true,"comment":"US client 1 / direct US","subId":"sub-us-1"},{"id":"55555555-5555-5555-5555-555555555555","email":"us-client-2@example.com","enable":true,"comment":"US client 2 / direct US","subId":"sub-us-2"},{"id":"66666666-6666-6666-6666-666666666666","email":"us-client-3@example.com","enable":true,"comment":"US client 3 / forward to COX VLESS client 1","subId":"sub-us-3"}]}`,
					"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"us-client.demo.test"}}`,
					"clientStats": []map[string]any{
						{"email": "us-client-1@example.com", "enable": true, "up": int64(600 * 1024 * 1024), "down": int64(2400 * 1024 * 1024), "allTime": int64(3000 * 1024 * 1024), "lastOnline": lastOnline},
						{"email": "us-client-2@example.com", "enable": true, "up": int64(320 * 1024 * 1024), "down": int64(1200 * 1024 * 1024), "allTime": int64(1520 * 1024 * 1024), "lastOnline": olderOnline},
						{"email": "us-client-3@example.com", "enable": true, "up": int64(910 * 1024 * 1024), "down": int64(3300 * 1024 * 1024), "allTime": int64(4210 * 1024 * 1024), "lastOnline": recentOnline},
					},
				},
			},
			Outbounds: []map[string]any{
				{"tag": "direct", "protocol": "freedom"},
				{
					"tag":      "to-cox-vless-1",
					"protocol": "vless",
					"settings": map[string]any{
						"vnext": []map[string]any{
							{"address": "cox-us.demo.test", "port": 9443},
						},
					},
					"streamSettings": map[string]any{
						"network":  "tcp",
						"security": "tls",
						"tlsSettings": map[string]any{
							"serverName": "cox-us.demo.test",
						},
					},
				},
				{"tag": "blocked", "protocol": "blackhole"},
			},
			OutboundTraffic: []map[string]any{
				{"tag": "direct", "up": int64(920 * 1024 * 1024), "down": int64(3600 * 1024 * 1024), "total": int64(4520 * 1024 * 1024)},
				{"tag": "to-cox-vless-1", "up": int64(910 * 1024 * 1024), "down": int64(3300 * 1024 * 1024), "total": int64(4210 * 1024 * 1024)},
			},
			RoutingRules: []map[string]any{
				{"type": "field", "user": []string{"us-client-3@example.com"}, "outboundTag": "to-cox-vless-1"},
				{"type": "field", "inboundTag": []string{"us-vless-entry"}, "outboundTag": "direct"},
			},
		},
	}
}

func buildCOXSnapshot(profile DemoProfile, now time.Time) model.AgentSnapshot {
	lastOnline := now.Add(-3 * time.Minute).UnixMilli()

	return model.AgentSnapshot{
		AgentID:    profile.AgentID,
		AgentName:  profile.AgentName,
		ReportedAt: now,
		Summary: model.VPSSummary{
			Hostname:         profile.Hostname,
			PublicIPv4:       profile.PublicIPv4,
			PublicIPv6:       profile.PublicIPv6,
			CPU:              14.2,
			MemUsed:          438 * 1024 * 1024,
			MemTotal:         1024 * 1024 * 1024,
			NetTrafficSent:   64 * 1024 * 1024 * 1024,
			NetTrafficRecv:   214 * 1024 * 1024 * 1024,
			NetTrafficTotal:  278 * 1024 * 1024 * 1024,
			NetIOUp:          920 * 1024,
			NetIODown:        6 * 1024 * 1024,
			XrayState:        "running",
			InboundCount:     2,
			OutboundCount:    1,
			RoutingRuleCount: 2,
		},
		XUI: &model.XUISnapshot{
			BaseURL:     profile.BaseURL,
			CollectedAt: now,
			Certificates: []model.XUILocalCertificate{
				{
					ID:        "cox-us-cert",
					Name:      "cox-us.demo.test",
					Subject:   "cox-us.demo.test",
					Issuer:    "Let's Encrypt",
					DNSNames:  []string{"cox-us.demo.test"},
					CertPath:  "/etc/letsencrypt/live/cox-us.demo.test/fullchain.pem",
					KeyPath:   "/etc/letsencrypt/live/cox-us.demo.test/privkey.pem",
					SourceDir: "/etc/letsencrypt/live/cox-us.demo.test",
					NotAfter:  ptrTime(now.Add(52 * 24 * time.Hour)),
				},
			},
			ServerStatus: demoServerStatus(profile, 14.2, 438*1024*1024, 1024*1024*1024, 9, 126, 12, 48*1024*1024, 3600),
			Inbounds: []map[string]any{
				{
					"id":             1,
					"tag":            "cox-vless-entry",
					"remark":         "COX VLESS Landing",
					"protocol":       "vless",
					"listen":         "",
					"port":           9443,
					"enable":         true,
					"settings":       `{"clients":[{"id":"77777777-7777-7777-7777-777777777777","email":"cox-vless-client-1@example.com","enable":true,"comment":"COX VLESS client 1 / US landing","subId":"sub-cox-vless-1"}]}`,
					"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"cox-us.demo.test"}}`,
					"clientStats": []map[string]any{
						{"email": "cox-vless-client-1@example.com", "enable": true, "up": int64(260 * 1024 * 1024), "down": int64(1140 * 1024 * 1024), "allTime": int64(1400 * 1024 * 1024), "lastOnline": lastOnline},
					},
				},
				{
					"id":             2,
					"tag":            "cox-http-entry",
					"remark":         "COX HTTP Landing",
					"protocol":       "http",
					"listen":         "",
					"port":           8080,
					"enable":         true,
					"settings":       `{"accounts":[]}`,
					"streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"cox-us.demo.test"}}`,
				},
			},
			Outbounds: []map[string]any{
				{"tag": "direct", "protocol": "freedom"},
			},
			OutboundTraffic: []map[string]any{
				{"tag": "direct", "up": int64(320 * 1024 * 1024), "down": int64(1680 * 1024 * 1024), "total": int64(2000 * 1024 * 1024)},
			},
			RoutingRules: []map[string]any{
				{"type": "field", "inboundTag": []string{"cox-vless-entry"}, "outboundTag": "direct"},
				{"type": "field", "inboundTag": []string{"cox-http-entry"}, "outboundTag": "direct"},
			},
		},
	}
}

func demoServerStatus(profile DemoProfile, cpu float64, memCurrent uint64, memTotal uint64, threads uint32, tcpCount int, udpCount int, appMem uint64, appUptime uint64) model.XUIServerStatus {
	return model.XUIServerStatus{
		CPU:      cpu,
		Uptime:   86400,
		TCPCount: tcpCount,
		UDPCount: udpCount,
		PublicIP: model.XUIPublicIP{
			IPv4: profile.PublicIPv4,
			IPv6: profile.PublicIPv6,
		},
		Mem: model.XUIUsage{
			Current: memCurrent,
			Total:   memTotal,
		},
		Disk: model.XUIUsage{
			Current: 8 * 1024 * 1024 * 1024,
			Total:   30 * 1024 * 1024 * 1024,
		},
		NetIO: model.XUINetIO{
			Up:   uint64(tcpCount) * 3 * 1024,
			Down: uint64(udpCount) * 128 * 1024,
		},
		NetTraffic: model.XUINetTraffic{
			Sent: uint64(tcpCount) * 256 * 1024 * 1024,
			Recv: uint64(udpCount) * 1024 * 1024 * 1024,
		},
		Xray: model.XUIXrayStatus{
			State:   "running",
			Version: "25.1.30",
		},
		AppStats: model.XUIAppStats{
			Threads: threads,
			Mem:     appMem,
			Uptime:  appUptime,
		},
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
