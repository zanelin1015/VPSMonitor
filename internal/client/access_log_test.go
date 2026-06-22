package client

import "testing"

func TestParseXrayAccessLogLine(t *testing.T) {
	line := "2026/06/22 12:34:56 1.2.3.4:12345 accepted tcp:example.com:443 [proxy] email: user@example.com"
	entry, ok := parseXrayAccessLogLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if entry.SourceIP != "1.2.3.4" || entry.SourcePort != 12345 {
		t.Fatalf("unexpected source: %s:%d", entry.SourceIP, entry.SourcePort)
	}
	if entry.Network != "tcp" || entry.TargetHost != "example.com" || entry.TargetPort != 443 {
		t.Fatalf("unexpected target: network=%s host=%s port=%d", entry.Network, entry.TargetHost, entry.TargetPort)
	}
	if entry.OutboundTag != "proxy" {
		t.Fatalf("unexpected outbound tag: %s", entry.OutboundTag)
	}
	if entry.ClientEmail != "user@example.com" {
		t.Fatalf("unexpected email: %s", entry.ClientEmail)
	}
	if entry.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set")
	}
}

func TestParseXrayAccessLogLineIPTarget(t *testing.T) {
	line := "2026/06/22 12:34:56 [2001:db8::1]:12345 accepted udp:8.8.8.8:53 [dns-out]"
	entry, ok := parseXrayAccessLogLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if entry.TargetIP != "8.8.8.8" || entry.TargetHost != "" || entry.TargetPort != 53 {
		t.Fatalf("unexpected target: host=%s ip=%s port=%d", entry.TargetHost, entry.TargetIP, entry.TargetPort)
	}
}

func TestParseXrayAccessLogEntriesSkipsRejected(t *testing.T) {
	data := []byte("2026/06/22 12:34:56 1.2.3.4:12345 rejected tcp:example.com:443 [blocked]\n")
	if entries := parseXrayAccessLogEntries(data, 10); len(entries) != 0 {
		t.Fatalf("expected rejected lines to be skipped, got %d", len(entries))
	}
}
