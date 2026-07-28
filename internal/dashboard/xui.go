package dashboard

import (
	"fmt"
	"sort"
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

type XUIOverviewOptions struct {
	Entry model.AgentEntryConfig
}

func BuildXUIOverview(snapshot model.AgentSnapshot) *model.XUIOverview {
	return BuildXUIOverviewWithOptions(snapshot, XUIOverviewOptions{})
}

func BuildXUIOverviewWithOptions(snapshot model.AgentSnapshot, options XUIOverviewOptions) *model.XUIOverview {
	if snapshot.XUI == nil {
		return nil
	}

	rules := normalizeRouteRules(snapshot.XUI.RoutingRules)
	trafficByTag := outboundTrafficByTag(snapshot.XUI.OutboundTraffic)
	outbounds, defaultOutboundTag := normalizeOutbounds(snapshot.XUI.Outbounds, trafficByTag)
	if defaultOutboundTag == "" && len(snapshot.XUI.Inbounds) > 0 {
		traffic := trafficByTag["direct"]
		outbounds = append(outbounds, model.XUIOutboundView{
			Tag:       "direct",
			Protocol:  "freedom",
			Up:        traffic.up,
			Down:      traffic.down,
			Total:     chooseInt64(traffic.total, traffic.up+traffic.down),
			IsDefault: true,
		})
		defaultOutboundTag = "direct"
	}
	balancers := normalizeBalancers(snapshot.XUI.RawConfig, outbounds)
	globalRuleIndexes := collectGlobalRuleIndexes(rules)
	certificates := filterDomainCertificates(snapshot.XUI.Certificates)
	inbounds := normalizeInbounds(snapshot.XUI.Inbounds, rules, defaultOutboundTag, globalRuleIndexes, snapshot, certificates, options.Entry)
	clients, onlineCount := normalizeClients(inbounds, rules, defaultOutboundTag, globalRuleIndexes, snapshot.ReportedAt)
	applyClientTrafficFallbackToOutbounds(outbounds, clients, trafficByTag)

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
		Certificates:      append([]model.XUILocalCertificate{}, certificates...),
	}
}

func applyClientTrafficFallbackToOutbounds(outbounds []model.XUIOutboundView, clients []model.XUIClientView, trafficByTag map[string]outboundTraffic) {
	byTag := make(map[string]outboundTraffic)
	for _, client := range clients {
		tag := strings.TrimSpace(client.Route.OutboundTag)
		if tag == "" || strings.TrimSpace(client.Route.BalancerTag) != "" {
			continue
		}
		traffic := byTag[tag]
		traffic.up += client.Up
		traffic.down += client.Down
		traffic.total += chooseInt64(client.TrafficTotal, client.Up+client.Down)
		byTag[tag] = traffic
	}
	for index := range outbounds {
		if collected, found := trafficByTag[outbounds[index].Tag]; found && (collected.up != 0 || collected.down != 0 || collected.total != 0) {
			continue
		}
		traffic, found := byTag[outbounds[index].Tag]
		if !found {
			continue
		}
		outbounds[index].Up = traffic.up
		outbounds[index].Down = traffic.down
		outbounds[index].Total = chooseInt64(traffic.total, traffic.up+traffic.down)
	}
}

func normalizeInbounds(rawInbounds []map[string]any, rules []routeRule, defaultOutboundTag string, globalRuleIndexes []int, snapshot model.AgentSnapshot, certificates []model.XUILocalCertificate, entry model.AgentEntryConfig) []inboundRecord {
	result := make([]inboundRecord, 0, len(rawInbounds))
	importHost := chooseSingleNodeImportHost(entry, certificates, snapshot)
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
			clients:             parseInboundClients(raw["settings"], stringValue(raw["protocol"])),
			importHost:          importHost,
			vlessEncryption:     protocolMeta.vlessEncryption,
			shadowsocksMethod:   protocolMeta.shadowsocksMethod,
			shadowsocksPassword: protocolMeta.shadowsocksPassword,
			hysteriaVersion:     protocolMeta.hysteriaVersion,
		}
		record.view.ClientCount = len(record.clients)
		record.view.OnlineCount = countOnlineClients(record.clientStats, snapshot.ReportedAt)
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
		tag, direction := outboundTrafficTagAndDirection(item)
		if tag == "" {
			continue
		}
		traffic := result[tag]
		if up := outboundTrafficValue(item, direction, "up"); up != 0 {
			traffic.up += up
		}
		if down := outboundTrafficValue(item, direction, "down"); down != 0 {
			traffic.down += down
		}
		if total := outboundTrafficValue(item, direction, "total"); total != 0 {
			traffic.total += total
		}
		result[tag] = traffic
	}
	return result
}

func outboundTrafficTagAndDirection(item map[string]any) (string, string) {
	tag := firstNonEmptyString(
		stringValue(item["tag"]),
		stringValue(item["outboundTag"]),
		stringValue(item["outbound_tag"]),
		stringValue(item["outbound"]),
	)
	direction := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		stringValue(item["direction"]),
		stringValue(item["type"]),
	)))
	if tag != "" {
		return tag, direction
	}

	name := stringValue(item["name"])
	parts := strings.Split(name, ">>>")
	if len(parts) >= 4 && strings.EqualFold(parts[0], "outbound") && strings.EqualFold(parts[2], "traffic") {
		return strings.TrimSpace(parts[1]), strings.ToLower(strings.TrimSpace(parts[3]))
	}
	return "", direction
}

func outboundTrafficValue(item map[string]any, direction string, field string) int64 {
	switch field {
	case "up":
		value := firstNonZeroInt64(item["up"], item["uplink"], item["upload"], item["sent"], item["tx"])
		if value == 0 && isOutboundUplinkDirection(direction) {
			value = int64Value(item["value"])
		}
		return value
	case "down":
		value := firstNonZeroInt64(item["down"], item["downlink"], item["download"], item["recv"], item["rx"])
		if value == 0 && isOutboundDownlinkDirection(direction) {
			value = int64Value(item["value"])
		}
		return value
	case "total":
		value := firstNonZeroInt64(item["total"], item["allTime"], item["all_time"], item["traffic"])
		if value == 0 && direction == "" {
			value = int64Value(item["value"])
		}
		return value
	default:
		return 0
	}
}

func firstNonZeroInt64(values ...any) int64 {
	for _, value := range values {
		if n := int64Value(value); n != 0 {
			return n
		}
	}
	return 0
}

func isOutboundUplinkDirection(direction string) bool {
	switch direction {
	case "up", "uplink", "upload", "sent", "tx":
		return true
	default:
		return false
	}
}

func isOutboundDownlinkDirection(direction string) bool {
	switch direction {
	case "down", "downlink", "download", "recv", "rx":
		return true
	default:
		return false
	}
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

func parseInboundClients(raw any, protocol string) []clientConfig {
	payload := decodeStringObject(raw)
	if len(payload) == 0 {
		return nil
	}

	protocol = normalizedTopologyProtocol(protocol)
	if protocol == "http" || protocol == "socks" {
		accounts := objectList(payload["accounts"])
		result := make([]clientConfig, 0, len(accounts))
		for _, account := range accounts {
			username := stringValue(account["user"])
			password := stringValue(account["pass"])
			if username == "" || password == "" {
				continue
			}
			result = append(result, clientConfig{
				email:        username,
				enable:       true,
				authUUID:     username,
				authPassword: password,
			})
		}
		return result
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
	case "vless", "vmess", "http", "socks", "shadowsocks", "realm":
		return true
	default:
		return false
	}
}

func normalizedTopologyProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "socks5":
		return "socks"
	case "ss":
		return "shadowsocks"
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
