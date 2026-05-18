package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const threeXUIUpdateScriptURL = "https://raw.githubusercontent.com/MHSanaei/3x-ui/main/update.sh"

func update3XUI(ctx context.Context, payload map[string]any) (map[string]any, error) {
	targetVersion := normalize3XUIVersion(payloadString(payload, "target_version", ""))
	force := payloadBool(payload, "force", false)
	currentVersion := detectLocal3XUIVersion(ctx)
	baseResult := map[string]any{
		"action":          "update_3xui",
		"current_version": currentVersion,
		"target_version":  targetVersion,
		"force":           force,
	}
	if runtime.GOOS == "windows" {
		result := map[string]any{
			"action": "update_3xui",
			"os":     runtime.GOOS,
			"stderr": "3x-ui official source only provides a Linux updater; Windows releases are zip packages without an official updater/service flow",
		}
		copyMap(result, baseResult)
		return result, fmt.Errorf("3x-ui official Windows package does not provide an updater")
	}
	if !commandExists("bash") {
		result := map[string]any{
			"action": "update_3xui",
			"os":     runtime.GOOS,
			"stderr": "bash is required to run the official 3x-ui updater",
		}
		copyMap(result, baseResult)
		return result, fmt.Errorf("bash is required to run the official 3x-ui updater")
	}
	if !force && targetVersion != "" && currentVersion != "" && !semverNewer(targetVersion, currentVersion) {
		baseResult["status"] = "skipped"
		baseResult["reason"] = "3x-ui is already up to date"
		baseResult["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		return baseResult, nil
	}

	timeoutSeconds := remoteCommandPayloadInt(payload["timeout_seconds"])
	if timeoutSeconds <= 0 {
		timeoutSeconds = 900
	}
	commandPayload := map[string]any{
		"command":         buildUpdate3XUICommand(),
		"shell":           "bash",
		"timeout_seconds": timeoutSeconds,
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := executeRemoteCommandWithOptions(ctx, commandPayload, remoteCommandOptions{
		DefaultTimeoutSeconds: 900,
		MaxTimeoutSeconds:     1800,
	})
	if result == nil {
		result = map[string]any{}
	}
	result["action"] = "update_3xui"
	result["current_version"] = currentVersion
	result["target_version"] = targetVersion
	result["update_script_url"] = threeXUIUpdateScriptURL
	result["started_at"] = startedAt
	result["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	return result, err
}

func copyMap(dst, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
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

func semverNewer(candidate string, current string) bool {
	candidateParts, candidateOK := parseSemver3(candidate)
	currentParts, currentOK := parseSemver3(current)
	if !candidateOK || !currentOK {
		return false
	}
	for i := 0; i < len(candidateParts); i++ {
		if candidateParts[i] != currentParts[i] {
			return candidateParts[i] > currentParts[i]
		}
	}
	return false
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

func buildUpdate3XUICommand() string {
	return fmt.Sprintf(`set -e
set -o pipefail
echo "[VPSMonitor] upgrading 3x-ui via official update.sh..."
if ! command -v curl >/dev/null 2>&1; then
  echo "[VPSMonitor] curl is required to download the official 3x-ui updater" >&2
  exit 127
fi
tmp="$(mktemp /tmp/3x-ui-update.XXXXXX.sh)"
trap 'rm -f "$tmp"' EXIT
curl -fsSL %q -o "$tmp"
chmod +x "$tmp"
bash "$tmp"`, threeXUIUpdateScriptURL)
}
