package model

import (
	"strings"
	"testing"
)

func TestOutboundsDeletePayloadValidation(t *testing.T) {
	hash := strings.Repeat("a", 64)
	good := OutboundDeleteTarget{Tag: "old", Fingerprint: hash}
	for _, tc := range []struct {
		name    string
		config  string
		targets []OutboundDeleteTarget
		valid   bool
	}{
		{"valid", hash, []OutboundDeleteTarget{good}, true},
		{"no config guard", "", []OutboundDeleteTarget{good}, false},
		{"bad guard", "xyz", []OutboundDeleteTarget{good}, false},
		{"empty selection", hash, nil, false},
		{"duplicate", hash, []OutboundDeleteTarget{good, good}, false},
		{"blank tag", hash, []OutboundDeleteTarget{{Tag: " ", Fingerprint: hash}}, false},
		{"missing object guard", hash, []OutboundDeleteTarget{{Tag: "old"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseOutboundsDeletePayload(map[string]any{"config_fingerprint": tc.config, "outbounds": tc.targets})
			if (err == nil) != tc.valid {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
}
