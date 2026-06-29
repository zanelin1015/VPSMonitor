package server

import (
	"time"

	"bridge-core/internal/model"
)

func (a *App) dispatchDisableClientService(agent model.AgentRecord) bool {
	return a.realtime.sendAgentControl(agent.AgentID, model.AgentControlMessage{
		Type: model.AgentControlDisableClient,
		Payload: map[string]any{
			"service_name":         "vpsmonitor-client",
			"windows_service_name": "VPSMonitorClient",
			"agent_os":             agent.OS,
		},
	})
}

func (a *App) removeAgentRealtimeAfterDisable(agentID string, sent bool) {
	if !sent {
		a.realtime.removeAgent(agentID)
		return
	}
	go func() {
		time.Sleep(5 * time.Second)
		a.realtime.removeAgent(agentID)
	}()
}
