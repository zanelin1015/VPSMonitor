package client

import (
	"os/exec"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestBuildUnixSelfUpdateCommandUsesVerifiedPackageWithoutRemoteScript(t *testing.T) {
	tests := []struct {
		name    string
		openWrt bool
	}{
		{name: "linux"},
		{name: "openwrt", openWrt: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := buildUnixSelfUpdateCommand("https://example.com/client.tar.gz", "/opt/vpsmonitor/client", "vpsmonitor-client", strings.Repeat("a", 64), test.openWrt)
			for _, forbidden := range []string{"install.sh", "raw.githubusercontent.com", "bash -s -- client", "sh -s -- client"} {
				if strings.Contains(command, forbidden) {
					t.Fatalf("unexpected remote installer execution %q in %q", forbidden, command)
				}
			}
			if !strings.Contains(command, "sha256sum") || !strings.Contains(command, "client package SHA-256 mismatch") {
				t.Fatalf("expected verified package flow, got %q", command)
			}
			if output, err := exec.Command("sh", "-n", "-c", command).CombinedOutput(); err != nil {
				t.Fatalf("generated shell update command is invalid: %v\n%s", err, output)
			}
		})
	}
}

func TestBuildWindowsSelfUpdateCommandUsesVerifiedPackageWithoutRemoteScript(t *testing.T) {
	command := buildWindowsSelfUpdateCommand("https://example.com/client.zip", `C:\VPSMonitor`, "VPSMonitorClient", strings.Repeat("b", 64))
	for _, expected := range []string{"[guid]::NewGuid()", "Get-FileHash -Algorithm SHA256", "Expand-Archive", "Remove-Item -Recurse -Force"} {
		if !strings.Contains(command, expected) {
			t.Fatalf("expected command to contain %q, got %q", expected, command)
		}
	}
	for _, forbidden := range []string{"install.ps1", "raw.githubusercontent.com", "-File $scriptPath"} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("unexpected remote installer execution %q in %q", forbidden, command)
		}
	}
}

func TestVerifiedUpdatePackageRequiresOfficialPinnedRelease(t *testing.T) {
	url, err := verifiedUpdatePackageURL("zanelin1015/VPSMonitor", "v0.3.24", "VPSMonitor-client-linux-amd64.tar.gz")
	if err != nil || url != "https://github.com/zanelin1015/VPSMonitor/releases/download/v0.3.24/VPSMonitor-client-linux-amd64.tar.gz" {
		t.Fatalf("verifiedUpdatePackageURL() = %q, %v", url, err)
	}
	if _, err := verifiedUpdatePackageURL("other/repo", "v0.3.24", "VPSMonitor-client-linux-amd64.tar.gz"); err == nil {
		t.Fatal("custom repository must be rejected")
	}
	if _, err := verifiedUpdatePackageURL("zanelin1015/VPSMonitor", "main", "VPSMonitor-client-linux-amd64.tar.gz"); err == nil {
		t.Fatal("mutable branch must be rejected")
	}
	if _, err := requiredUpdatePackageSHA256(strings.Repeat("a", 64)); err != nil {
		t.Fatalf("valid digest rejected: %v", err)
	}
	if _, err := requiredUpdatePackageSHA256("not-a-digest"); err == nil {
		t.Fatal("invalid digest must be rejected")
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
