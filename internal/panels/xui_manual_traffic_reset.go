package panels

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"bridge-core/internal/model"
)

// This is deliberately separate from the retired scheduled-reset action.
func (c *XUIClient) manualResetClientTraffic(ctx context.Context, payload map[string]any) (map[string]any, error) {
	req, err := model.ParseXUIManualTrafficResetPayload(payload)
	if err != nil {
		return nil, err
	}
	target, err := c.loadClientUpdateTarget(ctx, req.InboundID, req.InboundTag, req.Email, req.ClientID)
	if err != nil {
		return nil, err
	}
	// Other actions support a legacy port/tag lookup. A destructive reset must
	// match the actual inbound ID, even when a stale tag resolves elsewhere.
	if intValue(target.inbound["id"]) != req.InboundID || (req.InboundTag != "" && stringValue(target.inbound["tag"]) != req.InboundTag) {
		return nil, fmt.Errorf("客户端入站信息已变化，请刷新后重试")
	}
	if isAccountBasedInboundProtocol(stringValue(target.inbound["protocol"])) {
		return nil, fmt.Errorf("节点账号不支持单客户端流量重置")
	}
	enabled, valid := boolPayloadValue(target.client, "enable")
	if !valid {
		return nil, fmt.Errorf("无法确认客户端启用状态，未执行流量重置")
	}
	before, err := c.readManualResetTraffic(ctx, req.Email, target.clientAPI)
	if err != nil {
		return nil, err
	}
	path := "/panel/api/clients/resetTraffic/" + url.PathEscape(req.Email)
	if !target.clientAPI {
		path = fmt.Sprintf("/panel/api/inbounds/%d/resetClientTraffic/%s", req.InboundID, url.PathEscape(req.Email))
	}
	result := map[string]any{
		"email": req.Email, "inbound_id": req.InboundID,
		"submitted": true, "reset_done": false, "verified": false,
		"before_up": before.up, "before_down": before.down,
	}
	// Never replay this POST, including after an ambiguous transport failure.
	_, resetErr := c.postJSON(ctx, path, map[string]any{})
	result["reset_done"] = resetErr == nil
	verifyCtx := ctx
	if !enabled {
		// v3's reset endpoint auto-enables a disabled client. Restore its state
		// even if the reset response was lost or the action deadline elapsed.
		var cancel context.CancelFunc
		verifyCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
	}
	fresh, readErr := c.loadClientUpdateTarget(verifyCtx, req.InboundID, "", req.Email, req.ClientID)
	if readErr != nil {
		return result, fmt.Errorf("流量重置已请求，但无法回读客户端配置或确认启用状态；请核对面板，勿重复提交: %w", readErr)
	}
	if intValue(fresh.inbound["id"]) != req.InboundID {
		return result, fmt.Errorf("重置后客户端入站已变化，无法完成核验")
	}
	if !enabled && fresh.client["enable"] != false {
		fresh.client["enable"] = false
		if _, err := c.saveClientUpdate(verifyCtx, fresh, "enable"); err != nil {
			return result, fmt.Errorf("流量重置已请求，但恢复客户端禁用状态失败，请立即检查面板: %w", err)
		}
	}
	for _, field := range []string{"enable", "totalGB", "expiryTime", "trafficReset", "trafficResetDay", "reset", "resetDay", "resetWeekday", "resetMax"} {
		if _, exists := target.client[field]; exists {
			if err := verifyClientUpdate(target.client, fresh.client, field); err != nil {
				return result, fmt.Errorf("重置后的客户端配置与原配置不一致: %w", err)
			}
		}
	}
	result["enabled"] = enabled
	if resetErr != nil {
		return result, fmt.Errorf("重置请求未确认完成，请先刷新核对流量，勿重复提交: %w", resetErr)
	}
	after, err := c.readManualResetTraffic(verifyCtx, req.Email, target.clientAPI)
	if err != nil {
		return result, fmt.Errorf("面板已接受重置，但流量回读失败，请刷新核对: %w", err)
	}
	result["after_up"], result["after_down"] = after.up, after.down
	// Active connections can accrue traffic immediately after a reset. A
	// decrease in each previously nonzero counter proves the reset.
	if (before.up > 0 && after.up >= before.up) || (before.down > 0 && after.down >= before.down) ||
		(before.up == 0 && before.down == 0 && (after.up != 0 || after.down != 0)) {
		return result, fmt.Errorf("面板返回成功，但未观察到流量清零或下降，请刷新核对；不会自动重复重置")
	}
	result["verified"] = true
	return result, nil
}

type manualResetTraffic struct{ up, down int64 }

func (c *XUIClient) readManualResetTraffic(ctx context.Context, email string, clientAPI bool) (manualResetTraffic, error) {
	path := "/panel/api/clients/traffic/" + url.PathEscape(email)
	if !clientAPI {
		path = "/panel/api/inbounds/getClientTraffics/" + url.PathEscape(email)
	}
	traffic, err := c.getJSONObject(ctx, path)
	if err != nil {
		return manualResetTraffic{}, err
	}
	if stringValue(traffic["email"]) != email {
		return manualResetTraffic{}, fmt.Errorf("流量记录与目标客户端不匹配")
	}
	up, upErr := clientUpdateInt64(traffic["up"], "up")
	down, downErr := clientUpdateInt64(traffic["down"], "down")
	if upErr != nil || downErr != nil || up < 0 || down < 0 {
		return manualResetTraffic{}, fmt.Errorf("客户端流量记录无效")
	}
	return manualResetTraffic{up: up, down: down}, nil
}
