package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func update3XUI(ctx context.Context, payload map[string]any) (map[string]any, error) {
	_ = ctx
	targetVersion := normalize3XUIVersion(payloadString(payload, "target_version", ""))
	force := payloadBool(payload, "force", false)
	result := map[string]any{
		"action":         "update_3xui",
		"target_version": targetVersion,
		"force":          force,
		"status":         "rejected",
		"reason":         "the upstream 3x-ui updater does not publish a verifiable package digest",
	}
	return result, fmt.Errorf("automatic 3x-ui updates are disabled because the upstream updater does not publish a verifiable package digest")
}

func detectLocal3XUIVersion(ctx context.Context) string {
	candidates := [][]string{}
	if commandExists("x-ui") {
		candidates = append(candidates, []string{"x-ui", "version"})
	}
	for _, path := range []string{"/usr/local/x-ui/x-ui", "/usr/bin/x-ui"} {
		if _, err := os.Stat(path); err == nil {
			candidates = append(candidates, []string{path, "version"})
		}
	}
	for _, command := range candidates {
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		output, err := exec.CommandContext(runCtx, command[0], command[1:]...).CombinedOutput()
		cancel()
		if err == nil && runCtx.Err() == nil {
			if version := normalize3XUIVersion(string(output)); version != "" {
				return version
			}
		}
	}
	return ""
}

func normalize3XUIVersion(value string) string {
	value = strings.TrimSpace(value)
	for start := 0; start < len(value); start++ {
		if value[start] < '0' || value[start] > '9' {
			continue
		}
		end := start
		dots := 0
		for end < len(value) {
			ch := value[end]
			if ch == '.' {
				dots++
				end++
				continue
			}
			if ch < '0' || ch > '9' {
				break
			}
			end++
		}
		if dots == 2 {
			candidate := value[start:end]
			if _, ok := parseSemver3(candidate); ok {
				return candidate
			}
		}
	}
	return ""
}

func parseSemver3(value string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 3 {
		return result, false
	}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return result, false
		}
		result[i] = number
	}
	return result, true
}
