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
	assertContains(t, content, `mixed-port: 7890`)
	assertContains(t, content, `allow-lan: true`)
	assertContains(t, content, `name: 🚀 节点选择`)
	assertContains(t, content, `name: ♻️ 自动选择`)
	assertContains(t, content, `name: 🐟 漏网之鱼`)
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
	assertContains(t, content, `- GEOIP,CN,🎯 全球直连`)
	assertContains(t, content, `- MATCH,🐟 漏网之鱼`)
	if got := strings.Count(content, `      - "CN-HK-VLESS"`); got != 8 {
		t.Fatalf("expected proxy in all 8 ACL4SSR groups, got %d", got)
	}
	if strings.Contains(content, "未解析") {
		t.Fatalf("unresolved links should not be included:\n%s", content)
	}
	if strings.Contains(content, "US-NTT2") || strings.Contains(content, "usdm10g1.zanelin.top") {
		t.Fatal("reference subscription proxy data must not leak into generated subscriptions")
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
	assertContains(t, content, "name: ♻️ 自动选择\n    type: url-test")
	if strings.Contains(content, mihomoProxyMarker) || strings.Contains(content, mihomoProxyNameMarker) {
		t.Fatal("subscription template markers must be fully rendered")
	}
}

func TestBuildMihomoSubscriptionIncludesOpenClashFriendlyDNS(t *testing.T) {
	content := buildMihomoSubscription(model.CustomerUser{Username: "alice"}, nil)
	assertContains(t, content, "dns:\n  enable: true")
	assertContains(t, content, "proxy-server-nameserver:")
	assertContains(t, content, "nameserver-policy:")
	assertContains(t, content, `    "+.zanelin.top":`)
	assertContains(t, content, "    - 223.5.5.5")
	assertContains(t, content, "    - 119.29.29.29")
	if strings.Contains(content, "dns.google") {
		t.Fatalf("customer subscription must not depend on dns.google in mainland OpenClash environments:\n%s", content)
	}
	if strings.Contains(content, "8.8.8.8") || strings.Contains(content, "1.1.1.1") {
		t.Fatalf("customer subscription must not depend on public Google/Cloudflare DNS in mainland OpenClash environments:\n%s", content)
	}
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
