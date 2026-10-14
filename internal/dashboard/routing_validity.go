package dashboard

import (
	"fmt"
	"regexp"
	"strings"

	"bridge-core/internal/model"
)

// Configuration validity is distinct from a live route-test: offline clients
// and zero counters do not make a saved matcher invalid. Conditions within a
// field are OR-ed, while inbound and user fields must match the same client.
func applyRoutingValidity(rules []model.XUIRoutingRuleView, nodes []model.XUINodeView, clients []model.XUIClientView, outbounds []model.XUIOutboundView, balancers []model.XUIBalancerView) {
	inboundTags := make(map[string]bool)
	outboundTags := make(map[string]bool)
	balancerByTag := make(map[string]model.XUIBalancerView)
	for _, balancer := range balancers {
		balancerByTag[balancer.Tag] = balancer
	}
	for _, node := range nodes {
		if node.Tag != "" {
			inboundTags[node.Tag] = true
		}
	}
	for _, outbound := range outbounds {
		if outbound.Tag != "" {
			outboundTags[outbound.Tag] = true
		}
	}
	for i := range rules {
		rule := &rules[i]
		patterns := make(map[string]*regexp.Regexp)
		rule.ValidityReasons = nil
		if rule.Enabled != nil && !*rule.Enabled {
			rule.Validity, rule.ValidityReasons = "disabled", []string{"已在面板停用"}
			continue
		}
		if rule.Protected {
			rule.Validity, rule.ValidityReasons = "valid", []string{"系统 API 路由"}
			continue
		}
		if len(rule.InboundTags) == 0 && len(rule.Users) == 0 {
			rule.Validity, rule.ValidityReasons = "valid", []string{"全局条件规则"}
			continue
		}
		matched := 0
		missing := make([]string, 0)
		for _, tag := range rule.InboundTags {
			if !inboundTags[tag] {
				missing = append(missing, "入站不存在："+tag)
			}
		}
		for _, user := range rule.Users {
			found := false
			foundElsewhere := false
			if len(user) > 7 && strings.HasPrefix(user, "regexp:") {
				pattern, err := regexp.Compile(strings.TrimPrefix(user, "regexp:"))
				if err != nil {
					missing = append(missing, "客户端正则无效："+user)
					continue
				}
				patterns[user] = pattern
			}
			for _, client := range clients {
				if routeUserMatches(user, client.Email, patterns) {
					foundElsewhere = true
					if len(rule.InboundTags) == 0 || containsString(rule.InboundTags, client.InboundTag) {
						found = true
						break
					}
				}
			}
			if !found {
				if foundElsewhere {
					missing = append(missing, "客户端不属于所选入站："+user)
				} else {
					missing = append(missing, "无匹配客户端："+user)
				}
			}
		}
		if len(rule.Users) > 0 {
			for _, client := range clients {
				if len(rule.InboundTags) > 0 && !containsString(rule.InboundTags, client.InboundTag) {
					continue
				}
				for _, user := range rule.Users {
					if routeUserMatches(user, client.Email, patterns) {
						matched++
						break
					}
				}
			}
		} else {
			for _, tag := range rule.InboundTags {
				if inboundTags[tag] {
					matched++
				}
			}
		}
		if matched == 0 {
			rule.Validity = "invalid"
			if len(missing) == 0 {
				missing = append(missing, "入口与客户端条件无法同时匹配")
			}
		} else if len(missing) > 0 {
			rule.Validity = "partial"
		} else {
			rule.Validity = "valid"
		}
		if rule.OutboundTag != "" && !outboundTags[rule.OutboundTag] {
			rule.Validity = "invalid"
			missing = append(missing, "出站不存在："+rule.OutboundTag)
		}
		if rule.BalancerTag != "" {
			_, found := balancerByTag[rule.BalancerTag]
			hasTarget := balancerHasOutbound(rule.BalancerTag, balancerByTag, outboundTags, make(map[string]bool))
			if !found {
				rule.Validity = "invalid"
				missing = append(missing, "均衡器不存在："+rule.BalancerTag)
			} else if !hasTarget {
				rule.Validity = "invalid"
				missing = append(missing, "均衡器没有匹配的出站")
			}
		}
		if rule.OutboundTag == "" && rule.BalancerTag == "" {
			rule.Validity = "invalid"
			missing = append(missing, "未指定出站或均衡器")
		}
		if len(missing) == 0 {
			kind := "入站"
			if len(rule.Users) > 0 {
				kind = "客户端"
			}
			missing = append(missing, fmt.Sprintf("匹配 %d 个%s", matched, kind))
		}
		rule.ValidityReasons = missing
	}
}

func balancerHasOutbound(tag string, balancers map[string]model.XUIBalancerView, outbounds, seen map[string]bool) bool {
	if seen[tag] {
		return false
	}
	seen[tag] = true
	balancer, found := balancers[tag]
	if !found {
		return false
	}
	for _, target := range balancer.OutboundTags {
		if outbounds[target] {
			return true
		}
	}
	if outbounds[balancer.FallbackTag] {
		return true
	}
	return balancer.FallbackTag != "" && balancerHasOutbound(balancer.FallbackTag, balancers, outbounds, seen)
}

func routeUserMatches(matcher, email string, patterns map[string]*regexp.Regexp) bool {
	if email == "" {
		return false
	}
	if len(matcher) > 7 && strings.HasPrefix(matcher, "regexp:") {
		pattern := patterns[matcher]
		return pattern != nil && pattern.MatchString(email)
	}
	return matcher == email
}

func applyOutboundDeletionView(snapshot *model.XUISnapshot, outbounds []model.XUIOutboundView) string {
	if snapshot.OutboundTemplate == nil {
		return ""
	}
	template := snapshot.OutboundTemplate
	byTag := make(map[string]map[string]any)
	for _, raw := range template.Outbounds {
		byTag[stringValue(raw["tag"])] = raw
	}
	for i := range outbounds {
		outbound := &outbounds[i]
		if raw, found := byTag[outbound.Tag]; found {
			outbound.Fingerprint = model.RoutingFingerprint(raw)
			outbound.DeleteBlockedReason = template.BlockedReasons[outbound.Tag]
		} else {
			outbound.DeleteBlockedReason = "运行时或订阅生成的出站，请到来源面板维护"
		}
	}
	return template.ConfigFingerprint
}
