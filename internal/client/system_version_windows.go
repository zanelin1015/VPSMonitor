//go:build windows

package client

import (
	"os/exec"
	"strings"
)

func detectSystemVersion() string {
	const script = `$cv=Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -ErrorAction SilentlyContinue; $name=''; if ($cv) { $name=$cv.ProductName }; if (-not $name) { $name=(Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue).Caption }; if ($name) { $name.Trim() }`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).Output()
	if err != nil {
		return "Windows"
	}
	name := strings.TrimSpace(string(output))
	name = strings.TrimPrefix(name, "Microsoft ")
	if name == "" {
		return "Windows"
	}
	return name
}
