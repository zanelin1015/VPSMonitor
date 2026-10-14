package dashboard

import (
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestRoutingValidityMatchersAndReferences(t *testing.T) {
	no := false
	nodes := []model.XUINodeView{{Tag: "in-a"}, {Tag: "in-b"}}
	// Disabled/offline clients are still existing matcher targets.
	clients := []model.XUIClientView{{Email: "alice", InboundTag: "in-a"}, {Email: "bob", InboundTag: "in-b"}}
	outbounds := []model.XUIOutboundView{{Tag: "direct"}}
	balancers := []model.XUIBalancerView{{Tag: "pool", OutboundTags: []string{"direct"}}, {Tag: "empty", OutboundTags: []string{"missing"}}, {Tag: "fallback", FallbackTag: "pool"}, {Tag: "loop", FallbackTag: "loop"}}
	for _, tc := range []struct {
		name          string
		rule          model.XUIRoutingRuleView
		state, reason string
	}{
		{"global always valid", model.XUIRoutingRuleView{Domain: []string{"geosite:private"}, OutboundTag: "missing"}, "valid", "全局"},
		{"API runtime only", model.XUIRoutingRuleView{Protected: true, InboundTags: []string{"api"}, OutboundTag: "api"}, "valid", "API"},
		{"disabled", model.XUIRoutingRuleView{Enabled: &no, Users: []string{"alice"}, OutboundTag: "direct"}, "disabled", "停用"},
		{"existing offline user", model.XUIRoutingRuleView{Users: []string{"alice"}, OutboundTag: "direct"}, "valid", "匹配 1"},
		{"deleted user", model.XUIRoutingRuleView{Users: []string{"removed"}, OutboundTag: "direct"}, "invalid", "无匹配客户端"},
		{"case sensitive", model.XUIRoutingRuleView{Users: []string{"Alice"}, OutboundTag: "direct"}, "invalid", "Alice"},
		{"partly orphaned", model.XUIRoutingRuleView{Users: []string{"alice", "removed"}, OutboundTag: "direct"}, "partial", "removed"},
		{"same client intersection", model.XUIRoutingRuleView{Users: []string{"bob"}, InboundTags: []string{"in-a"}, OutboundTag: "direct"}, "invalid", "bob"},
		{"existing inbound", model.XUIRoutingRuleView{InboundTags: []string{"in-a"}, OutboundTag: "direct"}, "valid", "入站"},
		{"deleted inbound", model.XUIRoutingRuleView{InboundTags: []string{"old"}, OutboundTag: "direct"}, "invalid", "入站不存在"},
		{"partial inbound", model.XUIRoutingRuleView{InboundTags: []string{"in-a", "old"}, OutboundTag: "direct"}, "partial", "old"},
		{"user regex", model.XUIRoutingRuleView{Users: []string{"regexp:^a"}, OutboundTag: "direct"}, "valid", "匹配"},
		{"invalid regex", model.XUIRoutingRuleView{Users: []string{"regexp:["}, OutboundTag: "direct"}, "invalid", "正则无效"},
		{"empty regex prefix is literal", model.XUIRoutingRuleView{Users: []string{"regexp:"}, OutboundTag: "direct"}, "invalid", "无匹配客户端"},
		{"missing outbound", model.XUIRoutingRuleView{Users: []string{"alice"}, OutboundTag: "missing"}, "invalid", "出站不存在"},
		{"balancer", model.XUIRoutingRuleView{Users: []string{"alice"}, BalancerTag: "pool"}, "valid", "匹配"},
		{"balancer fallback", model.XUIRoutingRuleView{Users: []string{"alice"}, BalancerTag: "fallback"}, "valid", "匹配"},
		{"balancer fallback cycle", model.XUIRoutingRuleView{Users: []string{"alice"}, BalancerTag: "loop"}, "invalid", "没有匹配"},
		{"empty balancer", model.XUIRoutingRuleView{Users: []string{"alice"}, BalancerTag: "empty"}, "invalid", "没有匹配"},
		{"missing balancer", model.XUIRoutingRuleView{Users: []string{"alice"}, BalancerTag: "old"}, "invalid", "均衡器不存在"},
		{"no destination", model.XUIRoutingRuleView{Users: []string{"alice"}}, "invalid", "未指定"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := []model.XUIRoutingRuleView{tc.rule}
			applyRoutingValidity(rules, nodes, clients, outbounds, balancers)
			if rules[0].Validity != tc.state || !strings.Contains(strings.Join(rules[0].ValidityReasons, ";"), tc.reason) {
				t.Fatalf("got %#v", rules[0])
			}
		})
	}
}

func TestRoutingValidityIncludesReverseGeneratedEndpoints(t *testing.T) {
	rules := []map[string]any{{"inboundTag": []string{"bridge"}, "outboundTag": "portal"}}
	overview := BuildXUIOverview(model.AgentSnapshot{XUI: &model.XUISnapshot{RoutingRules: rules, RawConfig: map[string]any{"reverse": map[string]any{"bridges": []map[string]any{{"tag": "bridge"}}, "portals": []map[string]any{{"tag": "portal"}}}}}})
	if overview.RoutingRules[0].Validity != "valid" {
		t.Fatalf("generated reverse route misclassified: %#v", overview.RoutingRules[0])
	}
	if len(overview.Nodes) != 0 || len(overview.Outbounds) != 0 {
		t.Fatal("validity targets must not become managed cards")
	}
}

func TestOverviewValidityUsesSavedMatchersAndDeletionMetadata(t *testing.T) {
	rules := []map[string]any{{"user": []string{"removed"}, "outboundTag": "direct"}}
	manual := []map[string]any{{"tag": "direct", "protocol": "freedom"}, {"tag": "old-relay", "protocol": "vless"}}
	overview := BuildXUIOverview(model.AgentSnapshot{XUI: &model.XUISnapshot{
		RoutingRules: rules, RoutingTemplate: &model.XUIRoutingTemplate{Rules: rules},
		Outbounds:        append(manual, map[string]any{"tag": "sub-relay", "protocol": "vless"}),
		OutboundTemplate: &model.XUIOutboundTemplate{Outbounds: manual, ConfigFingerprint: "saved", BlockedReasons: map[string]string{"direct": "默认出口不能删除"}},
	}})
	if overview.RoutingRules[0].Validity != "invalid" || (*overview.EditableRoutingRules)[0].Validity != "invalid" {
		t.Fatal("runtime and saved rules both need validity")
	}
	if overview.OutboundsFingerprint != "saved" {
		t.Fatal("saved guard missing")
	}
	for _, outbound := range overview.Outbounds {
		if outbound.Tag == "old-relay" && outbound.Fingerprint != model.RoutingFingerprint(manual[1]) {
			t.Fatal("saved outbound guard missing")
		}
		if outbound.Tag == "sub-relay" && (outbound.Fingerprint != "" || outbound.DeleteBlockedReason == "") {
			t.Fatal("runtime generated outbound became editable")
		}
	}
}
