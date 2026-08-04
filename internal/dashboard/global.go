package dashboard

import (
	"sort"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/realmconfig"
)

type GlobalDashboardOptions struct {
	IncludeTopology    bool
	IncludeGeo         bool
	AllowNetworkLookup bool
	ResolverData       TopologyResolverData
}

func BuildGlobalDashboard(agents []model.AgentRecord, snapshots []model.AgentSnapshot) model.GlobalDashboardView {
	return BuildGlobalDashboardWithOptions(agents, snapshots, GlobalDashboardOptions{
		IncludeTopology:    true,
		IncludeGeo:         true,
		AllowNetworkLookup: true,
	})
}

func BuildGlobalDashboardWithOptions(agents []model.AgentRecord, snapshots []model.AgentSnapshot, options GlobalDashboardOptions) model.GlobalDashboardView {
	resolver := newTopologyResolverWithData(options.ResolverData, options.AllowNetworkLookup)
	entryByAgent := make(map[string]model.AgentEntryConfig, len(agents))
	for _, agent := range agents {
		entryByAgent[agent.AgentID] = agent.Config.Entry
	}

	overviewByAgent := make(map[string]*model.XUIOverview, len(snapshots))
	realmByAgent := make(map[string]*model.RealmSnapshot, len(snapshots))
	networkPolicyByAgent := make(map[string]*model.NetworkPolicySnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if overview := BuildXUIOverviewWithOptions(snapshot, XUIOverviewOptions{Entry: entryByAgent[snapshot.AgentID]}); overview != nil {
			overviewByAgent[snapshot.AgentID] = overview
		}
		if snapshot.Realm != nil {
			realmByAgent[snapshot.AgentID] = snapshot.Realm
		}
		if snapshot.NetworkPolicy != nil {
			networkPolicyByAgent[snapshot.AgentID] = snapshot.NetworkPolicy
		}
	}

	agentViews := make([]model.DashboardAgentView, 0, len(agents))
	agentViewByID := make(map[string]model.DashboardAgentView, len(agents))
	tagStats := make(map[string]*model.DashboardTagView)
	totals := model.DashboardTotals{AgentCount: len(agents)}

	for _, agent := range agents {
		overview := overviewByAgent[agent.AgentID]
		summary := agent.Summary
		if summary.Hostname == "" {
			summary.Hostname = agent.Hostname
		}
		if summary.PublicIPv4 == "" {
			summary.PublicIPv4 = agent.PublicIPv4
		}
		if summary.PublicIPv6 == "" {
			summary.PublicIPv6 = agent.PublicIPv6
		}

		entry := MergeRealmSnapshotIntoEntry(agent.Config.Entry, realmByAgent[agent.AgentID])
		view := model.DashboardAgentView{
			AgentID:             agent.AgentID,
			AgentName:           agent.AgentName,
			CustomerDisplayName: agent.CustomerDisplayName,
			ClientVersion:       agent.Version,
			ClientOS:            agent.OS,
			ClientArch:          agent.Arch,
			SystemVersion:       agent.SystemVersion,
			SortOrder:           agent.SortOrder,
			Tags:                cloneStrings(agent.Tags),
			Renewal:             agent.Config.Renewal,
			Entry:               entry,
			ReportedAt:          agent.ReportedAt,
			RegisteredAt:        &agent.RegisteredAt,
			UpdatedAt:           &agent.UpdatedAt,
			LastSeenAt:          agent.LastSeenAt,
			HasConfig:           agent.HasConfig,
			Summary:             summary,
			Realm:               realmByAgent[agent.AgentID],
			NetworkPolicy:       networkPolicyByAgent[agent.AgentID],
			FinanceClients:      make([]model.FinanceClientView, 0),
		}
		if options.IncludeGeo {
			view.Geo = lookupAgentGeo(summary, resolver)
			if view.Geo == nil && overview != nil {
				view.Geo = lookupOverviewGeo(overview, resolver)
			}
		}
		if overview != nil {
			view.FinanceClientsReady = true
			view.FinanceClients = make([]model.FinanceClientView, 0, len(overview.Clients))
			for _, client := range overview.Clients {
				view.FinanceClients = append(view.FinanceClients, model.FinanceClientView{
					InboundID:     client.InboundID,
					InboundTag:    client.InboundTag,
					InboundRemark: client.InboundRemark,
					NodeEnabled:   clientInboundEnabled(overview.Nodes, client),
					Email:         client.Email,
					Comment:       client.Comment,
					Enabled:       client.Enabled,
					ExpiryTime:    client.ExpiryTime,
				})
			}
			view.NodeCount = overview.NodeCount
			view.ClientCount = overview.ClientCount
			view.OnlineClientCount = overview.OnlineClientCount
			view.OutboundCount = len(overview.Outbounds)
			view.RoutingRuleCount = len(overview.RoutingRules)
		} else {
			view.NodeCount = agent.Summary.InboundCount
			view.OutboundCount = agent.Summary.OutboundCount
			view.RoutingRuleCount = agent.Summary.RoutingRuleCount
		}

		agentViews = append(agentViews, view)
		agentViewByID[view.AgentID] = view

		totals.NodeCount += view.NodeCount
		totals.ClientCount += view.ClientCount
		totals.OnlineClientCount += view.OnlineClientCount
		totals.OutboundCount += view.OutboundCount
		totals.RoutingRuleCount += view.RoutingRuleCount
		if len(view.Tags) > 0 {
			totals.TaggedAgentCount++
		}
		for _, tag := range view.Tags {
			tagView := tagStats[tag]
			if tagView == nil {
				tagView = &model.DashboardTagView{Tag: tag}
				tagStats[tag] = tagView
			}
			tagView.AgentCount++
			tagView.NodeCount += view.NodeCount
			tagView.ClientCount += view.ClientCount
			tagView.OnlineClientCount += view.OnlineClientCount
		}
	}

	sort.Slice(agentViews, func(i, j int) bool {
		left := time.Time{}
		right := time.Time{}
		if agentViews[i].ReportedAt != nil {
			left = *agentViews[i].ReportedAt
		}
		if agentViews[j].ReportedAt != nil {
			right = *agentViews[j].ReportedAt
		}
		if !left.Equal(right) {
			return left.After(right)
		}
		return agentViews[i].AgentID < agentViews[j].AgentID
	})

	tags := make([]model.DashboardTagView, 0, len(tagStats))
	for _, tagView := range tagStats {
		tags = append(tags, *tagView)
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].ClientCount != tags[j].ClientCount {
			return tags[i].ClientCount > tags[j].ClientCount
		}
		return tags[i].Tag < tags[j].Tag
	})

	var links []model.TopologyLinkView
	var chains []model.ClientChainView
	if options.IncludeTopology {
		inboundCandidates := buildInboundCandidates(agentViewByID, overviewByAgent, resolver)
		outboundCandidates := buildOutboundCandidates(agentViewByID, overviewByAgent, resolver)
		balancerCandidates := buildBalancerCandidates(overviewByAgent)
		var linkByOutboundKey map[string]model.TopologyLinkView
		links, linkByOutboundKey = matchTopologyLinks(inboundCandidates, outboundCandidates)
		chains = buildClientChains(agentViewByID, overviewByAgent, inboundCandidates, outboundCandidates, balancerCandidates, linkByOutboundKey)
		totals.LinkCount = len(links)
		totals.ChainCount = len(chains)
	}

	return model.GlobalDashboardView{
		GeneratedAt:  time.Now().UTC(),
		Totals:       totals,
		Tags:         tags,
		Agents:       agentViews,
		Links:        links,
		ClientChains: chains,
	}
}

func clientInboundEnabled(nodes []model.XUINodeView, client model.XUIClientView) bool {
	for _, node := range nodes {
		if client.InboundID > 0 && node.ID == client.InboundID {
			return node.Enabled
		}
		if client.InboundID <= 0 && client.InboundTag != "" && node.Tag == client.InboundTag {
			return node.Enabled
		}
	}
	return true
}

func lookupAgentGeo(summary model.VPSSummary, resolver *topologyResolver) *model.IPGeoView {
	if resolver == nil {
		return nil
	}
	for _, address := range []string{summary.ServerSeenIP, summary.ObservedIP, summary.PublicIPv4, summary.PublicIPv6} {
		if address == "" {
			continue
		}
		if _, geo := resolver.lookupGeo(address, nil); geo != nil {
			return geo
		}
	}
	if summary.Hostname == "" {
		return nil
	}
	resolved := resolver.lookupHost(summary.Hostname)
	_, geo := resolver.lookupGeo(summary.Hostname, resolved)
	return geo
}

func lookupOverviewGeo(overview *model.XUIOverview, resolver *topologyResolver) *model.IPGeoView {
	if overview == nil || resolver == nil {
		return nil
	}
	candidates := []string{overview.Summary.ObservedIP, overview.Summary.PublicIPv4, overview.Summary.PublicIPv6, overview.BaseURL}
	for _, value := range candidates {
		if value == "" {
			continue
		}
		resolved := resolver.lookupHost(value)
		if _, geo := resolver.lookupGeo(value, resolved); geo != nil {
			return geo
		}
	}

	certDomains := collectCertificateDomains(overview.Certificates)
	domains := mergeStringSets(collectAgentDomains(overview), certDomains)
	ips := collectAgentIPs(overview)
	for _, node := range overview.Nodes {
		domains = mergeStringSets(domains, collectInboundDomains(node, certDomains))
		ips = mergeStringSets(ips, collectInboundIPs(node.Listen, overview.Summary.PublicIPv4, overview.Summary.PublicIPv6, overview.Summary.ObservedIP))
	}
	for _, ip := range ips {
		if _, geo := resolver.lookupGeo(ip, nil); geo != nil {
			return geo
		}
	}
	for _, domain := range domains {
		resolved := resolver.lookupHost(domain)
		if _, geo := resolver.lookupGeo(domain, resolved); geo != nil {
			return geo
		}
	}
	return nil
}

func MergeRealmSnapshotIntoEntry(entry model.AgentEntryConfig, snapshot *model.RealmSnapshot) model.AgentEntryConfig {
	return realmconfig.MergeSnapshotIntoEntry(entry, snapshot)
}
