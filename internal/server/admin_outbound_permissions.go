package server

import (
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

type areaManagerOutboundSource struct {
	AgentID     string
	InboundID   int
	InboundTag  string
	ClientEmail string
}

type outboundEndpoint struct {
	Protocol string
	Address  string
	Port     int
	Username string
	Password string
	Method   string
}

func (a *App) areaManagerCanCreateOutbound(user model.AdminUser, agentID string, payload map[string]any) bool {
	outboundTag := outboundTagFromPayload(payload)
	if !user.OutboundCreateEnabled || outboundTag == "" || !a.xuiOutboundTagCanBeCreated(agentID, outboundTag) {
		return false
	}
	return a.areaManagerOutboundSourceAllowed(user, payload)
}

func (a *App) areaManagerOutboundSourceAllowed(user model.AdminUser, payload map[string]any) bool {
	source, ok := parseAreaManagerOutboundSource(payload)
	if !ok {
		return false
	}
	clientScope := a.areaManagerClientScope(user)
	if _, ok := clientScope.agents[source.AgentID]; !ok {
		return false
	}
	overview := a.xuiOverviewForOutboundAuthorization(source.AgentID)
	if overview == nil {
		return false
	}
	var sourceClient *model.XUIClientView
	for index := range overview.Clients {
		client := &overview.Clients[index]
		if client.InboundID == source.InboundID &&
			strings.EqualFold(strings.TrimSpace(client.InboundTag), source.InboundTag) &&
			strings.EqualFold(strings.TrimSpace(client.Email), source.ClientEmail) {
			sourceClient = client
			break
		}
	}
	if sourceClient == nil {
		return false
	}
	if !clientScope.allowsClient(source.AgentID, source.InboundID, source.InboundTag, source.ClientEmail) &&
		!a.areaManagerCanViewForwardedClient(user, source.AgentID, *sourceClient, clientScope) {
		return false
	}
	var sourceNode *model.XUINodeView
	for index := range overview.Nodes {
		node := &overview.Nodes[index]
		if node.ID == source.InboundID && strings.EqualFold(strings.TrimSpace(node.Tag), source.InboundTag) {
			sourceNode = node
			break
		}
	}
	if sourceNode == nil {
		return false
	}
	outbound, ok := payload["outbound"].(map[string]any)
	if !ok {
		return false
	}
	requested, ok := outboundEndpointFromConfig(outbound)
	if !ok {
		return false
	}
	expected, ok := outboundEndpointFromImportURL(sourceClient.ImportURL)
	if !ok {
		return false
	}
	return outboundEndpointsMatch(requested, expected, sourceNode.Protocol)
}

func parseAreaManagerOutboundSource(payload map[string]any) (areaManagerOutboundSource, bool) {
	if payload == nil {
		return areaManagerOutboundSource{}, false
	}
	raw, ok := payload["outbound_source"].(map[string]any)
	if !ok || strings.TrimSpace(stringFromAny(raw["type"])) != "authorized_client_node" {
		return areaManagerOutboundSource{}, false
	}
	source := areaManagerOutboundSource{
		AgentID:     strings.TrimSpace(stringFromAny(raw["agent_id"])),
		InboundID:   intFromAny(raw["inbound_id"]),
		InboundTag:  strings.TrimSpace(stringFromAny(raw["inbound_tag"])),
		ClientEmail: strings.TrimSpace(stringFromAny(raw["client_email"])),
	}
	return source, source.AgentID != "" && source.InboundID > 0 && source.ClientEmail != ""
}

func (a *App) xuiOverviewForOutboundAuthorization(agentID string) *model.XUIOverview {
	if a == nil || a.store == nil {
		return nil
	}
	snapshot, ok := a.store.GetLatest(strings.TrimSpace(agentID))
	if !ok {
		return nil
	}
	cfg, _, err := a.store.GetAgentConfig(agentID)
	if err != nil {
		return nil
	}
	overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{Entry: cfg.Entry})
	if overview == nil {
		overview = emptyAgentXUIOverview(snapshot, cfg)
	}
	a.appendForwardedImportURLs(agentID, overview)
	return overview
}

func (a *App) xuiOutboundTagCanBeCreated(agentID, outboundTag string) bool {
	if a == nil || a.store == nil {
		return false
	}
	snapshot, ok := a.store.GetLatest(strings.TrimSpace(agentID))
	if !ok || snapshot.XUI == nil {
		return false
	}
	overview := dashboard.BuildXUIOverview(snapshot)
	if overview == nil {
		return false
	}
	for _, outbound := range overview.Outbounds {
		if strings.TrimSpace(outbound.Tag) == strings.TrimSpace(outboundTag) {
			return false
		}
	}
	return true
}

func outboundEndpointFromImportURL(rawURL string) (outboundEndpoint, bool) {
	proxy, ok := parseMihomoProxy(strings.TrimSpace(rawURL), "source")
	if !ok {
		return outboundEndpoint{}, false
	}
	endpoint := outboundEndpoint{
		Protocol: normalizeAuthorizedOutboundProtocol(mihomoFieldString(proxy.Fields, "type")),
		Address:  strings.TrimSpace(mihomoFieldString(proxy.Fields, "server")),
		Port:     intFromAny(outboundMihomoFieldValue(proxy.Fields, "port")),
		Username: firstNonEmptyString(mihomoFieldString(proxy.Fields, "uuid"), mihomoFieldString(proxy.Fields, "username")),
		Password: mihomoFieldString(proxy.Fields, "password"),
		Method:   mihomoFieldString(proxy.Fields, "cipher"),
	}
	return endpoint, endpoint.Protocol != "" && endpoint.Address != "" && endpoint.Port > 0
}

func outboundEndpointFromConfig(outbound map[string]any) (outboundEndpoint, bool) {
	endpoint := outboundEndpoint{Protocol: normalizeAuthorizedOutboundProtocol(stringFromAny(outbound["protocol"]))}
	settings, ok := outbound["settings"].(map[string]any)
	if !ok {
		return outboundEndpoint{}, false
	}
	switch endpoint.Protocol {
	case "vless":
		endpoint.Address = stringFromAny(settings["address"])
		endpoint.Port = intFromAny(settings["port"])
		endpoint.Username = stringFromAny(settings["id"])
	case "vmess":
		server := firstObjectFromAny(settings["vnext"])
		user := firstObjectFromAny(server["users"])
		endpoint.Address = stringFromAny(server["address"])
		endpoint.Port = intFromAny(server["port"])
		endpoint.Username = stringFromAny(user["id"])
	case "shadowsocks":
		server := firstObjectFromAny(settings["servers"])
		endpoint.Address = stringFromAny(server["address"])
		endpoint.Port = intFromAny(server["port"])
		endpoint.Password = stringFromAny(server["password"])
		endpoint.Method = stringFromAny(server["method"])
	case "http", "socks":
		server := firstObjectFromAny(settings["servers"])
		user := firstObjectFromAny(server["users"])
		endpoint.Address = stringFromAny(server["address"])
		endpoint.Port = intFromAny(server["port"])
		endpoint.Username = stringFromAny(user["user"])
		endpoint.Password = stringFromAny(user["pass"])
	default:
		return outboundEndpoint{}, false
	}
	return endpoint, strings.TrimSpace(endpoint.Address) != "" && endpoint.Port > 0
}

func outboundEndpointsMatch(requested, expected outboundEndpoint, sourceProtocol string) bool {
	sourceProtocol = normalizeAuthorizedOutboundProtocol(sourceProtocol)
	if requested.Protocol != expected.Protocol || requested.Protocol != sourceProtocol ||
		!strings.EqualFold(strings.Trim(strings.TrimSpace(requested.Address), "[]"), strings.Trim(strings.TrimSpace(expected.Address), "[]")) ||
		requested.Port != expected.Port || requested.Username != expected.Username || requested.Password != expected.Password {
		return false
	}
	return requested.Protocol != "shadowsocks" || strings.EqualFold(strings.TrimSpace(requested.Method), strings.TrimSpace(expected.Method))
}

func normalizeAuthorizedOutboundProtocol(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ss":
		return "shadowsocks"
	case "socks5":
		return "socks"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func outboundMihomoFieldValue(fields []mihomoField, key string) any {
	for _, field := range fields {
		if field.Key == key {
			return field.Value
		}
	}
	return nil
}

func mihomoFieldString(fields []mihomoField, key string) string {
	value := outboundMihomoFieldValue(fields, key)
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstObjectFromAny(value any) map[string]any {
	switch items := value.(type) {
	case []any:
		if len(items) > 0 {
			item, _ := items[0].(map[string]any)
			return item
		}
	case []map[string]any:
		if len(items) > 0 {
			return items[0]
		}
	}
	return nil
}
