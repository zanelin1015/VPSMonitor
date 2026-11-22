//go:build windows

package client

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func collectNetworkInterfaces(ctx context.Context) *model.NetworkInterfacesSnapshot {
	const script = `$ErrorActionPreference='Stop'
[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
$boot=(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime().ToString('o')
$adapters=@(Get-NetAdapter -IncludeHidden)
$stats=@(Get-NetAdapterStatistics -IncludeHidden -ErrorAction SilentlyContinue)
$routes=@(Get-NetRoute -ErrorAction SilentlyContinue | Where-Object {$_.DestinationPrefix -eq '0.0.0.0/0' -or $_.DestinationPrefix -eq '::/0'} | Sort-Object @{Expression={[long]$_.RouteMetric+[long]$_.InterfaceMetric}})
$preferred=@(foreach($r in $routes){$adapters | Where-Object {$_.ifIndex -eq $r.InterfaceIndex} | ForEach-Object {$_.Name}})
$items=@(foreach($a in $adapters){
  $s=$stats | Where-Object {$_.Name -eq $a.Name} | Select-Object -First 1
  $loop=($a.InterfaceDescription -match 'Loopback')
  $kind=if($loop){'loopback'}elseif($a.HardwareInterface){'physical'}else{'virtual'}
  $problem=if($null -eq $s){'该网卡未提供收发计数器'}else{''}
  [pscustomobject]@{name=[string]$a.Name;index=[int]$a.ifIndex;mac=[string]$a.MacAddress;kind=$kind;up=($a.Status -eq 'Up');loopback=[bool]$loop;received_bytes=[uint64]$s.ReceivedBytes;sent_bytes=[uint64]$s.SentBytes;error=$problem}
})
[pscustomobject]@{boot_id=$boot;preferred=@($preferred);interfaces=@($items)} | ConvertTo-Json -Compress -Depth 5`
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	body, err := cmd.Output()
	if err != nil {
		return &model.NetworkInterfacesSnapshot{CollectedAt: time.Now().UTC(), Error: "读取 Windows 网卡统计失败: " + firstNonEmpty(strings.TrimSpace(stderr.String()), err.Error()), Interfaces: []model.NetworkInterfaceCounter{}}
	}
	return decodeWindowsNetworkInterfaces(body)
}
