package server

import (
	"fmt"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) validateSnellForwardRules(sourceAgentID string, cfg model.SnellConfig) error {
	if !cfg.Enabled {
		return nil
	}
	for _, rule := range cfg.ForwardRules {
		if !rule.Enabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(sourceAgentID), strings.TrimSpace(rule.TargetAgentID)) {
			return fmt.Errorf("Snell 转发不能指向当前 Client")
		}
		overview := a.xuiOverviewForOutboundAuthorization(rule.TargetAgentID)
		if overview == nil {
			return fmt.Errorf("Snell 转发目标 Client %s 当前没有可用的 X-UI 快照", rule.TargetAgentID)
		}
		found := false
		for _, client := range overview.Clients {
			if rule.TargetInboundID > 0 && client.InboundID != rule.TargetInboundID {
				continue
			}
			if rule.TargetInboundTag != "" && !strings.EqualFold(rule.TargetInboundTag, client.InboundTag) {
				continue
			}
			if rule.TargetClientID != "" {
				if rule.TargetClientID != client.ClientID {
					continue
				}
			} else if rule.TargetClientEmail != "" && !strings.EqualFold(rule.TargetClientEmail, client.Email) {
				continue
			}
			if strings.TrimSpace(client.ImportURL) == "" {
				return fmt.Errorf("Snell 转发目标客户端 %s 没有可用的导入地址", firstNonEmptyString(client.Email, client.ClientID, client.InboundTag))
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("Snell 转发目标客户端不存在或已变化")
		}
	}
	return nil
}
