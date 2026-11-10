package server

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func buildCustomerChainMap(chains []model.ClientChainView, links []model.TopologyLinkView) map[string]model.ClientChainView {
	result := make(map[string]model.ClientChainView, len(chains))
	for _, chain := range chains {
		result[chain.Key] = chain
	}
	for _, link := range links {
		if !isForwardingProtocol(link.Source.Protocol) || link.Source.ListenPort <= 0 || link.FinalTarget == nil {
			continue
		}
		for _, chain := range chains {
			if !customerChainMatchesInbound(chain, *link.FinalTarget) || chain.RootClientEmail == "" {
				continue
			}
			addCustomerForwardingChainAlias(result, link.Source.AgentID, link.Source.ListenPort, chain)
			for _, hop := range link.RealmHops {
				addCustomerForwardingChainAlias(result, hop.AgentID, hop.Port, chain)
			}
		}
	}
	return result
}

// appendForwardedCustomerChainAliases makes forwarding-listener assignments
// resolve to the final X-UI client even when the generic topology matcher did
// not produce a link for the Realm/HAProxy target address.  The forwarding
// resolver uses the configured target Client and target port directly, which
// is the same information used to generate the forwarded import URL.
func appendForwardedCustomerChainAliases(chainMap map[string]model.ClientChainView, context forwardedOverviewContext) {
	if chainMap == nil || len(context.agentMap) == 0 {
		return
	}
	agentIDs := make([]string, 0, len(context.agentMap))
	for agentID := range context.agentMap {
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)
	for _, sourceAgentID := range agentIDs {
		sourceAgent := context.agentMap[sourceAgentID]
		if realmForwardingActive(sourceAgent.Entry.PortForwarding) {
			for _, rule := range sourceAgent.Entry.PortForwarding.Rules {
				if !rule.Enabled || rule.ListenPort <= 0 || rule.TargetPort <= 0 {
					continue
				}
				resolution := resolveRealmForwardTarget(sourceAgentID, rule, context.agentMap, context.targetOverviewByAgent)
				if !resolution.Resolved {
					continue
				}
				appendForwardedCustomerChainAliasesForResolution(chainMap, context, sourceAgentID, rule.ListenPort, resolution)
			}
		}
		if !sourceAgent.Entry.HAProxy.Enabled {
			continue
		}
		for _, rule := range sourceAgent.Entry.HAProxy.Rules {
			if !rule.Enabled || rule.ListenPort <= 0 {
				continue
			}
			paths, err := resolveHAProxyRulePaths(rule, context)
			if err != nil || validateHAProxyResolvedPaths(rule, paths) != nil || len(paths) == 0 {
				continue
			}
			appendForwardedCustomerChainAliasesForResolution(chainMap, context, sourceAgentID, rule.ListenPort, paths[0].Resolution)
		}
	}
}

func appendForwardedCustomerChainAliasesForResolution(
	chainMap map[string]model.ClientChainView,
	context forwardedOverviewContext,
	sourceAgentID string,
	listenPort int,
	resolution realmForwardResolution,
) {
	if listenPort <= 0 || !resolution.Resolved || resolution.FinalAgentID == "" {
		return
	}
	targetOverview := context.targetOverviewByAgent[resolution.FinalAgentID]
	if targetOverview == nil {
		return
	}
	for _, client := range targetOverview.Clients {
		if !realmClientMatchesNode(client, resolution.FinalNode) {
			continue
		}
		targetKey := customerAssignmentKey(resolution.FinalAgentID, client.InboundID, client.Email)
		chain, ok := chainMap[targetKey]
		if !ok {
			chain = customerForwardedFallbackChain(context, resolution.FinalAgentID, resolution.FinalNode, client)
		}
		aliasKey := customerAssignmentKey(sourceAgentID, listenPort, client.Email)
		if _, exists := chainMap[aliasKey]; !exists {
			chainMap[aliasKey] = chain
		}
	}
}

func customerForwardedFallbackChain(context forwardedOverviewContext, targetAgentID string, node model.XUINodeView, client model.XUIClientView) model.ClientChainView {
	agent := context.agentMap[targetAgentID]
	inboundID := client.InboundID
	if inboundID <= 0 {
		inboundID = node.ID
	}
	inboundTag := firstNonEmptyString(client.InboundTag, node.Tag)
	chain := model.ClientChainView{
		Key:                customerAssignmentKey(targetAgentID, inboundID, client.Email),
		RootAgentID:        targetAgentID,
		RootAgentName:      agent.AgentName,
		RootAgentTags:      append([]string(nil), agent.Tags...),
		RootClientEmail:    client.Email,
		RootClientRemark:   firstNonEmptyString(client.Comment, client.SubID),
		RootClientEnabled:  client.Enabled,
		RootInboundID:      inboundID,
		RootInboundTag:     inboundTag,
		RootInboundEnabled: node.Enabled,
	}
	clientLabel := firstNonEmptyString(client.Email, client.ClientID, client.AuthUUID, client.AuthPassword)
	if clientLabel != "" {
		chain.Steps = append(chain.Steps, model.ClientChainStep{
			StepType:  "client",
			AgentID:   targetAgentID,
			AgentName: agent.AgentName,
			AgentTags: append([]string(nil), agent.Tags...),
			Label:     clientLabel,
			Detail:    firstNonEmptyString(client.Comment, client.SubID),
			Protocol:  client.Protocol,
		})
	}
	port := node.Port
	if port <= 0 {
		port = client.InboundID
	}
	chain.Steps = append(chain.Steps, model.ClientChainStep{
		StepType:  "inbound",
		AgentID:   targetAgentID,
		AgentName: agent.AgentName,
		AgentTags: append([]string(nil), agent.Tags...),
		Label:     firstNonEmptyString(node.Remark, node.Tag, inboundTag, fmt.Sprintf("Inbound #%d", inboundID)),
		Detail:    firstNonEmptyString(node.Tag, inboundTag),
		Protocol:  firstNonEmptyString(node.Protocol, client.Protocol),
		Port:      port,
	})
	return chain
}

func customerChainMatchesInbound(chain model.ClientChainView, inbound model.TopologyInboundRef) bool {
	if chain.RootAgentID == "" || !strings.EqualFold(chain.RootAgentID, inbound.AgentID) {
		return false
	}
	if chain.RootInboundID > 0 && inbound.InboundID > 0 {
		return chain.RootInboundID == inbound.InboundID
	}
	return chain.RootInboundTag != "" && inbound.InboundTag != "" && strings.EqualFold(chain.RootInboundTag, inbound.InboundTag)
}

func addCustomerForwardingChainAlias(result map[string]model.ClientChainView, agentID string, listenPort int, chain model.ClientChainView) {
	if agentID == "" || listenPort <= 0 || chain.RootClientEmail == "" {
		return
	}
	key := customerAssignmentKey(agentID, listenPort, chain.RootClientEmail)
	if _, exists := result[key]; !exists {
		result[key] = chain
	}
}

type customerClientRef struct {
	Client            model.XUIClientView
	TemplateImportURL string
	XrayVersion       string
}

func buildCustomerClientMap(snapshots []model.AgentSnapshot, agents []model.AgentRecord) map[string]customerClientRef {
	entryByAgent := make(map[string]model.AgentEntryConfig, len(agents))
	for _, agent := range agents {
		entryByAgent[agent.AgentID] = agent.Config.Entry
	}
	result := make(map[string]customerClientRef)
	for _, snapshot := range snapshots {
		overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{Entry: entryByAgent[snapshot.AgentID]})
		if overview == nil {
			continue
		}
		templateImportURLs := customerTemplateImportURLs(snapshot)
		for _, client := range overview.Clients {
			key := customerAssignmentKey(snapshot.AgentID, client.InboundID, client.Email)
			ref := customerClientRef{
				Client:            client,
				TemplateImportURL: templateImportURLs[key],
				XrayVersion:       snapshot.XUI.ServerStatus.Xray.Version,
			}
			result[key] = ref
			if client.ClientID != "" {
				result[customerAssignmentClientKey(snapshot.AgentID, client.InboundID, client.ClientID)] = ref
			}
		}
	}
	return result
}

func customerTemplateImportURLs(snapshot model.AgentSnapshot) map[string]string {
	overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{
		Entry: model.AgentEntryConfig{ImportDomain: "vpsmonitor.local"},
	})
	result := make(map[string]string)
	if overview == nil {
		return result
	}
	for _, client := range overview.Clients {
		if client.ImportURL == "" {
			continue
		}
		result[customerAssignmentKey(snapshot.AgentID, client.InboundID, client.Email)] = client.ImportURL
		if client.ClientID != "" {
			result[customerAssignmentClientKey(snapshot.AgentID, client.InboundID, client.ClientID)] = client.ImportURL
		}
	}
	return result
}

func buildCustomerLinkView(
	assignment model.CustomerAssignment,
	chainMap map[string]model.ClientChainView,
	clientMap map[string]customerClientRef,
	agentMap map[string]model.DashboardAgentView,
) model.CustomerLinkView {
	entryName := firstNonEmptyString(customerAgentDisplayName(assignment.AgentID, agentMap), assignment.PublicClientName, assignment.ClientEmail, assignment.InboundTag, assignment.AgentID)
	link := model.CustomerLinkView{
		AssignmentID:    assignment.ID,
		EntryClientName: entryName,
		InboundTag:      assignment.InboundTag,
		ClientEmail:     assignment.ClientEmail,
		Remark:          assignment.Remark,
		UpdatedAt:       assignment.UpdatedAt,
		Steps: []model.CustomerLinkStep{
			{Role: "entry", Label: entryName},
		},
	}
	if agent, ok := agentMap[assignment.AgentID]; ok {
		for _, rawSnell := range model.NormalizeSnellConfigs(agent.Snells, agent.Snell) {
			snell := model.NormalizeSnellConfig(rawSnell)
			if !snell.Enabled || model.ValidateSnellConfig(snell) != nil {
				continue
			}
			link.Snells = append(link.Snells, &snell)
			if link.Snell == nil {
				link.Snell = &snell
			}
		}
	}
	link.NodeExpireTime = customerNodeExpireTime(assignment.AgentID, agentMap)
	chain, chainFound := findCustomerChain(assignment, chainMap)
	trafficMultiplier := 1.0
	var clientRef customerClientRef
	clientRef, clientFound := customerClientRef{}, false
	if assignment.ClientID != "" {
		clientRef, clientFound = clientMap[customerAssignmentClientKey(assignment.AgentID, assignment.InboundID, assignment.ClientID)]
	}
	if !clientFound {
		clientRef, clientFound = clientMap[customerAssignmentKey(assignment.AgentID, assignment.InboundID, assignment.ClientEmail)]
	}
	if !clientFound && chainFound {
		clientRef, clientFound = clientMap[customerAssignmentKey(chain.RootAgentID, chain.RootInboundID, chain.RootClientEmail)]
	}
	if !clientFound && chainFound && assignment.ClientID != "" {
		clientRef, clientFound = clientMap[customerAssignmentClientKey(chain.RootAgentID, chain.RootInboundID, assignment.ClientID)]
	}
	if clientFound {
		link.ImportURL = clientRef.Client.ImportURL
		link.XrayVersion = clientRef.XrayVersion
		link.ClientRemark = firstNonEmptyString(clientRef.Client.Comment, clientRef.Client.SubID)
		link.TrafficReset = clientRef.Client.TrafficReset
		link.TrafficResetDay = clientRef.Client.TrafficResetDay
		link.TrafficResetSupported = clientRef.Client.TrafficResetSupported
		if clientRef.Client.ExpiryTime > 0 {
			// Keep the Xray expiry separate from the billing expiry. Xray is the
			// source of truth for whether the client is still valid; the billing
			// expiry is only used for the customer-facing billing cycle display.
			link.XrayExpireTime = clientRef.Client.ExpiryTime
			link.ExpireTime = clientRef.Client.ExpiryTime
		}
	}
	if billing, ok := customerBillingForAssignment(assignment, agentMap); ok {
		trafficMultiplier = normalizeClientTrafficMultiplier(billing.TrafficMultiplier)
		if billing.RevenueAmount > 0 {
			amount := billing.RevenueAmount
			link.RevenueAmount = &amount
			link.RevenueCurrency = firstNonEmptyString(billing.RevenueCurrency, "CNY")
			link.RevenueCycle = firstNonEmptyString(billing.RevenueCycle, "month")
		}
		if billing.ExpireTime > 0 {
			link.StartTime = billing.StartTime
			link.ExpireTime = billing.ExpireTime
			link.ExpireCycle = billing.ExpireCycle
			link.ExpireAutoRenew = billing.ExpireAutoRenew
		}
	}
	link.TrafficMultiplier = trafficMultiplier
	link.TrafficUploadBytes = max(clientRef.Client.Up, int64(0))
	link.TrafficDownloadBytes = max(clientRef.Client.Down, int64(0))
	link.TrafficUsedBytes = scaleCustomerTraffic(customerClientTrafficUsed(clientRef.Client), trafficMultiplier)
	link.TrafficLimitBytes = scaleCustomerTraffic(clientRef.Client.TotalGB, trafficMultiplier)

	if !chainFound {
		link.ShadowrocketImportURL = model.ShadowrocketImportURL(link.ImportURL, link.XrayVersion)
		link.UnresolvedReason = "当前最新上报中没有找到该用户链路"
		link.Summary = entryName + " 暂无链路数据"
		return link
	}
	link.Resolved = true
	endpointError := ""
	if clientRef.Client.ExportEndpoint != nil {
		endpointError = clientRef.Client.ExportEndpoint.Error
	}
	if publicEntry, ok := customerForwardingPublicEntry(assignment, chain, agentMap); ok {
		endpointError = publicEntry.Error
		if customerRealmAssignment(assignment, agentMap) {
			link.ImportName = customerRealmImportName(assignment, chain, clientRef.Client, agentMap)
		}
		link.ImportURL = ""
		if endpointError == "" {
			sourceURL := firstNonEmptyString(clientRef.Client.ImportURL, clientRef.TemplateImportURL)
			link.ImportURL = rewriteCustomerImportURL(sourceURL, publicEntry.Host, publicEntry.Port)
		}
	}
	if endpointError != "" {
		link.ImportURL = ""
		link.Resolved = false
		link.UnresolvedReason = endpointError
	}
	if link.ClientRemark == "" {
		link.ClientRemark = chain.RootClientRemark
	}
	if link.InboundTag == "" {
		link.InboundTag = chain.RootInboundTag
	}
	relays := customerRelayNames(chain, assignment.AgentID, agentMap)
	for _, relay := range relays {
		link.Steps = append(link.Steps, model.CustomerLinkStep{Role: "relay", Label: relay})
	}
	exitIP, exitCountryCode, exitCountryName := customerExitInfo(chain, agentMap)
	link.ExitIP = exitIP
	link.ExitCountryCode = exitCountryCode
	link.ExitCountryName = exitCountryName
	if exitCountryCode != "" || exitCountryName != "" || exitIP != "" {
		link.Steps = append(link.Steps, model.CustomerLinkStep{
			Role:        "exit",
			Label:       customerExitLabel(exitCountryCode, exitCountryName, exitIP),
			CountryCode: exitCountryCode,
			CountryName: exitCountryName,
			ExitIP:      exitIP,
		})
	}
	link.Summary = customerLinkSummary(entryName, relays)
	link.ShadowrocketImportURL = model.ShadowrocketImportURL(link.ImportURL, link.XrayVersion)
	if chain.UnresolvedReason != "" && link.UnresolvedReason == "" {
		link.UnresolvedReason = chain.UnresolvedReason
	}
	applySnellForwardToCustomerLink(&link, assignment, agentMap)
	return link
}

func customerRealmAssignment(assignment model.CustomerAssignment, agentMap map[string]model.DashboardAgentView) bool {
	agent, ok := agentMap[assignment.AgentID]
	if !ok || !realmForwardingActive(agent.Entry.PortForwarding) {
		return false
	}
	_, found := realmRuleListeningOnPort(agent.Entry.PortForwarding.Rules, assignment.InboundID)
	return found
}

func customerRealmImportName(assignment model.CustomerAssignment, chain model.ClientChainView, client model.XUIClientView, agentMap map[string]model.DashboardAgentView) string {
	parts := make([]string, 0, len(chain.Steps)+2)
	appendPart := func(value string) {
		value = normalizeRealmImportName(value)
		if value == "" || (len(parts) > 0 && parts[len(parts)-1] == value) {
			return
		}
		parts = append(parts, value)
	}
	appendAgent := func(agentID, fallback string) {
		agent := agentMap[agentID]
		appendPart(firstNonEmptyString(agent.CustomerDisplayName, fallback, agent.AgentName, agentID))
	}

	appendAgent(assignment.AgentID, "")
	currentAgentID := assignment.AgentID
	currentPort := assignment.InboundID
	visited := map[string]struct{}{}
	for len(parts) < maxRealmForwardDepth {
		endpointKey := realmForwardEndpointKey(currentAgentID, currentPort)
		if _, seen := visited[endpointKey]; seen {
			break
		}
		visited[endpointKey] = struct{}{}
		currentAgent, ok := agentMap[currentAgentID]
		if !ok || !realmForwardingActive(currentAgent.Entry.PortForwarding) {
			break
		}
		rule, found := realmRuleListeningOnPort(currentAgent.Entry.PortForwarding.Rules, currentPort)
		if !found {
			break
		}
		targetAgentID := findRealmTargetAgentID(rule, agentMap)
		if targetAgentID == "" || strings.EqualFold(targetAgentID, chain.RootAgentID) {
			break
		}
		targetAgent, targetExists := agentMap[targetAgentID]
		if !targetExists || !realmForwardingActive(targetAgent.Entry.PortForwarding) {
			break
		}
		nextPort := rule.TargetPort
		if _, nextFound := realmRuleListeningOnPort(targetAgent.Entry.PortForwarding.Rules, nextPort); !nextFound {
			break
		}
		appendAgent(targetAgentID, targetAgent.AgentName)
		currentAgentID = targetAgentID
		currentPort = nextPort
	}
	appendPart(firstNonEmptyString(client.Email, assignment.ClientEmail, client.Comment, client.ClientID, client.InboundRemark, client.InboundTag))
	return strings.Join(parts, "-")
}

func applySnellForwardToCustomerLink(link *model.CustomerLinkView, assignment model.CustomerAssignment, agentMap map[string]model.DashboardAgentView) {
	if link == nil || !link.Resolved || strings.TrimSpace(link.ImportURL) == "" || len(agentMap) == 0 {
		return
	}
	agentIDs := make([]string, 0, len(agentMap))
	for agentID := range agentMap {
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)
	for _, sourceAgentID := range agentIDs {
		if sourceAgentID == assignment.AgentID {
			continue
		}
		for _, rawSnell := range model.NormalizeSnellConfigs(agentMap[sourceAgentID].Snells, agentMap[sourceAgentID].Snell) {
			cfg := model.NormalizeSnellConfig(rawSnell)
			if !cfg.Enabled || model.ValidateSnellConfig(cfg) != nil {
				continue
			}
			for _, rule := range cfg.ForwardRules {
				if !rule.Enabled || !snellForwardMatchesAssignment(rule, assignment) {
					continue
				}
				link.Snell = nil
				link.Snells = nil
				link.SnellChain = &cfg
				link.SubscriptionImportURL = link.ImportURL
				link.ImportURL = ""
				link.ShadowrocketImportURL = ""
				link.SubscriptionOnly = true
				link.Steps = append([]model.CustomerLinkStep{{Role: "snell", Label: cfg.Name}}, link.Steps...)
				link.Summary = cfg.Name + " → " + link.Summary
				return
			}
		}
	}
}

func snellForwardMatchesAssignment(rule model.SnellForwardRule, assignment model.CustomerAssignment) bool {
	if rule.TargetAgentID != assignment.AgentID {
		return false
	}
	if rule.TargetInboundID > 0 && rule.TargetInboundID != assignment.InboundID {
		return false
	}
	if rule.TargetInboundTag != "" && !strings.EqualFold(rule.TargetInboundTag, assignment.InboundTag) {
		return false
	}
	if rule.TargetClientID != "" {
		return rule.TargetClientID == assignment.ClientID
	}
	if rule.TargetClientEmail != "" {
		return strings.EqualFold(rule.TargetClientEmail, assignment.ClientEmail)
	}
	return false
}

func customerClientTrafficUsed(client model.XUIClientView) int64 {
	up := max(client.Up, int64(0))
	down := max(client.Down, int64(0))
	if up > 0 || down > 0 {
		return up + down
	}
	return max(client.TrafficTotal, int64(0))
}

func scaleCustomerTraffic(value int64, multiplier float64) int64 {
	if value <= 0 {
		return 0
	}
	scaled := float64(value) * normalizeClientTrafficMultiplier(multiplier)
	const maxInt64 = int64(^uint64(0) >> 1)
	if scaled >= float64(maxInt64) {
		return maxInt64
	}
	return int64(math.Round(scaled))
}

func customerNodeExpireTime(agentID string, agentMap map[string]model.DashboardAgentView) int64 {
	if agentMap == nil || agentID == "" {
		return 0
	}
	agent, ok := agentMap[agentID]
	if !ok {
		return 0
	}
	cfg := normalizeAlertRenewal(agent.Renewal)
	if !cfg.Enabled || (cfg.StartDate == "" && cfg.ExpireDate == "") {
		return 0
	}
	now := startOfDay(time.Now())
	if cfg.ExpireDate != "" {
		end, ok := parseDate(cfg.ExpireDate)
		if !ok {
			return 0
		}
		for cfg.Cycle != "" && !end.After(now) {
			end = addRenewalCycle(end, cfg.Cycle)
		}
		return end.UnixMilli()
	}
	start, ok := parseDate(cfg.StartDate)
	if !ok || cfg.Cycle == "" {
		return 0
	}
	end := addRenewalCycle(start, cfg.Cycle)
	for !end.After(now) {
		start = end
		end = addRenewalCycle(start, cfg.Cycle)
	}
	return end.UnixMilli()
}

func customerBillingForAssignment(assignment model.CustomerAssignment, agentMap map[string]model.DashboardAgentView) (model.XUIClientBillingConfig, bool) {
	if assignment.AgentID == "" || agentMap == nil {
		return model.XUIClientBillingConfig{}, false
	}
	agent, ok := agentMap[assignment.AgentID]
	if !ok {
		return model.XUIClientBillingConfig{}, false
	}
	exactKey := customerBillingKey(assignment.InboundID, assignment.InboundTag, assignment.ClientEmail)
	emailKey := customerBillingEmailKey(assignment.InboundID, assignment.ClientEmail)
	base := model.XUIClientBillingConfig{
		ClientID:          assignment.ClientID,
		InboundID:         assignment.InboundID,
		InboundTag:        assignment.InboundTag,
		Email:             assignment.ClientEmail,
		TrafficMultiplier: 1,
	}
	found := false
	for _, billing := range agent.Renewal.ClientBillings {
		if assignment.ClientID != "" && billing.ClientID == assignment.ClientID {
			base, found = billing, true
			break
		}
		if customerBillingKey(billing.InboundID, billing.InboundTag, billing.Email) == exactKey {
			base, found = billing, true
			break
		}
		if emailKey != "" && customerBillingEmailKey(billing.InboundID, billing.Email) == emailKey {
			base, found = billing, true
			break
		}
	}
	if assignment.PriceMode == "override" && assignment.RevenueAmount != nil {
		base.RevenueAmount = max(*assignment.RevenueAmount, 0)
		base.RevenueCurrency = firstNonEmptyString(assignment.RevenueCurrency, "CNY")
		base.RevenueCycle = firstNonEmptyString(assignment.RevenueCycle, "month")
		base.ExpireCycle = base.RevenueCycle
		return base, true
	}
	return base, found
}

func customerRelayNames(chain model.ClientChainView, rootAgentID string, agentMap map[string]model.DashboardAgentView) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	pendingOutbound := ""
	appendRelay := func(label string) {
		label = strings.TrimSpace(label)
		if label == "" {
			return
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, label)
	}
	for _, step := range chain.Steps {
		if step.StepType == "outbound" {
			pendingOutbound = firstNonEmptyString(step.Label, step.OutboundTag, step.Target)
			continue
		}
		if step.AgentID == "" || step.AgentID == rootAgentID {
			continue
		}
		if step.StepType != "match" && step.StepType != "inbound" {
			continue
		}
		label := firstNonEmptyString(customerAgentDisplayName(step.AgentID, agentMap), step.AgentName, step.Label)
		// A forwarding assignment is rooted at the Realm/HAProxy listener,
		// while the chain root is the final X-UI Client. Include the final
		// inbound label so the customer can see the actual node behind the CN
		// entry instead of only the target Client name.
		if step.StepType == "inbound" && step.AgentID == chain.RootAgentID && chain.RootAgentID != rootAgentID {
			nodeLabel := firstNonEmptyString(step.Label, step.Detail, chain.RootInboundTag)
			agentLabel := firstNonEmptyString(customerAgentDisplayName(step.AgentID, agentMap), step.AgentName, step.AgentID)
			if nodeLabel != "" && !strings.EqualFold(agentLabel, nodeLabel) {
				label = agentLabel + " / " + nodeLabel
			}
		}
		appendRelay(label)
		pendingOutbound = ""
	}
	if pendingOutbound != "" && strings.Contains(chain.UnresolvedReason, "outbound target did not match") {
		appendRelay(pendingOutbound)
	}
	return result
}

func customerAgentDisplayName(agentID string, agentMap map[string]model.DashboardAgentView) string {
	if agentID == "" {
		return ""
	}
	if agent, ok := agentMap[agentID]; ok {
		return strings.TrimSpace(agent.CustomerDisplayName)
	}
	return ""
}

func findCustomerChain(assignment model.CustomerAssignment, chainMap map[string]model.ClientChainView) (model.ClientChainView, bool) {
	if assignment.ClientEmail != "" {
		chain, ok := chainMap[customerAssignmentKey(assignment.AgentID, assignment.InboundID, assignment.ClientEmail)]
		return chain, ok
	}
	prefix := fmt.Sprintf("%s::%d::", assignment.AgentID, assignment.InboundID)
	var best model.ClientChainView
	found := false
	for key, chain := range chainMap {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if !found || chain.RootClientEmail < best.RootClientEmail {
			best = chain
			found = true
		}
	}
	return best, found
}

func customerExitInfo(chain model.ClientChainView, agentMap map[string]model.DashboardAgentView) (string, string, string) {
	for i := len(chain.Steps) - 1; i >= 0; i-- {
		step := chain.Steps[i]
		if step.AgentID != "" && step.AgentID != chain.RootAgentID && (step.StepType == "match" || step.StepType == "inbound" || step.StepType == "client") {
			if agent, ok := agentMap[step.AgentID]; ok && agent.Geo != nil {
				return customerGeoParts(agent.Geo.IP, agent.Geo)
			}
		}
		if step.TargetIP != "" || step.TargetGeo != nil {
			return customerGeoParts(step.TargetIP, step.TargetGeo)
		}
		if step.StepType == "outbound" {
			if agent, ok := agentMap[step.AgentID]; ok && agent.Geo != nil {
				return customerGeoParts(agent.Geo.IP, agent.Geo)
			}
		}
	}
	return "", "", ""
}

func customerGeoParts(ip string, geo *model.IPGeoView) (string, string, string) {
	countryCode := ""
	countryName := ""
	if geo != nil {
		if ip == "" {
			ip = geo.IP
		}
		countryCode = strings.ToUpper(strings.TrimSpace(geo.CountryCode))
		countryName = strings.TrimSpace(geo.CountryName)
	}
	return strings.TrimSpace(ip), countryCode, countryName
}

func customerExitLabel(countryCode, countryName, exitIP string) string {
	country := firstNonEmptyString(countryCode, countryName, "未知")
	if exitIP == "" {
		return country
	}
	return country + " " + exitIP
}

func customerLinkSummary(entryName string, relays []string) string {
	parts := []string{entryName}
	if len(relays) > 0 {
		parts = append(parts, "转发 "+strings.Join(relays, " -> "))
	}
	return strings.Join(parts, " ")
}

func customerAssignmentKey(agentID string, inboundID int, email string) string {
	return fmt.Sprintf("%s::%d::%s", agentID, inboundID, strings.TrimSpace(email))
}

func customerAssignmentClientKey(agentID string, inboundID int, clientID string) string {
	return fmt.Sprintf("%s::%d::id:%s", agentID, inboundID, strings.TrimSpace(clientID))
}
