package dashboard

import (
	"bridge-core/internal/model"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestMappedNodeExportsKeepInternalPortAndCredentials(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks", "hysteria2", "http", "socks"} {
		t.Run(protocol, func(t *testing.T) {
			settings := `{"method":"aes-128-gcm","clients":[{"id":"uuid-7","email":"alice","password":"secret","enable":true}]}`
			if protocol == "http" || protocol == "socks" {
				settings = `{"accounts":[{"user":"alice","pass":"secret"}]}`
			}
			snapshot := model.AgentSnapshot{AgentID: "node", XUI: &model.XUISnapshot{Inbounds: []map[string]any{{"id": 7, "port": 20001, "protocol": protocol, "settings": settings, "streamSettings": `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"tls.example.com"}}`}}}}
			entry := model.AgentEntryConfig{Connection: model.EntryConnectionConfig{Deployment: "docker", DockerNetwork: "bridge", PublicAccess: "nat", PublicAddress: "entry.example.com"}, Mappings: []model.AgentEntryMapping{{UseForExport: true, TargetType: "xui", TargetID: "7", InternalPort: 20001, HostPort: 30001, ExternalPort: 333, Transport: "both"}}}
			overview := BuildXUIOverviewWithOptions(snapshot, XUIOverviewOptions{Entry: entry})
			if len(overview.Clients) != 1 || overview.Nodes[0].Port != 20001 {
				t.Fatalf("internal identity changed: %+v", overview)
			}
			raw := overview.Clients[0].ImportURL
			if protocol == "vmess" {
				data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "vmess://"))
				if err != nil {
					t.Fatal(err)
				}
				var payload map[string]any
				if err = json.Unmarshal(data, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["add"] != "entry.example.com" || payload["port"] != float64(333) || payload["id"] != "uuid-7" {
					t.Fatalf("wrong VMess export: %s", data)
				}
			} else {
				parsed, err := url.Parse(raw)
				if err != nil || parsed.Host != "entry.example.com:333" {
					t.Fatalf("wrong export %q: %v", raw, err)
				}
				if protocol == "vless" && (parsed.Query().Get("sni") != "tls.example.com" || parsed.User.Username() != "uuid-7") {
					t.Fatal("TLS/UUID changed")
				}
			}
			entry.Mappings[0].InternalPort = 20002
			overview = BuildXUIOverviewWithOptions(snapshot, XUIOverviewOptions{Entry: entry})
			if overview.Clients[0].ImportURL != "" || overview.Clients[0].ShadowrocketImportURL != "" || overview.Clients[0].ExportEndpoint.Error == "" {
				t.Fatal("stale mapping still exported")
			}
		})
	}
}
