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

func TestAddGenericSingBoxUser(t *testing.T) {
	cfg := map[string]any{"inbounds": []any{map[string]any{"type": "vless", "tag": "vless-in", "listen_port": float64(443)}}}
	if _, err := addUser(cfg, map[string]any{
		"inbound_tag": "vless-in",
		"user":        map[string]any{"name": "alice", "uuid": "uuid-1", "flow": "xtls-rprx-vision"},
	}); err != nil {
		t.Fatalf("add generic user: %v", err)
	}
	inbound := objectList(cfg["inbounds"])[0]
	if users := objectList(inbound["users"]); len(users) != 1 || users[0]["name"] != "alice" {
		t.Fatalf("unexpected users: %#v", inbound["users"])
	}
	if _, err := addUser(cfg, map[string]any{
		"inbound_tag": "vless-in",
		"user":        map[string]any{"name": "alice", "uuid": "uuid-2"},
	}); err == nil {
		t.Fatal("expected duplicate user to be rejected")
	}
	if _, err := deleteUser(cfg, map[string]any{"inbound_tag": "vless-in", "user_key": "name:alice"}); err != nil {
		t.Fatalf("delete generic user: %v", err)
	}
	if users := objectList(objectList(cfg["inbounds"])[0]["users"]); len(users) != 0 {
		t.Fatalf("expected user to be deleted, got %#v", users)
	}
}

func TestSnellUserRequiresUserkey(t *testing.T) {
	cfg := map[string]any{"inbounds": []any{map[string]any{"type": "snell", "tag": "snell-in", "listen_port": float64(20001)}}}
	if _, err := addUser(cfg, map[string]any{"inbound_tag": "snell-in", "user": map[string]any{"name": "alice"}}); err == nil {
		t.Fatal("expected Snell user without userkey to be rejected")
	}
	if _, err := addUser(cfg, map[string]any{"inbound_tag": "snell-in", "user": map[string]any{"name": "alice", "userkey": "user-secret"}}); err != nil {
		t.Fatalf("expected Snell user with userkey to be accepted: %v", err)
	}
}

func TestSnellVersion6Fields(t *testing.T) {
	inbound := map[string]any{"type": "snell", "tag": "snell-v6", "listen_port": float64(20001), "psk": "123456789012"}
	if err := validateClient(inbound); err != nil {
		t.Fatalf("validate Snell v6 inbound: %v", err)
	}
	if inbound["version"] != 5 {
		t.Fatalf("expected legacy default inbound version 5, got %#v", inbound["version"])
	}
	inbound["version"] = 6
	if err := validateClient(inbound); err != nil {
		t.Fatalf("validate Snell v6 inbound: %v", err)
	}
	if inbound["mode"] != "default" {
		t.Fatalf("expected default v6 mode, got %#v", inbound["mode"])
	}
	outbound := map[string]any{"type": "snell", "tag": "snell-out-v6", "server": "198.51.100.1", "server_port": float64(443), "version": float64(6), "psk": "123456789012", "userkey": "user-1", "network": "tcp"}
	if err := validateNode(outbound); err != nil {
		t.Fatalf("validate Snell v6 outbound: %v", err)
	}
	if outbound["mode"] != "default" {
		t.Fatalf("expected default v6 outbound mode, got %#v", outbound["mode"])
	}
	short := map[string]any{"type": "snell", "tag": "bad-v6", "server": "198.51.100.1", "server_port": float64(443), "version": float64(6), "psk": "short"}
	if err := validateNode(short); err == nil {
		t.Fatal("expected short Snell v6 psk to be rejected")
	}
}

func TestOutboundRejectsDomainName(t *testing.T) {
	node := map[string]any{"type": "shadowsocks", "tag": "bad", "server": "example.com", "server_port": float64(443), "method": "aes-128-gcm", "password": "secret"}
	if err := validateNode(node); err == nil {
		t.Fatal("expected outbound domain name to be rejected")
	}
	if err := validateNode(map[string]any{"type": "selector", "tag": "auto", "outbounds": []any{"direct"}}); err != nil {
		t.Fatalf("selector outbound should not require a server: %v", err)
	}
}

func TestForwardingRuleImportsOutboundFromSourceClient(t *testing.T) {
	cfg := map[string]any{"outbounds": []any{}, "route": map[string]any{"rules": []any{}}}
	payload := map[string]any{
		"outbound": map[string]any{"type": "shadowsocks", "tag": "remote-ss", "server": "198.51.100.20", "server_port": float64(20002), "method": "aes-128-gcm", "password": "secret"},
		"rule":     map[string]any{"inbound": []any{"proxy"}, "outbound": "remote-ss"},
	}
	if _, err := addForwardingRule(cfg, payload); err != nil {
		t.Fatalf("add rule with imported outbound: %v", err)
	}
	if len(objectList(cfg["outbounds"])) != 1 || len(objectList(cfg["route"].(map[string]any)["rules"])) != 1 {
		t.Fatalf("expected imported outbound and rule, got %#v", cfg)
	}
	cfg = map[string]any{"outbounds": []any{}, "route": map[string]any{"rules": []any{}}}
	if _, err := addForwardingRule(cfg, map[string]any{
		"outbound": map[string]any{"type": "shadowsocks", "tag": "normalized-tag", "server": "198.51.100.20", "server_port": float64(20002), "method": "aes-128-gcm", "password": "secret"},
		"rule":     map[string]any{"outbound": "stale-generated-tag"},
	}); err != nil {
		t.Fatalf("add rule with normalized imported outbound: %v", err)
	}
	if got := objectList(cfg["route"].(map[string]any)["rules"])[0]["outbound"]; got != "normalized-tag" {
		t.Fatalf("expected route to use imported tag, got %#v", got)
	}
}
