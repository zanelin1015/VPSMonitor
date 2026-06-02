package server

import (
	"strings"

	"bridge-core/internal/model"
)

func (a *App) hydrateRealmForwardTargets(cfg model.ManagedAgentConfig) model.ManagedAgentConfig {
	if len(cfg.Entry.PortForwarding.Rules) == 0 {
		return cfg
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return cfg
	}
	agentByID := make(map[string]model.AgentRecord, len(agents))
	for _, agent := range agents {
		agentByID[agent.AgentID] = agent
	}
	for i := range cfg.Entry.PortForwarding.Rules {
		rule := &cfg.Entry.PortForwarding.Rules[i]
		if strings.TrimSpace(rule.TargetAddress) != "" || strings.TrimSpace(rule.TargetAgentID) == "" {
			continue
		}
		if agent, ok := agentByID[rule.TargetAgentID]; ok {
			rule.TargetAddress = preferredRealmForwardTargetAddress(agent)
		}
	}
	return cfg
}

func preferredRealmForwardTargetAddress(agent model.AgentRecord) string {
	if strings.TrimSpace(agent.Config.Entry.ImportDomain) != "" {
		return strings.TrimSpace(agent.Config.Entry.ImportDomain)
	}
	for _, address := range agent.Config.Entry.Addresses {
		if strings.TrimSpace(address) != "" {
			return strings.TrimSpace(address)
		}
	}
	if strings.TrimSpace(agent.Summary.ObservedIP) != "" {
		return strings.TrimSpace(agent.Summary.ObservedIP)
	}
	if strings.TrimSpace(agent.PublicIPv4) != "" {
		return strings.TrimSpace(agent.PublicIPv4)
	}
	if strings.TrimSpace(agent.Summary.PublicIPv4) != "" {
		return strings.TrimSpace(agent.Summary.PublicIPv4)
	}
	if strings.TrimSpace(agent.PublicIPv6) != "" {
		return strings.TrimSpace(agent.PublicIPv6)
	}
	return strings.TrimSpace(agent.Summary.PublicIPv6)
}
