package panels

import (
	"context"
	"fmt"
	"sort"

	"bridge-core/internal/model"
)

func (c *XUIClient) deleteRoutingRules(ctx context.Context, payload map[string]any) (map[string]any, error) {
	request, err := model.ParseRoutingRulesDeletePayload(payload)
	if err != nil {
		return nil, err
	}
	mutable, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	routing, ok := mutable.config["routing"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("routing configuration is missing; no rules were deleted")
	}
	rules, err := strictRoutingRules(routing["rules"])
	if err != nil {
		return nil, err
	}
	if model.RoutingFingerprint(rules) != request.RulesFingerprint {
		return nil, fmt.Errorf("路由规则已变化，请刷新 Client 信息后重新选择；未删除任何规则")
	}
	// Validate the whole batch before modifying anything. Guarding the complete
	// ordered list also prevents a retry from deleting identical adjacent rules.
	for _, target := range request.Rules {
		if target.Index > len(rules) {
			return nil, fmt.Errorf("routing rule R%d does not exist; no rules were deleted", target.Index)
		}
		rule := rules[target.Index-1]
		if model.RoutingFingerprint(rule) != target.Fingerprint {
			return nil, fmt.Errorf("路由规则 R%d 已变化，请刷新后重新选择；未删除任何规则", target.Index)
		}
		if model.IsProtectedRoutingRule(rule, mutable.config) {
			return nil, fmt.Errorf("R%d 是 x-ui 系统 API 路由，不能删除；未删除任何规则", target.Index)
		}
	}
	sort.Slice(request.Rules, func(i, j int) bool { return request.Rules[i].Index > request.Rules[j].Index })
	deleted := make([]int, 0, len(request.Rules))
	for _, target := range request.Rules {
		index := target.Index - 1
		rules = append(rules[:index], rules[index+1:]...)
		deleted = append(deleted, target.Index)
	}
	routing["rules"] = rules
	if err := c.updateMutableXrayConfig(ctx, mutable); err != nil {
		return nil, fmt.Errorf("save routing rule deletion: %w", err)
	}
	result := map[string]any{
		"deleted_rule_indexes": deleted,
		"deleted_count":        len(deleted),
		"remaining_count":      len(rules),
		"saved":                true,
		"restarted":            false,
	}
	if err := c.restartXrayService(ctx); err != nil {
		return result, fmt.Errorf("路由规则已保存，但 Xray 重载失败，请手动重启 x-ui / Xray，不要重复删除：%w", err)
	}
	result["restarted"] = true
	return result, nil
}

// Unlike objectSlice, this must not silently drop malformed entries and shift
// their indices during a destructive operation.
func strictRoutingRules(raw any) ([]map[string]any, error) {
	if rules, ok := raw.([]map[string]any); ok {
		for _, rule := range rules {
			if rule == nil {
				return nil, fmt.Errorf("invalid routing rule; no rules were deleted")
			}
		}
		return rules, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("routing.rules must be an array; no rules were deleted")
	}
	rules := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rule, ok := item.(map[string]any)
		if !ok || rule == nil {
			return nil, fmt.Errorf("invalid routing rule; no rules were deleted")
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func routingTemplateForSnapshot(config map[string]any) *model.XUIRoutingTemplate {
	rules, err := strictRoutingRules(objectMap(config["routing"])["rules"])
	if err != nil {
		return nil
	}
	apiTag := stringFromMap(objectMap(config["api"]), "tag")
	return &model.XUIRoutingTemplate{Rules: rules, APITag: apiTag}
}
