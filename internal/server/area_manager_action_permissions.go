package server

import (
	"encoding/json"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) customerVisibleToAdmin(user model.AdminUser, customerID int64) (bool, error) {
	if isRootAdmin(user) || isOperatorAdmin(user) {
		_, found, err := a.store.GetCustomer(customerID)
		return found, err
	}
	return a.store.CustomerOwnedBy(customerID, model.AdminRoleAreaManager, user.ID)
}

func (a *App) areaManagerCustomerAssignmentAllowed(user model.AdminUser, req model.CustomerAssignmentRequest) bool {
	if !isAreaManager(user) || strings.TrimSpace(req.AgentID) == "" || req.InboundID <= 0 {
		return false
	}
	if a.areaManagerCustomerAssignmentAgentTargetsForwarding(user, req.AgentID) {
		return false
	}
	clientScope := a.areaManagerAssignmentClientScope(user, req.AgentID)
	if strings.TrimSpace(req.ClientEmail) == "" {
		return clientScope.allowsWholeInbound(req.AgentID, req.InboundID, req.InboundTag)
	}
	if clientScope.allowsClient(req.AgentID, req.InboundID, req.InboundTag, req.ClientEmail) {
		return true
	}
	overview := a.xuiOverviewForOutboundAuthorization(req.AgentID)
	if overview == nil {
		return false
	}
	for _, client := range overview.Clients {
		if client.InboundID != req.InboundID ||
			!strings.EqualFold(strings.TrimSpace(client.InboundTag), strings.TrimSpace(req.InboundTag)) ||
			!strings.EqualFold(strings.TrimSpace(client.Email), strings.TrimSpace(req.ClientEmail)) {
			continue
		}
		return a.areaManagerCanViewForwardedClient(user, req.AgentID, client, clientScope)
	}
	return false
}

func (a *App) areaManagerXUIActionAllowed(user model.AdminUser, agentID string, req model.XUIActionRequest) bool {
	if !a.adminCanAccessAgent(user, agentID) {
		return false
	}
	switch strings.TrimSpace(req.Kind) {
	case model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule:
		return a.areaManagerRoutingPayloadAllowed(user, agentID, req.Payload)
	case model.XUIActionAddOutbound:
		return a.areaManagerCanCreateOutbound(user, agentID, req.Payload)
	case model.XUIActionAddClient:
		return a.areaManagerAddClientPayloadAllowed(user, agentID, req.Payload)
	case model.XUIActionSetClientEnabled, model.XUIActionDeleteClient:
		return a.areaManagerDeleteClientPayloadAllowed(user, agentID, req.Payload)
	default:
		return false
	}
}

func (a *App) areaManagerRoutingPayloadAllowed(user model.AdminUser, agentID string, payload map[string]any) bool {
	if payload == nil {
		return false
	}
	rule, ok := payload["rule"].(map[string]any)
	if !ok || rule == nil || strings.TrimSpace(stringFromAny(rule["balancerTag"])) != "" {
		return false
	}
	outboundTag := strings.TrimSpace(stringFromAny(rule["outboundTag"]))
	if outboundTag == "" {
		return false
	}
	outboundScope := a.areaManagerOutboundScope(user)
	previousTag := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(payload["previous_outbound_tag"]),
		stringFromAny(payload["old_outbound_tag"]),
	))
	if previousTag != "" && !outboundScope.allows(agentID, previousTag) {
		return false
	}
	if createdTag := outboundTagFromPayload(payload); createdTag != "" {
		return createdTag == outboundTag && a.areaManagerCanCreateOutbound(user, agentID, payload)
	}
	return outboundScope.allows(agentID, outboundTag)
}

func outboundTagFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	outbound, ok := payload["outbound"].(map[string]any)
	if !ok || outbound == nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(outbound["tag"]))
}

func (a *App) areaManagerAddClientPayloadAllowed(user model.AdminUser, agentID string, payload map[string]any) bool {
	if payload == nil {
		return false
	}
	inboundID := intFromAny(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromAny(payload["inbound_tag"]))
	if inboundID <= 0 && inboundTag == "" {
		return false
	}
	client, ok := payload["client"].(map[string]any)
	if !ok || client == nil {
		return false
	}
	if strings.TrimSpace(stringFromAny(client["email"])) == "" {
		return false
	}
	return a.areaManagerClientScope(user).allowsManageInbound(agentID, inboundID, inboundTag)
}

func (a *App) areaManagerDeleteClientPayloadAllowed(user model.AdminUser, agentID string, payload map[string]any) bool {
	if payload == nil {
		return false
	}
	inboundID := intFromAny(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromAny(payload["inbound_tag"]))
	if inboundID <= 0 && inboundTag == "" {
		return false
	}
	email := strings.TrimSpace(stringFromAny(payload["email"]))
	clientID := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(payload["client_id"]),
		stringFromAny(payload["client_uuid"]),
		stringFromAny(payload["auth_uuid"]),
	))
	if email == "" && clientID == "" {
		return false
	}
	if email != "" {
		return a.areaManagerClientScope(user).allowsClient(agentID, inboundID, inboundTag, email)
	}
	return a.areaManagerClientScope(user).allowsWholeInbound(agentID, inboundID, inboundTag)
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		number, _ := typed.Int64()
		return int(number)
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
