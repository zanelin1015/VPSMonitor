package client

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func scheduleDisableClientService(ctx context.Context, payload map[string]any) error {
	if runtime.GOOS == "windows" {
		return scheduleDisableWindowsClientService(ctx, payload)
	}
	return scheduleDisableUnixClientService(ctx, payload)
}

func scheduleDisableUnixClientService(ctx context.Context, payload map[string]any) error {
	serviceName := payloadString(payload, "service_name", "vpsmonitor-client")
	logPath := payloadString(payload, "log_path", "/tmp/vpsmonitor-client-disable.log")
	command := fmt.Sprintf(`(sleep 2; {
if command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now %[1]q || { systemctl disable %[1]q || true; systemctl stop %[1]q || true; }
elif command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
  rc-update del %[1]q default || true
  rc-service %[1]q stop || true
elif command -v service >/dev/null 2>&1; then
  service %[1]q stop || true
  if command -v update-rc.d >/dev/null 2>&1; then update-rc.d -f %[1]q remove || true; fi
  if command -v chkconfig >/dev/null 2>&1; then chkconfig %[1]q off || true; fi
else
  pkill -f bridge-client || true
fi
} >>%[2]q 2>&1) >/dev/null 2>&1 &`, serviceName, logPath)
	return exec.CommandContext(ctx, "sh", "-c", command).Start()
}

func scheduleDisableWindowsClientService(ctx context.Context, payload map[string]any) error {
	serviceName := payloadString(payload, "windows_service_name", payloadString(payload, "service_name", "VPSMonitorClient"))
	command := fmt.Sprintf(`Start-Sleep -Seconds 2; $svc = '%s'; sc.exe config $svc start= disabled; Stop-Service -Name $svc -Force`, escapePowerShellSingleQuoted(serviceName))
	return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", command).Start()
}

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
