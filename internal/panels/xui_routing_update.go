package panels

import (
	"context"
	"fmt"

	"bridge-core/internal/model"
)

func (c *XUIClient) updateRoutingRule(ctx context.Context, payload map[string]any) (map[string]any, error) {
	request, err := model.ParseRoutingRuleUpdatePayload(payload)
	if err != nil {
		return nil, err
	}
	mutable, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	routing := objectMap(mutable.config["routing"])
	rules, err := strictRoutingRules(routing["rules"])
	if err != nil {
		return nil, err
	}
	if model.RoutingFingerprint(rules) != request.RulesFingerprint || request.RuleIndex > len(rules) || model.RoutingFingerprint(rules[request.RuleIndex-1]) != request.RuleFingerprint {
		return nil, fmt.Errorf("路由规则已变化，请刷新后重新编辑；未修改任何规则")
	}
	if model.IsProtectedRoutingRule(rules[request.RuleIndex-1], mutable.config) || model.IsProtectedRoutingRule(request.Rule, mutable.config) {
		return nil, fmt.Errorf("系统 API 路由不能修改，也不能将普通规则指向系统 API 出站")
	}
	target, key := stringFromMap(request.Rule, "outboundTag"), "outbounds"
	objects := objectSlice(mutable.config[key])
	if target == "" {
		target, key = stringFromMap(request.Rule, "balancerTag"), "均衡器"
		objects = objectSlice(routing["balancers"])
	}
	found := false
	for _, object := range objects {
		if stringFromMap(object, "tag") == target {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%s 目标不存在：%s；未修改任何规则", key, target)
	}
	// Replace exactly this saved rule. Do not move it to the front like the
	// routing-creation workflow, and keep the rest of the template intact.
	rules[request.RuleIndex-1] = request.Rule
	routing["rules"] = rules
	wanted := model.RoutingFingerprint(rules)
	writeErr := c.updateMutableXrayConfig(ctx, mutable)
	result := map[string]any{"rule_index": request.RuleIndex, "submitted": true, "saved": writeErr == nil, "verified": false, "restarted": false}
	readback, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return result, fmt.Errorf("路由修改已提交，但回读失败，请刷新核对，不要重复提交：%w", err)
	}
	actual, err := strictRoutingRules(objectMap(readback.config["routing"])["rules"])
	if err != nil {
		return result, fmt.Errorf("路由修改已提交，但回读的规则无效，请刷新核对：%w", err)
	}
	if model.RoutingFingerprint(actual) != wanted {
		if writeErr != nil && model.RoutingFingerprint(actual) == request.RulesFingerprint {
			return nil, fmt.Errorf("保存路由失败：%w", writeErr)
		}
		return result, fmt.Errorf("路由修改未通过回读核验，请刷新核对，不要重复提交")
	}
	result["saved"], result["verified"] = true, true
	if err := c.restartXrayService(ctx); err != nil {
		return result, fmt.Errorf("路由已保存并核验，但 Xray 重载失败，请手动重启 x-ui / Xray，不要重复保存：%w", err)
	}
	result["restarted"] = true
	return result, nil
}
