package client

import (
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestOpenWrtInstallerURL(t *testing.T) {
	tests := map[string]string{
		"https://example.com/install.sh":          "https://example.com/install-openwrt.sh",
		"https://example.com/install.sh?v=1":      "https://example.com/install-openwrt.sh?v=1",
		"https://example.com/custom-installer.sh": "https://example.com/custom-installer.sh",
	}
	for input, want := range tests {
		if got := openWrtInstallerURL(input); got != want {
			t.Fatalf("openWrtInstallerURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildUnixSelfUpdateCommandUnlinksInstallerBeforeExecution(t *testing.T) {
	tests := []struct {
		name     string
		openWrt  bool
		executor string
	}{
		{name: "linux", executor: "bash -s -- client <&3"},
		{name: "openwrt", openWrt: true, executor: "sh -s -- client <&3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := buildUnixSelfUpdateCommand("https://example.com/install.sh", "v1.2.3", "owner/repo", "VPSMonitor", "/opt/vpsmonitor/client", "vpsmonitor-client", false, "v2.9.4", "", false, test.openWrt)
			removeIndex := strings.Index(command, `rm -f "$tmp"`)
			executeIndex := strings.Index(command, test.executor)
			if removeIndex < 0 || executeIndex < 0 || removeIndex > executeIndex {
				t.Fatalf("expected installer to be unlinked before execution, got %q", command)
			}
			if !strings.Contains(command, `exec 3<"$tmp"`) {
				t.Fatalf("expected installer to remain available through a file descriptor, got %q", command)
			}
		})
	}
}

func TestBuildWindowsSelfUpdateCommandRemovesInstaller(t *testing.T) {
	command := buildWindowsSelfUpdateCommand("https://example.com/install.ps1", "v1.2.3", "owner/repo", "VPSMonitor", `C:\VPSMonitor`, "VPSMonitorClient")
	for _, expected := range []string{"[guid]::NewGuid()", "try {", "finally {", "Remove-Item -Force -ErrorAction SilentlyContinue $scriptPath"} {
		if !strings.Contains(command, expected) {
			t.Fatalf("expected command to contain %q, got %q", expected, command)
		}
	}
}

func TestEnforceExclusiveForwardingMode(t *testing.T) {
	bothEnabled := model.ManagedAgentConfig{
		Entry: model.AgentEntryConfig{
			PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm"},
			HAProxy:        model.HAProxyConfig{Enabled: true},
		},
	}

	haProxyMode := bothEnabled
	haProxyMode.Features = model.AgentFeatureConfig{HAProxy: true}
	haProxyMode = enforceExclusiveForwardingMode(haProxyMode)
	if haProxyMode.Entry.PortForwarding.Enabled || haProxyMode.Entry.PortForwarding.Backend != "none" || !haProxyMode.Entry.HAProxy.Enabled {
		t.Fatalf("HAProxy mode must disable Realm: %#v", haProxyMode.Entry)
	}

	realmMode := bothEnabled
	realmMode.Features = model.AgentFeatureConfig{Realm: true}
	realmMode = enforceExclusiveForwardingMode(realmMode)
	if !realmMode.Entry.PortForwarding.Enabled || realmMode.Entry.HAProxy.Enabled {
		t.Fatalf("Realm mode must disable HAProxy: %#v", realmMode.Entry)
	}
}
