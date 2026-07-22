package server

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestAuthorizedOutboundEndpointMatchingSupportedProtocols(t *testing.T) {
	vmessPayload, _ := json.Marshal(map[string]any{
		"add":  "vmess.example.com",
		"port": 8443,
		"id":   "22222222-2222-2222-2222-222222222222",
		"aid":  0,
		"scy":  "auto",
		"net":  "tcp",
	})
	ssUser := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:ss-secret"))
	tests := []struct {
		name           string
		importURL      string
		sourceProtocol string
		outbound       map[string]any
	}{
		{
			name:           "vless",
			importURL:      "vless://11111111-1111-1111-1111-111111111111@vless.example.com:443?security=tls&type=tcp",
			sourceProtocol: "vless",
			outbound: map[string]any{
				"protocol": "vless",
				"settings": map[string]any{"address": "vless.example.com", "port": 443, "id": "11111111-1111-1111-1111-111111111111"},
			},
		},
		{
			name:           "vmess",
			importURL:      "vmess://" + base64.RawStdEncoding.EncodeToString(vmessPayload),
			sourceProtocol: "vmess",
			outbound: map[string]any{
				"protocol": "vmess",
				"settings": map[string]any{"vnext": []any{map[string]any{
					"address": "vmess.example.com",
					"port":    8443,
					"users":   []any{map[string]any{"id": "22222222-2222-2222-2222-222222222222"}},
				}}},
			},
		},
		{
			name:           "socks",
			importURL:      "socks://proxy-user:socks-secret@socks.example.com:1080",
			sourceProtocol: "socks5",
			outbound: map[string]any{
				"protocol": "socks",
				"settings": map[string]any{"servers": []any{map[string]any{
					"address": "socks.example.com",
					"port":    1080,
					"users":   []any{map[string]any{"user": "proxy-user", "pass": "socks-secret"}},
				}}},
			},
		},
		{
			name:           "http",
			importURL:      "http://proxy-user:http-secret@http.example.com:8080",
			sourceProtocol: "http",
			outbound: map[string]any{
				"protocol": "http",
				"settings": map[string]any{"servers": []any{map[string]any{
					"address": "http.example.com",
					"port":    8080,
					"users":   []any{map[string]any{"user": "proxy-user", "pass": "http-secret"}},
				}}},
			},
		},
		{
			name:           "shadowsocks",
			importURL:      "ss://" + ssUser + "@ss.example.com:8388",
			sourceProtocol: "shadowsocks",
			outbound: map[string]any{
				"protocol": "shadowsocks",
				"settings": map[string]any{"servers": []any{map[string]any{
					"address":  "ss.example.com",
					"port":     8388,
					"method":   "aes-256-gcm",
					"password": "ss-secret",
				}}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected, ok := outboundEndpointFromImportURL(test.importURL)
			if !ok {
				t.Fatalf("parse import URL %q", test.importURL)
			}
			requested, ok := outboundEndpointFromConfig(test.outbound)
			if !ok {
				t.Fatalf("parse outbound: %#v", test.outbound)
			}
			if !outboundEndpointsMatch(requested, expected, test.sourceProtocol) {
				t.Fatalf("expected endpoint match, requested=%#v expected=%#v", requested, expected)
			}
			requested.Address = "unauthorized.example.com"
			if outboundEndpointsMatch(requested, expected, test.sourceProtocol) {
				t.Fatal("expected modified endpoint to be rejected")
			}
		})
	}
}
