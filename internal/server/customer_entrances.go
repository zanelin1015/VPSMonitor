package server

import (
	"slices"
	"strings"
	"time"

	"bridge-core/internal/model"
)

type customerEntranceRule struct {
	kind, id, network string
	port              int
	enabled           bool
	publication       model.CustomerRouteConfig
	realm             model.RealmForwardRule
	haproxy           model.HAProxyRule
}

type customerEntranceResolution struct {
	realmForwardResolution
	viaIX bool
}

func findCustomerEntranceRule(entry model.AgentEntryConfig, port int) customerEntranceRule {
	if entry.HAProxy.Enabled {
		for _, rule := range entry.HAProxy.Rules {
			if rule.ListenPort == port {
				return customerEntranceRule{kind: "haproxy", id: rule.ID, network: "tcp", port: port, enabled: rule.Enabled, publication: rule.CustomerRoute, haproxy: rule}
			}
		}
	}
	for _, rule := range entry.PortForwarding.Rules {
		if rule.ListenPort == port {
			return customerEntranceRule{kind: "realm", id: rule.ID, network: rule.Network, port: port, enabled: rule.Enabled && realmForwardingActive(entry.PortForwarding), publication: rule.CustomerRoute, realm: rule}
		}
	}
	for _, rule := range entry.HAProxy.Rules {
		if rule.ListenPort == port {
			return customerEntranceRule{kind: "haproxy", id: rule.ID, network: "tcp", port: port, publication: rule.CustomerRoute, haproxy: rule}
		}
	}
	return customerEntranceRule{}
}

func agentHasPurpose(agent model.DashboardAgentView, purpose string) bool {
	return agent.Entry.Classification != nil && slices.Contains(agent.Entry.Classification.Purposes, purpose)
}

func internalCustomerEntrance(agent model.DashboardAgentView, publication model.CustomerRouteConfig) bool {
	return publication.Visibility == "internal" ||
		(publication.Visibility != "public" && agentHasPurpose(agent, model.AgentPurposeIXRelay) &&
			!agentHasPurpose(agent, "domestic_entry") && !agentHasPurpose(agent, "optimized_relay"))
}

func (rule customerEntranceRule) resolve(agent model.DashboardAgentView, context forwardedOverviewContext) customerEntranceResolution {
	result := customerEntranceResolution{viaIX: agentHasPurpose(agent, model.AgentPurposeIXRelay)}
	switch rule.kind {
	case "realm":
		result.realmForwardResolution = resolveRealmForwardTarget(agent.AgentID, rule.realm, context.agentMap, context.targetOverviewByAgent)
		result.viaIX = result.viaIX || customerRealmPathUsesIX(result.realmForwardResolution, context.agentMap)
	case "haproxy":
		paths, err := resolveHAProxyRulePaths(rule.haproxy, context)
		if err == nil && len(paths) > 0 && validateHAProxyResolvedPaths(rule.haproxy, paths) == nil {
			result.realmForwardResolution = paths[0].Resolution
			for _, path := range paths {
				result.viaIX = result.viaIX || customerRealmPathUsesIX(path.Resolution, context.agentMap)
			}
		} else {
			for _, target := range append([]model.HAProxyRealmTarget{rule.haproxy.Primary}, rule.haproxy.Backups...) {
				result.viaIX = result.viaIX || agentHasPurpose(context.agentMap[target.AgentID], model.AgentPurposeIXRelay)
			}
		}
	}
	return result
}

func customerRealmPathUsesIX(path realmForwardResolution, agents map[string]model.DashboardAgentView) bool {
	if agentHasPurpose(agents[path.FinalAgentID], model.AgentPurposeIXRelay) {
		return true
	}
	for _, hop := range path.Hops {
		if agentHasPurpose(agents[hop.SourceAgentID], model.AgentPurposeIXRelay) {
			return true
		}
	}
	if !path.Resolved && len(path.Hops) > 0 {
		targetID := findRealmTargetAgentID(path.Hops[len(path.Hops)-1].Rule, agents)
		return agentHasPurpose(agents[targetID], model.AgentPurposeIXRelay)
	}
	return false
}

func (resolution customerEntranceResolution) routeType(configured string) string {
	if resolution.viaIX {
		return model.CustomerRouteTypeIEPL
	}
	return configured
}

func (rule customerEntranceRule) publicEntry(agent model.DashboardAgentView) customerPublicEntry {
	host := customerRealmSourceHost(agent, rule.realm)
	if rule.kind == "haproxy" {
		host = customerHAProxySourceHost(agent, rule.haproxy)
	}
	return resolveForwardPublicEntry(agent, rule.kind, rule.id, rule.port, rule.network, host)
}

func decorateCustomerEntrance(link *model.CustomerLinkView, assignment model.CustomerAssignment, configured, observed customerEntranceRule, context forwardedOverviewContext, agents map[string]model.DashboardAgentView, clients map[string]customerClientRef, chains map[string]model.ClientChainView, now time.Time) bool {
	agent := agents[assignment.AgentID]
	if configured.kind == "" {
		configured = observed
	}
	directXray := configured.kind == "" && (agentHasPurpose(agent, "xray_line") ||
		agentHasPurpose(agent, "residential_exit") || agentHasPurpose(agent, "datacenter_exit"))
	if !directXray && internalCustomerEntrance(agent, configured.publication) {
		return false
	}
	if configured.kind == "" {
		return true
	}
	if observed.kind == "" || !configured.enabled {
		observed = configured
	}
	publication := configured.publication
	resolution := observed.resolve(agent, context)
	if link.RouteType != model.CustomerRouteTypeIEPL {
		link.RouteType = resolution.routeType(publication.Type)
	}
	managed := publication != (model.CustomerRouteConfig{}) || agentHasPurpose(agent, "domestic_entry") || link.RouteType == model.CustomerRouteTypeIEPL
	name := firstNonEmptyString(publication.Name, assignment.PublicClientName, agent.CustomerDisplayName, agent.AgentName, "入口")
	status := "unresolved"
	if !configured.enabled {
		status = "disabled"
	}
	if !resolution.Resolved {
		if managed {
			setCustomerEntranceUnavailable(link, name, status)
		}
		return true
	}
	ref, ok := resolvedEntranceClient(assignment, resolution.realmForwardResolution, clients)
	if !ok {
		if managed {
			setCustomerEntranceUnavailable(link, name, status)
		}
		return true
	}
	client := ref.Client
	// A disabled listener can still resolve to an Xray account without a topology alias.
	chain, hasChain := chains[customerAssignmentKey(resolution.FinalAgentID, client.InboundID, client.Email)]
	if hasChain && customerChainUsesIX(chain, agents) {
		link.RouteType, managed = model.CustomerRouteTypeIEPL, true
	}
	link.TrafficSource = &model.CustomerTrafficSource{
		AssignmentID: assignment.ID, AgentID: resolution.FinalAgentID, InboundID: client.InboundID, ClientID: client.ClientID, Email: client.Email,
	}
	if !ref.ReportedAt.IsZero() {
		link.TrafficReportedAt = &ref.ReportedAt
	}
	link.XrayVersion = ref.XrayVersion
	link.TrafficMultiplier = 1
	link.TrafficUploadBytes = max(client.Up, 0)
	link.TrafficDownloadBytes = max(client.Down, 0)
	link.TrafficUsedBytes = customerClientTrafficUsed(client)
	link.TrafficLimitBytes = max(client.TotalGB, 0)
	link.TrafficReset, link.TrafficResetDay, link.TrafficResetSupported = client.TrafficReset, client.TrafficResetDay, client.TrafficResetSupported
	link.XrayExpireTime, link.ExpireTime, link.NodeExpireTime = client.ExpiryTime, client.ExpiryTime, 0
	link.StartTime, link.ExpireCycle, link.ExpireAutoRenew = 0, "", false

	endpoint := observed.publicEntry(agent)
	link.Resolved = configured.enabled && endpoint.Error == ""
	link.ImportURL, link.ShadowrocketImportURL = "", ""
	if link.Resolved {
		url := firstNonEmptyString(ref.Client.ImportURL, ref.TemplateImportURL)
		if managed {
			link.ImportName = name
			url = rewriteCustomerImportURLWithName(url, endpoint.Host, endpoint.Port, name)
		} else {
			url = rewriteCustomerImportURL(url, endpoint.Host, endpoint.Port)
		}
		link.ImportURL = url
		link.ShadowrocketImportURL = model.ShadowrocketImportURL(url, link.XrayVersion)
		link.Resolved = url != ""
	}
	if link.Resolved {
		status = "online"
		if !recentCustomerEntranceAgent(agent, now) || !recentCustomerEntranceAgent(agents[resolution.FinalAgentID], now) {
			status = "offline"
		}
		for _, hop := range resolution.Hops {
			if !recentCustomerEntranceAgent(agents[hop.SourceAgentID], now) {
				status = "offline"
			}
		}
	}
	if managed && !client.Enabled {
		link.Resolved, status = false, "disabled"
		link.ImportURL, link.ShadowrocketImportURL = "", ""
	}
	if managed {
		link.EntryClientName, link.InboundTag, link.Summary = name, "", name
		link.UnresolvedReason = ""
		if !link.Resolved {
			link.UnresolvedReason = "入口暂不可用"
		}
		if hasChain {
			link.Steps = link.Steps[:0]
			applyCustomerLinkExit(link, chain, agents, true)
		}
		presentCustomerXrayChain(link, agents[resolution.FinalAgentID])
	}
	link.Entrances = []model.CustomerLinkEntrance{{AssignmentID: assignment.ID, Name: firstNonEmptyString(publication.Name, link.Remark, name), RouteType: link.RouteType,
		Status: status, ImportURL: link.ImportURL, ShadowrocketImportURL: link.ShadowrocketImportURL}}
	return true
}

func resolvedEntranceClient(assignment model.CustomerAssignment, resolution realmForwardResolution, clients map[string]customerClientRef) (customerClientRef, bool) {
	if assignment.ClientID != "" {
		key := customerAssignmentClientKey(resolution.FinalAgentID, resolution.FinalNode.ID, assignment.ClientID)
		return clients[key], clients[key].Client.InboundID > 0
	}
	if assignment.ClientEmail == "" {
		return customerClientRef{}, false
	}
	ref, found := clients[customerAssignmentKey(resolution.FinalAgentID, resolution.FinalNode.ID, assignment.ClientEmail)]
	return ref, found
}

func recentCustomerEntranceAgent(agent model.DashboardAgentView, now time.Time) bool {
	for _, timestamp := range []*time.Time{agent.RealtimeAt, agent.LastSeenAt, agent.ReportedAt} {
		if timestamp != nil && now.Sub(*timestamp) <= 5*time.Minute {
			return true
		}
	}
	return false
}

func setCustomerEntranceUnavailable(link *model.CustomerLinkView, name, status string) {
	link.EntryClientName, link.ImportName, link.Summary = name, name, name
	link.InboundTag, link.ImportURL, link.ShadowrocketImportURL = "", "", ""
	link.UnresolvedReason, link.Resolved = "入口暂不可用", false
	link.Steps = []model.CustomerLinkStep{{Role: "entry", Label: name}}
	link.Entrances = []model.CustomerLinkEntrance{{AssignmentID: link.AssignmentID, Name: name, RouteType: link.RouteType, Status: status}}
	link.TrafficUploadBytes, link.TrafficDownloadBytes, link.TrafficUsedBytes, link.TrafficLimitBytes = 0, 0, 0, 0
	link.ExpireTime, link.XrayExpireTime, link.NodeExpireTime = 0, 0, 0
	link.TrafficReset, link.TrafficResetDay, link.TrafficResetSupported = "", 0, false
	link.StartTime, link.ExpireCycle, link.ExpireAutoRenew = 0, "", false
	link.ExitIP, link.ExitType, link.ExitCountryCode, link.ExitCountryName = "", "", "", ""
}

func presentCustomerXrayChain(link *model.CustomerLinkView, target model.DashboardAgentView) {
	name := firstNonEmptyString(target.CustomerDisplayName, "Xray 线路机")
	steps := []model.CustomerLinkStep{{Role: "relay", Label: name}}
	for _, step := range link.Steps {
		if step.Role == "exit" {
			steps = append(steps, step)
		}
	}
	link.Steps = steps
}

func groupCustomerEntrances(links []model.CustomerLinkView, agents map[string]model.DashboardAgentView) []model.CustomerLinkView {
	type sourceKey struct {
		agentID, clientID, email string
		inboundID                int
	}
	grouped := make([]model.CustomerLinkView, 0, len(links))
	groups := make(map[sourceKey]int)
	for _, link := range links {
		if link.TrafficSource == nil {
			grouped = append(grouped, link)
			continue
		}
		source := link.TrafficSource
		key := sourceKey{agentID: source.AgentID, inboundID: source.InboundID, clientID: source.ClientID}
		if key.clientID == "" {
			key.email = strings.ToLower(source.Email)
		}
		index, found := groups[key]
		if !found {
			groups[key] = len(grouped)
			link.Entrances = slices.Clone(link.Entrances)
			grouped = append(grouped, link)
			continue
		}
		group := &grouped[index]
		group.Entrances = append(group.Entrances, link.Entrances...)
		if group.RouteType != link.RouteType {
			group.RouteType = ""
		}
		group.Resolved = group.Resolved || link.Resolved
		group.EntryClientName = firstNonEmptyString(agents[source.AgentID].CustomerDisplayName, "Xray 线路机")
		group.ImportName, group.Summary = group.EntryClientName, group.EntryClientName
		if group.Resolved {
			group.UnresolvedReason = ""
		}
		presentCustomerXrayChain(group, agents[source.AgentID])
	}
	return grouped
}
