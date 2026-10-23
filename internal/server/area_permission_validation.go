package server

import (
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (a *App) validateAreaRoleIDs(req model.AreaManagerAccountRequest) error {
	for _, group := range []struct {
		ids     []string
		role    string
		enabled *bool
	}{{req.LineAgentIDs, model.AgentRoleLine, req.LineEnabled}, {req.LandingAgentIDs, model.AgentRoleLanding, req.LandingEnabled}} {
		if group.enabled != nil && !*group.enabled && len(group.ids) > 0 {
			return fmt.Errorf("请先开启%s权限", group.role)
		}
		for _, id := range group.ids {
			agent, found, err := a.store.GetAgent(id)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("Client 不存在")
			}
			v := agentRoleView(agent)
			if group.role == model.AgentRoleLine && !v.LineEnabled || group.role == model.AgentRoleLanding && !v.LandingEnabled {
				return fmt.Errorf("Client %s 未开启%s", id, group.role)
			}
		}
	}
	if req.LandingEnabled != nil && !*req.LandingEnabled && req.OutboundCreateEnabled != nil && *req.OutboundCreateEnabled {
		return fmt.Errorf("请先开启落地机权限")
	}
	if req.LineEnabled != nil && !*req.LineEnabled && len(req.OutboundGrants) > 0 {
		return fmt.Errorf("请先开启线路机权限")
	}
	if req.FrontLinkEnabled != nil && !*req.FrontLinkEnabled && len(req.FrontProxyNodeIDs) > 0 {
		return fmt.Errorf("请先开启前置链接权限")
	}
	return nil
}
func (a *App) validateAreaAssignments(id int64, assignments []model.AreaManagerAssignmentRequest) error {
	manager, found, err := a.store.GetAreaManager(id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("area manager not found")
	}
	for _, item := range assignments {
		agent, found, err := a.store.GetAgent(item.AgentID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("Client 不存在")
		}
		tag := strings.ToLower(item.InboundTag)
		if strings.HasPrefix(tag, "realm:") || strings.HasPrefix(tag, "haproxy:") {
			if !manager.RealmEnabled {
				return fmt.Errorf("前置入口权限未开启")
			}
			snapshot, ok := a.store.GetLatest(item.AgentID)
			if !ok || time.Since(snapshot.ReportedAt) > 2*time.Minute {
				return fmt.Errorf("Client 尚未上报运行状态")
			}
			running := false
			if strings.HasPrefix(tag, "realm:") && snapshot.Realm != nil {
				running = snapshot.Realm.Running && snapshot.Realm.Error == ""
			}
			if strings.HasPrefix(tag, "haproxy:") && snapshot.HAProxy != nil {
				running = snapshot.HAProxy.Running && snapshot.HAProxy.Error == ""
			}
			if !running {
				return fmt.Errorf("Client %s 的 Realm / HAProxy 未运行", item.AgentID)
			}
		} else {
			roles := agentRoleView(agent)
			if !(manager.LineEnabled && roles.LineEnabled || manager.LandingEnabled && roles.LandingEnabled) {
				return fmt.Errorf("Client %s 未开启线路机或落地机权限", item.AgentID)
			}
		}
	}
	return nil
}
