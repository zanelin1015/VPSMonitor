package client

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

const threeXUIInstallScriptURL = "https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh"

func update3XUI(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if runtime.GOOS == "windows" {
		result := map[string]any{
			"action": "update_3xui",
			"os":     runtime.GOOS,
			"stderr": "3x-ui update is only supported on linux clients",
		}
		return result, fmt.Errorf("3x-ui update is only supported on linux clients")
	}
	if !commandExists("bash") {
		result := map[string]any{
			"action": "update_3xui",
			"os":     runtime.GOOS,
			"stderr": "bash is required to run the official 3x-ui installer",
		}
		return result, fmt.Errorf("bash is required to run the official 3x-ui installer")
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
	result["install_script_url"] = threeXUIInstallScriptURL
	result["started_at"] = startedAt
	result["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	return result, err
}

func buildUpdate3XUICommand() string {
	return fmt.Sprintf(`set -o pipefail
echo "[VPSMonitor] updating 3x-ui..."
if command -v x-ui >/dev/null 2>&1; then
  echo "[VPSMonitor] found x-ui CLI, trying: x-ui update"
  if x-ui update; then
    echo "[VPSMonitor] x-ui update completed"
    exit 0
  fi
  echo "[VPSMonitor] x-ui update failed; falling back to official install script" >&2
else
  echo "[VPSMonitor] x-ui CLI not found; running official install script"
fi
bash <(curl -Ls %q)`, threeXUIInstallScriptURL)
}
