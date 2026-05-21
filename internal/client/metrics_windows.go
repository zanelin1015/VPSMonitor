//go:build windows

package client

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"bridge-core/internal/model"
)

type windowsMetricsPayload struct {
	CPU        float64 `json:"cpu"`
	MemTotalKB uint64  `json:"mem_total_kb"`
	MemFreeKB  uint64  `json:"mem_free_kb"`
	DiskTotal  uint64  `json:"disk_total"`
	DiskFree   uint64  `json:"disk_free"`
	NetRecv    uint64  `json:"net_recv"`
	NetSent    uint64  `json:"net_sent"`
}

func sampleSystemMetrics(s *systemMetricsSampler) model.VPSSummary {
	summary := model.VPSSummary{Hostname: currentHostname()}
	payload, ok := readWindowsMetrics()
	if !ok {
		return summary
	}
	if payload.CPU >= 0 {
		summary.CPU = payload.CPU
	}
	if payload.MemTotalKB > 0 {
		freeKB := payload.MemFreeKB
		if freeKB > payload.MemTotalKB {
			freeKB = payload.MemTotalKB
		}
		summary.MemTotal = payload.MemTotalKB * 1024
		summary.MemUsed = (payload.MemTotalKB - freeKB) * 1024
	}
	if payload.DiskTotal > 0 {
		free := payload.DiskFree
		if free > payload.DiskTotal {
			free = payload.DiskTotal
		}
		summary.DiskTotal = payload.DiskTotal
		summary.DiskUsed = payload.DiskTotal - free
	}
	net := netCounters{rx: payload.NetRecv, tx: payload.NetSent}
	if net.rx > 0 || net.tx > 0 {
		now := time.Now()
		if s.hasNet && net.rx >= s.lastNet.rx && net.tx >= s.lastNet.tx {
			elapsed := now.Sub(s.lastNetTime).Seconds()
			if elapsed > 0 {
				summary.NetIODown = uint64(float64(net.rx-s.lastNet.rx) / elapsed)
				summary.NetIOUp = uint64(float64(net.tx-s.lastNet.tx) / elapsed)
			}
		}
		summary.NetTrafficRecv = net.rx
		summary.NetTrafficSent = net.tx
		summary.NetTrafficTotal = net.rx + net.tx
		s.lastNet = net
		s.lastNetTime = now
		s.hasNet = true
	}
	return summary
}

func readCPUCounters() (cpuCounters, bool) { return cpuCounters{}, false }

func readNetCounters() (netCounters, bool) { return netCounters{}, false }

func readDiskUsage() (uint64, uint64, bool) {
	payload, ok := readWindowsMetrics()
	if !ok || payload.DiskTotal == 0 {
		return 0, 0, false
	}
	free := payload.DiskFree
	if free > payload.DiskTotal {
		free = payload.DiskTotal
	}
	return payload.DiskTotal - free, payload.DiskTotal, true
}

func readWindowsMetrics() (windowsMetricsPayload, bool) {
	const script = `$cpu=(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average; $os=Get-CimInstance Win32_OperatingSystem; $disk=Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | Measure-Object -Property Size,FreeSpace -Sum; $net=Get-NetAdapterStatistics | Where-Object {$_.Name -notmatch 'Loopback|Teredo|isatap'} | Measure-Object -Property ReceivedBytes,SentBytes -Sum; [pscustomobject]@{cpu=[double]($cpu -as [double]); mem_total_kb=[uint64]$os.TotalVisibleMemorySize; mem_free_kb=[uint64]$os.FreePhysicalMemory; disk_total=[uint64]($disk | Where-Object Property -eq 'Size').Sum; disk_free=[uint64]($disk | Where-Object Property -eq 'FreeSpace').Sum; net_recv=[uint64]($net | Where-Object Property -eq 'ReceivedBytes').Sum; net_sent=[uint64]($net | Where-Object Property -eq 'SentBytes').Sum} | ConvertTo-Json -Compress`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return windowsMetricsPayload{}, false
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return windowsMetricsPayload{}, false
	}
	var payload windowsMetricsPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return windowsMetricsPayload{}, false
	}
	return payload, true
}
