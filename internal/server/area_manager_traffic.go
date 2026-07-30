package server

import (
	"sort"
	"strconv"
	"strings"
	"time"

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

type areaManagerRealtimeContextCache struct {
	expiresAt       time.Time
	snapshotByAgent map[string]model.AgentSnapshot
	forwarding      forwardedOverviewContext
}

const xuiRealtimeTrafficTTL = 10 * time.Second

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
	if (client.IsRealmForwarded || strings.TrimSpace(client.ForwardType) != "") && client.RealmTargetAgentID != "" {
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

func applyScopedClientTrafficToOutbounds(outbounds []model.XUIOutboundView, clients []model.XUIClientView) {
	byTag := make(map[string]scopedClientTraffic)
	for _, client := range clients {
		tag := strings.TrimSpace(client.Route.OutboundTag)
		if tag == "" || strings.TrimSpace(client.Route.BalancerTag) != "" {
			continue
		}
		totals := byTag[tag]
		totals.Sent += positiveTraffic(client.Up)
		totals.Recv += positiveTraffic(client.Down)
		totals.Total += positiveTraffic(client.TrafficTotal)
		totals.Count++
		byTag[tag] = totals
	}
	for index := range outbounds {
		totals := byTag[outbounds[index].Tag]
		outbounds[index].Up = int64(totals.Sent)
		outbounds[index].Down = int64(totals.Recv)
		if totals.Total > 0 {
			outbounds[index].Total = int64(totals.Total)
		} else {
			outbounds[index].Total = int64(totals.Sent + totals.Recv)
		}
	}
}

func (a *App) applyAreaManagerDashboardTraffic(user model.AdminUser, view *model.GlobalDashboardView, clientScope areaManagerClientScope) {
	a.applyAreaManagerDashboardTrafficWithRealtime(user, view, clientScope, nil)
}

func (a *App) applyAreaManagerDashboardTrafficWithRealtime(user model.AdminUser, view *model.GlobalDashboardView, clientScope areaManagerClientScope, metrics []model.AgentRealtimeMetrics) {
	if a == nil || a.store == nil || view == nil || !isAreaManager(user) {
		return
	}
	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return
	}
	snapshotByAgent := make(map[string]model.AgentSnapshot)
	for _, snapshot := range snapshots {
		snapshotByAgent[snapshot.AgentID] = snapshot
	}
	forwardingContext := buildForwardedOverviewContext(agents, snapshots)
	a.applyAreaManagerDashboardTrafficFromContext(user, view, clientScope, metrics, snapshotByAgent, forwardingContext)
}

func (a *App) applyAreaManagerDashboardTrafficFromContext(user model.AdminUser, view *model.GlobalDashboardView, clientScope areaManagerClientScope, metrics []model.AgentRealtimeMetrics, snapshotByAgent map[string]model.AgentSnapshot, forwardingContext forwardedOverviewContext) {
	realtimeAtByClient := applyRealtimeTrafficToForwardingContext(forwardingContext, metrics)
	for index := range view.Agents {
		agent := &view.Agents[index]
		snapshot, found := snapshotByAgent[agent.AgentID]
		if !found {
			agent.Summary = sanitizeAreaManagerSummary(agent.Summary)
			continue
		}
		overview := cloneForwardedBaseOverview(forwardingContext.targetOverviewByAgent[agent.AgentID])
		if overview == nil {
			overview = emptyAgentXUIOverview(snapshot, model.ManagedAgentConfig{
				AgentID:   agent.AgentID,
				AgentName: agent.AgentName,
			})
		}
		appendForwardedImportURLsWithContext(agent.AgentID, overview, forwardingContext)
		visibleClients := make([]model.XUIClientView, 0, len(overview.Clients))
		for _, client := range overview.Clients {
			if clientScope.allowsClient(agent.AgentID, client.InboundID, client.InboundTag, client.Email) ||
				a.areaManagerCanViewForwardedClient(user, agent.AgentID, client, clientScope) {
				visibleClients = append(visibleClients, client)
			}
		}
		overview.Clients = visibleClients
		if reportedAt := latestScopedRealtimeTrafficAt(agent.AgentID, visibleClients, realtimeAtByClient); !reportedAt.IsZero() {
			overview.ReportedAt = reportedAt
			overview.CollectedAt = reportedAt
		}
		// The dashboard path has not sanitized nodes; only use its filtered clients.
		overview.Nodes = nil
		agent.Summary = a.areaManagerScopedTrafficSummary(user, overview)
	}
}

func applyRealtimeTrafficToForwardingContext(context forwardedOverviewContext, metrics []model.AgentRealtimeMetrics) map[string]time.Time {
	realtimeAtByClient := make(map[string]time.Time)
	for _, metric := range metrics {
		traffic := metric.XUITraffic
		if traffic == nil || !freshXUIRealtimeTraffic(traffic.CollectedAt) {
			continue
		}
		overview := context.targetOverviewByAgent[metric.AgentID]
		if overview == nil {
			continue
		}
		byClient := make(map[string]model.XUIRealtimeClientTraffic, len(traffic.Clients)*2)
		for _, client := range traffic.Clients {
			key := areaClientExactKey(metric.AgentID, client.InboundID, client.InboundTag, client.Email)
			byClient[key] = client
			if strings.TrimSpace(client.InboundTag) != "" {
				byClient[areaClientExactKey(metric.AgentID, client.InboundID, "", client.Email)] = client
			}
		}
		for index := range overview.Clients {
			client := &overview.Clients[index]
			key := areaClientExactKey(metric.AgentID, client.InboundID, client.InboundTag, client.Email)
			realtimeClient, found := byClient[key]
			if !found && strings.TrimSpace(client.InboundTag) != "" {
				realtimeClient, found = byClient[areaClientExactKey(metric.AgentID, client.InboundID, "", client.Email)]
			}
			if !found {
				continue
			}
			client.Up = realtimeClient.Up
			client.Down = realtimeClient.Down
			client.TrafficTotal = realtimeClient.Up + realtimeClient.Down
			realtimeAtByClient[scopedClientTrafficKey(metric.AgentID, *client)] = traffic.CollectedAt
		}
	}
	return realtimeAtByClient
}

func freshXUIRealtimeTraffic(collectedAt time.Time) bool {
	if collectedAt.IsZero() {
		return false
	}
	age := time.Since(collectedAt)
	return age >= 0 && age <= xuiRealtimeTrafficTTL
}

func latestScopedRealtimeTrafficAt(agentID string, clients []model.XUIClientView, realtimeAtByClient map[string]time.Time) time.Time {
	var latest time.Time
	for _, client := range clients {
		if collectedAt := realtimeAtByClient[scopedClientTrafficKey(agentID, client)]; collectedAt.After(latest) {
			latest = collectedAt
		}
	}
	return latest
}

func (a *App) areaManagerRealtimeMetrics(user model.AdminUser, rawMetrics []model.AgentRealtimeMetrics) []model.AgentRealtimeMetrics {
	if a == nil || !isAreaManager(user) {
		return nil
	}
	clientScope := a.areaManagerClientScope(user)
	agentIDs := make([]string, 0, len(clientScope.agents))
	for agentID := range clientScope.agents {
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)
	view := model.GlobalDashboardView{Agents: make([]model.DashboardAgentView, 0, len(agentIDs))}
	for _, agentID := range agentIDs {
		view.Agents = append(view.Agents, model.DashboardAgentView{AgentID: agentID})
	}
	snapshotByAgent, forwardingContext, ok := a.areaManagerRealtimeContext()
	if ok {
		a.applyAreaManagerDashboardTrafficFromContext(user, &view, clientScope, rawMetrics, snapshotByAgent, forwardingContext)
	}

	reportedAt := time.Now().UTC()
	metrics := make([]model.AgentRealtimeMetrics, 0, len(view.Agents))
	for _, agent := range view.Agents {
		metrics = append(metrics, model.AgentRealtimeMetrics{
			AgentID:    agent.AgentID,
			ReportedAt: reportedAt,
			Summary:    agent.Summary,
		})
	}
	return metrics
}

func (a *App) areaManagerRealtimeContext() (map[string]model.AgentSnapshot, forwardedOverviewContext, bool) {
	if a == nil || a.store == nil {
		return nil, forwardedOverviewContext{}, false
	}
	now := time.Now()
	a.areaRealtimeMu.Lock()
	cached := a.areaRealtimeCache
	a.areaRealtimeMu.Unlock()
	if now.Before(cached.expiresAt) {
		return cloneAreaManagerRealtimeContext(cached)
	}

	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return nil, forwardedOverviewContext{}, false
	}
	snapshotByAgent := make(map[string]model.AgentSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotByAgent[snapshot.AgentID] = snapshot
	}
	cached = areaManagerRealtimeContextCache{
		expiresAt:       now.Add(dashboardCacheTTL),
		snapshotByAgent: snapshotByAgent,
		forwarding:      buildForwardedOverviewContext(agents, snapshots),
	}
	a.areaRealtimeMu.Lock()
	a.areaRealtimeCache = cached
	a.areaRealtimeMu.Unlock()
	return cloneAreaManagerRealtimeContext(cached)
}

func cloneAreaManagerRealtimeContext(cached areaManagerRealtimeContextCache) (map[string]model.AgentSnapshot, forwardedOverviewContext, bool) {
	if cached.snapshotByAgent == nil {
		return nil, forwardedOverviewContext{}, false
	}
	snapshots := make(map[string]model.AgentSnapshot, len(cached.snapshotByAgent))
	for agentID, snapshot := range cached.snapshotByAgent {
		snapshots[agentID] = snapshot
	}
	context := forwardedOverviewContext{
		agentMap:              make(map[string]model.DashboardAgentView, len(cached.forwarding.agentMap)),
		targetOverviewByAgent: make(map[string]*model.XUIOverview, len(cached.forwarding.targetOverviewByAgent)),
	}
	for agentID, agent := range cached.forwarding.agentMap {
		context.agentMap[agentID] = agent
	}
	for agentID, overview := range cached.forwarding.targetOverviewByAgent {
		context.targetOverviewByAgent[agentID] = cloneForwardedBaseOverview(overview)
	}
	return snapshots, context, true
}
