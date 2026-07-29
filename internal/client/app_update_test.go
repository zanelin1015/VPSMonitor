package client

import (
	"testing"

	"bridge-core/internal/model"
)

func TestOpenWrtInstallerURL(t *testing.T) {
	tests := map[string]string{
		"https://example.com/install.sh":          "https://example.com/install-openwrt.sh",
		"https://example.com/install.sh?v=1":      "https://example.com/install-openwrt.sh?v=1",
		"https://example.com/custom-installer.sh": "https://example.com/custom-installer.sh",
	}
	for input, want := range tests {
		if got := openWrtInstallerURL(input); got != want {
			t.Fatalf("openWrtInstallerURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEnforceExclusiveForwardingMode(t *testing.T) {
	bothEnabled := model.ManagedAgentConfig{
		Entry: model.AgentEntryConfig{
			PortForwarding: model.RealmForwardConfig{Enabled: true, Backend: "realm"},
			HAProxy:        model.HAProxyConfig{Enabled: true},
		},
	}

	haProxyMode := bothEnabled
	haProxyMode.Features = model.AgentFeatureConfig{HAProxy: true}
	haProxyMode = enforceExclusiveForwardingMode(haProxyMode)
	if haProxyMode.Entry.PortForwarding.Enabled || haProxyMode.Entry.PortForwarding.Backend != "none" || !haProxyMode.Entry.HAProxy.Enabled {
		t.Fatalf("HAProxy mode must disable Realm: %#v", haProxyMode.Entry)
	}

	realmMode := bothEnabled
	realmMode.Features = model.AgentFeatureConfig{Realm: true}
	realmMode = enforceExclusiveForwardingMode(realmMode)
	if !realmMode.Entry.PortForwarding.Enabled || realmMode.Entry.HAProxy.Enabled {
		t.Fatalf("Realm mode must disable HAProxy: %#v", realmMode.Entry)
	}
}
