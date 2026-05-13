package dashboard

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

type routeRule struct {
	view model.XUIRoutingRuleView
}

type outboundTraffic struct {
	up    int64
	down  int64
	total int64
}

type inboundRecord struct {
	view                model.XUINodeView
	clientStats         map[string]clientStat
	clients             []clientConfig
	importHost          string
	vlessEncryption     string
	shadowsocksMethod   string
	shadowsocksPassword string
	hysteriaVersion     int
}

type clientConfig struct {
	email        string
	comment      string
	enable       bool
	authUUID     string
	authPassword string
	auth         string
	alterID      int
	security     string
	flow         string
	limitIP      int
	totalGB      int64
	expiryTime   int64
	subID        string
	createdAt    int64
	updatedAt    int64
}

type inboundStreamMeta struct {
	network        string
	security       string
	tlsServerName  string
	tlsFingerprint string
	alpn           string
	wsPath         string
	wsHost         string
	grpcService    string
	realityServer  string
	realityPubKey  string
	realityShortID string
	realityFP      string
	realitySpider  string
}

type inboundProtocolMeta struct {
	vlessEncryption     string
	shadowsocksMethod   string
	shadowsocksPassword string
	hysteriaVersion     int
}

type clientStat struct {
	enable     bool
	up         int64
	down       int64
	allTime    int64
	total      int64
	expiryTime int64
	lastOnline int64
}

func BuildXUIOverview(snapshot model.AgentSnapshot) *model.XUIOverview {
	if snapshot.XUI == nil {
		return nil
	}

	rules := normalizeRouteRules(snapshot.XUI.RoutingRules)
	trafficByTag := outboundTrafficByTag(snapshot.XUI.OutboundTraffic)
	outbounds, defaultOutboundTag := normalizeOutbounds(snapshot.XUI.Outbounds, trafficByTag)
	balancers := normalizeBalancers(snapshot.XUI.RawConfig, outbounds)
	globalRuleIndexes := collectGlobalRuleIndexes(rules)
	inbounds := normalizeInbounds(snapshot.XUI.Inbounds, rules, defaultOutboundTag, globalRuleIndexes, snapshot.ReportedAt, snapshot.Summary, snapshot.XUI.BaseURL)
	clients, onlineCount := normalizeClients(inbounds, rules, defaultOutboundTag, globalRuleIndexes, snapshot.ReportedAt)

	nodes := make([]model.XUINodeView, 0, len(inbounds))
	for _, inbound := range inbounds {
		nodes = append(nodes, inbound.view)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Port != nodes[j].Port {
			return nodes[i].Port < nodes[j].Port
		}
		return nodes[i].Tag < nodes[j].Tag
	})
	sort.Slice(clients, func(i, j int) bool {
		if clients[i].InboundTag != clients[j].InboundTag {
			return clients[i].InboundTag < clients[j].InboundTag
		}
		return clients[i].Email < clients[j].Email
	})
	sort.Slice(outbounds, func(i, j int) bool {
		if outbounds[i].IsDefault != outbounds[j].IsDefault {
			return outbounds[i].IsDefault
		}
		return outbounds[i].Tag < outbounds[j].Tag
	})

	return &model.XUIOverview{
		AgentID:           snapshot.AgentID,
		AgentName:         snapshot.AgentName,
		BaseURL:           snapshot.XUI.BaseURL,
		ReportedAt:        snapshot.ReportedAt,
		CollectedAt:       snapshot.XUI.CollectedAt,
		Summary:           snapshot.Summary,
		NodeCount:         len(nodes),
		ClientCount:       len(clients),
		OnlineClientCount: onlineCount,
		Nodes:             nodes,
		Clients:           clients,
		Outbounds:         outbounds,
		Balancers:         balancers,
		RoutingRules:      unwrapRules(rules),
		Certificates:      append([]model.XUILocalCertificate{}, snapshot.XUI.Certificates...),
	}
}

func normalizeInbounds(rawInbounds []map[string]any, rules []routeRule, defaultOutboundTag string, globalRuleIndexes []int, reportedAt time.Time, summary model.VPSSummary, baseURL string) []inboundRecord {
	result := make([]inboundRecord, 0, len(rawInbounds))
	for _, raw := range rawInbounds {
		streamMeta := parseInboundStreamSettings(raw["streamSettings"])
		protocolMeta := parseInboundProtocolSettings(raw["settings"])
		record := inboundRecord{
			view: model.XUINodeView{
				ID:                 intValue(raw["id"]),
				Tag:                stringValue(raw["tag"]),
				Remark:             stringValue(raw["remark"]),
				Protocol:           stringValue(raw["protocol"]),
				Listen:             stringValue(raw["listen"]),
				Port:               intValue(raw["port"]),
				Network:            streamMeta.network,
				Security:           streamMeta.security,
				TLSServerName:      defaultString(streamMeta.tlsServerName, streamMeta.realityServer),
				ALPN:               streamMeta.alpn,
				WSPath:             streamMeta.wsPath,
				WSHost:             streamMeta.wsHost,
				GRPCService:        streamMeta.grpcService,
				RealityPubKey:      streamMeta.realityPubKey,
				RealityShortID:     streamMeta.realityShortID,
				RealityFingerprint: defaultString(streamMeta.realityFP, streamMeta.tlsFingerprint),
				RealitySpiderX:     streamMeta.realitySpider,
				Enabled:            boolValue(raw["enable"]),
				ExpiryTime:         int64Value(raw["expiryTime"]),
				Up:                 int64Value(raw["up"]),
				Down:               int64Value(raw["down"]),
				Total:              int64Value(raw["total"]),
				AllTime:            int64Value(raw["allTime"]),
				AuthKeys:           authKeysForInbound(stringValue(raw["protocol"]), raw["settings"]),
			},
			clientStats:         parseClientStats(raw["clientStats"]),
			clients:             parseInboundClients(raw["settings"]),
			importHost:          chooseSingleNodeImportHost(stringValue(raw["listen"]), summary, baseURL),
			vlessEncryption:     protocolMeta.vlessEncryption,
			shadowsocksMethod:   protocolMeta.shadowsocksMethod,
			shadowsocksPassword: protocolMeta.shadowsocksPassword,
			hysteriaVersion:     protocolMeta.hysteriaVersion,
		}
		record.view.ClientCount = len(record.clients)
		record.view.OnlineCount = countOnlineClients(record.clientStats, reportedAt)
		record.view.Route = resolveRoute("", record.view.Tag, rules, defaultOutboundTag, globalRuleIndexes)
		result = append(result, record)
	}
	return result
}

func normalizeClients(inbounds []inboundRecord, rules []routeRule, defaultOutboundTag string, globalRuleIndexes []int, reportedAt time.Time) ([]model.XUIClientView, int) {
	var clients []model.XUIClientView
	onlineCount := 0

	for _, inbound := range inbounds {
		for _, cfg := range inbound.clients {
			stats := inbound.clientStats[cfg.email]
			if isRecentlyOnline(stats.lastOnline, reportedAt) {
				onlineCount++
			}

			enabled := cfg.enable
			if _, ok := inbound.clientStats[cfg.email]; ok {
				enabled = stats.enable
			}

			expiryTime := cfg.expiryTime
			if stats.expiryTime != 0 {
				expiryTime = stats.expiryTime
			}

			clients = append(clients, model.XUIClientView{
				InboundID:     inbound.view.ID,
				InboundTag:    inbound.view.Tag,
				InboundRemark: inbound.view.Remark,
				Protocol:      inbound.view.Protocol,
				Email:         cfg.email,
				Comment:       cfg.comment,
				Enabled:       enabled,
				AuthUUID:      cfg.authUUID,
				AuthPassword:  cfg.authPassword,
				Flow:          cfg.flow,
				ImportURL:     buildSingleNodeImportURL(inbound, cfg),
				LimitIP:       cfg.limitIP,
				TotalGB:       cfg.totalGB,
				ExpiryTime:    expiryTime,
				SubID:         cfg.subID,
				CreatedAt:     cfg.createdAt,
				UpdatedAt:     cfg.updatedAt,
				Up:            stats.up,
				Down:          stats.down,
				AllTime:       stats.allTime,
				TrafficTotal:  chooseInt64(stats.allTime, stats.up+stats.down),
				LastOnline:    stats.lastOnline,
				Route:         resolveRoute(cfg.email, inbound.view.Tag, rules, defaultOutboundTag, globalRuleIndexes),
			})
		}
	}

	return clients, onlineCount
}

func normalizeOutbounds(rawOutbounds []map[string]any, trafficByTag map[string]outboundTraffic) ([]model.XUIOutboundView, string) {
	result := make([]model.XUIOutboundView, 0, len(rawOutbounds))
	defaultOutboundTag := ""

	for _, raw := range rawOutbounds {
		tag := stringValue(raw["tag"])
		if tag == "" {
			continue
		}
		if tag == "direct" {
			defaultOutboundTag = tag
		} else if defaultOutboundTag == "" && tag != "api" {
			defaultOutboundTag = tag
		}
		traffic := trafficByTag[tag]
		address, port := parseOutboundEndpoint(raw)
		streamMeta := parseInboundStreamSettings(raw["streamSettings"])
		result = append(result, model.XUIOutboundView{
			Tag:           tag,
			Protocol:      stringValue(raw["protocol"]),
			Target:        outboundTarget(raw),
			Address:       address,
			Port:          port,
			SendThrough:   stringValue(raw["sendThrough"]),
			Network:       streamMeta.network,
			Security:      streamMeta.security,
			TLSServerName: streamMeta.tlsServerName,
			WSPath:        streamMeta.wsPath,
			WSHost:        streamMeta.wsHost,
			Up:            traffic.up,
			Down:          traffic.down,
			Total:         chooseInt64(traffic.total, traffic.up+traffic.down),
			IsDefault:     false,
			AuthKeys:      authKeysForOutbound(stringValue(raw["protocol"]), raw["settings"]),
		})
	}

	for i := range result {
		if result[i].Tag == defaultOutboundTag {
			result[i].IsDefault = true
		}
	}

	return result, defaultOutboundTag
}

func normalizeBalancers(rawConfig map[string]any, outbounds []model.XUIOutboundView) []model.XUIBalancerView {
	routing := objectMap(rawConfig["routing"])
	items := objectList(routing["balancers"])
	result := make([]model.XUIBalancerView, 0, len(items))

	for _, item := range items {
		tag := stringValue(item["tag"])
		if tag == "" {
			continue
		}
		selectors := stringList(zeroIfNil(item["selector"]))
		if len(selectors) == 0 {
			selectors = stringList(zeroIfNil(item["selectors"]))
		}
		fallbackTag := stringValue(item["fallbackTag"])
		matchedOutbounds := selectBalancerOutbounds(selectors, fallbackTag, outbounds)
		result = append(result, model.XUIBalancerView{
			Tag:          tag,
			Selectors:    selectors,
			Strategy:     stringValue(item["strategy"]),
			FallbackTag:  fallbackTag,
			OutboundTags: matchedOutbounds,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Tag < result[j].Tag
	})
	return result
}

func normalizeRouteRules(rawRules []map[string]any) []routeRule {
	rules := make([]routeRule, 0, len(rawRules))
	for idx, raw := range rawRules {
		view := model.XUIRoutingRuleView{
			Index:       idx + 1,
			Type:        stringValue(raw["type"]),
			InboundTags: stringList(raw["inboundTag"]),
			Users:       stringList(raw["user"]),
			OutboundTag: stringValue(raw["outboundTag"]),
			BalancerTag: stringValue(raw["balancerTag"]),
			Domain:      stringList(raw["domain"]),
			IP:          stringList(raw["ip"]),
			Port:        stringList(raw["port"]),
			SourcePort:  stringList(raw["sourcePort"]),
			SourceIP:    stringList(raw["sourceIP"]),
			Network:     stringList(raw["network"]),
			Protocol:    stringList(raw["protocol"]),
			VLESSRoute:  stringList(raw["vlessRoute"]),
		}
		view.Summary = routeSummary(view)
		rules = append(rules, routeRule{view: view})
	}
	return rules
}

func unwrapRules(rules []routeRule) []model.XUIRoutingRuleView {
	out := make([]model.XUIRoutingRuleView, 0, len(rules))
	for _, rule := range rules {
		out = append(out, rule.view)
	}
	return out
}

func collectGlobalRuleIndexes(rules []routeRule) []int {
	indexes := make([]int, 0)
	for _, rule := range rules {
		if len(rule.view.InboundTags) == 0 && len(rule.view.Users) == 0 && (rule.view.OutboundTag != "" || rule.view.BalancerTag != "") {
			indexes = append(indexes, rule.view.Index)
		}
	}
	return indexes
}

func resolveRoute(email, inboundTag string, rules []routeRule, defaultOutboundTag string, globalRuleIndexes []int) model.XUIRouteTrace {
	for _, rule := range rules {
		if !ruleApplies(rule.view, email, inboundTag) {
			continue
		}
		scope := "inbound"
		if len(rule.view.Users) > 0 {
			scope = "user"
		}
		if len(rule.view.InboundTags) == 0 && len(rule.view.Users) == 0 {
			scope = "global"
		}
		return model.XUIRouteTrace{
			MatchScope:        scope,
			RuleIndex:         rule.view.Index,
			OutboundTag:       rule.view.OutboundTag,
			BalancerTag:       rule.view.BalancerTag,
			HasGlobalRules:    len(globalRuleIndexes) > 0,
			GlobalRuleIndexes: cloneInts(globalRuleIndexes),
			Note:              routeTraceNote(scope, rule.view),
		}
	}

	if defaultOutboundTag != "" {
		return model.XUIRouteTrace{
			MatchScope:        "default",
			OutboundTag:       defaultOutboundTag,
			HasGlobalRules:    len(globalRuleIndexes) > 0,
			GlobalRuleIndexes: cloneInts(globalRuleIndexes),
			Note:              "no direct inbound/user rule matched, using default outbound",
		}
	}

	return model.XUIRouteTrace{
		MatchScope:        "unmatched",
		HasGlobalRules:    len(globalRuleIndexes) > 0,
		GlobalRuleIndexes: cloneInts(globalRuleIndexes),
		Note:              "no outbound target could be inferred from the current config",
	}
}

func ruleApplies(rule model.XUIRoutingRuleView, email, inboundTag string) bool {
	if len(rule.Users) > 0 && !containsString(rule.Users, email) {
		return false
	}
	if len(rule.InboundTags) > 0 && !containsString(rule.InboundTags, inboundTag) {
		return false
	}
	if len(rule.Users) == 0 && len(rule.InboundTags) == 0 {
		return false
	}
	return rule.OutboundTag != "" || rule.BalancerTag != ""
}

func routeSummary(rule model.XUIRoutingRuleView) string {
	parts := make([]string, 0, 4)
	if len(rule.InboundTags) > 0 {
		parts = append(parts, "inbound="+strings.Join(rule.InboundTags, ","))
	}
	if len(rule.Users) > 0 {
		parts = append(parts, "user="+strings.Join(rule.Users, ","))
	}
	if rule.OutboundTag != "" {
		parts = append(parts, "outbound="+rule.OutboundTag)
	}
	if rule.BalancerTag != "" {
		parts = append(parts, "balancer="+rule.BalancerTag)
	}
	if len(parts) == 0 {
		return "global conditional rule"
	}
	return strings.Join(parts, " | ")
}

func routeTraceNote(scope string, rule model.XUIRoutingRuleView) string {
	switch scope {
	case "user":
		return fmt.Sprintf("matched user rule #%d", rule.Index)
	case "inbound":
		return fmt.Sprintf("matched inbound rule #%d", rule.Index)
	case "global":
		return fmt.Sprintf("matched global rule #%d", rule.Index)
	default:
		return ""
	}
}

func selectBalancerOutbounds(selectors []string, fallbackTag string, outbounds []model.XUIOutboundView) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, outbound := range outbounds {
		if outbound.Tag == "" {
			continue
		}
		if len(selectors) > 0 && !matchesAnySelector(outbound.Tag, selectors) {
			continue
		}
		if _, ok := seen[outbound.Tag]; ok {
			continue
		}
		seen[outbound.Tag] = struct{}{}
		result = append(result, outbound.Tag)
	}
	if fallbackTag != "" {
		if _, ok := seen[fallbackTag]; !ok {
			result = append(result, fallbackTag)
		}
	}
	sort.Strings(result)
	return result
}

func matchesAnySelector(tag string, selectors []string) bool {
	for _, selector := range selectors {
		if matchesSelector(tag, selector) {
			return true
		}
	}
	return false
}

func matchesSelector(tag string, selector string) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false
	}
	if selector == tag {
		return true
	}
	if strings.HasSuffix(selector, "*") {
		return strings.HasPrefix(tag, strings.TrimSuffix(selector, "*"))
	}
	if strings.Contains(selector, "*") {
		parts := strings.Split(selector, "*")
		current := tag
		for idx, part := range parts {
			if part == "" {
				continue
			}
			pos := strings.Index(current, part)
			if pos < 0 {
				return false
			}
			if idx == 0 && !strings.HasPrefix(selector, "*") && pos != 0 {
				return false
			}
			current = current[pos+len(part):]
		}
		return strings.HasSuffix(selector, "*") || current == ""
	}
	return strings.HasPrefix(tag, selector)
}

func outboundTrafficByTag(items []map[string]any) map[string]outboundTraffic {
	result := make(map[string]outboundTraffic, len(items))
	for _, item := range items {
		tag := stringValue(item["tag"])
		if tag == "" {
			continue
		}
		result[tag] = outboundTraffic{
			up:    int64Value(item["up"]),
			down:  int64Value(item["down"]),
			total: int64Value(item["total"]),
		}
	}
	return result
}

func parseClientStats(raw any) map[string]clientStat {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		}
	}
	result := make(map[string]clientStat, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		email := stringValue(entry["email"])
		if email == "" {
			continue
		}
		result[email] = clientStat{
			enable:     boolValue(entry["enable"]),
			up:         int64Value(entry["up"]),
			down:       int64Value(entry["down"]),
			allTime:    int64Value(entry["allTime"]),
			total:      int64Value(entry["total"]),
			expiryTime: int64Value(entry["expiryTime"]),
			lastOnline: int64Value(entry["lastOnline"]),
		}
	}
	return result
}

func parseInboundClients(raw any) []clientConfig {
	payload := decodeStringObject(raw)
	if len(payload) == 0 {
		return nil
	}

	items := objectList(payload["clients"])
	result := make([]clientConfig, 0, len(items))
	for _, item := range items {
		email := stringValue(item["email"])
		if email == "" {
			continue
		}
		result = append(result, clientConfig{
			email:        email,
			comment:      stringValue(item["comment"]),
			enable:       boolValue(item["enable"]),
			authUUID:     stringValue(item["id"]),
			authPassword: stringValue(item["password"]),
			auth:         defaultString(stringValue(item["auth"]), stringValue(item["password"])),
			alterID:      intValue(item["alterId"]),
			security:     stringValue(item["security"]),
			flow:         stringValue(item["flow"]),
			limitIP:      intValue(item["limitIp"]),
			totalGB:      int64Value(item["totalGB"]),
			expiryTime:   int64Value(item["expiryTime"]),
			subID:        stringValue(item["subId"]),
			createdAt:    int64Value(item["created_at"]),
			updatedAt:    int64Value(item["updated_at"]),
		})
	}
	return result
}

func parseInboundProtocolSettings(raw any) inboundProtocolMeta {
	settings := decodeStringObject(raw)
	if len(settings) == 0 {
		return inboundProtocolMeta{}
	}
	return inboundProtocolMeta{
		vlessEncryption:     stringValue(settings["encryption"]),
		shadowsocksMethod:   stringValue(settings["method"]),
		shadowsocksPassword: stringValue(settings["password"]),
		hysteriaVersion:     intValue(settings["version"]),
	}
}

func parseInboundStreamSettings(raw any) inboundStreamMeta {
	settings := decodeStringObject(raw)
	meta := inboundStreamMeta{
		network:  stringValue(settings["network"]),
		security: stringValue(settings["security"]),
	}
	if tlsSettings := objectMap(settings["tlsSettings"]); len(tlsSettings) > 0 {
		tlsOption := objectMap(tlsSettings["settings"])
		meta.tlsServerName = defaultString(stringValue(tlsSettings["serverName"]), stringValue(tlsOption["serverName"]))
		meta.tlsFingerprint = defaultString(stringValue(tlsSettings["fingerprint"]), stringValue(tlsOption["fingerprint"]))
		meta.alpn = strings.Join(defaultStringList(stringList(tlsSettings["alpn"]), stringList(tlsOption["alpn"])), ",")
	}
	if wsSettings := objectMap(settings["wsSettings"]); len(wsSettings) > 0 {
		meta.wsPath = stringValue(wsSettings["path"])
		if headers := objectMap(wsSettings["headers"]); len(headers) > 0 {
			meta.wsHost = defaultString(firstString(stringList(headers["Host"])), firstString(stringList(headers["host"])))
		}
	}
	if grpcSettings := objectMap(settings["grpcSettings"]); len(grpcSettings) > 0 {
		meta.grpcService = stringValue(grpcSettings["serviceName"])
		meta.wsHost = defaultString(meta.wsHost, stringValue(grpcSettings["authority"]))
	}
	if realitySettings := objectMap(settings["realitySettings"]); len(realitySettings) > 0 {
		realityOption := objectMap(realitySettings["settings"])
		meta.realityServer = defaultString(firstString(stringList(realitySettings["serverNames"])), defaultString(stringValue(realitySettings["serverName"]), stringValue(realityOption["serverName"])))
		meta.realityPubKey = defaultString(stringValue(realitySettings["publicKey"]), stringValue(realityOption["publicKey"]))
		meta.realityShortID = defaultString(firstString(stringList(realitySettings["shortIds"])), defaultString(stringValue(realitySettings["shortId"]), defaultString(firstString(stringList(realityOption["shortIds"])), stringValue(realityOption["shortId"]))))
		meta.realityFP = defaultString(stringValue(realitySettings["fingerprint"]), stringValue(realityOption["fingerprint"]))
		meta.realitySpider = defaultString(stringValue(realitySettings["spiderX"]), stringValue(realityOption["spiderX"]))
	}
	return meta
}

func chooseSingleNodeImportHost(listen string, summary model.VPSSummary, baseURL string) string {
	listen = strings.TrimSpace(listen)
	if listen != "" && listen != "0.0.0.0" && listen != "::" && listen != "[::]" {
		return strings.Trim(listen, "[]")
	}
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if summary.PublicIPv4 != "" {
		return summary.PublicIPv4
	}
	return summary.PublicIPv6
}

func buildSingleNodeImportURL(inbound inboundRecord, cfg clientConfig) string {
	protocol := strings.ToLower(strings.TrimSpace(inbound.view.Protocol))
	if inbound.importHost == "" || inbound.view.Port == 0 {
		return ""
	}
	switch protocol {
	case "vless":
		return buildVLESSImportURL(inbound, cfg)
	case "vmess":
		return buildVMessImportURL(inbound, cfg)
	case "trojan":
		return buildTrojanImportURL(inbound, cfg)
	case "shadowsocks":
		return buildShadowsocksImportURL(inbound, cfg)
	case "hysteria", "hysteria2":
		return buildHysteriaImportURL(inbound, cfg)
	case "http":
		return buildUserPassURL("http", inbound, cfg)
	case "socks", "socks5":
		return buildUserPassURL("socks", inbound, cfg)
	default:
		return ""
	}
}

func buildVLESSImportURL(inbound inboundRecord, cfg clientConfig) string {
	if cfg.authUUID == "" {
		return ""
	}
	query := url.Values{}
	query.Set("type", defaultString(inbound.view.Network, "tcp"))
	query.Set("encryption", defaultString(inbound.vlessEncryption, "none"))
	if inbound.view.Security == "tls" || inbound.view.Security == "reality" {
		query.Set("security", inbound.view.Security)
	} else {
		query.Set("security", "none")
	}
	if cfg.flow != "" && inbound.view.Network == "tcp" {
		query.Set("flow", cfg.flow)
	}
	addSingleNodeStreamQuery(query, inbound.view)
	return "vless://" + cfg.authUUID + "@" + hostPortForShare(inbound.importHost, inbound.view.Port) + "?" + query.Encode() + "#" + url.PathEscape(shareRemark(inbound, cfg))
}

func buildVMessImportURL(inbound inboundRecord, cfg clientConfig) string {
	if cfg.authUUID == "" {
		return ""
	}
	payload := map[string]any{
		"v":    "2",
		"ps":   shareRemark(inbound, cfg),
		"add":  inbound.importHost,
		"port": inbound.view.Port,
		"id":   cfg.authUUID,
		"scy":  defaultString(cfg.security, "auto"),
		"net":  defaultString(inbound.view.Network, "tcp"),
		"tls":  "none",
		"type": "none",
	}
	if cfg.alterID > 0 {
		payload["aid"] = strconv.Itoa(cfg.alterID)
	}
	if inbound.view.WSHost != "" {
		payload["host"] = inbound.view.WSHost
	}
	if inbound.view.WSPath != "" {
		payload["path"] = inbound.view.WSPath
	}
	if inbound.view.GRPCService != "" {
		payload["path"] = inbound.view.GRPCService
	}
	if inbound.view.Security == "tls" || inbound.view.Security == "reality" {
		payload["tls"] = inbound.view.Security
	}
	if inbound.view.TLSServerName != "" {
		payload["sni"] = inbound.view.TLSServerName
	}
	if inbound.view.ALPN != "" {
		payload["alpn"] = inbound.view.ALPN
	}
	if inbound.view.RealityFingerprint != "" {
		payload["fp"] = inbound.view.RealityFingerprint
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(body)
}

func buildTrojanImportURL(inbound inboundRecord, cfg clientConfig) string {
	if cfg.authPassword == "" {
		return ""
	}
	query := url.Values{}
	query.Set("type", defaultString(inbound.view.Network, "tcp"))
	if inbound.view.Security == "tls" || inbound.view.Security == "reality" {
		query.Set("security", inbound.view.Security)
	} else {
		query.Set("security", "none")
	}
	if cfg.flow != "" && inbound.view.Security == "reality" && inbound.view.Network == "tcp" {
		query.Set("flow", cfg.flow)
	}
	addSingleNodeStreamQuery(query, inbound.view)
	return buildUserInfoURL("trojan", url.User(cfg.authPassword), inbound, cfg, query)
}

func buildShadowsocksImportURL(inbound inboundRecord, cfg clientConfig) string {
	if inbound.shadowsocksMethod == "" || cfg.authPassword == "" {
		return ""
	}
	encPart := inbound.shadowsocksMethod + ":" + cfg.authPassword
	if strings.HasPrefix(inbound.shadowsocksMethod, "2022-") && inbound.shadowsocksPassword != "" {
		encPart = inbound.shadowsocksMethod + ":" + inbound.shadowsocksPassword + ":" + cfg.authPassword
	}
	query := url.Values{}
	query.Set("type", defaultString(inbound.view.Network, "tcp"))
	if inbound.view.Security == "tls" {
		query.Set("security", "tls")
	}
	addSingleNodeStreamQuery(query, inbound.view)
	return buildUserInfoURL("ss", url.User(base64.StdEncoding.EncodeToString([]byte(encPart))), inbound, cfg, query)
}

func buildHysteriaImportURL(inbound inboundRecord, cfg clientConfig) string {
	if cfg.auth == "" {
		return ""
	}
	query := url.Values{}
	query.Set("security", "tls")
	addSingleNodeStreamQuery(query, inbound.view)
	scheme := "hysteria2"
	if inbound.hysteriaVersion == 1 || strings.EqualFold(inbound.view.Protocol, "hysteria") {
		scheme = "hysteria"
	}
	return buildUserInfoURL(scheme, url.User(cfg.auth), inbound, cfg, query)
}

func buildUserInfoURL(scheme string, user *url.Userinfo, inbound inboundRecord, cfg clientConfig, query url.Values) string {
	uri := url.URL{
		Scheme:   scheme,
		User:     user,
		Host:     hostPortForShare(inbound.importHost, inbound.view.Port),
		RawQuery: query.Encode(),
		Fragment: shareRemark(inbound, cfg),
	}
	return uri.String()
}

func buildUserPassURL(scheme string, inbound inboundRecord, cfg clientConfig) string {
	if cfg.authUUID == "" && cfg.authPassword == "" {
		return ""
	}
	uri := url.URL{
		Scheme: scheme,
		Host:   hostPortForShare(inbound.importHost, inbound.view.Port),
		Path:   "/",
	}
	if cfg.authUUID != "" || cfg.authPassword != "" {
		uri.User = url.UserPassword(cfg.authUUID, cfg.authPassword)
	}
	uri.Fragment = shareRemark(inbound, cfg)
	return uri.String()
}

func addSingleNodeStreamQuery(query url.Values, node model.XUINodeView) {
	if node.TLSServerName != "" {
		query.Set("sni", node.TLSServerName)
	}
	if node.ALPN != "" {
		query.Set("alpn", node.ALPN)
	}
	if node.Security == "reality" {
		if node.RealityFingerprint != "" {
			query.Set("fp", node.RealityFingerprint)
		}
		if node.RealityPubKey != "" {
			query.Set("pbk", node.RealityPubKey)
		}
		if node.RealityShortID != "" {
			query.Set("sid", node.RealityShortID)
		}
		if node.RealitySpiderX != "" {
			query.Set("spx", node.RealitySpiderX)
		}
	}
	if node.Security == "tls" && node.RealityFingerprint != "" {
		query.Set("fp", node.RealityFingerprint)
	}
	if node.WSHost != "" {
		query.Set("host", node.WSHost)
	}
	if node.WSPath != "" {
		query.Set("path", node.WSPath)
	}
	if node.GRPCService != "" {
		query.Set("serviceName", node.GRPCService)
	}
}

func shareRemark(inbound inboundRecord, cfg clientConfig) string {
	return strings.TrimSpace(strings.Join(nonEmptyStrings(inbound.view.Remark, cfg.email), "-"))
}

func hostPortForShare(host string, port int) string {
	if strings.Contains(host, ":") && net.ParseIP(host) != nil {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func defaultStringList(values []string, fallback []string) []string {
	if len(values) > 0 {
		return values
	}
	return fallback
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func countOnlineClients(stats map[string]clientStat, reportedAt time.Time) int {
	total := 0
	for _, stat := range stats {
		if isRecentlyOnline(stat.lastOnline, reportedAt) {
			total++
		}
	}
	return total
}

func isRecentlyOnline(lastOnlineMillis int64, reportedAt time.Time) bool {
	if lastOnlineMillis <= 0 {
		return false
	}
	lastOnline := time.UnixMilli(lastOnlineMillis)
	if reportedAt.IsZero() {
		reportedAt = time.Now().UTC()
	}
	return reportedAt.Sub(lastOnline) <= 5*time.Minute
}

func outboundTarget(raw map[string]any) string {
	settings, _ := raw["settings"].(map[string]any)
	if settings == nil {
		return ""
	}

	if target := addressPortFromObject(settings, "address", "port"); target != "" {
		return target
	}
	if target := firstAddressPort(objectList(settings["vnext"]), "address", "port"); target != "" {
		return target
	}
	if target := firstAddressPort(objectList(settings["servers"]), "address", "port"); target != "" {
		return target
	}
	if target := firstAddressPort(objectList(settings["peers"]), "endpoint", ""); target != "" {
		return target
	}
	return ""
}

func parseOutboundEndpoint(raw map[string]any) (string, int) {
	settings, _ := raw["settings"].(map[string]any)
	if settings == nil {
		return "", 0
	}
	if address, port := addressPortPairFromObject(settings, "address", "port"); address != "" {
		return address, port
	}
	if address, port := firstAddressPortPair(objectList(settings["vnext"]), "address", "port"); address != "" {
		return address, port
	}
	if address, port := firstAddressPortPair(objectList(settings["servers"]), "address", "port"); address != "" {
		return address, port
	}
	if address, port := firstAddressPortPair(objectList(settings["peers"]), "endpoint", ""); address != "" {
		return address, port
	}
	return "", 0
}

func authKeysForInbound(protocol string, raw any) []string {
	protocol = normalizedTopologyProtocol(protocol)
	settings := decodeStringObject(raw)
	if len(settings) == 0 || !isSupportedNodeProtocol(protocol) {
		return nil
	}

	keys := make([]string, 0)
	switch protocol {
	case "vless", "vmess":
		for _, client := range objectList(settings["clients"]) {
			keys = append(keys, idAuthKey(protocol, stringValue(client["id"])))
		}
	case "http", "socks":
		for _, account := range objectList(settings["accounts"]) {
			keys = append(keys, userPasswordAuthKey(protocol, stringValue(account["user"]), stringValue(account["pass"])))
		}
	}
	return filterEmpty(uniqueStrings(keys))
}

func authKeysForOutbound(protocol string, raw any) []string {
	protocol = normalizedTopologyProtocol(protocol)
	settings := decodeStringObject(raw)
	if len(settings) == 0 || !isSupportedNodeProtocol(protocol) {
		return nil
	}

	keys := make([]string, 0)
	switch protocol {
	case "vless", "vmess":
		keys = append(keys, idAuthKey(protocol, stringValue(settings["id"])))
		for _, vnext := range objectList(settings["vnext"]) {
			for _, user := range objectList(vnext["users"]) {
				keys = append(keys, idAuthKey(protocol, stringValue(user["id"])))
			}
		}
	case "http", "socks":
		keys = append(keys, userPasswordAuthKey(protocol, stringValue(settings["user"]), stringValue(settings["pass"])))
		for _, server := range objectList(settings["servers"]) {
			keys = append(keys, userPasswordAuthKey(protocol, stringValue(server["user"]), stringValue(server["pass"])))
			for _, user := range objectList(server["users"]) {
				keys = append(keys, userPasswordAuthKey(protocol, stringValue(user["user"]), stringValue(user["pass"])))
			}
		}
	}
	return filterEmpty(uniqueStrings(keys))
}

func idAuthKey(protocol, id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(protocol)) + ":id:" + id
}

func userPasswordAuthKey(protocol, user, password string) string {
	user = strings.TrimSpace(user)
	password = strings.TrimSpace(password)
	if user == "" || password == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(protocol)) + ":userpass:" + user + ":" + password
}

func addressPortFromObject(item map[string]any, addressKey, portKey string) string {
	address, port := addressPortPairFromObject(item, addressKey, portKey)
	if address == "" {
		return ""
	}
	if port == 0 {
		return address
	}
	return fmt.Sprintf("%s:%d", address, port)
}

func isSupportedNodeProtocol(protocol string) bool {
	switch normalizedTopologyProtocol(protocol) {
	case "vless", "vmess", "http", "socks":
		return true
	default:
		return false
	}
}

func normalizedTopologyProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "socks5":
		return "socks"
	default:
		return strings.ToLower(strings.TrimSpace(protocol))
	}
}

func addressPortPairFromObject(item map[string]any, addressKey, portKey string) (string, int) {
	address := stringValue(item[addressKey])
	if address == "" || isPlaceholderEndpointValue(address) {
		return "", 0
	}
	port := intValue(item[portKey])
	return address, port
}

func firstAddressPort(items []map[string]any, addressKey, portKey string) string {
	address, port := firstAddressPortPair(items, addressKey, portKey)
	if address == "" {
		return ""
	}
	if port == 0 {
		return address
	}
	return fmt.Sprintf("%s:%d", address, port)
}

func firstAddressPortPair(items []map[string]any, addressKey, portKey string) (string, int) {
	if len(items) == 0 {
		return "", 0
	}
	address := stringValue(items[0][addressKey])
	if isPlaceholderEndpointValue(address) {
		return "", 0
	}
	if portKey == "" || address == "" {
		return address, 0
	}
	port := intValue(items[0][portKey])
	if port == 0 {
		return address, 0
	}
	return address, port
}

func isPlaceholderEndpointValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "undefined", "null", "nan":
		return true
	default:
		return false
	}
}

func decodeStringObject(raw any) map[string]any {
	switch value := raw.(type) {
	case map[string]any:
		return value
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return nil
		}
		return decoded
	default:
		return nil
	}
}

func objectMap(raw any) map[string]any {
	obj, _ := raw.(map[string]any)
	return obj
}

func objectList(raw any) []map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		return items
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	default:
		return nil
	}
}

func stringList(raw any) []string {
	switch value := raw.(type) {
	case string:
		if value == "" {
			return nil
		}
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	case []string:
		return append([]string(nil), value...)
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if s := stringValue(item); s != "" {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return fmt.Sprintf("%v", zeroIfNil(v))
	}
}

func intValue(v any) int {
	return int(int64Value(v))
}

func int64Value(v any) int64 {
	switch value := v.(type) {
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case uint64:
		return int64(value)
	case float64:
		return int64(value)
	case float32:
		return int64(value)
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}

func boolValue(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(value, "true")
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func cloneInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	return append([]int(nil), values...)
}

func chooseInt64(primary, fallback int64) int64 {
	if primary != 0 {
		return primary
	}
	return fallback
}

func zeroIfNil(v any) any {
	if v == nil {
		return ""
	}
	return v
}
