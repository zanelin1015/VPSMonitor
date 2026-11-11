package server

import (
	"net/url"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestFilterCustomerSubscriptionLinksByAssignmentSelection(t *testing.T) {
	links := []model.CustomerLinkView{
		{AssignmentID: 1, EntryClientName: "one"},
		{AssignmentID: 2, EntryClientName: "two"},
		{AssignmentID: 3, EntryClientName: "three"},
	}
	query := url.Values{}
	query.Set("assignments", "3,1,3")
	filtered := filterCustomerSubscriptionLinks(links, query)
	if len(filtered) != 2 || filtered[0].AssignmentID != 1 || filtered[1].AssignmentID != 3 {
		t.Fatalf("unexpected filtered links: %#v", filtered)
	}
	if got := filterCustomerSubscriptionLinks(links, nil); len(got) != len(links) {
		t.Fatalf("missing selection should preserve all links, got %#v", got)
	}
}

func TestCustomerSubscriptionNodeNameUsesEntryAndRelayRoute(t *testing.T) {
	link := model.CustomerLinkView{
		EntryClientName: "US-优化",
		Steps: []model.CustomerLinkStep{
			{Role: "entry", Label: "US-优化"},
			{Role: "relay", Label: "COX落地"},
			{Role: "exit", Label: "出口 US"},
		},
	}
	if got := customerSubscriptionNodeName(link); got != "US-优化-COX落地" {
		t.Fatalf("expected entry and relay route name, got %q", got)
	}
	link.Remark = "自定义名称"
	if got := customerSubscriptionNodeName(link); got != "自定义名称" {
		t.Fatalf("expected explicit remark to remain an override, got %q", got)
	}
}

func TestBuildMihomoSubscriptionConvertsCustomerLinks(t *testing.T) {
	user := model.CustomerUser{Username: "alice"}
	links := []model.CustomerLinkView{
		{
			AssignmentID:    1,
			EntryClientName: "广州入口",
			Remark:          "CN-HK-VLESS",
			ImportURL:       "vless://11111111-1111-1111-1111-111111111111@gz.example.com:20001?encryption=none&security=tls&type=ws&host=hk.example.com&path=%2Fws&sni=hk.example.com#old",
			Resolved:        true,
		},
		{
			AssignmentID:    2,
			EntryClientName: "HK SS",
			ImportURL:       "ss://YWVzLTI1Ni1nY206cGFzcw@example.com:8388#ss-old",
			Resolved:        true,
		},
		{
			AssignmentID:    3,
			EntryClientName: "未解析",
			ImportURL:       "vless://22222222-2222-2222-2222-222222222222@example.com:443?encryption=none",
			Resolved:        false,
		},
		{
			AssignmentID:    4,
			EntryClientName: "GZ HTTP",
			ImportURL:       "http://proxy-user:p%40ss%3Aword@gz.example.com:20080#HTTP",
			Resolved:        true,
		},
	}

	content := buildMihomoSubscription(user, links)
	assertContains(t, content, `port: 7890`)
	assertContains(t, content, `socks-port: 7891`)
	assertContains(t, content, `allow-lan: true`)
	assertContains(t, content, `bind-address: "*"`)
	assertContains(t, content, `external-controller: 127.0.0.1:9090`)
	assertContains(t, content, `listen: 0.0.0.0:53`)
	assertContains(t, content, `respect-rules: true`)
	assertContains(t, content, `enhanced-mode: fake-ip`)
	assertContains(t, content, `fake-ip-filter-mode: blacklist`)
	assertContains(t, content, `fake-ip-filter:`)
	assertContains(t, content, `- https://doh.pub/dns-query`)
	assertContains(t, content, `- "+.google.com"`)
	assertContains(t, content, `geosite:geolocation-!cn:`)
	assertContains(t, content, `fallback:`)
	assertContains(t, content, `name: 🚀 节点选择`)
	assertContains(t, content, `name: 🚀 手动切换`)
	assertContains(t, content, `name: 🐟 漏网之鱼`)
	assertContains(t, content, `name: 💬 Ai平台`)
	assertContains(t, content, `name: Ⓜ️ 微软Bing`)
	assertContains(t, content, `name: 🛑 广告拦截`)
	assertContains(t, content, `name: 🇭🇰 香港节点`)
	assertContains(t, content, `name: "CN-HK-VLESS"`)
	assertContains(t, content, `type: "vless"`)
	assertContains(t, content, `uuid: "11111111-1111-1111-1111-111111111111"`)
	assertContains(t, content, `ws-opts:`)
	assertContains(t, content, `headers: {"Host": "hk.example.com"}`)
	assertContains(t, content, `type: "ss"`)
	assertContains(t, content, `cipher: "aes-256-gcm"`)
	assertContains(t, content, `password: "pass"`)
	assertContains(t, content, `name: "GZ HTTP"`)
	assertContains(t, content, `type: "http"`)
	assertContains(t, content, `username: "proxy-user"`)
	assertContains(t, content, `password: "p@ss:word"`)
	assertContains(t, content, `- "CN-HK-VLESS"`)
	assertContains(t, content, `- "HK SS"`)
	assertContains(t, content, `- "GZ HTTP"`)
	assertContains(t, content, `- DOMAIN-SUFFIX,acl4.ssr,🎯 全球直连`)
	assertContains(t, content, `- DOMAIN-SUFFIX,bing.com,Ⓜ️ 微软Bing`)
	assertContains(t, content, `- GEOIP,CN,🎯 全球直连`)
	assertContains(t, content, `- MATCH,🐟 漏网之鱼`)
	if got := strings.Count(content, `      - "CN-HK-VLESS"`); got != 2 {
		t.Fatalf("expected proxy in manual and Hong Kong groups, got %d", got)
	}
	if strings.Contains(content, "未解析") {
		t.Fatalf("unresolved links should not be included:\n%s", content)
	}
	if strings.Contains(content, "US-NTT2") || strings.Contains(content, "usdm10g1.zanelin.top") {
		t.Fatal("reference subscription proxy data must not leak into generated subscriptions")
	}
}

func TestBuildMihomoSubscriptionUsesManualProxyAsDefault(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{{
		AssignmentID:    99,
		EntryClientName: "unclassified-node",
		ImportURL:       "ss://YWVzLTI1Ni1nY206cGFzcw@example.com:8388#node",
		Resolved:        true,
	}})

	assertContains(t, content, "name: 🚀 节点选择\n    type: select\n    proxies:\n      - 🚀 手动切换")
	assertContains(t, content, `name: "unclassified-node"`)
}

func TestBuildMihomoSubscriptionIncludesManualSnellConfig(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{{
		AssignmentID:    42,
		EntryClientName: "unused-xui-link",
		Resolved:        false,
		Snell:           &model.SnellConfig{Enabled: true, Name: "HK Snell", Server: "snell.example.com", Port: 443, PSK: "secret-psk", Version: 5, ObfsMode: "http", ObfsHost: "www.example.com", ObfsPath: "/connect"},
	}})
	assertContains(t, content, `name: "HK Snell"`)
	assertContains(t, content, `type: "snell"`)
	assertContains(t, content, `server: "snell.example.com"`)
	assertContains(t, content, `port: 443`)
	assertContains(t, content, `psk: "secret-psk"`)
	assertContains(t, content, `version: 5`)
	assertContains(t, content, `obfs-opts:`)
	assertContains(t, content, `host: "www.example.com"`)
	assertContains(t, content, `path: "/connect"`)
}

func TestBuildMihomoSubscriptionIncludesMultipleSnellConfigs(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{{
		AssignmentID: 44,
		Snells: []*model.SnellConfig{
			{Enabled: true, Name: "CM-HK-COX", Server: "hk.example.com", Port: 27412, PSK: "secret-1", Version: 5},
			{Enabled: true, Name: "CM-US-COX", Server: "us.example.com", Port: 27413, PSK: "secret-2", Version: 5},
		},
	}})
	assertContains(t, content, `name: "CM-HK-COX"`)
	assertContains(t, content, `name: "CM-US-COX"`)
	assertContains(t, content, `server: "hk.example.com"`)
	assertContains(t, content, `server: "us.example.com"`)
}

func TestBuildMihomoSubscriptionBuildsSnellForwardChain(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{}, []model.CustomerLinkView{{
		AssignmentID:          43,
		EntryClientName:       "HK XUI",
		Resolved:              true,
		SubscriptionOnly:      true,
		SubscriptionImportURL: "vless://11111111-1111-1111-1111-111111111111@hk.example.com:443?encryption=none#HK",
		SnellChain:            &model.SnellConfig{Enabled: true, Name: "Snell Entry", Server: "snell.example.com", Port: 443, PSK: "secret-psk", Version: 5},
	}})
	assertContains(t, content, `name: "Snell Entry"`)
	assertContains(t, content, `name: "HK XUI"`)
	assertContains(t, content, `dialer-proxy: "Snell Entry"`)
	if strings.Contains(content, `server: "snell.example.com"\n    port: 443\n    psk: "secret-psk"\n    version: 5\n    udp: true\n    dialer-proxy`) {
		t.Fatal("Snell front proxy must not be emitted as a selectable target")
	}
}

func TestBuildMihomoSubscriptionUsesAssignmentFrontProxyGroup(t *testing.T) {
	user := model.CustomerUser{Username: "alice"}
	links := []model.CustomerLinkView{
		{
			AssignmentID:    10,
			EntryClientName: "CS1",
			Remark:          "CS1",
			ImportURL:       "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none#CS1",
			Resolved:        true,
			FrontProxies: []model.CustomerLinkFrontProxy{
				{
					ID:       1,
					Name:     "HK IEPL",
					ShareURL: "ss://YWVzLTI1Ni1nY206cGFzcw@hk-iepl.example.com:8388#HK",
				},
				{
					ID:       2,
					Name:     "JP IEPL",
					ShareURL: "ss://YWVzLTI1Ni1nY206cGFzczI@jp-iepl.example.com:12014#JP",
				},
			},
		},
		{
			AssignmentID:    11,
			EntryClientName: "CS2",
			Remark:          "CS2",
			ImportURL:       "vless://22222222-2222-2222-2222-222222222222@example.net:443?encryption=none#CS2",
			Resolved:        true,
		},
	}

	content := buildMihomoSubscription(user, links)
	assertContains(t, content, `name: "CS1"`)
	assertContains(t, content, `dialer-proxy: "CS1 前置代理"`)
	assertContains(t, content, `name: "HK IEPL"`)
	assertContains(t, content, `server: "hk-iepl.example.com"`)
	assertContains(t, content, `name: "JP IEPL"`)
	assertContains(t, content, `name: "CS1 前置代理"`)
	assertContains(t, content, `type: select`)
	assertContains(t, content, `      - "HK IEPL"`)
	assertContains(t, content, `      - "JP IEPL"`)
	if strings.Contains(content, `name: "CS2 前置代理"`) {
		t.Fatalf("link without front proxies should not get a front proxy group:\n%s", content)
	}
}

func TestBuildMihomoSubscriptionWithoutLinksRemainsUsable(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{Username: "empty"}, nil)
	assertContains(t, content, "proxies:\n  []")
	assertContains(t, content, "name: 🚀 手动切换\n    type: select")
	if strings.Contains(content, mihomoProxyMarker) || strings.Contains(content, mihomoProxyNameMarker) ||
		strings.Contains(content, mihomoHKProxyNameMarker) || strings.Contains(content, mihomoJPProxyNameMarker) ||
		strings.Contains(content, mihomoUSProxyNameMarker) || strings.Contains(content, mihomoSGProxyNameMarker) ||
		strings.Contains(content, mihomoTWProxyNameMarker) || strings.Contains(content, mihomoKRProxyNameMarker) ||
		strings.Contains(content, mihomoNetflixNameMarker) {
		t.Fatal("subscription template markers must be fully rendered")
	}
}

func TestMihomoProxyRegionMatching(t *testing.T) {
	tests := []struct {
		name   string
		region mihomoProxyRegion
		want   bool
	}{
		{name: "HK-SS", region: mihomoProxyRegionHK, want: true},
		{name: "US-ATT", region: mihomoProxyRegionUS, want: true},
		{name: "JP-Tokyo", region: mihomoProxyRegionJP, want: true},
		{name: "普通节点", region: mihomoProxyRegionHK, want: false},
		{name: "Netflix-US", region: mihomoProxyRegionNetflix, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mihomoProxyMatchesRegion(test.name, test.region); got != test.want {
				t.Fatalf("expected %v for %q in %s, got %v", test.want, test.name, test.region, got)
			}
		})
	}
}

func TestBuildMihomoSubscriptionUsesThirdPartyDNSHeader(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{Username: "alice"}, nil)
	assertContains(t, content, "dns:\n  enable: true")
	assertContains(t, content, "port: 7890")
	assertContains(t, content, "socks-port: 7891")
	assertContains(t, content, "bind-address: \"*\"")
	assertContains(t, content, "external-controller: 127.0.0.1:9090")
	assertContains(t, content, "listen: 0.0.0.0:53")
	assertContains(t, content, "fake-ip-filter:")
	assertContains(t, content, "proxy-server-nameserver:")
	assertContains(t, content, "nameserver-policy:")
	assertContains(t, content, "    geosite:cn:")
	assertContains(t, content, "      - 119.29.29.29\n      - 223.5.5.5")
	assertContains(t, content, "    geosite:geolocation-!cn:")
	assertContains(t, content, "    - tls://1.0.0.1:853")
	assertContains(t, content, "    - tls://8.8.8.8:853")
	assertContains(t, content, "  fallback-filter:")
}

func TestCustomerSubscriptionContentDispositionUsesUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		expected string
	}{
		{
			name:     "ascii username",
			username: "TT",
			expected: "inline; filename=TT; filename*=UTF-8''TT",
		},
		{
			name:     "unicode username",
			username: "测试 TT",
			expected: "inline; filename=TT; filename*=UTF-8''%E6%B5%8B%E8%AF%95%20TT",
		},
		{
			name:     "empty username",
			username: "  ",
			expected: "inline; filename=vpsmonitor; filename*=UTF-8''vpsmonitor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := customerSubscriptionContentDisposition(test.username); got != test.expected {
				t.Fatalf("unexpected Content-Disposition: got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestParseMihomoProxySupportsHTTPAndSocks(t *testing.T) {
	httpProxy, ok := parseMihomoProxy("https://user:pass@proxy.example.com:8443", "HTTP Exit")
	if !ok {
		t.Fatal("expected https proxy to parse")
	}
	if got := mihomoFieldValue(httpProxy.Fields, "type"); got != "http" {
		t.Fatalf("expected http type, got %#v", got)
	}
	if got := mihomoFieldValue(httpProxy.Fields, "tls"); got != true {
		t.Fatalf("expected https proxy tls=true, got %#v", got)
	}

	socksProxy, ok := parseMihomoProxy("socks5://proxy.example.com:1080", "SOCKS Exit")
	if !ok {
		t.Fatal("expected socks proxy to parse")
	}
	if got := mihomoFieldValue(socksProxy.Fields, "type"); got != "socks5" {
		t.Fatalf("expected socks5 type, got %#v", got)
	}
}

func assertContains(t *testing.T, content, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("expected content to contain %q:\n%s", expected, content)
	}
}

func mihomoFieldValue(fields []mihomoField, key string) any {
	for _, field := range fields {
		if field.Key == key {
			return field.Value
		}
	}
	return nil
}
