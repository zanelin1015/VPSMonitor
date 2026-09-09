package server

import (
	"os/exec"
	"strings"
	"testing"
)

func TestBuildServerSelfUpdateCommandUsesVerifiedPackageWithoutRemoteScript(t *testing.T) {
	command := buildServerSelfUpdateCommand("https://example.com/server.tar.gz", "/opt/vpsmonitor/server", "vpsmonitor-server", strings.Repeat("a", 64))
	if !strings.Contains(command, `${VPSMONITOR_TMP_DIR:-/var/tmp}`) {
		t.Fatalf("expected /var/tmp-oriented temporary storage with override support, got %q", command)
	}
	for _, forbidden := range []string{"install.sh", "raw.githubusercontent.com", "bash -s -- server"} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("unexpected remote installer execution %q in %q", forbidden, command)
		}
	}
	if !strings.Contains(command, "sha256sum") || !strings.Contains(command, "server package SHA-256 mismatch") {
		t.Fatalf("expected verified package flow, got %q", command)
	}
	if output, err := exec.Command("sh", "-n", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("generated shell update command is invalid: %v\n%s", err, output)
	}
}
