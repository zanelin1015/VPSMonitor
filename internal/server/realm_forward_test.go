package server

import (
	"testing"

	"bridge-core/internal/model"
)

func TestPreferredRealmForwardTargetAddressPrefersImportDomain(t *testing.T) {
	agent := model.AgentRecord{
		Config: model.ManagedAgentConfig{
			Entry: model.AgentEntryConfig{
				ImportDomain: "hkq2.zanelin.top",
				Addresses:    []string{"47.83.192.255"},
			},
		},
		Summary: model.VPSSummary{ObservedIP: "47.83.192.255"},
	}
	if got := preferredRealmForwardTargetAddress(agent); got != "hkq2.zanelin.top" {
		t.Fatalf("expected import domain, got %q", got)
	}
}
