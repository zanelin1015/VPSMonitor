package client

import (
	"strings"
	"testing"
)

func TestBuildUpdate3XUICommand(t *testing.T) {
	command := buildUpdate3XUICommand()
	if !strings.Contains(command, "x-ui update") {
		t.Fatalf("expected command to prefer x-ui update, got %q", command)
	}
	if !strings.Contains(command, threeXUIInstallScriptURL) {
		t.Fatalf("expected command to include official install script URL, got %q", command)
	}
}
