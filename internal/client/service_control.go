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
	service_name=%[1]q
	if command -v systemctl >/dev/null 2>&1; then
	  systemctl disable --now "$service_name" || { systemctl disable "$service_name" || true; systemctl stop "$service_name" || true; }
	elif command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
	  rc-update del "$service_name" default || true
	  rc-service "$service_name" stop || true
	elif [ -x "/etc/init.d/$service_name" ] && [ -f /etc/openwrt_release ]; then
	  "/etc/init.d/$service_name" disable || true
	  "/etc/init.d/$service_name" stop || true
	elif command -v service >/dev/null 2>&1; then
	  service "$service_name" stop || true
	  if command -v update-rc.d >/dev/null 2>&1; then update-rc.d -f "$service_name" remove || true; fi
	  if command -v chkconfig >/dev/null 2>&1; then chkconfig "$service_name" off || true; fi
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
