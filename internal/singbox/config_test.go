package singbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bridge-core/internal/config"
)

func TestCollectNormalizesSingBoxSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := map[string]any{
		"log":       map[string]any{"level": "info"},
		"outbounds": []any{map[string]any{"type": "snell", "tag": "line-hk"}},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "proxy", "listen_port": 1080}},
		"route":     map[string]any{"rules": []any{map[string]any{"domain": []any{"example.com"}, "outbound": "line-hk"}}},
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := NewManager(config.SingBoxConfig{ConfigPath: path, ServiceName: "no-such-service"}).Collect(context.Background())
	if snapshot.Error != "" || len(snapshot.Nodes) != 1 || len(snapshot.Clients) != 1 || len(snapshot.ForwardingRules) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestAddSnellNodeAndForwardingRule(t *testing.T) {
	cfg := map[string]any{"outbounds": []any{}, "inbounds": []any{}}
	node := map[string]any{"type": "snell", "tag": "dedicated", "server": "198.51.100.10", "server_port": float64(443), "psk": "secret"}
	if _, err := addObject(cfg, "outbounds", map[string]any{"node": node}, "node", validateNode); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if version := objectList(cfg["outbounds"])[0]["version"]; version != 4 {
		t.Fatalf("expected default Snell version 4, got %#v", version)
	}
	if _, err := addForwardingRule(cfg, map[string]any{"rule": map[string]any{"domain": []any{"example.com"}, "outbound": "dedicated"}}); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if _, err := addForwardingRule(cfg, map[string]any{"rule": map[string]any{"outbound": "missing"}}); err == nil {
		t.Fatal("expected unknown outbound to be rejected")
	}
}

func TestAddSnellClientRequiresOneUniquePort(t *testing.T) {
	cfg := map[string]any{"inbounds": []any{}}
	client := map[string]any{
		"type":        "snell",
		"remark":      "snell-27411",
		"tag":         "snell-27411",
		"listen":      "::",
		"listen_port": float64(27411),
		"psk":         "secret",
	}
	if _, err := addObject(cfg, "inbounds", map[string]any{"client": client}, "client", validateClient); err != nil {
		t.Fatalf("add Snell client: %v", err)
	}
	duplicate := map[string]any{
		"type":        "snell",
		"tag":         "snell-duplicate",
		"listen":      "::",
		"listen_port": float64(27411),
		"psk":         "secret",
	}
	if _, err := addObject(cfg, "inbounds", map[string]any{"client": duplicate}, "client", validateClient); err == nil {
		t.Fatal("expected duplicate listen_port to be rejected")
	}
}

func TestOutboundRejectsDomainName(t *testing.T) {
	node := map[string]any{"type": "shadowsocks", "tag": "bad", "server": "example.com", "server_port": float64(443), "method": "aes-128-gcm", "password": "secret"}
	if err := validateNode(node); err == nil {
		t.Fatal("expected outbound domain name to be rejected")
	}
}
