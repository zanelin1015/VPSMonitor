package panels

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"bridge-core/internal/model"
)

func boolValue(value any) bool { v, ok := value.(bool); return ok && v }

// A reset endpoint may enable the traffic row. Refuse expired, manually
// disabled and ambiguously-disabled clients before touching that endpoint.
func (c *XUIClient) trafficResetTarget(ctx context.Context, payload map[string]any, verifying bool) (map[string]any, map[string]any, map[string]any, error) {
	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, nil, nil, err
	}
	inboundID := intValue(payload["inbound_id"])
	email := stringValue(payload["email"])
	clientID := stringValue(payload["client_id"])
	if inboundID <= 0 || email == "" || clientID == "" {
		return nil, nil, nil, fmt.Errorf("stable client identity is required")
	}
	var target, client, stats map[string]any
	matches := 0
	idMatches := 0
	for _, inbound := range inbounds {
		settings, _, e := decodeInboundSettings(inbound["settings"])
		if e != nil {
			return nil, nil, nil, e
		}
		for _, item := range objectSlice(settings["clients"]) {
			if model.TrafficResetClientIdentity(stringValue(inbound["protocol"]), stringValue(item["id"]), stringValue(item["password"])) == clientID {
				idMatches++
			}
			if stringValue(item["email"]) != email {
				continue
			}
			matches++
			if intValue(inbound["id"]) == inboundID {
				target = inbound
				client = item
			}
		}
	}
	if matches != 1 || idMatches != 1 || target == nil || model.TrafficResetClientIdentity(stringValue(target["protocol"]), stringValue(client["id"]), stringValue(client["password"])) != clientID {
		return nil, nil, nil, fmt.Errorf("client identity changed or email spans multiple inbounds; reset refused")
	}
	for _, stat := range objectSlice(target["clientStats"]) {
		if stringValue(stat["email"]) == email {
			stats = stat
			break
		}
	}
	if stats == nil {
		return nil, nil, nil, fmt.Errorf("client traffic row missing; reset refused")
	}
	if !boolValue(target["enable"]) || !boolValue(client["enable"]) {
		return nil, nil, nil, fmt.Errorf("manually disabled client or inbound; reset refused")
	}
	if stringValue(target["protocol"]) != stringValue(payload["expected_protocol"]) || int64Value(client["totalGB"]) != int64Value(payload["expected_total"]) || int64Value(client["expiryTime"]) != int64Value(payload["expected_expiry"]) {
		return nil, nil, nil, fmt.Errorf("quota, protocol or expiry changed after planning")
	}
	now := time.Now().UnixMilli()
	expiry := int64Value(client["expiryTime"])
	if expiry < 0 {
		return nil, nil, nil, fmt.Errorf("relative expiry is not supported for automatic traffic reset")
	}
	if expiry > 0 && expiry <= now {
		return nil, nil, nil, fmt.Errorf("client expired; awaiting manual renewal, expiry was not changed")
	}
	if statsExpiry := int64Value(stats["expiryTime"]); statsExpiry > 0 && statsExpiry <= now {
		return nil, nil, nil, fmt.Errorf("traffic row expired; awaiting manual renewal")
	}
	if !verifying && !boolValue(stats["enable"]) {
		total := int64Value(stats["total"])
		used := int64Value(stats["up"]) + int64Value(stats["down"])
		if total <= 0 || used < total {
			return nil, nil, nil, fmt.Errorf("disable cause is not a depleted quota; reset refused")
		}
	}
	return target, client, stats, nil
}

func (c *XUIClient) ResetClientPeriodTraffic(ctx context.Context, payload map[string]any, beforeMutation func() error) (map[string]any, error) {
	if err := c.ensureActionSession(ctx); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if now < int64Value(payload["not_before"]) || now > int64Value(payload["deadline"]) {
		return nil, fmt.Errorf("outside traffic reset execution window")
	}
	inbound, client, before, err := c.trafficResetTarget(ctx, payload, false)
	if err != nil {
		return nil, err
	}
	original, err := json.Marshal(client)
	if err != nil {
		return nil, err
	}
	if beforeMutation == nil {
		return nil, fmt.Errorf("durable reset journal is required")
	}
	if err = beforeMutation(); err != nil {
		return nil, err
	}
	result := map[string]any{"reset_attempted": true, "reset_done": false, "uncertain": true, "restarted": false, "previous_all_time": int64Value(before["allTime"]), "configuration_fingerprint": fmt.Sprintf("%x", sha256.Sum256(original))}
	// No authentication retry here: the response can be lost after the reset.
	_, err = c.postJSON(ctx, fmt.Sprintf("/panel/api/inbounds/%d/resetClientTraffic/%s", intValue(inbound["id"]), url.PathEscape(stringValue(client["email"]))), map[string]any{})
	if err != nil {
		return result, err
	}
	result["reset_done"] = true
	result["uncertain"] = false
	return c.verifyClientPeriodTraffic(ctx, payload, result)
}

func (c *XUIClient) verifyClientPeriodTraffic(ctx context.Context, payload map[string]any, result map[string]any) (map[string]any, error) {
	_, client, stats, err := c.trafficResetTarget(ctx, payload, true)
	if err != nil {
		return result, err
	}
	current, err := json.Marshal(client)
	if err != nil {
		return result, err
	}
	if fingerprint := stringValue(result["configuration_fingerprint"]); fingerprint == "" || fingerprint != fmt.Sprintf("%x", sha256.Sum256(current)) {
		return result, fmt.Errorf("client configuration changed during reset; investigate before retry")
	}
	if int64Value(stats["allTime"]) < int64Value(result["previous_all_time"]) {
		return result, fmt.Errorf("panel reset changed historical traffic; disable this policy and investigate")
	}
	if !boolValue(stats["enable"]) {
		return result, fmt.Errorf("traffic reset completed but client remains disabled; only verification may be retried")
	}
	if int64Value(stats["up"]) != 0 || int64Value(stats["down"]) != 0 {
		result["up"] = int64Value(stats["up"])
		result["down"] = int64Value(stats["down"])
		return result, fmt.Errorf("reset acknowledged but readback has non-zero usage (new traffic or delayed statistics); verify only, never reset again")
	}
	result["enabled"] = true
	result["up"] = int64Value(stats["up"])
	result["down"] = int64Value(stats["down"])
	result["message"] = "本周期流量已重置并回读确认；额度、到期时间及客户端配置未修改"
	return result, nil
}

// Verification is intentionally non-destructive. A lost reset acknowledgement
// must not trigger a second reset of newly accumulated traffic.
func (c *XUIClient) VerifyClientPeriodTraffic(ctx context.Context, payload map[string]any, result map[string]any) (map[string]any, error) {
	if err := c.ensureActionSession(ctx); err != nil {
		return result, err
	}
	return c.verifyClientPeriodTraffic(ctx, payload, result)
}
