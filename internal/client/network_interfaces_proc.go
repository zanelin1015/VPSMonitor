//go:build !windows

package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func collectNetworkInterfaces(_ context.Context) *model.NetworkInterfacesSnapshot {
	if runtime.GOOS != "linux" {
		return &model.NetworkInterfacesSnapshot{CollectedAt: time.Now().UTC(), Error: "逐网卡计量当前支持 Linux/OpenWrt 与 Windows", Interfaces: []model.NetworkInterfaceCounter{}}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return &model.NetworkInterfacesSnapshot{CollectedAt: time.Now().UTC(), Error: "读取网卡列表失败: " + err.Error(), Interfaces: []model.NetworkInterfaceCounter{}}
	}
	return collectProcNetworkInterfaces("/proc", "/sys", interfaces)
}

func collectProcNetworkInterfaces(procRoot, sysRoot string, interfaces []net.Interface) *model.NetworkInterfacesSnapshot {
	result := &model.NetworkInterfacesSnapshot{CollectedAt: time.Now().UTC(), Interfaces: []model.NetworkInterfaceCounter{}}
	fail := func(err error) *model.NetworkInterfacesSnapshot { result.Error = err.Error(); return result }
	file, err := os.Open(filepath.Join(procRoot, "net/dev"))
	if err != nil {
		return fail(fmt.Errorf("读取网卡计数器失败: %w", err))
	}
	counters, err := parseNetworkInterfaceCounters(file)
	_ = file.Close()
	if err != nil {
		return fail(err)
	}
	if len(counters) == 0 && len(interfaces) > 0 {
		return fail(fmt.Errorf("系统存在网卡，但本次未读取到字节计数器"))
	}
	boot, _ := os.ReadFile(filepath.Join(procRoot, "sys/kernel/random/boot_id"))
	bootID := strings.TrimSpace(string(boot))
	if bootID == "" {
		body, _ := os.ReadFile(filepath.Join(procRoot, "stat"))
		for _, line := range strings.Split(string(body), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "btime" {
				bootID = "btime:" + fields[1]
				break
			}
		}
	}
	if bootID == "" {
		return fail(fmt.Errorf("无法识别系统启动代次，暂不记录网卡周期流量"))
	}
	metadata := make(map[string]net.Interface, len(interfaces))
	for _, item := range interfaces {
		metadata[item.Name] = item
	}
	for name, counter := range counters {
		// Names from procfs are used only as one sysfs path component.
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			continue
		}
		info, known := metadata[name]
		base := filepath.Join(sysRoot, "class/net", name)
		if !known {
			indexText, _ := os.ReadFile(filepath.Join(base, "ifindex"))
			info.Index, _ = strconv.Atoi(strings.TrimSpace(string(indexText)))
			mac, _ := os.ReadFile(filepath.Join(base, "address"))
			info.HardwareAddr, _ = net.ParseMAC(strings.TrimSpace(string(mac)))
			flags, _ := os.ReadFile(filepath.Join(base, "flags"))
			value, _ := strconv.ParseUint(strings.TrimSpace(string(flags)), 0, 32)
			if value&1 != 0 {
				info.Flags |= net.FlagUp
			}
			if value&8 != 0 {
				info.Flags |= net.FlagLoopback
			}
		}
		if info.Index <= 0 {
			result.Warnings = append(result.Warnings, name+" 的接口索引暂不可用，已跳过本次采样")
			continue
		}
		loopback := info.Flags&net.FlagLoopback != 0 || isLoopbackInterface(name)
		if state, err := os.ReadFile(filepath.Join(base, "operstate")); err == nil {
			switch strings.TrimSpace(string(state)) {
			case "down", "lowerlayerdown", "notpresent", "dormant":
				info.Flags &^= net.FlagUp
			}
		}
		kind := linuxNetworkInterfaceKind(base, name, loopback)
		result.Interfaces = append(result.Interfaces, model.NetworkInterfaceCounter{Name: name, Index: info.Index, MAC: info.HardwareAddr.String(),
			Kind: kind, Up: info.Flags&net.FlagUp != 0, Loopback: loopback, ReceivedBytes: counter.rx, SentBytes: counter.tx})
	}
	if len(counters) > 0 && len(result.Interfaces) == 0 {
		return fail(fmt.Errorf("所有网卡的元数据读取失败，暂不记录本次采样"))
	}
	routes, _ := os.ReadFile(filepath.Join(procRoot, "net/route"))
	preferred := defaultIPv4Interfaces(string(routes))
	if len(preferred) == 0 {
		routes, _ = os.ReadFile(filepath.Join(procRoot, "net/ipv6_route"))
		preferred = defaultIPv6Interfaces(string(routes))
	}
	result.Interfaces = finalizeNetworkInterfaces(result.Interfaces, bootID, preferred)
	return result
}

func parseNetworkInterfaceCounters(reader io.Reader) (map[string]netCounters, error) {
	result := map[string]netCounters{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) != 2 {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			return nil, fmt.Errorf("网卡 %q 的计数器格式不完整", strings.TrimSpace(parts[0]))
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			return nil, fmt.Errorf("网卡 %q 的字节计数无效", strings.TrimSpace(parts[0]))
		}
		result[strings.TrimSpace(parts[0])] = netCounters{rx: rx, tx: tx}
	}
	return result, scanner.Err()
}

func linuxNetworkInterfaceKind(base, name string, loopback bool) string {
	if loopback {
		return "loopback"
	}
	if _, err := os.Stat(filepath.Join(base, "device")); err == nil {
		return "physical"
	}
	if _, err := os.Stat(filepath.Join(base, "bridge")); err == nil {
		return "bridge"
	}
	if _, err := os.Stat(filepath.Join(base, "bonding")); err == nil {
		return "bond"
	}
	for _, prefix := range []string{"veth", "docker", "br-", "virbr", "tun", "tap", "wg", "ppp"} {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return "virtual"
		}
	}
	return "virtual"
}

func defaultIPv4Interfaces(body string) []string {
	type route struct {
		name   string
		metric uint64
	}
	var routes []route
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(fields[3], 16, 64)
		if err != nil || flags&1 == 0 || flags&0x200 != 0 {
			continue
		}
		metric, err := strconv.ParseUint(fields[6], 10, 64)
		if err == nil {
			routes = append(routes, route{fields[0], metric})
		}
	}
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].metric < routes[j].metric })
	names := make([]string, 0, len(routes))
	for _, item := range routes {
		names = append(names, item.name)
	}
	return names
}

func defaultIPv6Interfaces(body string) []string {
	type route struct {
		name   string
		metric uint64
	}
	var routes []route
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 10 || fields[0] != strings.Repeat("0", 32) || fields[1] != "00" {
			continue
		}
		flags, err := strconv.ParseUint(fields[8], 16, 64)
		if err != nil || flags&1 == 0 || flags&0x200 != 0 {
			continue
		}
		metric, err := strconv.ParseUint(fields[5], 16, 64)
		if err == nil {
			routes = append(routes, route{fields[9], metric})
		}
	}
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].metric < routes[j].metric })
	names := make([]string, 0, len(routes))
	for _, item := range routes {
		names = append(names, item.name)
	}
	return names
}
