package server

import (
	"strings"
	"testing"
)

func TestBuildServerSelfUpdateCommandUnlinksInstallerBeforeExecution(t *testing.T) {
	command := buildServerSelfUpdateCommand("https://example.com/install.sh", "v1.2.3", "owner/repo", "VPSMonitor", "/opt/vpsmonitor/server", "vpsmonitor-server")
	removeIndex := strings.Index(command, `rm -f "$tmp"`)
	executeIndex := strings.Index(command, `bash -s -- server <&3`)
	if removeIndex < 0 || executeIndex < 0 || removeIndex > executeIndex {
		t.Fatalf("expected installer to be unlinked before execution, got %q", command)
	}
	if !strings.Contains(command, `exec 3<"$tmp"`) {
		t.Fatalf("expected installer to remain available through a file descriptor, got %q", command)
	}
	if !strings.Contains(command, `${VPSMONITOR_TMP_DIR:-/var/tmp}`) {
		t.Fatalf("expected /var/tmp-oriented temporary storage with override support, got %q", command)
	}
}
