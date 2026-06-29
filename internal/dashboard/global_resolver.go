package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"bridge-core/internal/model"
)

type topologyResolver struct {
	cache        map[string][]string
	geoCache     map[string]model.IPGeoView
	allowNetwork bool
}

var topologyLookupHostIPs = defaultTopologyLookupHostIPs
var topologyLookupIPGeo = defaultTopologyLookupIPGeo

type TopologyResolverData struct {
	Hosts map[string][]string
	Geos  map[string]model.IPGeoView
}

func NewTopologyResolverData(values []string) TopologyResolverData {
	resolver := newTopologyResolverWithData(TopologyResolverData{}, true)
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		resolved := resolver.lookupHost(value)
		_, _ = resolver.lookupGeo(value, resolved)
	}
	return TopologyResolverData{
		Hosts: cloneResolverHosts(resolver.cache),
		Geos:  cloneResolverGeos(resolver.geoCache),
	}
}

func newTopologyResolverWithData(data TopologyResolverData, allowNetwork bool) *topologyResolver {
	resolver := &topologyResolver{
		cache:        make(map[string][]string),
		geoCache:     make(map[string]model.IPGeoView),
		allowNetwork: allowNetwork,
	}
	for host, ips := range data.Hosts {
		normalizedHost := normalizeHost(host)
		if normalizedHost == "" {
			continue
		}
		resolver.cache[normalizedHost] = uniqueNormalizedIPs(ips)
	}
	for ip, geo := range data.Geos {
		normalizedIP := normalizeIP(ip)
		if normalizedIP == "" && geo.IP != "" {
			normalizedIP = normalizeIP(geo.IP)
		}
		if normalizedIP == "" {
			continue
		}
		if geo.IP == "" {
			geo.IP = normalizedIP
		}
		resolver.geoCache[normalizedIP] = geo
	}
	return resolver
}

func (r *topologyResolver) lookupAll(values []string) []string {
	result := make([]string, 0)
	for _, value := range values {
		result = append(result, r.lookupHost(value)...)
	}
	return uniqueNormalizedIPs(result)
}

func (r *topologyResolver) lookupHost(value string) []string {
	host := normalizeHost(value)
	if host == "" {
		return nil
	}
	if cached, ok := r.cache[host]; ok {
		return append([]string(nil), cached...)
	}
	if !r.allowNetwork {
		r.cache[host] = nil
		return nil
	}
	resolved := topologyLookupHostIPs(host)
	resolved = uniqueNormalizedIPs(resolved)
	r.cache[host] = resolved
	return append([]string(nil), resolved...)
}

func (r *topologyResolver) lookupGeo(address string, resolvedIPs []string) (string, *model.IPGeoView) {
	candidates := make([]string, 0, len(resolvedIPs)+1)
	if ip := normalizeIP(extractEndpointHost(address)); ip != "" {
		candidates = append(candidates, ip)
	}
	candidates = append(candidates, resolvedIPs...)
	for _, candidate := range uniqueNormalizedIPs(candidates) {
		if !isPublicGeoIP(candidate) {
			continue
		}
		if cached, ok := r.geoCache[candidate]; ok {
			return candidate, cloneGeo(cached)
		}
		if !r.allowNetwork {
			continue
		}
		geo, ok := topologyLookupIPGeo(candidate)
		if !ok {
			r.geoCache[candidate] = model.IPGeoView{IP: candidate}
			continue
		}
		if geo.IP == "" {
			geo.IP = candidate
		}
		r.geoCache[candidate] = geo
		return candidate, cloneGeo(geo)
	}
	return "", nil
}

func cloneResolverHosts(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, item := range values {
		cloned[key] = append([]string(nil), item...)
	}
	return cloned
}

func cloneResolverGeos(values map[string]model.IPGeoView) map[string]model.IPGeoView {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]model.IPGeoView, len(values))
	for key, item := range values {
		cloned[key] = item
	}
	return cloned
}

func cloneGeo(geo model.IPGeoView) *model.IPGeoView {
	if geo == (model.IPGeoView{}) {
		return nil
	}
	cloned := geo
	return &cloned
}

func defaultTopologyLookupHostIPs(host string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(ips))
	for _, ip := range ips {
		result = append(result, ip.String())
	}
	return result
}

func defaultTopologyLookupIPGeo(ip string) (model.IPGeoView, bool) {
	if !isPublicGeoIP(ip) {
		return model.IPGeoView{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	endpoint := "http://ip-api.com/json/" + url.PathEscape(ip) + "?fields=status,message,country,countryCode,regionName,city,query"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return model.IPGeoView{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return model.IPGeoView{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return model.IPGeoView{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return model.IPGeoView{}, false
	}
	var payload struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		RegionName  string `json:"regionName"`
		City        string `json:"city"`
		Query       string `json:"query"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Status != "success" {
		return model.IPGeoView{}, false
	}
	return model.IPGeoView{
		IP:          firstNonEmpty(payload.Query, ip),
		CountryCode: strings.ToUpper(payload.CountryCode),
		CountryName: payload.Country,
		RegionName:  payload.RegionName,
		City:        payload.City,
	}, true
}

func isPublicGeoIP(value string) bool {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return false
	}
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return false
	}
	for _, prefixText := range []string{
		"192.0.2.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	} {
		prefix := netip.MustParsePrefix(prefixText)
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}
