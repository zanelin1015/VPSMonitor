package server

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

type areaManagerTrafficSample struct {
	ReportedAt time.Time
	Scope      string
	Sent       uint64
	Recv       uint64
	UpSpeed    uint64
	DownSpeed  uint64
}

type scopedClientTraffic struct {
	Sent    uint64
	Recv    uint64
	Total   uint64
	AllTime uint64
	Count   int
}

func (a *App) areaManagerScopedTrafficSummary(user model.AdminUser, overview *model.XUIOverview) model.VPSSummary {
	if overview == nil {
		return model.VPSSummary{}
	}
	summary := sanitizeAreaManagerSummary(overview.Summary)
	totals, scope := scopedOverviewTrafficTotals(overview)
	summary.NetTrafficSent = totals.Sent
	summary.NetTrafficRecv = totals.Recv
	summary.NetTrafficTotal = totals.Sent + totals.Recv
	if a == nil || !isAreaManager(user) || user.ID <= 0 {
		return summary
	}

	reportedAt := overview.ReportedAt.UTC()
	if reportedAt.IsZero() {
		reportedAt = overview.CollectedAt.UTC()
	}
	cacheKey := strings.Join([]string{user.Role, strconv.FormatInt(user.ID, 10), overview.AgentID}, "\x00")
	a.areaTrafficMu.Lock()
	if a.areaTrafficSamples == nil {
		a.areaTrafficSamples = make(map[string]areaManagerTrafficSample)
	}
	previous, found := a.areaTrafficSamples[cacheKey]
	next := areaManagerTrafficSample{
		ReportedAt: reportedAt,
		Scope:      scope,
		Sent:       totals.Sent,
		Recv:       totals.Recv,
	}
	if found && previous.Scope == scope {
		next.UpSpeed = previous.UpSpeed
		next.DownSpeed = previous.DownSpeed
		if reportedAt.After(previous.ReportedAt) {
			seconds := reportedAt.Sub(previous.ReportedAt).Seconds()
			next.UpSpeed = trafficRate(totals.Sent, previous.Sent, seconds)
			next.DownSpeed = trafficRate(totals.Recv, previous.Recv, seconds)
		}
	}
	a.areaTrafficSamples[cacheKey] = next
	a.areaTrafficMu.Unlock()

	if reportedAt.IsZero() || time.Since(reportedAt) > realtimeMetricTTL {
		return summary
	}
	summary.NetIOUp = next.UpSpeed
	summary.NetIODown = next.DownSpeed
	return summary
}

func trafficRate(current, previous uint64, seconds float64) uint64 {
	if seconds <= 0 || current < previous {
		return 0
	}
	return uint64(float64(current-previous) / seconds)
}

func scopedClientTrafficTotals(agentID string, clients []model.XUIClientView) (scopedClientTraffic, string) {
	totals := scopedClientTraffic{}
	keys := make([]string, 0, len(clients))
	seen := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		key := scopedClientTrafficKey(agentID, client)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
		totals.Sent += positiveTraffic(client.Up)
		totals.Recv += positiveTraffic(client.Down)
		totals.Total += positiveTraffic(client.TrafficTotal)
		totals.AllTime += positiveTraffic(client.AllTime)
		totals.Count++
	}
	sort.Strings(keys)
	return totals, strings.Join(keys, "\x01")
}

func scopedOverviewTrafficTotals(overview *model.XUIOverview) (scopedClientTraffic, string) {
	if overview == nil {
		return scopedClientTraffic{}, ""
	}
	totals, clientScope := scopedClientTrafficTotals(overview.AgentID, overview.Clients)
	scopeKeys := []string{clientScope}
	for _, node := range overview.Nodes {
		if node.ClientCount != 0 || (node.Up <= 0 && node.Down <= 0) {
			continue
		}
		totals.Sent += positiveTraffic(node.Up)
		totals.Recv += positiveTraffic(node.Down)
		scopeKeys = append(scopeKeys, "node:"+overviewInboundKey(node.ID, node.Tag))
	}
	sort.Strings(scopeKeys)
	return totals, strings.Join(scopeKeys, "\x01")
}

func scopedClientTrafficKey(agentID string, client model.XUIClientView) string {
	if client.IsRealmForwarded && client.RealmTargetAgentID != "" {
		return strings.Join([]string{
			client.RealmTargetAgentID,
			strconv.Itoa(client.RealmTargetInboundID),
			strings.ToLower(strings.TrimSpace(client.RealmTargetInboundTag)),
			strings.ToLower(strings.TrimSpace(client.Email)),
		}, "\x00")
	}
	return areaClientExactKey(agentID, client.InboundID, client.InboundTag, client.Email)
}

func positiveTraffic(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func applyScopedClientTrafficToNodes(agentID string, nodes []model.XUINodeView, clients []model.XUIClientView, clientScope areaManagerClientScope) {
	byInbound := make(map[string]scopedClientTraffic)
	for _, client := range clients {
		key := overviewInboundKey(client.InboundID, client.InboundTag)
		totals := byInbound[key]
		totals.Sent += positiveTraffic(client.Up)
		totals.Recv += positiveTraffic(client.Down)
		totals.Total += positiveTraffic(client.TrafficTotal)
		totals.AllTime += positiveTraffic(client.AllTime)
		totals.Count++
		byInbound[key] = totals
	}
	for index := range nodes {
		totals := byInbound[overviewInboundKey(nodes[index].ID, nodes[index].Tag)]
		if totals.Count == 0 && clientScope.allowsInbound(agentID, nodes[index].ID, nodes[index].Tag) {
			nodes[index].ClientCount = 0
			continue
		}
		nodes[index].ClientCount = totals.Count
		nodes[index].Up = int64(totals.Sent)
		nodes[index].Down = int64(totals.Recv)
		nodes[index].Total = int64(totals.Sent + totals.Recv)
		if totals.AllTime > 0 {
			nodes[index].AllTime = int64(totals.AllTime)
		} else {
			nodes[index].AllTime = int64(totals.Total)
		}
	}
}

func (a *App) applyAreaManagerDashboardTraffic(user model.AdminUser, view *model.GlobalDashboardView, clientScope areaManagerClientScope) {
	if a == nil || a.store == nil || view == nil || !isAreaManager(user) {
		return
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return
	}
	entryByAgent := make(map[string]model.AgentEntryConfig, len(agents))
	for _, agent := range agents {
		entryByAgent[agent.AgentID] = agent.Config.Entry
	}
	snapshotByAgent := make(map[string]model.AgentSnapshot)
	for _, snapshot := range a.store.ListLatest() {
		snapshotByAgent[snapshot.AgentID] = snapshot
	}
	for index := range view.Agents {
		agent := &view.Agents[index]
		snapshot, found := snapshotByAgent[agent.AgentID]
		if !found {
			agent.Summary = sanitizeAreaManagerSummary(agent.Summary)
			continue
		}
		overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{Entry: entryByAgent[agent.AgentID]})
		if overview == nil {
			agent.Summary = sanitizeAreaManagerSummary(agent.Summary)
			continue
		}
		visibleClients := make([]model.XUIClientView, 0, len(overview.Clients))
		for _, client := range overview.Clients {
			if clientScope.allowsClient(agent.AgentID, client.InboundID, client.InboundTag, client.Email) {
				visibleClients = append(visibleClients, client)
			}
		}
		overview.Clients = visibleClients
		// The dashboard path has not sanitized nodes; only use its filtered clients.
		overview.Nodes = nil
		agent.Summary = a.areaManagerScopedTrafficSummary(user, overview)
	}
}
