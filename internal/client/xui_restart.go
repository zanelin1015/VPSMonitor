package client

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const maxXUIRestartLogLength = 12000

func restartXUIService(ctx context.Context, payload map[string]any) (map[string]any, error) {
	result := map[string]any{
		"mode":       "websocket",
		"started_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if runtime.GOOS == "windows" {
		err := fmt.Errorf("x-ui service restart is only supported on linux clients")
		result["logs"] = err.Error()
		return result, err
	}

	commands := restartCommandCandidates(payload)
	if len(commands) == 0 {
		err := fmt.Errorf("no x-ui restart command found: systemctl/service/x-ui are unavailable")
		result["logs"] = err.Error()
		return result, err
	}

	var attempts []map[string]any
	var lastErr error
	for _, command := range commands {
		attempt, err := runRestartCommand(ctx, command)
		attempts = append(attempts, attempt)
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
	}
	result["attempts"] = attempts

	status, statusErr := collectXUIRestartStatus(ctx)
	result["status"] = status
	if statusErr != nil && lastErr == nil {
		lastErr = statusErr
	}
	if lastErr != nil {
		logs := collectXUIRestartLogs(ctx)
		result["logs"] = truncateLog(logs, maxXUIRestartLogLength)
		return result, fmt.Errorf("restart x-ui failed: %w", lastErr)
	}

	result["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	result["restarted"] = true
	return result, nil
}

func restartCommandCandidates(payload map[string]any) [][]string {
	var commands [][]string
	if commandExists("systemctl") {
		commands = append(commands, []string{"systemctl", "restart", payloadString(payload, "service_name", "x-ui")})
	}
	if commandExists("service") {
		commands = append(commands, []string{"service", payloadString(payload, "service_name", "x-ui"), "restart"})
	}
	if commandExists("x-ui") {
		commands = append(commands, []string{"x-ui", "restart"})
	}
	return commands
}

func runRestartCommand(ctx context.Context, command []string) (map[string]any, error) {
	started := time.Now()
	output, err := runCommandSliceOutput(ctx, command)
	attempt := map[string]any{
		"command":  strings.Join(command, " "),
		"duration": time.Since(started).String(),
		"output":   truncateLog(output, 4000),
	}
	if err != nil {
		attempt["error"] = err.Error()
	}
	return attempt, err
}

func collectXUIRestartStatus(ctx context.Context) (map[string]any, error) {
	status := map[string]any{}
	var errs []string

	if commandExists("systemctl") {
		if output, err := runCommandOutput(ctx, "systemctl", "is-active", "x-ui"); err == nil {
			status["x_ui_state"] = strings.TrimSpace(output)
		} else {
			status["x_ui_state"] = strings.TrimSpace(output)
			errs = append(errs, "x-ui service is not active")
		}
	}
	if commandExists("x-ui") {
		output, err := runCommandOutput(ctx, "x-ui", "status")
		status["x_ui_status"] = truncateLog(output, 4000)
		if err == nil && (strings.Contains(strings.ToLower(output), "xray state: stopped") || strings.Contains(strings.ToLower(output), "xray state: not")) {
			errs = append(errs, "xray is not running after x-ui restart")
		}
	}
	if commandExists("pgrep") {
		if output, err := runCommandOutput(ctx, "pgrep", "-x", "xray"); err == nil && strings.TrimSpace(output) != "" {
			status["xray_pid"] = strings.TrimSpace(output)
		}
	}
	if len(errs) > 0 {
		return status, errors.New(strings.Join(errs, "; "))
	}
	return status, nil
}

func collectXUIRestartLogs(ctx context.Context) string {
	var parts []string
	for _, command := range [][]string{
		{"journalctl", "-u", "x-ui", "-n", "120", "--no-pager"},
		{"journalctl", "-u", "xray", "-n", "80", "--no-pager"},
		{"x-ui", "status"},
	} {
		if !commandExists(command[0]) {
			continue
		}
		output, err := runCommandSliceOutput(ctx, command)
		header := "$ " + strings.Join(command, " ")
		if err != nil {
			parts = append(parts, header+"\n"+output+"\nERROR: "+err.Error())
			continue
		}
		parts = append(parts, header+"\n"+output)
	}
	return strings.Join(parts, "\n\n")
}

func runCommandOutput(ctx context.Context, name string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, name, args...)
	output, err := cmd.CombinedOutput()
	if commandCtx.Err() != nil {
		return string(output), commandCtx.Err()
	}
	return string(output), err
}

func runCommandSliceOutput(ctx context.Context, command []string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("empty command")
	}
	return runCommandOutput(ctx, command[0], command[1:]...)
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func truncateLog(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}
