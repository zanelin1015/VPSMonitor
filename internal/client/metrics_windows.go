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

func readWindowsMetrics() (windowsMetricsPayload, bool) {
	const script = `$cpu=(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average; $os=Get-CimInstance Win32_OperatingSystem; $net=Get-NetAdapterStatistics | Where-Object {$_.Name -notmatch 'Loopback|Teredo|isatap'} | Measure-Object -Property ReceivedBytes,SentBytes -Sum; [pscustomobject]@{cpu=[double]($cpu -as [double]); mem_total_kb=[uint64]$os.TotalVisibleMemorySize; mem_free_kb=[uint64]$os.FreePhysicalMemory; net_recv=[uint64]($net | Where-Object Property -eq 'ReceivedBytes').Sum; net_sent=[uint64]($net | Where-Object Property -eq 'SentBytes').Sum} | ConvertTo-Json -Compress`
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
