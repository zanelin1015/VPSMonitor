package client

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"bridge-core/internal/config"
)

func (a *App) ensureXUIBootstrap(ctx context.Context, cfg config.XUIConfig) error {
	if !cfg.AutoInstall {
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("x-ui auto install only supports linux clients")
	}
	if detectLocal3XUIVersion(ctx) == "" {
		if err := install3XUI(ctx, cfg); err != nil {
			return err
		}
	}
	return configure3XUI(ctx, cfg)
}

func install3XUI(ctx context.Context, cfg config.XUIConfig) error {
	if !commandExists("bash") || !commandExists("curl") {
		return fmt.Errorf("bash and curl are required to install 3x-ui")
	}
	installURL := strings.TrimSpace(cfg.InstallScriptURL)
	if installURL == "" {
		installURL = "https://raw.githubusercontent.com/MHSanaei/3x-ui/master/install.sh"
	}
	command := fmt.Sprintf(`set -e
TMP="$(mktemp /tmp/3x-ui-install.XXXXXX.sh)"
cleanup() { rm -f "$TMP"; }
trap cleanup EXIT
curl -fsSL %q -o "$TMP"
# The official installer may ask whether to customize settings; use defaults first,
# skip installer SSL setup for unattended installs, then VPSMonitor applies the
# configured username/password/port/path below.
printf 'n\n4\n' | bash "$TMP"
`, installURL)
	_, err := executeRemoteCommandWithOptions(ctx, map[string]any{
		"command":         command,
		"shell":           "bash",
		"timeout_seconds": 900,
	}, remoteCommandOptions{DefaultTimeoutSeconds: 900, MaxTimeoutSeconds: 1800})
	return err
}

func configure3XUI(ctx context.Context, cfg config.XUIConfig) error {
	if !commandExists("x-ui") {
		return fmt.Errorf("x-ui command not found after install")
	}
	args := []string{"setting"}
	if strings.TrimSpace(cfg.Username) != "" {
		args = append(args, "-username", strings.TrimSpace(cfg.Username))
	}
	if strings.TrimSpace(cfg.Password) != "" {
		args = append(args, "-password", strings.TrimSpace(cfg.Password))
	}
	if cfg.PanelPort > 0 {
		args = append(args, "-port", fmt.Sprintf("%d", cfg.PanelPort))
	}
	if path := normalizeXUIBootstrapPath(cfg.WebPath); path != "" {
		args = append(args, "-webBasePath", path)
	}
	if len(args) > 1 {
		if output, err := runCommandOutput(ctx, "x-ui", args...); err != nil {
			return fmt.Errorf("configure x-ui: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	_, _ = runCommandOutput(ctx, "x-ui", "restart")
	_ = os.Setenv("XUI_DB_PATH", firstNonEmpty(cfg.DBPath, "/etc/x-ui/x-ui.db"))
	return nil
}

func normalizeXUIBootstrapPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = "/" + strings.Trim(value, "/")
	if value == "/" {
		return ""
	}
	return value + "/"
}

func (a *App) ensureXUIBootstrapIfNeeded(ctx context.Context, cfg config.XUIConfig) {
	if !cfg.AutoInstall {
		return
	}
	sig := xuiBootstrapSignature(cfg)
	if sig == "" || sig == a.xuiBootstrapSignature {
		return
	}
	bootstrapCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if err := a.ensureXUIBootstrap(bootstrapCtx, cfg); err != nil {
		return
	}
	a.xuiBootstrapSignature = sig
}

func xuiBootstrapSignature(cfg config.XUIConfig) string {
	if !cfg.AutoInstall {
		return ""
	}
	return strings.Join([]string{cfg.InstallScriptURL, cfg.Username, cfg.Password, fmt.Sprintf("%d", cfg.PanelPort), normalizeXUIBootstrapPath(cfg.WebPath)}, "\x00")
}
