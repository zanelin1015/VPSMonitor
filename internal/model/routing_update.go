package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const XUIRoutingRuleUpdateMinVersion = "0.3.50"

type RoutingRuleUpdatePayload struct {
	RulesFingerprint string         `json:"rules_fingerprint"`
	RuleIndex        int            `json:"rule_index"`
	RuleFingerprint  string         `json:"rule_fingerprint"`
	Rule             map[string]any `json:"rule"`
}

func ParseRoutingRuleUpdatePayload(payload map[string]any) (RoutingRuleUpdatePayload, error) {
	var result RoutingRuleUpdatePayload
	body, err := json.Marshal(payload)
	if err == nil && len(body) > 256*1024 {
		return result, fmt.Errorf("路由规则不能超过 256 KiB")
	}
	if err == nil {
		err = json.Unmarshal(body, &result)
	}
	if err != nil {
		return result, fmt.Errorf("invalid routing update payload: %w", err)
	}
	if result.RuleIndex <= 0 || !validRoutingFingerprint(result.RulesFingerprint) || !validRoutingFingerprint(result.RuleFingerprint) {
		return result, fmt.Errorf("路由规则校验信息无效，请刷新后重新编辑")
	}
	if err := ValidateRoutingRule(result.Rule); err != nil {
		return result, err
	}
	if RoutingFingerprint(result.Rule) == result.RuleFingerprint {
		return result, fmt.Errorf("规则内容未变化，无需保存")
	}
	return result, nil
}

func ValidateRoutingRule(rule map[string]any) error {
	if rule == nil {
		return fmt.Errorf("rule must be an object")
	}
	if value, ok := rule["type"]; ok && value != "field" {
		return fmt.Errorf("仅支持 field 路由规则")
	}
	outbound, outboundOK := rule["outboundTag"].(string)
	balancer, balancerOK := rule["balancerTag"].(string)
	if (rule["outboundTag"] != nil && !outboundOK) || (rule["balancerTag"] != nil && !balancerOK) || (strings.TrimSpace(outbound) == "") == (strings.TrimSpace(balancer) == "") {
		return fmt.Errorf("请选择一个出站或均衡器，不能同时指定两者")
	}
	if outbound != strings.TrimSpace(outbound) || balancer != strings.TrimSpace(balancer) {
		return fmt.Errorf("出站和均衡器标签不能包含首尾空格")
	}
	if value, ok := rule["enabled"]; ok {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("enabled must be a boolean")
		}
	}
	for _, key := range []string{"domain", "ip", "source", "sourceIP", "localIP", "user", "inboundTag", "protocol", "process"} {
		value, exists := rule[key]
		if !exists {
			continue
		}
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s 必须是字符串数组", key)
		}
		for _, item := range items {
			if text, ok := item.(string); !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("%s 不能包含空值或非字符串", key)
			}
		}
	}
	for _, key := range []string{"port", "sourcePort", "localPort"} {
		if value, ok := rule[key]; ok {
			if err := validateRoutingPorts(value); err != nil {
				return fmt.Errorf("%s：%w", key, err)
			}
		}
	}
	if value, ok := rule["network"]; ok {
		var networks []any
		switch value := value.(type) {
		case string:
			for _, network := range strings.Split(value, ",") {
				networks = append(networks, strings.TrimSpace(network))
			}
		case []any:
			networks = value
		}
		if len(networks) == 0 {
			return fmt.Errorf("network 必须是 tcp、udp 或 tcp,udp")
		}
		for _, network := range networks {
			if network != "tcp" && network != "udp" {
				return fmt.Errorf("network 必须是 tcp、udp 或 tcp,udp")
			}
		}
	}
	for _, key := range []string{"domain", "ip", "port", "sourcePort", "localPort", "network", "source", "sourceIP", "localIP", "user", "inboundTag", "protocol", "attrs", "process", "vlessRoute"} {
		switch value := rule[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return nil
			}
		case float64:
			if value >= 0 {
				return nil
			}
		case []any:
			if len(value) > 0 {
				return nil
			}
		case map[string]any:
			if len(value) > 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("请至少保留一项匹配条件，空条件规则不能保存")
}

func validateRoutingPorts(value any) error {
	text, ok := value.(string)
	if number, numeric := value.(float64); numeric && number == float64(int(number)) {
		text, ok = strconv.Itoa(int(number)), true
	}
	if !ok || strings.TrimSpace(text) == "" {
		return fmt.Errorf("请输入端口或范围，例如 443,1000-2000")
	}
	for _, part := range strings.Split(text, ",") {
		bounds := strings.Split(strings.TrimSpace(part), "-")
		if len(bounds) > 2 {
			return fmt.Errorf("端口范围无效")
		}
		previous := 0
		for _, bound := range bounds {
			port, err := strconv.Atoi(strings.TrimSpace(bound))
			if err != nil || port < 1 || port > 65535 || port < previous {
				return fmt.Errorf("端口必须为 1–65535，范围起点不能大于终点")
			}
			previous = port
		}
	}
	return nil
}
