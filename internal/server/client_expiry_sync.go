package server

import (
	"log"
	"math"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

const clientExpirySyncHour = 0

func (a *App) startClientExpiryScheduler() {
	if a == nil {
		return
	}
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), clientExpirySyncHour, 0, 0, 0, now.Location())
			if !next.After(now) {
				next = next.AddDate(0, 0, 1)
			}
			timer := time.NewTimer(time.Until(next))
			<-timer.C
			a.syncAllClientExpiryRules("daily_midnight")
		}
	}()
}

func (a *App) syncAllClientExpiryRules(reason string) {
	agents, err := a.store.ListAgents()
	if err != nil {
		log.Printf("client expiry sync list agents: %v", err)
		return
	}
	for _, agent := range agents {
		count := a.syncAgentClientExpiryRules(agent, reason)
		if count > 0 {
			log.Printf("client expiry sync queued %d x-ui action(s) for %s", count, agent.AgentID)
		}
	}
}

func (a *App) syncAgentClientExpiryRules(agent model.AgentRecord, reason string) int {
	if a == nil || agent.AgentID == "" || !agent.Config.XUI.Enabled || len(agent.Config.Renewal.ClientBillings) == 0 {
		return 0
	}
	snapshot, _ := a.store.GetLatest(agent.AgentID)
	var clients []model.XUIClientView
	if overview := dashboard.BuildXUIOverview(snapshot); overview != nil {
		clients = overview.Clients
	}
	now := time.Now()
	queued := 0
	for _, billing := range agent.Config.Renewal.ClientBillings {
		action, ok := buildClientExpirySyncAction(agent.AgentID, billing, clients, now, reason)
		if !ok {
			continue
		}
		created, err := a.store.CreateXUIAction(agent.AgentID, action)
		if err != nil {
			log.Printf("create client expiry sync action for %s: %v", agent.AgentID, err)
			continue
		}
		a.dispatchXUIActionRealtime(agent.AgentID, created)
		queued++
	}
	return queued
}

func buildClientExpirySyncAction(agentID string, billing model.XUIClientBillingConfig, clients []model.XUIClientView, now time.Time, reason string) (model.XUIActionRequest, bool) {
	if billing.Email == "" || (billing.InboundID <= 0 && billing.InboundTag == "") {
		return model.XUIActionRequest{}, false
	}
	expiryMillis := expectedClientBillingExpiryMillis(billing, now)
	if expiryMillis <= 0 {
		return model.XUIActionRequest{}, false
	}
	client := findOverviewClient(clients, billing)
	expired := !billing.ExpireAutoRenew && !time.UnixMilli(expiryMillis).After(now)
	needsUpdate := client == nil || !expiryMillisClose(client.ExpiryTime, expiryMillis)
	if expired && client != nil && client.Enabled {
		needsUpdate = true
	}
	if !needsUpdate {
		return model.XUIActionRequest{}, false
	}
	payload := map[string]any{
		"inbound_id":        billing.InboundID,
		"inbound_tag":       billing.InboundTag,
		"email":             billing.Email,
		"expiry_time":       expiryMillis,
		"expire_cycle":      normalizeClientExpireCycle(billing.ExpireCycle),
		"expire_auto_renew": billing.ExpireAutoRenew,
		"sync_reason":       reason,
	}
	if agentID != "" {
		payload["agent_id"] = agentID
	}
	if billing.StartTime > 0 {
		payload["start_time"] = billing.StartTime
	}
	if expired {
		payload["enabled"] = false
	}
	return model.XUIActionRequest{Kind: model.XUIActionUpdateClientExpiry, Payload: payload}, true
}

func expectedClientBillingExpiryMillis(billing model.XUIClientBillingConfig, now time.Time) int64 {
	if billing.StartTime > 0 {
		return calculateClientBillingExpiryMillis(billing.StartTime, normalizeClientExpireCycle(billing.ExpireCycle), billing.ExpireAutoRenew, now)
	}
	return billing.ExpireTime
}

func calculateClientBillingExpiryMillis(startMillis int64, cycle string, autoRenew bool, now time.Time) int64 {
	if startMillis <= 0 {
		return 0
	}
	periodStart := startOfClientBillingDay(time.UnixMilli(startMillis))
	nextStart := addClientExpireCycle(periodStart, cycle)
	periodEnd := nextStart.Add(-time.Second)
	for autoRenew && !periodEnd.After(now) {
		periodStart = nextStart
		nextStart = addClientExpireCycle(periodStart, cycle)
		periodEnd = nextStart.Add(-time.Second)
	}
	return periodEnd.UnixMilli()
}

func startOfClientBillingDay(value time.Time) time.Time {
	local := value.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

func expiryMillisClose(current int64, expected int64) bool {
	if current <= 0 || expected <= 0 {
		return current == expected
	}
	return math.Abs(float64(current-expected)) <= float64(time.Minute.Milliseconds())
}
