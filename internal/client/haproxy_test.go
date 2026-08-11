package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

type fakeHAProxyFileSystem struct {
	files map[string]string
	modes map[string]os.FileMode
}

func (f *fakeHAProxyFileSystem) MkdirAll(string, os.FileMode) error { return nil }
func (f *fakeHAProxyFileSystem) WriteFile(name string, data []byte, mode os.FileMode) error {
	if f.files == nil {
		f.files = make(map[string]string)
	}
	if f.modes == nil {
		f.modes = make(map[string]os.FileMode)
	}
	f.files[name] = string(data)
	f.modes[name] = mode
	return nil
}
func (f *fakeHAProxyFileSystem) Rename(oldPath, newPath string) error {
	f.files[newPath] = f.files[oldPath]
	delete(f.files, oldPath)
	return nil
}
func (f *fakeHAProxyFileSystem) Remove(name string) error {
	delete(f.files, name)
	return nil
}
func (f *fakeHAProxyFileSystem) Chmod(name string, mode os.FileMode) error {
	if f.modes == nil {
		f.modes = make(map[string]os.FileMode)
	}
	f.modes[name] = mode
	return nil
}

func TestRenderHAProxyConfigUsesOrderedBackups(t *testing.T) {
	cfg := normalizedHAProxyTestConfig()
	rendered := renderHAProxyConfig(cfg)
	for _, expected := range []string{
		"stats socket /run/vpsmonitor-haproxy.sock mode 660 level admin",
		"frontend vpsm_20001_gz-entry_frontend",
		"bind 0.0.0.0:20001",
		"default-server inter 3s fall 3 rise 2",
		"server primary_hk-b_realm-20001 hk-b.example.com:20001 check",
		"server backup_1_hk-c_realm-20001 hk-c.example.com:20001 check backup",
		"server backup_2_hk-e_realm-20001 hk-e.example.com:20001 check backup",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected %q in HAProxy config:\n%s", expected, rendered)
		}
	}
	if strings.Index(rendered, "backup_1_hk-c") > strings.Index(rendered, "backup_2_hk-e") {
		t.Fatalf("backup order was not preserved:\n%s", rendered)
	}
}

func TestHAProxyRuntimeStatusSelectsFirstHealthyBackup(t *testing.T) {
	cfg := normalizedHAProxyTestConfig()
	body := strings.Join([]string{
		"# pxname,svname,scur,stot,status,lastchg,downtime,type,check_status,check_duration,last_chk",
		"vpsm_20001_gz-entry_backend,primary_hk-b_realm-20001,0,12,DOWN,9,9,2,L4CON,1,Connection refused",
		"vpsm_20001_gz-entry_backend,backup_1_hk-c_realm-20001,3,21,UP,8,0,2,L4OK,2,",
		"vpsm_20001_gz-entry_backend,backup_2_hk-e_realm-20001,0,5,UP,30,0,2,L4OK,2,",
	}, "\n") + "\n"
	stats, err := parseHAProxyRuntimeStats([]byte(body))
	if err != nil {
		t.Fatalf("parseHAProxyRuntimeStats: %v", err)
	}
	snapshot := newHAProxySnapshot(cfg)
	populateHAProxySnapshot(cfg, snapshot, stats)
	if len(snapshot.Rules) != 1 {
		t.Fatalf("expected one rule, got %#v", snapshot.Rules)
	}
	rule := snapshot.Rules[0]
	if rule.Status != "backup" || rule.ActiveRole != "backup" || rule.ActiveBackupIndex != 1 || rule.ActiveAgentID != "hk-c" {
		t.Fatalf("expected first healthy backup to take over, got %#v", rule)
	}
	if rule.Targets[0].Healthy || rule.Targets[0].Status != "DOWN" || rule.Targets[0].CheckDescription != "Connection refused" {
		t.Fatalf("unexpected primary status: %#v", rule.Targets[0])
	}
	if !rule.Targets[1].Active || rule.Targets[1].CurrentSessions != 3 || rule.Targets[1].TotalSessions != 21 {
		t.Fatalf("unexpected active backup status: %#v", rule.Targets[1])
	}
	if rule.Targets[2].Active || !rule.Targets[2].Healthy {
		t.Fatalf("second healthy backup should remain standby: %#v", rule.Targets[2])
	}
}

func TestHAProxyRuntimeStatusPrefersHealthyPrimary(t *testing.T) {
	cfg := normalizedHAProxyTestConfig()
	body := strings.Join([]string{
		"# pxname,svname,status,type",
		"vpsm_20001_gz-entry_backend,primary_hk-b_realm-20001,UP,2",
		"vpsm_20001_gz-entry_backend,backup_1_hk-c_realm-20001,UP,2",
		"vpsm_20001_gz-entry_backend,backup_2_hk-e_realm-20001,UP,2",
	}, "\n") + "\n"
	stats, err := parseHAProxyRuntimeStats([]byte(body))
	if err != nil {
		t.Fatalf("parseHAProxyRuntimeStats: %v", err)
	}
	snapshot := newHAProxySnapshot(cfg)
	populateHAProxySnapshot(cfg, snapshot, stats)
	rule := snapshot.Rules[0]
	if rule.Status != "primary" || rule.ActiveRole != "primary" || !rule.Targets[0].Active {
		t.Fatalf("expected primary to handle new connections, got %#v", rule)
	}
}

func TestQueryHAProxyRuntimeStatsRetriesDuringReload(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "vpsm-haproxy-")
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	socketPath := tempDir + "/haproxy.sock"
	serverResult := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			serverResult <- err
			return
		}
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer conn.Close()
		request := make([]byte, 32)
		if _, err := conn.Read(request); err != nil {
			serverResult <- err
			return
		}
		if !strings.Contains(string(request), "show stat") {
			serverResult <- fmt.Errorf("unexpected runtime request %q", request)
			return
		}
		_, err = io.WriteString(conn, "# pxname,svname,status,type\n")
		serverResult <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body, err := queryHAProxyRuntimeStats(ctx, socketPath)
	if err != nil {
		t.Fatalf("queryHAProxyRuntimeStats: %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatalf("runtime socket server: %v", err)
	}
	if !strings.Contains(string(body), "pxname") {
		t.Fatalf("unexpected runtime response %q", body)
	}
}

func TestQueryHAProxyRuntimeStatsRetriesWhenReloadDropsConnection(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "vpsm-haproxy-")
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	socketPath := tempDir + "/haproxy.sock"
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on runtime socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	serverResult := make(chan error, 1)
	go func() {
		first, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		_ = first.Close()

		second, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer second.Close()
		request := make([]byte, 32)
		if _, err := second.Read(request); err != nil {
			serverResult <- err
			return
		}
		_, err = io.WriteString(second, "# pxname,svname,status,type\n")
		serverResult <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body, err := queryHAProxyRuntimeStats(ctx, socketPath)
	if err != nil {
		t.Fatalf("queryHAProxyRuntimeStats: %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatalf("runtime socket server: %v", err)
	}
	if !strings.Contains(string(body), "pxname") {
		t.Fatalf("unexpected runtime response %q", body)
	}
}

func TestQueryHAProxyRuntimeStatsRetriesTruncatedReloadResponse(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "vpsm-haproxy-")
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	socketPath := tempDir + "/haproxy.sock"
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on runtime socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	serverResult := make(chan error, 1)
	go func() {
		first, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		request := make([]byte, 32)
		if _, err := first.Read(request); err != nil {
			_ = first.Close()
			serverResult <- err
			return
		}
		_, err = io.WriteString(first, "# pxname,svname")
		_ = first.Close()
		if err != nil {
			serverResult <- err
			return
		}

		second, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer second.Close()
		if _, err := second.Read(request); err != nil {
			serverResult <- err
			return
		}
		_, err = io.WriteString(second, "# pxname,svname,status,type\n")
		serverResult <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body, err := queryHAProxyRuntimeStats(ctx, socketPath)
	if err != nil {
		t.Fatalf("queryHAProxyRuntimeStats: %v", err)
	}
	if !strings.Contains(string(body), "status,type") {
		t.Fatalf("expected complete response after retry, got %q", body)
	}
	if err := <-serverResult; err != nil {
		t.Fatalf("runtime socket server: %v", err)
	}
}

func TestApplyHAProxyValidatesThenReloadsManagedService(t *testing.T) {
	previousGOOS := haProxyRuntimeGOOS
	haProxyRuntimeGOOS = "linux"
	defer func() { haProxyRuntimeGOOS = previousGOOS }()
	cfg := normalizedHAProxyTestConfig()
	runner := &fakeCommandRunner{paths: map[string]bool{"haproxy": true, "systemctl": true}}
	fs := &fakeHAProxyFileSystem{}
	if err := applyHAProxy(context.Background(), cfg, runner, fs); err != nil {
		t.Fatalf("applyHAProxy: %v", err)
	}
	config := fs.files[defaultHAProxyConfigPath]
	if !strings.Contains(config, "hk-b.example.com:20001") {
		t.Fatalf("managed config was not atomically installed: %q", config)
	}
	service := fs.files["/etc/systemd/system/vpsmonitor-haproxy.service"]
	if !strings.Contains(service, "ExecReload=/bin/kill -USR2 $MAINPID") {
		t.Fatalf("systemd service does not support seamless reload:\n%s", service)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, expected := range []string{
		"/usr/sbin/haproxy -c -f /etc/vpsmonitor/haproxy.cfg.tmp",
		"systemctl enable vpsmonitor-haproxy",
		"systemctl reload vpsmonitor-haproxy",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %q in commands:\n%s", expected, joined)
		}
	}
}

func TestValidateClientHAProxyRulesRejectsDuplicateTarget(t *testing.T) {
	cfg := normalizedHAProxyTestConfig()
	cfg.Rules[0].Backups = append(cfg.Rules[0].Backups, cfg.Rules[0].Primary)
	if err := validateClientHAProxyRules(cfg.Rules); err == nil {
		t.Fatal("expected duplicate HAProxy target to be rejected")
	}
}

func normalizedHAProxyTestConfig() model.HAProxyConfig {
	return normalizeClientHAProxyConfig(model.HAProxyConfig{
		Enabled: true,
		Rules: []model.HAProxyRule{{
			ID:                    "gz-entry",
			Name:                  "GZ entry",
			Enabled:               true,
			ListenAddress:         "0.0.0.0",
			ListenPort:            20001,
			CheckIntervalSeconds:  3,
			ConnectTimeoutSeconds: 5,
			Fall:                  3,
			Rise:                  2,
			Primary: model.HAProxyRealmTarget{
				AgentID: "hk-b", RealmRuleID: "realm-20001", Address: "hk-b.example.com", Port: 20001,
			},
			Backups: []model.HAProxyRealmTarget{
				{AgentID: "hk-c", RealmRuleID: "realm-20001", Address: "hk-c.example.com", Port: 20001},
				{AgentID: "hk-e", RealmRuleID: "realm-20001", Address: "hk-e.example.com", Port: 20001},
			},
		}},
	})
}
