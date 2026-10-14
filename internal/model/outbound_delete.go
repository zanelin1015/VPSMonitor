package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const XUIOutboundsDeleteMinVersion = "0.3.49"

// Only saved template outbounds are editable. Runtime generated outbounds must
// never be removed by copying the running configuration back into the panel.
type XUIOutboundTemplate struct {
	Outbounds         []map[string]any  `json:"outbounds"`
	ConfigFingerprint string            `json:"config_fingerprint"`
	BlockedReasons    map[string]string `json:"blocked_reasons,omitempty"`
}

type OutboundDeleteTarget struct {
	Tag         string `json:"tag"`
	Fingerprint string `json:"fingerprint"`
}

type OutboundsDeletePayload struct {
	ConfigFingerprint string                 `json:"config_fingerprint"`
	Outbounds         []OutboundDeleteTarget `json:"outbounds"`
}

func ParseOutboundsDeletePayload(payload map[string]any) (OutboundsDeletePayload, error) {
	var result OutboundsDeletePayload
	body, err := json.Marshal(payload)
	if err == nil {
		err = json.Unmarshal(body, &result)
	}
	if err != nil {
		return result, fmt.Errorf("invalid outbound delete payload: %w", err)
	}
	if !validRoutingFingerprint(result.ConfigFingerprint) {
		return result, fmt.Errorf("config_fingerprint is required; refresh outbounds before deleting")
	}
	if len(result.Outbounds) == 0 || len(result.Outbounds) > 1000 {
		return result, fmt.Errorf("select between 1 and 1000 outbounds")
	}
	seen := make(map[string]bool, len(result.Outbounds))
	for _, target := range result.Outbounds {
		if strings.TrimSpace(target.Tag) == "" || !validRoutingFingerprint(target.Fingerprint) || seen[target.Tag] {
			return result, fmt.Errorf("each outbound requires a unique nonempty tag and fingerprint")
		}
		seen[target.Tag] = true
	}
	return result, nil
}

// Keep all references intact, including disabled routes and proxy chains. The
// administrator can remove a reference explicitly before deleting its target.
func OutboundDeleteBlockedReason(config map[string]any, tag string) string {
	outbounds := outboundObjectList(config["outbounds"])
	if len(outbounds) > 0 && outbounds[0]["tag"] == tag {
		return "默认出口不能删除"
	}
	apiTag := "api"
	if api, ok := config["api"].(map[string]any); ok {
		if value, ok := api["tag"].(string); ok && value != "" {
			apiTag = value
		}
	}
	if tag == apiTag {
		return "系统 API 出站不能删除"
	}
	if routing, ok := config["routing"].(map[string]any); ok {
		for i, rule := range outboundObjectList(routing["rules"]) {
			if rule["outboundTag"] == tag {
				return fmt.Sprintf("被路由 R%d 引用，请先调整或删除该路由", i+1)
			}
		}
		for _, balancer := range outboundObjectList(routing["balancers"]) {
			name, _ := balancer["tag"].(string)
			if balancer["fallbackTag"] == tag || outboundSelectorMatches(tag, balancer["selector"]) {
				return "被均衡器 " + name + " 引用，请先调整均衡器"
			}
		}
	}
	for _, key := range []string{"observatory", "burstObservatory"} {
		if observer, ok := config[key].(map[string]any); ok && outboundSelectorMatches(tag, observer["subjectSelector"]) {
			return "被 " + key + " 探测配置引用，请先调整探测配置"
		}
	}
	if path := outboundReferencePath(config, tag, "config"); path != "" {
		return "被 " + path + " 引用，请先调整代理链或配置"
	}
	return ""
}

func outboundReferencePath(value any, tag, path string) string {
	switch item := value.(type) {
	case map[string]any:
		for _, key := range []string{"outboundTag", "dialerProxy", "detour"} {
			if item[key] == tag {
				return path + "." + key
			}
		}
		if proxy, ok := item["proxySettings"].(map[string]any); ok && proxy["tag"] == tag {
			return path + ".proxySettings"
		}
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if found := outboundReferencePath(item[key], tag, path+"."+key); found != "" {
				return found
			}
		}
	case []map[string]any:
		for i, child := range item {
			if found := outboundReferencePath(child, tag, fmt.Sprintf("%s[%d]", path, i)); found != "" {
				return found
			}
		}
	case []any:
		for i, child := range item {
			if found := outboundReferencePath(child, tag, fmt.Sprintf("%s[%d]", path, i)); found != "" {
				return found
			}
		}
	}
	return ""
}

func outboundSelectorMatches(tag string, raw any) bool {
	var values []string
	switch selectors := raw.(type) {
	case []string:
		values = selectors
	case []any:
		for _, value := range selectors {
			if s, ok := value.(string); ok {
				values = append(values, s)
			}
		}
	}
	for _, prefix := range values {
		if strings.HasPrefix(tag, prefix) {
			return true
		}
	}
	return false
}

func outboundObjectList(value any) []map[string]any {
	if list, ok := value.([]map[string]any); ok {
		return list
	}
	var result []map[string]any
	if list, ok := value.([]any); ok {
		for _, item := range list {
			if object, ok := item.(map[string]any); ok {
				result = append(result, object)
			}
		}
	}
	return result
}
