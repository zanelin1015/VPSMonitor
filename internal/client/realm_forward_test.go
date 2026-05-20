package client

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

type fakeRealmFileSystem struct {
	files map[string]string
	modes map[string]os.FileMode
}

func (f *fakeRealmFileSystem) MkdirAll(_ string, _ os.FileMode) error {
	return nil
}

func (f *fakeRealmFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	if f.files == nil {
		f.files = map[string]string{}
	}
	if f.modes == nil {
		f.modes = map[string]os.FileMode{}
	}
	f.files[name] = string(data)
	f.modes[name] = perm
	return nil
}

func (f *fakeRealmFileSystem) Chmod(name string, mode os.FileMode) error {
	if f.modes == nil {
		f.modes = map[string]os.FileMode{}
	}
	f.modes[name] = mode
	return nil
}

func TestRenderRealmConfigIncludesTCPAndUDPForward(t *testing.T) {
	cfg := model.RealmForwardConfig{
		LogLevel: "debug",
		Rules: []model.RealmForwardRule{{
			Enabled:       true,
			ListenAddress: "0.0.0.0",
			ListenPort:    8443,
			TargetAddress: "hk.example.com",
			TargetPort:    443,
			Network:       "both",
		}},
	}
	rendered := renderRealmConfig(cfg)
	for _, want := range []string{
		"[log]",
		`level = "debug"`,
		"[[endpoints]]",
		`listen = "0.0.0.0:8443"`,
		`remote = "hk.example.com:443"`,
		"[endpoints.network]",
		"use_udp = true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in:\n%s", want, rendered)
		}
	}
}

func TestParseRealmConfigRules(t *testing.T) {
	rules, err := parseRealmConfigRules(`
[log]
level = "info"

[[endpoints]]
listen = "0.0.0.0:2443"
remote = "hk.example.com:443"

[[endpoints]]
listen = "[::]:5300"
remote = "10.0.0.2:53"
[endpoints.network]
use_udp = true
no_tcp = true
`)
	if err != nil {
		t.Fatalf("parseRealmConfigRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %#v", rules)
	}
	if rules[0].ListenAddress != "0.0.0.0" || rules[0].ListenPort != 2443 || rules[0].TargetAddress != "hk.example.com" || rules[0].TargetPort != 443 || rules[0].Network != "tcp" {
		t.Fatalf("unexpected first rule: %#v", rules[0])
	}
	if rules[1].ListenAddress != "::" || rules[1].ListenPort != 5300 || rules[1].Network != "udp" {
		t.Fatalf("unexpected udp rule: %#v", rules[1])
	}
}

func TestEmptyRealmForwardConfigIsUnmanaged(t *testing.T) {
	if !isEmptyClientRealmForwardConfig(model.RealmForwardConfig{}) {
		t.Fatal("expected zero realm config to be treated as unmanaged")
	}
	if isEmptyClientRealmForwardConfig(model.RealmForwardConfig{ServiceName: "vpsmonitor-realm"}) {
		t.Fatal("expected explicit realm service to be treated as managed")
	}
}

func TestRealmConfigPathFromCommand(t *testing.T) {
	for _, command := range []string{
		`/usr/local/bin/realm -c /etc/realm/config.toml`,
		`/usr/local/bin/realm --config="/etc/realm/config.toml"`,
		`/usr/local/bin/realm -c='/etc/realm/config.toml'`,
	} {
		if got := realmConfigPathFromCommand(command); got != "/etc/realm/config.toml" {
			t.Fatalf("expected config path from %q, got %q", command, got)
		}
	}
}

func TestConfigPathsFromRealmProcesses(t *testing.T) {
	procRoot := t.TempDir()
	processDir := filepath.Join(procRoot, "1234")
	if err := os.MkdirAll(processDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cmdline := strings.Join([]string{"/usr/local/bin/realm", "-c", "/root/custom-realm.toml"}, "\x00") + "\x00"
	if err := os.WriteFile(filepath.Join(processDir, "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	paths := configPathsFromRealmProcesses(procRoot)
	if len(paths) != 1 || paths[0] != "/root/custom-realm.toml" {
		t.Fatalf("unexpected process config paths: %#v", paths)
	}
}

func TestInstallRealmServiceWritesSystemdUnit(t *testing.T) {
	runner := &fakeCommandRunner{paths: map[string]bool{"systemctl": true}}
	fs := &fakeRealmFileSystem{}
	err := installOrRestartRealmService(context.Background(), "vpsmonitor-realm", "/usr/local/bin/realm", "/etc/vpsmonitor/realm.toml", runner, fs)
	if err != nil {
		t.Fatalf("installOrRestartRealmService: %v", err)
	}
	service := fs.files["/etc/systemd/system/vpsmonitor-realm.service"]
	if !strings.Contains(service, "ExecStart=/usr/local/bin/realm -c /etc/vpsmonitor/realm.toml") {
		t.Fatalf("unexpected service:\n%s", service)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"systemctl daemon-reload",
		"systemctl enable vpsmonitor-realm",
		"systemctl restart vpsmonitor-realm",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command %q in:\n%s", want, joined)
		}
	}
}
