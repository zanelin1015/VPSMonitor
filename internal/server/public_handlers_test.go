package server

import (
	"testing"

	"bridge-core/internal/model"
)

func TestSanitizePublicTopologyKeepsPublicChainsAndTitles(t *testing.T) {
	view := model.GlobalDashboardView{
		Agents: []model.DashboardAgentView{
			{
				AgentID:             "hk-real-agent",
				AgentName:           "Internal HK VPS",
				CustomerDisplayName: "HK Premium",
				Geo: &model.IPGeoView{
					IP:          "203.0.113.10",
					CountryCode: "HK",
					CountryName: "Hong Kong",
					City:        "Hong Kong",
				},
			},
		},
		Links: []model.TopologyLinkView{
			{
				Key: "hk-real-agent:secret-link",
				Source: model.TopologyOutboundRef{
					AgentID:     "hk-real-agent",
					AgentName:   "Internal HK VPS",
					OutboundTag: "private-outbound",
					TargetGeo: &model.IPGeoView{
						IP:          "198.51.100.8",
						CountryCode: "US",
						CountryName: "United States",
					},
				},
				Target: model.TopologyInboundRef{
					AgentID:     "hk-real-agent",
					AgentName:   "Internal HK VPS",
					InboundID:   20001,
					InboundTag:  "secret-inbound",
					ResolvedIPs: []string{"203.0.113.10"},
					Domains:     []string{"secret.example.com"},
				},
				MatchFields:      []string{"domain"},
				MatchExplanation: "secret.example.com matched",
			},
		},
		ClientChains: []model.ClientChainView{
			{
				Key:              "hk-real-agent:alice@example.com:20001",
				RootAgentID:      "hk-real-agent",
				RootAgentName:    "Internal HK VPS",
				RootClientEmail:  "alice@example.com",
				RootClientRemark: "Alice",
				RootInboundTag:   "secret-inbound",
				MatchedLinkCount: 1,
				Steps: []model.ClientChainStep{
					{StepType: "client", AgentID: "hk-real-agent", Label: "alice@example.com", Detail: "Alice"},
					{StepType: "inbound", AgentID: "hk-real-agent", Label: "secret-inbound", Protocol: "vless", Port: 20001},
					{
						StepType:    "outbound",
						AgentID:     "hk-real-agent",
						Label:       "private-outbound",
						OutboundTag: "private-outbound",
						Protocol:    "freedom",
						Target:      "secret.example.com",
						TargetIP:    "198.51.100.8",
						TargetGeo: &model.IPGeoView{
							IP:          "198.51.100.8",
							CountryCode: "US",
							CountryName: "United States",
						},
						MatchReason: "secret reason",
					},
				},
			},
		},
	}

	sanitized := sanitizePublicTopologyView(view)
	if len(sanitized.Agents) != 1 || sanitized.Agents[0].AgentName != "HK Premium" {
		t.Fatalf("expected public customer title to be used, got %#v", sanitized.Agents)
	}
	if sanitized.Agents[0].AgentID == "hk-real-agent" || sanitized.Agents[0].Geo.IP != "" {
		t.Fatalf("expected agent id and geo ip to be sanitized, got %#v", sanitized.Agents[0])
	}
	if len(sanitized.Links) != 1 || sanitized.Links[0].Key == "hk-real-agent:secret-link" {
		t.Fatalf("expected topology link key to be anonymized, got %#v", sanitized.Links)
	}
	if len(sanitized.Links[0].Target.ResolvedIPs) != 0 || len(sanitized.Links[0].Target.Domains) != 0 {
		t.Fatalf("expected topology target addresses to be hidden, got %#v", sanitized.Links[0].Target)
	}
	if len(sanitized.ClientChains) != 1 {
		t.Fatalf("expected public client chain to be retained, got %#v", sanitized.ClientChains)
	}
	chain := sanitized.ClientChains[0]
	if chain.Key == "hk-real-agent:alice@example.com:20001" || chain.RootClientEmail != "" || chain.RootClientRemark != "" {
		t.Fatalf("expected client chain identity to be hidden, got %#v", chain)
	}
	for _, step := range chain.Steps {
		if step.StepType == "client" || step.Target != "" || step.TargetIP != "" || step.MatchReason != "" {
			t.Fatalf("expected sensitive chain step fields to be hidden, got %#v", step)
		}
		if step.TargetGeo != nil && step.TargetGeo.IP != "" {
			t.Fatalf("expected target geo ip to be hidden, got %#v", step.TargetGeo)
		}
	}
}
