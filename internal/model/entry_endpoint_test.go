package model

import (
	"strings"
	"testing"
)

func testMappedEntry() AgentEntryConfig {
	return AgentEntryConfig{Connection: EntryConnectionConfig{Deployment: "docker", DockerNetwork: "bridge", PublicAccess: "nat", PublicAddress: "entry.example.com"}, Mappings: []AgentEntryMapping{{UseForExport: true, TargetType: "xui", TargetID: "7", InternalPort: 20001, HostPort: 30001, ExternalPort: 333, Transport: "tcp"}}}
}

func TestEntryEndpointDeploymentMatrix(t *testing.T) {
	for _, tc := range []struct {
		deployment, network, access string
		port, hostPort              int
	}{
		{"native", "", "direct", 20001, 20001}, {"native", "", "nat", 333, 20001},
		{"docker", "host", "direct", 20001, 20001}, {"docker", "host", "nat", 333, 20001},
		{"docker", "bridge", "direct", 30001, 30001}, {"docker", "bridge", "nat", 333, 30001},
	} {
		t.Run(tc.deployment+tc.network+tc.access, func(t *testing.T) {
			entry := testMappedEntry()
			entry.Connection.Deployment = tc.deployment
			entry.Connection.DockerNetwork = tc.network
			entry.Connection.PublicAccess = tc.access
			out := ResolveEntryEndpoint(entry, EntryTarget{Type: "xui", ID: "7", Port: 20001, Transport: "tcp"}, "egress.example.com")
			if out.Error != "" || out.Address != "entry.example.com" || out.Port != tc.port || out.HostPort != tc.hostPort || out.InternalPort != 20001 {
				t.Fatalf("unexpected endpoint: %+v", out)
			}
		})
	}
}

func TestEntryEndpointRejectsBrokenMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*AgentEntryConfig)
		want   string
	}{
		{"missing", func(e *AgentEntryConfig) { e.Mappings = nil }, "缺少节点"},
		{"stale", func(e *AgentEntryConfig) { e.Mappings[0].InternalPort = 20002 }, "已变化"},
		{"transport", func(e *AgentEntryConfig) { e.Mappings[0].Transport = "udp" }, "未覆盖"},
		{"port", func(e *AgentEntryConfig) { e.Mappings[0].ExternalPort = 65536 }, "公网端口"},
		{"address", func(e *AgentEntryConfig) { e.Connection.PublicAddress = "https://entry.example.com/path" }, "有效域名"},
		{"host missing", func(e *AgentEntryConfig) { e.Connection.PublicAddress = "" }, "公网入口"},
		{"ambiguous", func(e *AgentEntryConfig) { e.Mappings = append(e.Mappings, e.Mappings[0]) }, "默认导出"},
		{"conflict", func(e *AgentEntryConfig) {
			m := e.Mappings[0]
			m.TargetID = "8"
			m.HostPort = 30002
			e.Mappings = append(e.Mappings, m)
		}, "冲突"},
		{"host conflict", func(e *AgentEntryConfig) {
			m := e.Mappings[0]
			m.TargetID = "8"
			m.ExternalPort = 334
			e.Mappings = append(e.Mappings, m)
		}, "服务器端口"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := testMappedEntry()
			tc.change(&entry)
			result := ResolveEntryEndpoint(entry, EntryTarget{Type: "xui", ID: "7", Port: 20001, Transport: "tcp"}, "wrong-egress.example.com")
			if !strings.Contains(result.Error, tc.want) || result.Port != 0 || result.Address != "" {
				t.Fatalf("unsafe fallback: %+v", result)
			}
		})
	}
}

func TestEntryEndpointLegacyAndDefault(t *testing.T) {
	target := EntryTarget{Type: "xui", ID: "7", Port: 20001, Transport: "tcp"}
	legacy := AgentEntryConfig{Mappings: []AgentEntryMapping{{Address: "old.example.com", ExternalPort: 333, InternalPort: 20001, Protocol: "vless"}}}
	result := ResolveEntryEndpoint(legacy, target, "existing.example.com")
	if result.Error != "" || result.Port != 20001 || result.Address != "existing.example.com" {
		t.Fatalf("legacy behavior changed: %+v", result)
	}
	entry := testMappedEntry()
	second := entry.Mappings[0]
	second.ExternalPort = 444
	second.DefaultExport = true
	entry.Mappings = append(entry.Mappings, second)
	if result = ResolveEntryEndpoint(entry, target, ""); result.Error != "" || result.Port != 444 {
		t.Fatal(result)
	}
	entry.Mappings[0].DefaultExport = true
	if ValidateEntryConnection(entry) == nil {
		t.Fatal("duplicate defaults accepted")
	}
	entry = testMappedEntry()
	second = entry.Mappings[0]
	second.TargetID = "8"
	second.Transport = "udp"
	entry.Mappings = append(entry.Mappings, second)
	if err := ValidateEntryConnection(entry); err != nil {
		t.Fatalf("distinct transports may share a port: %v", err)
	}
}

func TestEntryHosts(t *testing.T) {
	for _, host := range []string{"nat.example.com", "1.161.63.29", "2001:db8::1", "[2001:db8::1]"} {
		if !ValidEntryHost(host) {
			t.Errorf("valid host rejected: %s", host)
		}
	}
	for _, host := range []string{"https://example.com", "example.com:333", "999.1.2.3", "0.0.0.0", "::", "::1", "127.0.0.1", "localhost.", "*.example.com", "a b.com"} {
		if ValidEntryHost(host) {
			t.Errorf("invalid host accepted: %s", host)
		}
	}
}
