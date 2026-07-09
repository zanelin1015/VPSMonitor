package server

import (
	"strings"
	"testing"

	"bridge-core/internal/model"
)

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
	}

	content := buildMihomoSubscription(user, links)
	assertContains(t, content, `mixed-port: 7890`)
	assertContains(t, content, `rule-providers:`)
	assertContains(t, content, `url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/reject.txt"`)
	assertContains(t, content, `behavior: ipcidr`)
	assertContains(t, content, `name: "CN-HK-VLESS"`)
	assertContains(t, content, `type: "vless"`)
	assertContains(t, content, `uuid: "11111111-1111-1111-1111-111111111111"`)
	assertContains(t, content, `ws-opts:`)
	assertContains(t, content, `headers: {"Host": "hk.example.com"}`)
	assertContains(t, content, `type: "ss"`)
	assertContains(t, content, `cipher: "aes-256-gcm"`)
	assertContains(t, content, `password: "pass"`)
	assertContains(t, content, `- "CN-HK-VLESS"`)
	assertContains(t, content, `- "HK SS"`)
	assertContains(t, content, `- RULE-SET,reject,REJECT`)
	assertContains(t, content, `- RULE-SET,cncidr,DIRECT`)
	assertContains(t, content, `- GEOIP,CN,DIRECT,no-resolve`)
	assertContains(t, content, `- MATCH,PROXY`)
	if strings.Contains(content, "未解析") {
		t.Fatalf("unresolved links should not be included:\n%s", content)
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
