package server

import (
	"testing"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

func Test3XUIUpdateUnknownVersionIsEligibleForClientSideCheck(t *testing.T) {
	agent := model.AgentRecord{
		AgentID: "agent-1",
		OS:      "linux",
		Config: model.ManagedAgentConfig{
			XUI: config.XUIConfig{Enabled: true},
		},
	}
	status := build3XUIUpdateAgentStatus(agent, model.AgentSnapshot{OS: "linux"}, "3.0.2")
	if !status.Supported {
		t.Fatalf("expected linux x-ui agent to be supported, got %#v", status)
	}
	if status.UpdateAvailable {
		t.Fatalf("unknown snapshot version should not be marked update-available before client-side check")
	}
	if !shouldCreate3XUIUpdateAction(status, false) {
		t.Fatalf("expected unknown version to receive a task so client can detect and compare local 3x-ui version")
	}
}

func Test3XUIUpdateForceAllowsUpToDateVersion(t *testing.T) {
	agent := model.AgentRecord{
		AgentID: "agent-1",
		OS:      "linux",
		Config: model.ManagedAgentConfig{
			XUI: config.XUIConfig{Enabled: true},
		},
	}
	status := build3XUIUpdateAgentStatus(agent, model.AgentSnapshot{OS: "linux", XUI: &model.XUISnapshot{AppVersion: "3.0.2"}}, "3.0.2")
	if status.UpdateAvailable {
		t.Fatalf("same version should not be marked update-available")
	}
	if shouldCreate3XUIUpdateAction(status, false) {
		t.Fatalf("same version should not receive a normal update task")
	}
	if !shouldCreate3XUIUpdateAction(status, true) {
		t.Fatalf("force update should create a task even when versions match")
	}
}

func TestUpdateClientPackageNameSupportsPublishedArchitectures(t *testing.T) {
	tests := []struct {
		osName string
		arch   string
		want   string
	}{
		{osName: "linux", arch: "amd64", want: "VPSMonitor-client-linux-amd64.tar.gz"},
		{osName: "linux", arch: "arm64", want: "VPSMonitor-client-linux-arm64.tar.gz"},
		{osName: "linux", arch: "arm", want: "VPSMonitor-client-linux-arm.tar.gz"},
		{osName: "windows", arch: "amd64", want: "VPSMonitor-client-windows-amd64.zip"},
		{osName: "windows", arch: "arm64", want: "VPSMonitor-client-windows-arm64.zip"},
	}
	for _, test := range tests {
		got, ok := updateClientPackageName("VPSMonitor", test.osName, test.arch)
		if !ok || got != test.want {
			t.Fatalf("updateClientPackageName(%q, %q) = %q, %v; want %q, true", test.osName, test.arch, got, ok, test.want)
		}
	}
}
