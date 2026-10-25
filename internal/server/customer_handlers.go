package server

import (
	"encoding/json"
	"fmt"
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
	if path == "support" || strings.HasPrefix(path, "support/") {
		rest := strings.Trim(strings.TrimPrefix(path, "support"), "/")
		parts := []string{}
		if rest != "" {
			parts = strings.Split(rest, "/")
		}
		a.handleCustomerSupport(w, r, parts)
		return
	}
	if strings.HasPrefix(path, "subscription/") {
		a.handleCustomerSubscription(w, r, strings.Split(path, "/")[1:])
		return
	}
	switch path {
	case "login":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerLogin(w, r)
	case "refresh":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerRefresh(w, r)
	case "logout":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerLogout(w, r)
	case "session":
		w.Header().Set("Cache-Control", "no-store")
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
	case "usage":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerUsage(w, r)
	case "announcements/read":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		a.handleCustomerAnnouncementRead(w, r)
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
	token, err := a.store.EnsureCustomerSubscriptionToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.ClashSubscriptionURL = customerSubscriptionURL(r, token, "clash.yaml")
	response.MihomoSubscriptionURL = customerSubscriptionURL(r, token, "mihomo.yaml")
	redactCustomerOverviewFrontProxyShareURLs(&response)
	writeJSON(w, http.StatusOK, response)
}

func redactCustomerOverviewFrontProxyShareURLs(response *model.CustomerOverviewResponse) {
	if response == nil {
		return
	}
	for linkIndex := range response.Links {
		for proxyIndex := range response.Links[linkIndex].FrontProxies {
			response.Links[linkIndex].FrontProxies[proxyIndex].ShareURL = ""
		}
	}
}

func customerSubscriptionURL(r *http.Request, token string, filename string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	filename = strings.Trim(strings.TrimSpace(filename), "/")
	if !isMihomoSubscriptionFile(filename) {
		filename = "clash.yaml"
	}
	return requestPublicBaseURL(r) + "/api/v1/customer/subscription/" + url.PathEscape(token) + "/" + filename
}

func customerSubscriptionURLForAssignments(r *http.Request, token string, filename string, assignmentIDs []int64) string {
	base := customerSubscriptionURL(r, token, filename)
	if len(assignmentIDs) == 0 {
		return base
	}
	values := make([]string, 0, len(assignmentIDs))
	for _, assignmentID := range assignmentIDs {
		values = append(values, strconv.FormatInt(assignmentID, 10))
	}
	query := url.Values{}
	query.Set("assignments", strings.Join(values, ","))
	return base + "?" + query.Encode()
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
	chainMap := buildCustomerChainMap(view.ClientChains, view.Links)
	agentMap := make(map[string]model.DashboardAgentView, len(view.Agents))
	for _, agent := range view.Agents {
		agentMap[agent.AgentID] = agent
	}

	links := make([]model.CustomerLinkView, 0, len(assignments))
	assignmentIDs := make([]int64, 0, len(assignments))
	for _, assignment := range assignments {
		assignmentIDs = append(assignmentIDs, assignment.ID)
	}
	frontProxyByAssignment, err := a.store.ListFrontProxyNodesForAssignments(assignmentIDs)
	if err != nil {
		return model.CustomerOverviewResponse{}, err
	}
	for _, assignment := range assignments {
		link := buildCustomerLinkView(assignment, chainMap, clientMap, agentMap)
		link.FrontProxies = customerLinkFrontProxies(frontProxyByAssignment[assignment.ID])
		links = append(links, link)
	}
	frontendSettings, _, err := a.store.GetFrontendSettings()
	if err != nil {
		return model.CustomerOverviewResponse{}, err
	}
	readAnnouncementIDs, err := a.store.ListCustomerAnnouncementReads(user.ID)
	if err != nil {
		return model.CustomerOverviewResponse{}, err
	}
	return model.CustomerOverviewResponse{
		User:                user,
		GeneratedAt:         time.Now().UTC(),
		Announcements:       activeCustomerAnnouncements(frontendSettings.Announcements, time.Now().UTC()),
		ReadAnnouncementIDs: readAnnouncementIDs,
		Links:               links,
	}, nil
}

func (a *App) handleCustomerAnnouncementRead(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireCustomer(w, r)
	if !ok {
		return
	}
	var req model.CustomerAnnouncementReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode announcement read request: %v", err))
		return
	}
	if err := a.store.MarkCustomerAnnouncementsRead(user.ID, req.AnnouncementIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func customerLinkFrontProxies(items []model.FrontProxyNode) []model.CustomerLinkFrontProxy {
	if len(items) == 0 {
		return nil
	}
	result := make([]model.CustomerLinkFrontProxy, 0, len(items))
	for _, item := range items {
		result = append(result, model.CustomerLinkFrontProxy{
			ID:       item.ID,
			Name:     item.Name,
			Protocol: item.Protocol,
			ShareURL: item.ShareURL,
		})
	}
	return result
}

func activeCustomerAnnouncements(items []model.CustomerAnnouncement, now time.Time) []model.CustomerAnnouncement {
	active := make([]model.CustomerAnnouncement, 0, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		if startsAt, err := time.Parse(time.RFC3339, item.StartsAt); item.StartsAt != "" && (err != nil || now.Before(startsAt)) {
			continue
		}
		if endsAt, err := time.Parse(time.RFC3339, item.EndsAt); item.EndsAt != "" && (err != nil || !now.Before(endsAt)) {
			continue
		}
		active = append(active, item)
	}
	if len(active) == 0 {
		return nil
	}
	return active
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
