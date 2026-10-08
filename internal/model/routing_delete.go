package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// RoutingFingerprint includes every field, including panel-specific metadata.
// Array order is significant because Xray evaluates routing rules in order.
func RoutingFingerprint(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type RoutingRuleDeleteTarget struct {
	Index       int    `json:"index"`
	Fingerprint string `json:"fingerprint"`
}

// Keep saved rules separate from the running configuration: the latter can
// omit disabled rules and include generated routes that are not editable.
type XUIRoutingTemplate struct {
	Rules  []map[string]any `json:"rules"`
	APITag string           `json:"api_tag,omitempty"`
}

type RoutingRulesDeletePayload struct {
	RulesFingerprint string                    `json:"rules_fingerprint"`
	Rules            []RoutingRuleDeleteTarget `json:"rules"`
}

func ParseRoutingRulesDeletePayload(payload map[string]any) (RoutingRulesDeletePayload, error) {
	var result RoutingRulesDeletePayload
	body, err := json.Marshal(payload)
	if err == nil {
		err = json.Unmarshal(body, &result)
	}
	if err != nil {
		return result, fmt.Errorf("invalid routing delete payload: %w", err)
	}
	if !validRoutingFingerprint(result.RulesFingerprint) {
		return result, fmt.Errorf("rules_fingerprint is required; refresh routing rules before deleting")
	}
	if len(result.Rules) == 0 || len(result.Rules) > 1000 {
		return result, fmt.Errorf("select between 1 and 1000 routing rules")
	}
	seen := make(map[int]bool, len(result.Rules))
	for _, target := range result.Rules {
		if target.Index <= 0 || !validRoutingFingerprint(target.Fingerprint) {
			return result, fmt.Errorf("each routing rule requires a positive integer index and fingerprint")
		}
		if seen[target.Index] {
			return result, fmt.Errorf("duplicate routing rule index: R%d", target.Index)
		}
		seen[target.Index] = true
	}
	return result, nil
}

func validRoutingFingerprint(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == hex.EncodeToString(decoded)
}

// The API route is necessary for panel control and must not be deleted.
func IsProtectedRoutingRule(rule, config map[string]any) bool {
	apiTag := "api"
	if api, ok := config["api"].(map[string]any); ok {
		if tag, ok := api["tag"].(string); ok && tag != "" {
			apiTag = tag
		}
	}
	return rule["outboundTag"] == apiTag
}
