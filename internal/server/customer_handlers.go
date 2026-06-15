package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (a *App) handleCustomer(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/customer/"), "/")
	switch path {
	case "login":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerLogin(w, r)
	case "logout":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerLogout(w, r)
	case "session":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		user, _, ok := a.requireCustomer(w, r)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, model.CustomerLoginResponse{User: user})
	case "overview":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerOverview(w, r)
	case "style":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerStyleUpdate(w, r)
	case "account":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerAccountUpdate(w, r)
	default:
		if strings.HasPrefix(path, "assignments/") {
			a.handleCustomerAssignmentRoute(w, r, strings.Split(path, "/"))
			return
		}
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (a *App) handleCustomerLogin(w http.ResponseWriter, r *http.Request) {
	var req model.CustomerLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode customer login request: %v", err))
		return
	}
	user, ok, err := a.store.AuthenticateCustomer(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	token, session, err := a.store.CreateCustomerSession(user.ID, customerSessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setCustomerSessionCookie(w, r, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, model.CustomerLoginResponse{User: user})
}

func (a *App) handleCustomerLogout(w http.ResponseWriter, r *http.Request) {
	if token := readCustomerSessionToken(r); token != "" {
		_ = a.store.DeleteCustomerSession(token)
	}
	clearCustomerSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *App) handleCustomerOverview(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireCustomer(w, r)
	if !ok {
		return
	}
	response, err := a.customerOverview(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleCustomerStyleUpdate(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireCustomer(w, r)
	if !ok {
		return
	}
	var req model.CustomerStyleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode style request: %v", err))
		return
	}
	updated, err := a.store.UpdateCustomerStyle(user.ID, req.StyleCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.CustomerLoginResponse{User: updated})
}

func (a *App) handleCustomerAccountUpdate(w http.ResponseWriter, r *http.Request) {
	user, token, ok := a.requireCustomer(w, r)
	if !ok {
		return
	}
	var req model.CustomerPasswordUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode customer account request: %v", err))
		return
	}
	updated, err := a.store.UpdateCustomerPassword(user.ID, req, token)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.CustomerLoginResponse{User: updated})
}

func (a *App) handleCustomerAssignmentRoute(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) != 3 || parts[0] != "assignments" || parts[2] != "remark" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireCustomer(w, r)
	if !ok {
		return
	}
	assignmentID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || assignmentID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid assignment id")
		return
	}
	var req model.CustomerRemarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode remark request: %v", err))
		return
	}
	assignment, err := a.store.UpdateCustomerAssignmentRemark(user.ID, assignmentID, req.Remark)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, assignment)
}

func (a *App) customerOverview(user model.CustomerUser) (model.CustomerOverviewResponse, error) {
	assignments, err := a.store.ListEnabledCustomerAssignments(user.ID)
	if err != nil {
		return model.CustomerOverviewResponse{}, err
	}
	view, agents, snapshots, err := a.customerOverviewContext()
	if err != nil {
		return model.CustomerOverviewResponse{}, err
	}
	a.realtime.applyToDashboard(&view)
	clientMap := buildCustomerClientMap(snapshots, agents)
	chainMap := make(map[string]model.ClientChainView, len(view.ClientChains))
	for _, chain := range view.ClientChains {
		chainMap[chain.Key] = chain
	}
	agentMap := make(map[string]model.DashboardAgentView, len(view.Agents))
	for _, agent := range view.Agents {
		agentMap[agent.AgentID] = agent
	}

	links := make([]model.CustomerLinkView, 0, len(assignments))
	for _, assignment := range assignments {
		links = append(links, buildCustomerLinkView(assignment, chainMap, clientMap, agentMap))
	}
	return model.CustomerOverviewResponse{
		User:        user,
		GeneratedAt: time.Now().UTC(),
		Links:       links,
	}, nil
}

func (a *App) customerOverviewContext() (model.GlobalDashboardView, []model.AgentRecord, []model.AgentSnapshot, error) {
	now := time.Now()
	a.dashboardCacheMu.Lock()
	if a.customerViewCache == nil {
		a.customerViewCache = make(map[string]customerOverviewCacheEntry)
	}
	if entry, ok := a.customerViewCache["global"]; ok && now.Before(entry.expiresAt) {
		view, agents, snapshots, err := cloneCustomerOverviewContext(entry)
		a.dashboardCacheMu.Unlock()
		return view, agents, snapshots, err
	}
	a.dashboardCacheMu.Unlock()

	agents, err := a.store.ListAgents()
	if err != nil {
		return model.GlobalDashboardView{}, nil, nil, err
	}
	snapshots := a.store.ListLatest()
	view := dashboard.BuildGlobalDashboardWithOptions(agents, snapshots, dashboard.GlobalDashboardOptions{
		IncludeTopology:    true,
		IncludeGeo:         true,
		AllowNetworkLookup: false,
		ResolverData:       a.dashboardTopologyResolverData(),
	})

	entry := customerOverviewCacheEntry{
		expiresAt: time.Now().Add(customerOverviewCacheTTL),
		view:      view,
		agents:    agents,
		snapshots: snapshots,
	}
	if cachedView, cachedAgents, cachedSnapshots, err := cloneCustomerOverviewContext(entry); err == nil {
		a.dashboardCacheMu.Lock()
		a.customerViewCache["global"] = customerOverviewCacheEntry{
			expiresAt: entry.expiresAt,
			view:      cachedView,
			agents:    cachedAgents,
			snapshots: cachedSnapshots,
		}
		a.dashboardCacheMu.Unlock()
	}
	return view, agents, snapshots, nil
}

func cloneCustomerOverviewContext(entry customerOverviewCacheEntry) (model.GlobalDashboardView, []model.AgentRecord, []model.AgentSnapshot, error) {
	view, err := cloneDashboardView(entry.view)
	if err != nil {
		return model.GlobalDashboardView{}, nil, nil, err
	}
	agents := append([]model.AgentRecord(nil), entry.agents...)
	snapshots := append([]model.AgentSnapshot(nil), entry.snapshots...)
	return view, agents, snapshots, nil
}

type customerClientRef struct {
	Client            model.XUIClientView
	TemplateImportURL string
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
			result[key] = customerClientRef{
				Client:            client,
				TemplateImportURL: templateImportURLs[key],
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
	var clientRef customerClientRef
	if ref, ok := clientMap[customerAssignmentKey(assignment.AgentID, assignment.InboundID, assignment.ClientEmail)]; ok {
		clientRef = ref
		link.ImportURL = clientRef.Client.ImportURL
		link.ClientRemark = firstNonEmptyString(clientRef.Client.Comment, clientRef.Client.SubID)
		if clientRef.Client.ExpiryTime > 0 {
			link.ExpireTime = clientRef.Client.ExpiryTime
		}
	}
	if billing, ok := customerBillingForAssignment(assignment, agentMap); ok {
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

	chain, ok := findCustomerChain(assignment, chainMap)
	if !ok {
		link.UnresolvedReason = "当前最新上报中没有找到该用户链路"
		link.Summary = entryName + " 暂无链路数据"
		return link
	}
	link.Resolved = true
	if publicEntry, ok := customerRealmPublicEntry(assignment, chain, agentMap); ok {
		sourceURL := firstNonEmptyString(link.ImportURL, clientRef.TemplateImportURL)
		if rewritten := rewriteCustomerImportURL(sourceURL, publicEntry.Host, publicEntry.Port); rewritten != "" {
			link.ImportURL = rewritten
		}
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
	link.Summary = customerLinkSummary(entryName, relays, exitCountryCode, exitCountryName, exitIP)
	if chain.UnresolvedReason != "" {
		link.UnresolvedReason = chain.UnresolvedReason
	}
	return link
}

type customerPublicEntry struct {
	Host string
	Port int
}

func customerRealmPublicEntry(assignment model.CustomerAssignment, chain model.ClientChainView, agentMap map[string]model.DashboardAgentView) (customerPublicEntry, bool) {
	if assignment.AgentID == "" || agentMap == nil {
		return customerPublicEntry{}, false
	}
	targetAgent, ok := agentMap[assignment.AgentID]
	if !ok {
		return customerPublicEntry{}, false
	}
	inboundPort := customerChainRootInboundPort(assignment, chain)
	targetAddresses := customerAgentAddressSet(targetAgent)
	for _, sourceAgent := range agentMap {
		if sourceAgent.AgentID == "" || sourceAgent.AgentID == assignment.AgentID {
			continue
		}
		for _, rule := range sourceAgent.Entry.PortForwarding.Rules {
			if !rule.Enabled || rule.ListenPort <= 0 || rule.TargetPort <= 0 {
				continue
			}
			if inboundPort > 0 && rule.TargetPort != inboundPort {
				continue
			}
			if !customerRealmRuleTargetsAgent(rule, assignment.AgentID, targetAddresses) {
				continue
			}
			host := customerRealmSourceHost(sourceAgent, rule)
			if host == "" {
				continue
			}
			return customerPublicEntry{Host: host, Port: rule.ListenPort}, true
		}
	}
	return customerPublicEntry{}, false
}

func customerChainRootInboundPort(assignment model.CustomerAssignment, chain model.ClientChainView) int {
	for _, step := range chain.Steps {
		if step.AgentID != assignment.AgentID || step.Port <= 0 {
			continue
		}
		if step.StepType == "inbound" || step.StepType == "match" {
			return step.Port
		}
	}
	return 0
}

func customerRealmRuleTargetsAgent(rule model.RealmForwardRule, targetAgentID string, targetAddresses map[string]struct{}) bool {
	if strings.TrimSpace(rule.TargetAgentID) != "" {
		return strings.EqualFold(strings.TrimSpace(rule.TargetAgentID), targetAgentID)
	}
	targetHost := normalizeCustomerShareHost(rule.TargetAddress)
	if targetHost == "" {
		return false
	}
	_, ok := targetAddresses[strings.ToLower(targetHost)]
	return ok
}

func customerAgentAddressSet(agent model.DashboardAgentView) map[string]struct{} {
	values := append([]string{}, agent.Entry.Addresses...)
	values = append(values, agent.Entry.ImportDomain)
	values = append(values, agent.Summary.ObservedIP, agent.Summary.PublicIPv4, agent.Summary.PublicIPv6)
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		host := normalizeCustomerShareHost(value)
		if host == "" {
			continue
		}
		result[strings.ToLower(host)] = struct{}{}
	}
	return result
}

func customerRealmSourceHost(agent model.DashboardAgentView, rule model.RealmForwardRule) string {
	for _, candidate := range []string{agent.Entry.ImportDomain} {
		if host := normalizeCustomerShareHost(candidate); host != "" && !isWildcardListenAddress(host) {
			return host
		}
	}
	for _, candidate := range agent.Entry.Addresses {
		if host := normalizeCustomerShareHost(candidate); host != "" && !isWildcardListenAddress(host) {
			return host
		}
	}
	for _, candidate := range []string{rule.ListenAddress, agent.Summary.ObservedIP, agent.Summary.PublicIPv4, agent.Summary.PublicIPv6} {
		if host := normalizeCustomerShareHost(candidate); host != "" && !isWildcardListenAddress(host) {
			return host
		}
	}
	return ""
}

func rewriteCustomerImportURL(rawURL string, host string, port int) string {
	rawURL = strings.TrimSpace(rawURL)
	host = normalizeCustomerShareHost(host)
	if rawURL == "" || host == "" || port <= 0 || port > 65535 {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "vmess://") {
		return rewriteCustomerVMessImportURL(rawURL, host, port)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return ""
	}
	parsed.Host = net.JoinHostPort(host, strconv.Itoa(port))
	return parsed.String()
}

func rewriteCustomerVMessImportURL(rawURL string, host string, port int) string {
	payload := strings.TrimSpace(rawURL[len("vmess://"):])
	decoded, err := decodeCustomerVMessPayload(payload)
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(decoded, &data); err != nil {
		return ""
	}
	data["add"] = host
	data["port"] = port
	next, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(next)
}

func decodeCustomerVMessPayload(payload string) ([]byte, error) {
	normalized := strings.NewReplacer("-", "+", "_", "/").Replace(payload)
	if padding := len(normalized) % 4; padding > 0 {
		normalized += strings.Repeat("=", 4-padding)
	}
	return base64.StdEncoding.DecodeString(normalized)
}

func normalizeCustomerShareHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Host
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(strings.TrimSpace(value), "[]")
	if slash := strings.Index(value, "/"); slash >= 0 {
		value = value[:slash]
	}
	return strings.TrimSpace(value)
}

func isWildcardListenAddress(host string) bool {
	switch strings.TrimSpace(strings.ToLower(host)) {
	case "", "0.0.0.0", "::", "[::]", "*":
		return true
	default:
		return false
	}
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
	for _, billing := range agent.Renewal.ClientBillings {
		if customerBillingKey(billing.InboundID, billing.InboundTag, billing.Email) == exactKey {
			return billing, true
		}
		if emailKey != "" && customerBillingEmailKey(billing.InboundID, billing.Email) == emailKey {
			return billing, true
		}
	}
	return model.XUIClientBillingConfig{}, false
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
		return "出口 " + country
	}
	return "出口 " + country + " " + exitIP
}

func customerLinkSummary(entryName string, relays []string, countryCode, countryName, exitIP string) string {
	parts := []string{entryName}
	if len(relays) > 0 {
		parts = append(parts, "转发 "+strings.Join(relays, " -> "))
	}
	parts = append(parts, customerExitLabel(countryCode, countryName, exitIP))
	return strings.Join(parts, " ")
}

func customerAssignmentKey(agentID string, inboundID int, email string) string {
	return fmt.Sprintf("%s::%d::%s", agentID, inboundID, strings.TrimSpace(email))
}

func (a *App) requireCustomer(w http.ResponseWriter, r *http.Request) (model.CustomerUser, string, bool) {
	user, token, ok := a.currentCustomer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "customer login required")
		return model.CustomerUser{}, "", false
	}
	return user, token, true
}

func (a *App) currentCustomer(r *http.Request) (model.CustomerUser, string, bool) {
	token := readCustomerSessionToken(r)
	user, _, ok, err := a.store.ValidateCustomerSession(token)
	if err != nil || !ok {
		return model.CustomerUser{}, "", false
	}
	return user, token, true
}

func readCustomerSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(customerSessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setCustomerSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     customerSessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

func clearCustomerSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     customerSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}
