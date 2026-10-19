package model

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Connection describes the existing network. It never changes Docker or NAT.
type EntryConnectionConfig struct {
	Deployment    string `json:"deployment,omitempty"`     // native, docker
	DockerNetwork string `json:"docker_network,omitempty"` // host, bridge
	PublicAccess  string `json:"public_access,omitempty"`  // direct, nat
	PublicAddress string `json:"public_address,omitempty"`
}

type EntryTarget struct {
	Type      string
	ID        string
	Port      int
	Transport string
}

type EntryEndpoint struct {
	Address      string `json:"address,omitempty"`
	Port         int    `json:"port,omitempty"`
	InternalPort int    `json:"internal_port,omitempty"`
	HostPort     int    `json:"host_port,omitempty"`
	Transport    string `json:"transport,omitempty"`
	Mapped       bool   `json:"mapped,omitempty"`
	Error        string `json:"error,omitempty"`
}

func ValidEntryHost(value string) bool {
	value = strings.TrimSpace(value)
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return !ip.IsUnspecified() && !ip.IsLoopback()
	}
	if strings.Trim(value, "0123456789.") == "" {
		return false
	}
	if value == "" || len(value) > 253 || strings.ContainsAny(value, ":/@?#*[] 	\r\n\\") || strings.EqualFold(strings.TrimSuffix(value, "."), "localhost") {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(value, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func EntryUsesDockerMapping(entry AgentEntryConfig, targetType string) bool {
	return targetType == "xui" && entry.Connection.Deployment == "docker" && entry.Connection.DockerNetwork == "bridge"
}

func ValidateEntryConnection(entry AgentEntryConfig) error {
	c := entry.Connection
	if c.Deployment != "" && c.Deployment != "native" && c.Deployment != "docker" {
		return fmt.Errorf("x-ui 部署方式无效")
	}
	if c.DockerNetwork != "" && c.DockerNetwork != "host" && c.DockerNetwork != "bridge" {
		return fmt.Errorf("Docker 网络方式无效")
	}
	if c.PublicAccess != "" && c.PublicAccess != "direct" && c.PublicAccess != "nat" {
		return fmt.Errorf("公网接入方式无效")
	}
	if c.Deployment == "docker" && c.DockerNetwork == "" {
		return fmt.Errorf("请选择 Docker 直通或映射")
	}
	if c.PublicAddress != "" && !ValidEntryHost(c.PublicAddress) {
		return fmt.Errorf("公网入口请填写有效域名或 IP，不包含端口或 URL 路径")
	}
	defaults := map[string]bool{}
	var active []AgentEntryMapping
	for i, m := range entry.Mappings {
		if !m.UseForExport {
			continue
		} // Legacy topology mappings are opt-in.
		prefix := fmt.Sprintf("第 %d 条连接映射：", i+1)
		if m.TargetType != "xui" && m.TargetType != "realm" && m.TargetType != "haproxy" {
			return fmt.Errorf("%s请选择节点类型", prefix)
		}
		if strings.TrimSpace(m.TargetID) == "" {
			return fmt.Errorf("%s请选择节点", prefix)
		}
		if m.TargetType == "xui" {
			if id, err := strconv.Atoi(m.TargetID); err != nil || id <= 0 {
				return fmt.Errorf("%s入站 ID 无效", prefix)
			}
		}
		if !validEntryPort(m.InternalPort) {
			return fmt.Errorf("%s内部端口必须为 1–65535", prefix)
		}
		if m.Transport != "tcp" && m.Transport != "udp" && m.Transport != "both" {
			return fmt.Errorf("%s请选择 TCP、UDP 或 TCP+UDP", prefix)
		}
		if EntryUsesDockerMapping(entry, m.TargetType) && !validEntryPort(m.HostPort) {
			return fmt.Errorf("%s请填写服务器端口（1–65535）", prefix)
		}
		if c.PublicAccess == "nat" && !validEntryPort(m.ExternalPort) {
			return fmt.Errorf("%s请填写公网端口（1–65535）", prefix)
		}
		host := firstEntryValue(m.Address, c.PublicAddress, entry.ImportDomain)
		if !ValidEntryHost(host) {
			return fmt.Errorf("%s请填写公网入口域名或 IP", prefix)
		}
		key := m.TargetType + ":" + m.TargetID
		if m.DefaultExport && defaults[key] {
			return fmt.Errorf("%s同一节点只能有一个默认导出入口", prefix)
		}
		defaults[key] = defaults[key] || m.DefaultExport
		for _, old := range active {
			if old.TargetType == m.TargetType && old.TargetID == m.TargetID {
				continue
			}
			if EntryUsesDockerMapping(entry, old.TargetType) && EntryUsesDockerMapping(entry, m.TargetType) && old.HostPort == m.HostPort && transportsOverlap(old.Transport, m.Transport) {
				return fmt.Errorf("%s服务器端口与其他 Docker 节点冲突", prefix)
			}
			oldHost := firstEntryValue(old.Address, c.PublicAddress, entry.ImportDomain)
			if strings.EqualFold(oldHost, host) && entryMappingPort(entry, old) == entryMappingPort(entry, m) && transportsOverlap(old.Transport, m.Transport) {
				return fmt.Errorf("%s公网地址、端口与其他节点冲突", prefix)
			}
		}
		active = append(active, m)
	}
	return nil
}

// ResolveEntryEndpoint is shared by direct exports and forwarding entrances.
// Ports used for x-ui management and topology are never rewritten in-place.
func ResolveEntryEndpoint(entry AgentEntryConfig, target EntryTarget, fallbackHost string) EntryEndpoint {
	c := entry.Connection
	bridge := EntryUsesDockerMapping(entry, target.Type)
	mapped := bridge || c.PublicAccess == "nat"
	out := EntryEndpoint{InternalPort: target.Port, HostPort: target.Port, Transport: target.Transport, Mapped: mapped}
	fail := func(reason string) EntryEndpoint { out.Error = reason; out.Address = ""; out.Port = 0; return out }
	if !validEntryPort(target.Port) {
		return fail("节点监听端口无效")
	}
	if err := ValidateEntryConnection(entry); err != nil {
		return fail(err.Error())
	}
	var candidates []AgentEntryMapping
	for _, m := range entry.Mappings {
		if m.UseForExport && m.TargetType == target.Type && m.TargetID == target.ID {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		if mapped {
			return fail("缺少节点端口映射，请在连接与端口中配置")
		}
		out.Address = firstEntryValue(c.PublicAddress, fallbackHost)
		out.Port = target.Port
		if out.Address == "" {
			return fail("缺少对外连接地址")
		}
		return out
	}
	selected := candidates[0]
	if len(candidates) > 1 {
		found := false
		for _, m := range candidates {
			if m.DefaultExport {
				selected = m
				found = true
			}
		}
		if !found {
			return fail("存在多个映射，请指定默认导出入口")
		}
	}
	if selected.InternalPort != target.Port {
		return fail("节点监听端口已变化，请重新确认映射")
	}
	if selected.Transport != "both" && selected.Transport != target.Transport {
		return fail("映射未覆盖节点所需的 " + strings.ToUpper(target.Transport))
	}
	out.Address = strings.Trim(firstEntryValue(selected.Address, c.PublicAddress, entry.ImportDomain), "[]")
	if bridge {
		out.HostPort = selected.HostPort
	}
	out.Port = entryMappingPort(entry, selected)
	out.Mapped = mapped || out.Port != target.Port
	return out
}

func XUIEntryTransport(node XUINodeView) string {
	switch strings.ToLower(node.Protocol) {
	case "hysteria", "hysteria2", "tuic", "wireguard":
		return "udp"
	}
	switch strings.ToLower(node.Network) {
	case "kcp", "mkcp", "quic", "udp":
		return "udp"
	}
	if strings.EqualFold(node.Protocol, "shadowsocks") {
		return "both"
	}
	return "tcp"
}

func entryMappingPort(entry AgentEntryConfig, m AgentEntryMapping) int {
	if entry.Connection.PublicAccess == "nat" {
		return m.ExternalPort
	}
	if EntryUsesDockerMapping(entry, m.TargetType) {
		return m.HostPort
	}
	return m.InternalPort
}
func validEntryPort(port int) bool       { return port > 0 && port <= 65535 }
func transportsOverlap(a, b string) bool { return a == b || a == "both" || b == "both" }
func firstEntryValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
