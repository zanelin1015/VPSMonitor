package server

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

const (
	mihomoProxyMarker           = "{{VPSMONITOR_PROXIES}}"
	mihomoProxyNameMarker       = "{{VPSMONITOR_PROXY_NAMES}}"
	mihomoFrontProxyGroupMarker = "{{VPSMONITOR_FRONT_PROXY_GROUPS}}"
	mihomoHKProxyNameMarker     = "{{VPSMONITOR_HK_PROXY_NAMES}}"
	mihomoJPProxyNameMarker     = "{{VPSMONITOR_JP_PROXY_NAMES}}"
	mihomoUSProxyNameMarker     = "{{VPSMONITOR_US_PROXY_NAMES}}"
	mihomoSGProxyNameMarker     = "{{VPSMONITOR_SG_PROXY_NAMES}}"
	mihomoTWProxyNameMarker     = "{{VPSMONITOR_TW_PROXY_NAMES}}"
	mihomoKRProxyNameMarker     = "{{VPSMONITOR_KR_PROXY_NAMES}}"
	mihomoNetflixNameMarker     = "{{VPSMONITOR_NETFLIX_PROXY_NAMES}}"
)

//go:embed customer_subscription_acl4ssr.yaml.tmpl
var mihomoSubscriptionTemplate string

func (a *App) handleCustomerSubscription(w http.ResponseWriter, r *http.Request, parts []string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if len(parts) != 2 || !isMihomoSubscriptionFile(parts[1]) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	token, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(token) == "" {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	user, found, err := a.store.GetCustomerBySubscriptionToken(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	overview, err := a.customerOverview(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	overview.Links = filterCustomerSubscriptionLinks(overview.Links, r.URL.Query())
	content := buildMihomoSubscription(user, overview.Links)
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", customerSubscriptionContentDisposition(user.Username))
	_, _ = w.Write([]byte(content))
}

func parseCustomerSubscriptionAssignmentIDs(query url.Values) ([]int64, bool, error) {
	values, present := query["assignments"]
	if !present {
		return nil, false, nil
	}
	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			id, err := strconv.ParseInt(item, 10, 64)
			if err != nil || id <= 0 {
				return nil, true, fmt.Errorf("invalid customer subscription assignment id")
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, true, nil
}

func filterCustomerSubscriptionLinks(links []model.CustomerLinkView, query url.Values) []model.CustomerLinkView {
	ids, present, err := parseCustomerSubscriptionAssignmentIDs(query)
	if !present {
		return links
	}
	if err != nil {
		return []model.CustomerLinkView{}
	}
	selected := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		selected[id] = struct{}{}
	}
	filtered := make([]model.CustomerLinkView, 0, len(ids))
	for _, link := range links {
		if _, ok := selected[link.AssignmentID]; ok {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func customerSubscriptionContentDisposition(username string) string {
	title := strings.TrimSpace(username)
	if title == "" {
		title = "vpsmonitor"
	}
	return fmt.Sprintf(
		"inline; filename=%s; filename*=UTF-8''%s",
		safeSubscriptionFilename(title),
		url.PathEscape(title),
	)
}

func isMihomoSubscriptionFile(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mihomo.yaml", "mihomo.yml", "clash.yaml", "clash.yml":
		return true
	default:
		return false
	}
}

func buildMihomoSubscription(user model.CustomerUser, links []model.CustomerLinkView) string {
	names := make(map[string]int, len(links))
	frontProxyNames := make(map[int64]string)
	proxies := make([]mihomoProxy, 0, len(links))
	exitProxies := make([]mihomoProxy, 0, len(links))
	frontGroups := make([]mihomoProxyGroup, 0)
	for _, link := range links {
		rawImportURL := strings.TrimSpace(link.ImportURL)
		if !link.Resolved || rawImportURL == "" {
			continue
		}
		name := uniqueSubscriptionName(customerSubscriptionNodeName(link), names)
		proxy, ok := parseMihomoProxy(rawImportURL, name)
		if !ok {
			continue
		}
		configureMihomoRealityHandshake(&proxy, link.XrayVersion)
		frontNames := make([]string, 0, len(link.FrontProxies))
		for _, frontProxy := range link.FrontProxies {
			if strings.TrimSpace(frontProxy.ShareURL) == "" {
				continue
			}
			frontName, found := frontProxyNames[frontProxy.ID]
			if !found || frontProxy.ID <= 0 {
				frontName = uniqueSubscriptionName(firstNonEmptyString(frontProxy.Name, "前置代理"), names)
				parsed, ok := parseMihomoProxy(frontProxy.ShareURL, frontName)
				if !ok {
					continue
				}
				proxies = append(proxies, parsed)
				if frontProxy.ID > 0 {
					frontProxyNames[frontProxy.ID] = frontName
				}
			}
			frontNames = append(frontNames, frontName)
		}
		if len(frontNames) > 0 {
			groupName := uniqueSubscriptionName(name+" 前置代理", names)
			proxy.DialerProxy = groupName
			frontGroups = append(frontGroups, mihomoProxyGroup{Name: groupName, Proxies: frontNames})
		}
		proxies = append(proxies, proxy)
		exitProxies = append(exitProxies, proxy)
	}
	sort.SliceStable(proxies, func(i, j int) bool {
		return proxies[i].Name < proxies[j].Name
	})
	sort.SliceStable(exitProxies, func(i, j int) bool {
		return exitProxies[i].Name < exitProxies[j].Name
	})

	return renderMihomoSubscription(proxies, exitProxies, frontGroups)
}
