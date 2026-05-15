package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const maxRemoteCommandOutputLength = 65536

func executeRemoteCommand(ctx context.Context, payload map[string]any) (map[string]any, error) {
	command := strings.TrimSpace(payloadString(payload, "command", ""))
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}
	timeoutSeconds := remoteCommandPayloadInt(payload["timeout_seconds"])
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}
	if timeoutSeconds > 600 {
		timeoutSeconds = 600
	}
	shell := normalizeRemoteShell(payloadString(payload, "shell", ""))
	name, args, err := remoteShellCommand(shell, command)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	commandCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	duration := time.Since(started)

	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if commandCtx.Err() != nil {
		err = commandCtx.Err()
	}

	result := map[string]any{
		"command":         command,
		"shell":           shell,
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"timeout_seconds": timeoutSeconds,
		"duration":        duration.String(),
		"exit_code":       exitCode,
		"stdout":          truncateRemoteCommandOutput(stdout.String()),
		"stderr":          truncateRemoteCommandOutput(stderr.String()),
	}
	if current, userErr := user.Current(); userErr == nil {
		result["run_as"] = current.Username
		result["uid"] = current.Uid
	}
	if err != nil {
		return result, fmt.Errorf("command exited with code %d: %w", exitCode, err)
	}
	return result, nil
}

func remoteCommandPayloadInt(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		return 0
	}
}

func normalizeRemoteShell(shell string) string {
	shell = strings.ToLower(strings.TrimSpace(shell))
	switch shell {
	case "cmd", "powershell", "pwsh", "sh", "bash":
		return shell
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	if commandExists("bash") {
		return "bash"
	}
	return "sh"
}

func remoteShellCommand(shell string, command string) (string, []string, error) {
	switch shell {
	case "powershell":
		return "powershell.exe", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command}, nil
	case "pwsh":
		return "pwsh", []string{"-NoProfile", "-Command", command}, nil
	case "cmd":
		return "cmd.exe", []string{"/C", command}, nil
	case "bash":
		return "bash", []string{"-lc", command}, nil
	case "sh":
		return "sh", []string{"-c", command}, nil
	default:
		return "", nil, fmt.Errorf("unsupported shell: %s", shell)
	}
}

func truncateRemoteCommandOutput(value string) string {
	if len(value) <= maxRemoteCommandOutputLength {
		return value
	}
	return value[len(value)-maxRemoteCommandOutputLength:]
}
