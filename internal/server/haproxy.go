package server

import (
	"fmt"
	"net"
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (a *App) hydrateHAProxyTargets(cfg model.ManagedAgentConfig) (model.ManagedAgentConfig, error) {
	if !cfg.Entry.HAProxy.Enabled || len(cfg.Entry.HAProxy.Rules) == 0 {
		return cfg, nil
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return cfg, fmt.Errorf("加载 HAProxy 目标 Client 失败: %w", err)
	}
	latest := latestSnapshotsByAgent(a.store.ListLatest())
	for index := range agents {
		if snapshot, ok := latest[agents[index].AgentID]; ok {
			agents[index].Config.Entry = dashboard.MergeRealmSnapshotIntoEntry(agents[index].Config.Entry, snapshot.Realm)
		}
	}
	return hydrateHAProxyTargetsFromAgents(cfg, agents)
}

func hydrateHAProxyTargetsFromAgents(cfg model.ManagedAgentConfig, agents []model.AgentRecord) (model.ManagedAgentConfig, error) {
	agentByID := make(map[string]model.AgentRecord, len(agents))
	for _, agent := range agents {
		agentByID[agent.AgentID] = agent
	}
	for ruleIndex := range cfg.Entry.HAProxy.Rules {
		rule := &cfg.Entry.HAProxy.Rules[ruleIndex]
		if !rule.Enabled {
			continue
		}
		rule.ListenAddress = strings.TrimSpace(rule.ListenAddress)
		if rule.ListenAddress == "" {
			rule.ListenAddress = "0.0.0.0"
		}
		if rule.CheckIntervalSeconds <= 0 {
			rule.CheckIntervalSeconds = 3
		}
		if rule.ConnectTimeoutSeconds <= 0 {
			rule.ConnectTimeoutSeconds = 5
		}
		if rule.Fall <= 0 {
			rule.Fall = 3
		}
		if rule.Rise <= 0 {
			rule.Rise = 2
		}
		label := haProxyRuleLabel(*rule)
		primary, err := hydrateHAProxyRealmTarget(cfg.AgentID, label, "主节点", rule.Primary, agentByID)
		if err != nil {
			return cfg, err
		}
		rule.Primary = primary
		for backupIndex := range rule.Backups {
			backup, err := hydrateHAProxyRealmTarget(cfg.AgentID, label, fmt.Sprintf("备用节点 %d", backupIndex+1), rule.Backups[backupIndex], agentByID)
			if err != nil {
				return cfg, err
			}
			rule.Backups[backupIndex] = backup
		}
	}
	return cfg, nil
}

func hydrateHAProxyRealmTarget(sourceAgentID, ruleLabel, targetLabel string, target model.HAProxyRealmTarget, agents map[string]model.AgentRecord) (model.HAProxyRealmTarget, error) {
	target.AgentID = strings.TrimSpace(target.AgentID)
	target.RealmRuleID = strings.TrimSpace(target.RealmRuleID)
	if target.AgentID == "" {
		return target, fmt.Errorf("HAProxy %s 的%s未选择 Client Realm 规则", ruleLabel, targetLabel)
	}
	if target.AgentID == sourceAgentID {
		return target, fmt.Errorf("HAProxy %s 的%s不能选择当前 Client", ruleLabel, targetLabel)
	}
	agent, ok := agents[target.AgentID]
	if !ok {
		return target, fmt.Errorf("HAProxy %s 的%s引用了不存在的 Client %q", ruleLabel, targetLabel, target.AgentID)
	}
	if !agent.Config.Entry.PortForwarding.Enabled || strings.EqualFold(strings.TrimSpace(agent.Config.Entry.PortForwarding.Backend), "none") {
		return target, fmt.Errorf("HAProxy %s 的%s Client %q 未启用 Realm", ruleLabel, targetLabel, firstNonEmptyString(agent.AgentName, agent.AgentID))
	}
	realmRule, ok := findHAProxyRealmRule(agent.Config.Entry.PortForwarding.Rules, target.RealmRuleID, target.Port)
	if !ok {
		return target, fmt.Errorf("HAProxy %s 的%s在 Client %q 中找不到对应的 Realm 监听规则", ruleLabel, targetLabel, firstNonEmptyString(agent.AgentName, agent.AgentID))
	}
	if !realmRule.Enabled {
		return target, fmt.Errorf("HAProxy %s 的%s引用的 Realm 规则未启用", ruleLabel, targetLabel)
	}
	network := strings.ToLower(strings.TrimSpace(realmRule.Network))
	if network == "udp" {
		return target, fmt.Errorf("HAProxy %s 的%s引用了仅 UDP 的 Realm 规则，HAProxy 主备仅支持 TCP", ruleLabel, targetLabel)
	}
	address := preferredRealmForwardTargetAddress(agent)
	if address == "" {
		return target, fmt.Errorf("HAProxy %s 的%s Client %q 没有可用的主域名或公网 IP", ruleLabel, targetLabel, firstNonEmptyString(agent.AgentName, agent.AgentID))
	}
	target.RealmRuleID = realmRule.ID
	target.Address = address
	target.Port = realmRule.ListenPort
	return target, nil
}

func findHAProxyRealmRule(rules []model.RealmForwardRule, ruleID string, listenPort int) (model.RealmForwardRule, bool) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID != "" {
		for _, rule := range rules {
			if strings.EqualFold(strings.TrimSpace(rule.ID), ruleID) {
				return rule, true
			}
		}
	}
	if listenPort > 0 {
		for _, rule := range rules {
			if rule.ListenPort == listenPort {
				return rule, true
			}
		}
	}
	return model.RealmForwardRule{}, false
}

func validateHAProxyConfig(cfg model.HAProxyConfig, realm model.RealmForwardConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if realm.Enabled && !strings.EqualFold(strings.TrimSpace(realm.Backend), "none") {
		return fmt.Errorf("同一个 Client 的 HAProxy 与 Realm 只能启用一个，请先关闭 Realm")
	}
	listenPorts := make(map[int]struct{}, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		if !rule.Enabled {
			continue
		}
		label := haProxyRuleLabel(rule)
		if rule.ListenPort <= 0 || rule.ListenPort > 65535 {
			return fmt.Errorf("HAProxy %s 的监听端口无效", label)
		}
		if _, exists := listenPorts[rule.ListenPort]; exists {
			return fmt.Errorf("HAProxy 监听端口 %d 重复，一个端口只能配置一条主备规则", rule.ListenPort)
		}
		listenPorts[rule.ListenPort] = struct{}{}
		listenAddress := strings.TrimSpace(rule.ListenAddress)
		if listenAddress != "" && listenAddress != "*" && net.ParseIP(strings.Trim(listenAddress, "[]")) == nil {
			return fmt.Errorf("HAProxy %s 的监听地址必须是本机 IP、0.0.0.0 或 ::", label)
		}
		if err := validateHydratedHAProxyTarget(label, "主节点", rule.Primary); err != nil {
			return err
		}
		if len(rule.Backups) == 0 {
			return fmt.Errorf("HAProxy %s 至少需要一个备用节点", label)
		}
		seenTargets := map[string]struct{}{haProxyResolvedTargetKey(rule.Primary): {}}
		for index, target := range rule.Backups {
			if err := validateHydratedHAProxyTarget(label, fmt.Sprintf("备用节点 %d", index+1), target); err != nil {
				return err
			}
			key := haProxyResolvedTargetKey(target)
			if _, exists := seenTargets[key]; exists {
				return fmt.Errorf("HAProxy %s 的主备节点存在重复目标 %s:%d", label, target.Address, target.Port)
			}
			seenTargets[key] = struct{}{}
		}
		if rule.CheckIntervalSeconds < 1 || rule.CheckIntervalSeconds > 300 {
			return fmt.Errorf("HAProxy %s 的健康检查间隔必须在 1 到 300 秒之间", label)
		}
		if rule.ConnectTimeoutSeconds < 1 || rule.ConnectTimeoutSeconds > 60 {
			return fmt.Errorf("HAProxy %s 的连接超时必须在 1 到 60 秒之间", label)
		}
		if rule.Fall < 1 || rule.Fall > 20 || rule.Rise < 1 || rule.Rise > 20 {
			return fmt.Errorf("HAProxy %s 的失败/恢复次数必须在 1 到 20 之间", label)
		}
	}
	return nil
}

func validateForwardingFeatureSelection(features model.AgentFeatureConfig) error {
	if features.Realm && features.HAProxy {
		return fmt.Errorf("同一个 Client 的 HAProxy 与 Realm 功能只能选择一个")
	}
	return nil
}

type haProxyResolvedPath struct {
	TargetLabel string
	Target      model.HAProxyRealmTarget
	Resolution  realmForwardResolution
}

func resolveHAProxyRulePaths(rule model.HAProxyRule, context forwardedOverviewContext) ([]haProxyResolvedPath, error) {
	targets := make([]struct {
		label  string
		target model.HAProxyRealmTarget
	}, 0, 1+len(rule.Backups))
	targets = append(targets, struct {
		label  string
		target model.HAProxyRealmTarget
	}{label: "主节点", target: rule.Primary})
	for index, target := range rule.Backups {
		targets = append(targets, struct {
			label  string
			target model.HAProxyRealmTarget
		}{label: fmt.Sprintf("备用节点 %d", index+1), target: target})
	}

	paths := make([]haProxyResolvedPath, 0, len(targets))
	for _, item := range targets {
		targetAgent, ok := context.agentMap[item.target.AgentID]
		if !ok {
			return nil, fmt.Errorf("%s引用的 Client %q 不存在", item.label, item.target.AgentID)
		}
		if !realmForwardingActive(targetAgent.Entry.PortForwarding) {
			return nil, fmt.Errorf("%s引用的 Client %q 未启用 Realm", item.label, firstNonEmptyString(targetAgent.AgentName, item.target.AgentID))
		}
		realmRule, ok := findHAProxyRealmRule(targetAgent.Entry.PortForwarding.Rules, item.target.RealmRuleID, item.target.Port)
		if !ok || !realmRule.Enabled {
			return nil, fmt.Errorf("%s引用的 Realm 规则不存在或未启用", item.label)
		}
		resolution := resolveRealmForwardTarget(item.target.AgentID, realmRule, context.agentMap, context.targetOverviewByAgent)
		if !resolution.Resolved {
			return nil, fmt.Errorf("%s无法解析到最终 x-ui 节点: %s", item.label, firstNonEmptyString(resolution.UnresolvedReason, "目标链路不可用"))
		}
		paths = append(paths, haProxyResolvedPath{TargetLabel: item.label, Target: item.target, Resolution: resolution})
	}
	return paths, nil
}

func validateHAProxyResolvedPaths(rule model.HAProxyRule, paths []haProxyResolvedPath) error {
	if len(paths) == 0 {
		return fmt.Errorf("HAProxy %s 没有可用的主备路径", haProxyRuleLabel(rule))
	}
	primary := paths[0]
	primaryKey := haProxyFinalNodeKey(primary.Resolution)
	for _, path := range paths[1:] {
		if haProxyFinalNodeKey(path.Resolution) == primaryKey {
			continue
		}
		return fmt.Errorf(
			"HAProxy %s 的%s与主节点最终落点不一致: 主节点为 %s，%s为 %s",
			haProxyRuleLabel(rule),
			path.TargetLabel,
			haProxyFinalNodeLabel(primary.Resolution),
			path.TargetLabel,
			haProxyFinalNodeLabel(path.Resolution),
		)
	}
	return nil
}

func haProxyFinalNodeKey(resolution realmForwardResolution) string {
	nodeIdentity := ""
	if resolution.FinalNode.ID > 0 {
		nodeIdentity = fmt.Sprintf("id:%d", resolution.FinalNode.ID)
	} else {
		nodeIdentity = "tag:" + strings.ToLower(strings.TrimSpace(resolution.FinalNode.Tag))
	}
	return strings.Join([]string{
		strings.TrimSpace(resolution.FinalAgentID),
		nodeIdentity,
		strings.ToLower(strings.TrimSpace(resolution.FinalNode.Protocol)),
		fmt.Sprint(resolution.FinalNode.Port),
	}, "\x00")
}

func haProxyFinalNodeLabel(resolution realmForwardResolution) string {
	node := firstNonEmptyString(resolution.FinalNode.Remark, resolution.FinalNode.Tag, fmt.Sprintf("Inbound #%d", resolution.FinalNode.ID))
	return fmt.Sprintf("%s / %s / %s:%d", resolution.FinalAgentID, node, resolution.FinalNode.Protocol, resolution.FinalNode.Port)
}

func (a *App) validateHAProxyTargetCompatibility(cfg model.HAProxyConfig) error {
	if a == nil || a.store == nil || !cfg.Enabled {
		return nil
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return fmt.Errorf("加载 HAProxy 兼容性校验数据失败: %w", err)
	}
	context := buildForwardedOverviewContext(agents, a.store.ListLatest())
	for _, rule := range cfg.Rules {
		if !rule.Enabled {
			continue
		}
		paths, err := resolveHAProxyRulePaths(rule, context)
		if err != nil {
			return fmt.Errorf("HAProxy %s 校验失败: %w", haProxyRuleLabel(rule), err)
		}
		if err := validateHAProxyResolvedPaths(rule, paths); err != nil {
			return err
		}
	}
	return nil
}

func validateHydratedHAProxyTarget(ruleLabel, targetLabel string, target model.HAProxyRealmTarget) error {
	if strings.TrimSpace(target.AgentID) == "" || strings.TrimSpace(target.RealmRuleID) == "" {
		return fmt.Errorf("HAProxy %s 的%s必须从 Client Realm 规则中选择", ruleLabel, targetLabel)
	}
	if target.Port <= 0 || target.Port > 65535 {
		return fmt.Errorf("HAProxy %s 的%s端口无效", ruleLabel, targetLabel)
	}
	host := strings.TrimSpace(target.Address)
	if host == "" || strings.Contains(host, "://") || strings.ContainsAny(host, "/?# \t\r\n") {
		return fmt.Errorf("HAProxy %s 的%s地址无效", ruleLabel, targetLabel)
	}
	return nil
}

func haProxyRuleLabel(rule model.HAProxyRule) string {
	if label := strings.TrimSpace(rule.Name); label != "" {
		return label
	}
	return fmt.Sprintf("监听端口 %d", rule.ListenPort)
}

func haProxyResolvedTargetKey(target model.HAProxyRealmTarget) string {
	return strings.ToLower(strings.TrimSpace(target.Address)) + "\x00" + fmt.Sprint(target.Port)
}
