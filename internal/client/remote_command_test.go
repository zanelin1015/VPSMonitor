package client

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestExecuteRemoteCommand(t *testing.T) {
	payload := map[string]any{
		"command":         "printf remote-ok",
		"shell":           "sh",
		"timeout_seconds": 5,
	}
	if runtime.GOOS == "windows" {
		payload["command"] = "echo remote-ok"
		payload["shell"] = "cmd"
	}

	result, err := executeRemoteCommand(context.Background(), payload)
	if err != nil {
		t.Fatalf("executeRemoteCommand: %v; result=%#v", err, result)
	}
	if got := strings.TrimSpace(stringFromAny(result["stdout"])); got != "remote-ok" {
		t.Fatalf("unexpected stdout %q; result=%#v", got, result)
	}
	if result["exit_code"] != 0 {
		t.Fatalf("expected exit_code 0, got %#v", result["exit_code"])
	}
}

func TestExecuteRemoteCommandRejectsEmptyCommand(t *testing.T) {
	_, err := executeRemoteCommand(context.Background(), map[string]any{"command": "  "})
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("expected command required error, got %v", err)
	}
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
