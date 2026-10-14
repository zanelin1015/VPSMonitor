package model

import (
	"net/url"
	"reflect"
	"testing"
)

func TestShadowrocketExportByServingCore(t *testing.T) {
	const original = "vless://11111111-1111-1111-1111-111111111111@[2001:db8::1]:26059?security=reality&type=tcp&encryption=none&fp=chrome&pbk=public-key&sid=abcd&sni=www.example.com&flow=xtls-rprx-vision&spx=%2Fhello%3Fq%3D1&alpn=h2%2Chttp%2F1.1#NTT%20%E8%8A%82%E7%82%B9"
	for _, tc := range []struct {
		version    string
		compatible bool
	}{
		{"", false}, {"unknown", false}, {"26.9", false}, {"3.10.0", false},
		{"26.6.27", false}, {"26.9.7", false}, {"26.9.8", true}, {"26.9.30", true},
		{"v26.10.10", true}, {"Xray 26.10.10 (Xray, Penetrates Everything.)", true},
		{"refs/tags/v26.10.10-custom", true}, {"27.1.1", true},
		{"27.1.999999999999999999999999", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			got := ShadowrocketImportURL(original, tc.version)
			if !tc.compatible {
				if got != "" {
					t.Fatalf("legacy or unknown core must keep the original export, got %q", got)
				}
				return
			}
			before, _ := url.Parse(original)
			after, err := url.Parse(got)
			if err != nil || after == nil {
				t.Fatalf("invalid export: %q", got)
			}
			if after.Scheme != before.Scheme || after.User.String() != before.User.String() || after.Host != before.Host || after.Fragment != before.Fragment {
				t.Fatal("export changed the credential, entry address or node name")
			}
			want := before.Query()
			want.Set("fp", "chrome140")
			if !reflect.DeepEqual(after.Query(), want) {
				t.Fatalf("export changed other REALITY parameters: %v", after.Query())
			}
		})
	}
}

func TestShadowrocketExportDoesNotChangeOtherProtocols(t *testing.T) {
	for _, raw := range []string{
		"", "not a node", "vless://host.example.com:443?security=reality",
		"vless://uuid@host.example.com:443?security=tls&fp=ios",
		"vless://uuid@host.example.com:443?security=none",
		"trojan://password@host.example.com:443?security=reality",
		"ss://YWVzLTEyOC1nY206cGFzcw@host.example.com:443",
	} {
		if got := ShadowrocketImportURL(raw, "26.10.10"); got != "" {
			t.Fatalf("unexpected companion URL for %q: %q", raw, got)
		}
	}
}
