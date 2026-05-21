//go:build !windows

package client

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bridge-core/internal/model"
)

func sampleSystemMetrics(s *systemMetricsSampler) model.VPSSummary {
	summary := model.VPSSummary{Hostname: currentHostname()}
	if cpu, ok := readCPUCounters(); ok {
		if s.hasCPU && cpu.total > s.lastCPU.total {
			totalDelta := cpu.total - s.lastCPU.total
			idleDelta := uint64(0)
			if cpu.idle > s.lastCPU.idle {
				idleDelta = cpu.idle - s.lastCPU.idle
			}
			if totalDelta > 0 && idleDelta <= totalDelta {
				summary.CPU = (1 - float64(idleDelta)/float64(totalDelta)) * 100
			}
		}
		s.lastCPU = cpu
		s.hasCPU = true
	}
	if memUsed, memTotal, ok := readMemUsage(); ok {
		summary.MemUsed = memUsed
		summary.MemTotal = memTotal
	}
	if diskUsed, diskTotal, ok := readDiskUsage(); ok {
		summary.DiskUsed = diskUsed
		summary.DiskTotal = diskTotal
	}
	if net, ok := readNetCounters(); ok {
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

func readCPUCounters() (cpuCounters, bool) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuCounters{}, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return cpuCounters{}, false
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuCounters{}, false
	}
	var total uint64
	var idle uint64
	for index, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuCounters{}, false
		}
		total += value
		if index == 3 || index == 4 {
			idle += value
		}
	}
	return cpuCounters{idle: idle, total: total}, total > 0
}

func readMemUsage() (uint64, uint64, bool) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()

	var totalKB uint64
	var availableKB uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			totalKB = value
		case "MemAvailable":
			availableKB = value
		}
	}
	if totalKB == 0 {
		return 0, 0, false
	}
	if availableKB > totalKB {
		availableKB = totalKB
	}
	return (totalKB - availableKB) * 1024, totalKB * 1024, true
}

func readDiskUsage() (uint64, uint64, bool) {
	if used, total, ok := readMountedDiskUsage("/proc/self/mounts"); ok {
		return used, total, true
	}
	return readDiskUsagePath("/")
}

func readMountedDiskUsage(mountsPath string) (uint64, uint64, bool) {
	file, err := os.Open(mountsPath)
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()

	var total uint64
	var free uint64
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		source, mountPoint, fsType := fields[0], unescapeMountPath(fields[1]), fields[2]
		if !isLocalDiskMount(source, fsType) {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		info, err := os.Stat(mountPoint)
		if err != nil || !info.IsDir() {
			continue
		}
		usedBytes, totalBytes, ok := readDiskUsagePath(mountPoint)
		if !ok || totalBytes == 0 || usedBytes > totalBytes {
			continue
		}
		seen[source] = struct{}{}
		total += totalBytes
		free += totalBytes - usedBytes
	}
	if total == 0 {
		return 0, 0, false
	}
	if free > total {
		free = total
	}
	return total - free, total, true
}

func readDiskUsagePath(path string) (uint64, uint64, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, false
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	if total == 0 {
		return 0, 0, false
	}
	if free > total {
		free = total
	}
	return total - free, total, true
}

func isLocalDiskMount(source, fsType string) bool {
	if strings.HasPrefix(source, "/dev/") {
		return true
	}
	switch fsType {
	case "ext2", "ext3", "ext4", "xfs", "btrfs", "f2fs", "zfs":
		return true
	default:
		return false
	}
}

func unescapeMountPath(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func readNetCounters() (netCounters, bool) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return netCounters{}, false
	}
	defer file.Close()

	var total netCounters
	var fallback netCounters
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo <= 2 {
			continue
		}
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		fallback.rx += rx
		fallback.tx += tx
		if isLoopbackInterface(name) {
			continue
		}
		total.rx += rx
		total.tx += tx
	}
	if total.rx == 0 && total.tx == 0 {
		total = fallback
	}
	return total, total.rx > 0 || total.tx > 0
}

func isLoopbackInterface(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "lo" || strings.HasPrefix(name, "lo:")
}
