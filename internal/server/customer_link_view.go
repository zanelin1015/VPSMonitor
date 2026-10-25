package server

import (
	"fmt"
	"math"
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
	return link
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
