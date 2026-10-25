package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}

	view, err := a.dashboardViewForAdmin(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) handleDashboardTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}

	view, err := a.dashboardTopologyViewForAdmin(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) dashboardViewForAdmin(user model.AdminUser) (model.GlobalDashboardView, error) {
	cacheKey := dashboardCacheKey(user)
	now := time.Now()

	a.dashboardCacheMu.Lock()
	if a.dashboardCache == nil {
		a.dashboardCache = make(map[string]dashboardCacheEntry)
	}
	if entry, ok := a.dashboardCache[cacheKey]; ok && now.Before(entry.expiresAt) {
		view, err := cloneDashboardView(entry.view)
		a.dashboardCacheMu.Unlock()
		return view, err
	}
	a.dashboardCacheMu.Unlock()

	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return model.GlobalDashboardView{}, err
	}
	agents = a.filterAgentRecordsForAdmin(user, agents)
	snapshots = a.filterSnapshotsForAdmin(user, snapshots)

	view := dashboard.BuildGlobalDashboardWithOptions(agents, snapshots, dashboard.GlobalDashboardOptions{
		IncludeTopology:    true,
		IncludeGeo:         true,
		AllowNetworkLookup: false,
		ResolverData:       a.dashboardTopologyResolverData(),
	})
	if a.realtime != nil {
		a.realtime.applyToDashboard(&view)
	}
	a.sanitizeDashboardForAdmin(user, &view)

	cachedView, err := cloneDashboardView(view)
	if err != nil {
		return view, nil
	}
	a.dashboardCacheMu.Lock()
	a.dashboardCache[cacheKey] = dashboardCacheEntry{
		expiresAt: time.Now().Add(dashboardCacheTTL),
		view:      cachedView,
	}
	a.dashboardCacheMu.Unlock()
	return view, nil
}

func (a *App) dashboardTopologyViewForAdmin(user model.AdminUser) (model.GlobalDashboardView, error) {
	cacheKey := dashboardCacheKey(user)
	now := time.Now()

	for {
		a.dashboardCacheMu.Lock()
		if a.topologyCache == nil {
			a.topologyCache = make(map[string]dashboardCacheEntry)
		}
		if a.topologyBuilds == nil {
			a.topologyBuilds = make(map[string]chan struct{})
		}
		if entry, ok := a.topologyCache[cacheKey]; ok && now.Before(entry.expiresAt) {
			view, err := cloneDashboardView(entry.view)
			a.dashboardCacheMu.Unlock()
			return view, err
		}
		if ch, ok := a.topologyBuilds[cacheKey]; ok {
			if entry, hasStale := a.topologyCache[cacheKey]; hasStale {
				view, err := cloneDashboardView(entry.view)
				a.dashboardCacheMu.Unlock()
				return view, err
			}
			a.dashboardCacheMu.Unlock()
			<-ch
			continue
		}
		a.topologyBuilds[cacheKey] = make(chan struct{})
		a.dashboardCacheMu.Unlock()
		break
	}

	view, err := a.buildDashboardViewForAdmin(user, true)

	a.dashboardCacheMu.Lock()
	if ch, ok := a.topologyBuilds[cacheKey]; ok {
		close(ch)
		delete(a.topologyBuilds, cacheKey)
	}
	if err == nil {
		if cachedView, cloneErr := cloneDashboardView(view); cloneErr == nil {
			a.topologyCache[cacheKey] = dashboardCacheEntry{
				expiresAt: time.Now().Add(topologyCacheTTL),
				view:      cachedView,
			}
		}
	}
	a.dashboardCacheMu.Unlock()

	return view, err
}

func (a *App) buildDashboardViewForAdmin(user model.AdminUser, includeTopology bool) (model.GlobalDashboardView, error) {
	agents, snapshots, err := a.store.ListAgentsWithLatestSnapshots()
	if err != nil {
		return model.GlobalDashboardView{}, err
	}
	// Area-manager forwarding paths can include relay Clients that are not
	// directly assigned. Topology must resolve against the complete graph first;
	// sanitizeDashboardForAdmin removes every Client outside an authorized path.
	if !includeTopology || isRootAdmin(user) {
		agents = a.filterAgentRecordsForAdmin(user, agents)
		snapshots = a.filterSnapshotsForAdmin(user, snapshots)
	}

	view := dashboard.BuildGlobalDashboardWithOptions(agents, snapshots, dashboard.GlobalDashboardOptions{
		IncludeTopology:    includeTopology,
		IncludeGeo:         true,
		AllowNetworkLookup: false,
		ResolverData:       a.dashboardTopologyResolverData(),
	})
	if a.realtime != nil {
		a.realtime.applyToDashboard(&view)
	}
	a.sanitizeDashboardForAdmin(user, &view)
	return view, nil
}

func (a *App) dashboardTopologyResolverData() dashboard.TopologyResolverData {
	cache, found, err := a.store.GetTopologyLookupCache()
	if err != nil {
		log.Printf("load topology lookup cache failed: %v", err)
		return dashboard.TopologyResolverData{}
	}
	if !found {
		return dashboard.TopologyResolverData{}
	}
	data := dashboard.TopologyResolverData{
		Hosts: make(map[string][]string, len(cache.Hosts)),
		Geos:  make(map[string]model.IPGeoView, len(cache.Geos)),
	}
	for key, entry := range cache.Hosts {
		data.Hosts[key] = append([]string(nil), entry.IPs...)
	}
	for key, entry := range cache.Geos {
		data.Geos[key] = entry.Geo
	}
	return data
}

func (a *App) refreshTopologyLookupCacheFromRegister(req model.AgentRegisterRequest, observedIP string) {
	values := []string{req.PublicIPv4, req.PublicIPv6, req.Hostname}
	if isUsableObservedIP(observedIP) {
		values = append(values, observedIP)
	}
	a.refreshTopologyLookupCache(req.AgentID, values)
}

func (a *App) refreshTopologyLookupCacheFromSnapshot(agentID string, snapshot model.AgentSnapshot) {
	values := []string{
		snapshot.Summary.ObservedIP,
		snapshot.Summary.ServerSeenIP,
		snapshot.Summary.PublicIPv4,
		snapshot.Summary.PublicIPv6,
	}
	if snapshot.XUI != nil {
		values = append(values, snapshot.XUI.BaseURL)
		overview := dashboard.BuildXUIOverview(snapshot)
		if overview != nil {
			for _, outbound := range overview.Outbounds {
				values = append(values, outbound.Address, outbound.TLSServerName, outbound.WSHost, outbound.SendThrough)
			}
		}
	}
	if snapshot.Realm != nil {
		for _, rule := range snapshot.Realm.Rules {
			values = append(values, rule.ListenAddress, rule.TargetAddress)
		}
	}
	a.refreshTopologyLookupCache(agentID, values)
}

func (a *App) refreshTopologyLookupCache(agentID string, values []string) {
	a.lookupCacheMu.Lock()
	defer a.lookupCacheMu.Unlock()

	cache, _, err := a.store.GetTopologyLookupCache()
	if err != nil {
		log.Printf("refresh topology lookup cache for %s failed: load cache: %v", agentID, err)
		return
	}
	values = pendingTopologyLookupValues(cache, values)
	if len(values) == 0 {
		return
	}
	resolved := dashboard.NewTopologyResolverData(values)
	if len(resolved.Hosts) == 0 && len(resolved.Geos) == 0 {
		return
	}
	now := time.Now().UTC()
	if cache.Hosts == nil {
		cache.Hosts = make(map[string]model.TopologyHostCacheEntry)
	}
	if cache.Geos == nil {
		cache.Geos = make(map[string]model.TopologyGeoCacheEntry)
	}
	for host, ips := range resolved.Hosts {
		cache.Hosts[host] = model.TopologyHostCacheEntry{
			IPs:       append([]string(nil), ips...),
			UpdatedAt: now,
			ExpiresAt: now.Add(20 * time.Minute),
		}
	}
	for ip, geo := range resolved.Geos {
		if geo.CountryCode == "" && geo.CountryName == "" {
			continue
		}
		cache.Geos[ip] = model.TopologyGeoCacheEntry{
			Geo:       geo,
			UpdatedAt: now,
			ExpiresAt: now.Add(7 * 24 * time.Hour),
		}
	}
	if err := a.store.SaveTopologyLookupCache(cache); err != nil {
		log.Printf("refresh topology lookup cache for %s failed: save cache: %v", agentID, err)
		return
	}
	a.clearDashboardTopologyCache()
}

func (a *App) clearDashboardTopologyCache() {
	a.dashboardCacheMu.Lock()
	a.topologyCache = make(map[string]dashboardCacheEntry)
	a.dashboardCacheMu.Unlock()
}

func (a *App) clearCustomerOverviewCache() {
	a.dashboardCacheMu.Lock()
	a.customerViewCache = make(map[string]customerOverviewCacheEntry)
	a.dashboardCacheMu.Unlock()
}

func pendingTopologyLookupValues(cache model.TopologyLookupCache, values []string) []string {
	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(values))
	pending := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeTopologyLookupValue(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		if ip := normalizeTopologyLookupIP(normalized); ip != "" {
			entry, ok := cache.Geos[ip]
			if ok && now.Before(entry.ExpiresAt) && (entry.Geo.CountryCode != "" || entry.Geo.CountryName != "") {
				continue
			}
		} else {
			entry, ok := cache.Hosts[normalized]
			if ok && now.Before(entry.ExpiresAt) && len(entry.IPs) > 0 {
				continue
			}
		}
		pending = append(pending, normalized)
	}
	return pending
}

func normalizeTopologyLookupValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if strings.Contains(value, "/") {
		value = strings.Split(value, "/")[0]
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(strings.TrimSuffix(value, "."), "[]")
	return value
}

func normalizeTopologyLookupIP(value string) string {
	value = normalizeTopologyLookupValue(value)
	if value == "" {
		return ""
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return ""
	}
	return addr.String()
}

func dashboardCacheKey(user model.AdminUser) string {
	return fmt.Sprintf("%s:%d:%s:%d", user.Role, user.ID, user.Username, user.UpdatedAt.UnixNano())
}

func cloneDashboardView(view model.GlobalDashboardView) (model.GlobalDashboardView, error) {
	body, err := json.Marshal(view)
	if err != nil {
		return model.GlobalDashboardView{}, err
	}
	var cloned model.GlobalDashboardView
	if err := json.Unmarshal(body, &cloned); err != nil {
		return model.GlobalDashboardView{}, err
	}
	return cloned, nil
}
