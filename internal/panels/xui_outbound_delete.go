package panels

import (
	"context"
	"fmt"

	"bridge-core/internal/model"
)

func outboundTemplateForSnapshot(config map[string]any) *model.XUIOutboundTemplate {
	outbounds, err := strictOutbounds(config["outbounds"])
	if err != nil {
		return nil
	}
	blocked := make(map[string]string)
	for _, outbound := range outbounds {
		tag := stringValue(outbound["tag"])
		if reason := model.OutboundDeleteBlockedReason(config, tag); reason != "" {
			blocked[tag] = reason
		}
	}
	return &model.XUIOutboundTemplate{Outbounds: outbounds, ConfigFingerprint: model.RoutingFingerprint(config), BlockedReasons: blocked}
}

func strictOutbounds(raw any) ([]map[string]any, error) {
	outbounds, err := strictRoutingRules(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid outbounds; no outbounds were deleted: %w", err)
	}
	seen := make(map[string]bool)
	for _, outbound := range outbounds {
		tag := stringValue(outbound["tag"])
		if tag == "" || seen[tag] {
			return nil, fmt.Errorf("outbounds require unique nonempty tags; no outbounds were deleted")
		}
		seen[tag] = true
	}
	return outbounds, nil
}

func (c *XUIClient) deleteOutbounds(ctx context.Context, payload map[string]any) (map[string]any, error) {
	request, err := model.ParseOutboundsDeletePayload(payload)
	if err != nil {
		return nil, err
	}
	mutable, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	if model.RoutingFingerprint(mutable.config) != request.ConfigFingerprint {
		return nil, fmt.Errorf("Xray 配置已变化，请刷新 Client 信息后重新选择；未删除任何出站")
	}
	outbounds, err := strictOutbounds(mutable.config["outbounds"])
	if err != nil {
		return nil, err
	}
	byTag := make(map[string]map[string]any, len(outbounds))
	for _, outbound := range outbounds {
		byTag[stringValue(outbound["tag"])] = outbound
	}
	selected := make(map[string]bool, len(request.Outbounds))
	deleted := make([]string, 0, len(request.Outbounds))
	// Validate every target and every dependency before the single write.
	for _, target := range request.Outbounds {
		outbound, found := byTag[target.Tag]
		if !found || model.RoutingFingerprint(outbound) != target.Fingerprint {
			return nil, fmt.Errorf("出站 %s 已变化，请刷新后重新选择；未删除任何出站", target.Tag)
		}
		if reason := model.OutboundDeleteBlockedReason(mutable.config, target.Tag); reason != "" {
			return nil, fmt.Errorf("%s：%s；未删除任何出站", target.Tag, reason)
		}
		selected[target.Tag] = true
		deleted = append(deleted, target.Tag)
	}
	remaining := make([]map[string]any, 0, len(outbounds)-len(selected))
	for _, outbound := range outbounds {
		if !selected[stringValue(outbound["tag"])] {
			remaining = append(remaining, outbound)
		}
	}
	mutable.config["outbounds"] = remaining
	writeErr := c.updateMutableXrayConfig(ctx, mutable)
	result := map[string]any{"submitted": true, "saved": writeErr == nil, "verified": false, "restarted": false, "requested_tags": deleted, "deleted_count": 0, "remaining_count": len(remaining)}
	// A panel's success envelope alone is not proof of a saved deletion.
	// An update can also persist and then fail during the panel's hot reload.
	// Read back even on errors and never blindly replay the write.
	readback, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return result, fmt.Errorf("出站删除已提交，但回读核验失败，请刷新核对，不要重复提交：%w", err)
	}
	actual, err := strictOutbounds(readback.config["outbounds"])
	if err != nil {
		return result, err
	}
	for _, outbound := range actual {
		if !selected[stringValue(outbound["tag"])] {
			continue
		}
		if writeErr != nil {
			if model.RoutingFingerprint(readback.config) == request.ConfigFingerprint {
				return nil, fmt.Errorf("save outbound deletion: %w", writeErr)
			}
			return result, fmt.Errorf("出站删除返回错误且配置已变化，请刷新核对，不要重复提交：%w", writeErr)
		}
		return result, fmt.Errorf("面板返回成功，但出站 %s 仍存在，请刷新核对", stringValue(outbound["tag"]))
	}
	result["saved"] = true
	result["verified"] = true
	result["deleted_tags"] = deleted
	result["deleted_count"] = len(deleted)
	if err := c.restartXrayService(ctx); err != nil {
		return result, fmt.Errorf("出站已删除并核验，但 Xray 重载失败，请手动重启 x-ui / Xray，不要重复删除：%w", err)
	}
	result["restarted"] = true
	return result, nil
}
