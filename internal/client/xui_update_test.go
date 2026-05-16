package client

import (
	"strings"
	"testing"
)

func TestBuildUpdate3XUICommand(t *testing.T) {
	command := buildUpdate3XUICommand()
	if strings.Contains(command, "x-ui update;") || strings.Contains(command, "x-ui update\n") {
		t.Fatalf("expected command to avoid the interactive x-ui update wrapper, got %q", command)
	}
	if strings.Contains(command, "install.sh") {
		t.Fatalf("expected command to avoid the installer fallback, got %q", command)
	}
	if !strings.Contains(command, threeXUIUpdateScriptURL) {
		t.Fatalf("expected command to include official updater script URL, got %q", command)
	}
	if !strings.Contains(command, "mktemp") {
		t.Fatalf("expected command to download updater to a temporary file, got %q", command)
	}
}
