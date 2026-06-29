package panels

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (c *XUIClient) ExecuteAction(ctx context.Context, action model.XUIAction) (map[string]any, error) {
	if err := c.ensureActionSession(ctx); err != nil {
		if !actionCanUseLocalXrayFallback(action.Kind) {
			return nil, err
		}
		c.invalidateSession()
	}
	result, err := c.executeActionAuthenticated(ctx, action)
	if err == nil || !isXUIAuthError(err) {
		return result, err
	}
	c.invalidateSession()
	if loginErr := c.login(ctx); loginErr != nil {
		return nil, loginErr
	}
	return c.executeActionAuthenticated(ctx, action)
}

func actionCanUseLocalXrayFallback(kind string) bool {
	switch kind {
	case model.XUIActionAddOutbound, model.XUIActionAddRoutingRule, model.XUIActionUpsertRoutingRule:
		return true
	default:
		return false
	}
}

func (c *XUIClient) executeActionAuthenticated(ctx context.Context, action model.XUIAction) (map[string]any, error) {
	switch action.Kind {
	case model.XUIActionAddClient:
		return c.addClient(ctx, action.Payload)
	case model.XUIActionAddOutbound:
		return c.addOutbound(ctx, action.Payload)
	case model.XUIActionAddRoutingRule:
		return c.addRoutingRule(ctx, action.Payload)
	case model.XUIActionUpsertRoutingRule:
		return c.upsertRoutingRule(ctx, action.Payload)
	case model.XUIActionUpdateClientExpiry:
		return c.updateClientExpiry(ctx, action.Payload)
	case model.XUIActionSetClientEnabled:
		return c.setClientEnabled(ctx, action.Payload)
	case model.XUIActionDeleteClient:
		return c.deleteClient(ctx, action.Payload)
	default:
		return nil, fmt.Errorf("unsupported x-ui action kind: %s", action.Kind)
	}
}

func (c *XUIClient) addInbound(ctx context.Context, payload map[string]any, localCertificates []model.XUILocalCertificate) (map[string]any, error) {
	inbound, err := payloadObject(payload, "inbound")
	if err != nil {
		return nil, err
	}
	inbounds, err := c.loadInboundsForAction(ctx)
	if err != nil {
		return nil, err
	}
	normalizeNewInboundPayloadClients(inbound, collectInboundClientUUIDs(inbounds))
	resolvedCertificate, err := injectLocalCertificate(inbound, payload, localCertificates)
	if err != nil {
		return nil, err
	}
	result, err := c.postJSON(ctx, "/panel/api/inbounds/add", inbound)
	if err != nil {
		return nil, err
	}
	response := map[string]any{
		"message": result.Msg,
		"obj":     decodeEnvelopeObject(result.Obj),
	}
	if resolvedCertificate != nil {
		response["certificate"] = resolvedCertificate
	}
	return response, nil
}

func (c *XUIClient) addClient(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	client, err := payloadObject(payload, "client")
	if err != nil {
		return nil, err
	}
	protocol := strings.TrimSpace(stringFromMap(payload, "protocol"))
	email := strings.TrimSpace(stringFromMap(client, "email"))
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" {
		return nil, fmt.Errorf("client.email is required")
	}

	inbounds, err := c.loadInboundsForAction(ctx)
	if err != nil {
		return nil, err
	}
	inbound := findInboundForAction(inbounds, inboundID, inboundTag)
	if inbound == nil {
		return nil, fmt.Errorf("inbound not found for new client %s", email)
	}
	inboundID = intValue(inbound["id"])
	effectiveProtocol := firstNonEmptyString(protocol, stringValue(inbound["protocol"]))
	ensureNewInboundClientAuth(client, effectiveProtocol, collectInboundClientUUIDs(inbounds))
	if result, err := c.addClientViaAPI(ctx, inboundID, client); err == nil {
		return map[string]any{"message": result.Msg, "email": email, "client_id": clientPrimaryID(client), "inbound_id": inboundID, "restarted": false}, nil
	}
	if result, err := c.addClientViaInboundDirectAPI(ctx, inboundID, effectiveProtocol, client); err == nil {
		return map[string]any{"message": result.Msg, "email": email, "client_id": clientPrimaryID(client), "inbound_id": inboundID, "restarted": false}, nil
	}
	if result, err := c.addClientViaInboundSettingsAPI(ctx, inboundID, effectiveProtocol, client); err == nil {
		return map[string]any{"message": result.Msg, "email": email, "client_id": clientPrimaryID(client), "inbound_id": inboundID, "restarted": false}, nil
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	for _, existing := range clients {
		if strings.EqualFold(strings.TrimSpace(stringValue(existing["email"])), email) {
			return nil, fmt.Errorf("client already exists in inbound: %s", email)
		}
	}
	clientSettings, _ := json.Marshal(map[string]any{"clients": []map[string]any{client}})
	if result, err := c.postJSON(ctx, "/panel/api/inbounds/addClient", map[string]any{
		"id":       inboundID,
		"settings": string(clientSettings),
	}); err == nil {
		return map[string]any{"message": result.Msg, "email": email, "client_id": clientPrimaryID(client), "inbound_id": inboundID, "restarted": false}, nil
	}

	clients = append(clients, client)
	settings["clients"] = clients
	body, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal inbound settings: %w", err)
	}
	if settingsText {
		inbound["settings"] = string(body)
	} else {
		inbound["settings"] = settings
	}
	result, err := c.postJSON(ctx, fmt.Sprintf("/panel/api/inbounds/update/%d", inboundID), inbound)
	if err != nil {
		return nil, err
	}
	return map[string]any{"message": result.Msg, "email": email, "client_id": clientPrimaryID(client), "inbound_id": inboundID, "restarted": false}, nil
}

func (c *XUIClient) addClientViaAPI(ctx context.Context, inboundID int, client map[string]any) (xuiEnvelope, error) {
	return c.postJSON(ctx, "/panel/api/clients/add", map[string]any{
		"client":     client,
		"inboundIds": []int{inboundID},
	})
}

func (c *XUIClient) addClientViaInboundDirectAPI(ctx context.Context, inboundID int, protocol string, client map[string]any) (xuiEnvelope, error) {
	return c.postJSON(ctx, "/panel/api/inbounds/addClient", map[string]any{
		"inboundId": inboundID,
		"client":    client,
	})
}

func (c *XUIClient) addClientViaInboundSettingsAPI(ctx context.Context, inboundID int, protocol string, client map[string]any) (xuiEnvelope, error) {
	clientSettings, _ := json.Marshal(map[string]any{"clients": []map[string]any{client}})
	return c.postJSON(ctx, "/panel/api/inbounds/addClient", map[string]any{
		"id":       inboundID,
		"settings": string(clientSettings),
	})
}

func (c *XUIClient) readLocalInbounds(ctx context.Context) ([]map[string]any, error) {
	dbPath, _, err := c.resolveLocalDBPath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(dbPath))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return readLocalInbounds(ctx, db)
}

func (c *XUIClient) loadInboundsForAction(ctx context.Context) ([]map[string]any, error) {
	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err == nil {
		return inbounds, nil
	}
	localInbounds, localErr := c.readLocalInbounds(ctx)
	if localErr != nil {
		return nil, err
	}
	return localInbounds, nil
}

func findInboundForAction(inbounds []map[string]any, inboundID int, inboundTag string) map[string]any {
	inboundTag = strings.TrimSpace(inboundTag)
	for _, item := range inbounds {
		if inboundID > 0 && intValue(item["id"]) == inboundID {
			return item
		}
		if inboundTag != "" && stringValue(item["tag"]) == inboundTag {
			return item
		}
	}
	for _, item := range inbounds {
		if inboundID > 0 && intValue(item["port"]) == inboundID {
			if inboundTag == "" || stringValue(item["tag"]) == inboundTag {
				return item
			}
		}
	}
	return nil
}

func (c *XUIClient) addOutbound(ctx context.Context, payload map[string]any) (map[string]any, error) {
	outbound, err := payloadObject(payload, "outbound")
	if err != nil {
		return nil, err
	}
	tag := stringFromMap(outbound, "tag")
	if tag == "" {
		return nil, fmt.Errorf("outbound.tag is required")
	}
	if err := validateOutboundConfig(outbound); err != nil {
		return nil, err
	}

	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	configJSON := mutableConfig.config
	previousTag := firstNonEmptyString(
		stringFromMap(payload, "previous_outbound_tag"),
		stringFromMap(payload, "old_outbound_tag"),
	)
	removedPrevious := false
	renamingOutbound := previousTag != "" && tag != "" && previousTag != tag
	if renamingOutbound {
		removedPrevious = removeOutboundByTag(configJSON, previousTag)
	}
	outboundResult, err := upsertOutboundInConfig(configJSON, outbound, !renamingOutbound)
	if err != nil {
		return nil, err
	}
	if previousTag != "" && outboundResult.Tag != "" && previousTag != outboundResult.Tag {
		outboundResult.RemovedPrevious = removedPrevious || removeOutboundByTag(configJSON, previousTag)
		outboundResult.RoutingRefsUpdated = replaceRoutingOutboundTag(configJSON, previousTag, outboundResult.Tag)
	}

	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"outbound_tag":     outboundResult.Tag,
		"outbound_added":   outboundResult.Added,
		"outbound_updated": outboundResult.Updated,
		"outbound_reused":  outboundResult.Reused,
		"outbound_removed": outboundResult.RemovedPrevious,
		"routing_refs":     outboundResult.RoutingRefsUpdated,
		"restarted":        true,
	}, nil
}

func (c *XUIClient) addRoutingRule(ctx context.Context, payload map[string]any) (map[string]any, error) {
	rule, err := payloadObject(payload, "rule")
	if err != nil {
		return nil, err
	}
	if stringFromMap(rule, "type") == "" {
		rule["type"] = "field"
	}
	if stringFromMap(rule, "outboundTag") == "" && stringFromMap(rule, "balancerTag") == "" {
		return nil, fmt.Errorf("rule.outboundTag or rule.balancerTag is required")
	}

	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	configJSON := mutableConfig.config
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	updated := false
	if existingIndex := findEquivalentRoutingRule(rules, rule); existingIndex >= 0 {
		rules[existingIndex] = rule
		rules = moveRoutingRuleToFront(rules, existingIndex)
		updated = true
	} else {
		rules = prependRoutingRule(rules, rule)
	}
	routing["rules"] = rules
	configJSON["routing"] = routing

	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"rule_index": 1,
		"updated":    updated,
		"restarted":  true,
	}, nil
}

func (c *XUIClient) upsertRoutingRule(ctx context.Context, payload map[string]any) (map[string]any, error) {
	rule, err := payloadObject(payload, "rule")
	if err != nil {
		return nil, err
	}
	if stringFromMap(rule, "type") == "" {
		rule["type"] = "field"
	}
	if stringFromMap(rule, "outboundTag") == "" && stringFromMap(rule, "balancerTag") == "" {
		return nil, fmt.Errorf("rule.outboundTag or rule.balancerTag is required")
	}

	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return nil, err
	}
	configJSON := mutableConfig.config

	outboundResult := outboundUpsertResult{}
	if rawOutbound, ok := payload["outbound"]; ok && rawOutbound != nil {
		outbound, ok := rawOutbound.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("outbound must be an object")
		}
		requestedTag := stringFromMap(outbound, "tag")
		previousTag := firstNonEmptyString(
			stringFromMap(payload, "previous_outbound_tag"),
			stringFromMap(payload, "old_outbound_tag"),
		)
		removedPrevious := false
		renamingOutbound := previousTag != "" && requestedTag != "" && previousTag != requestedTag
		if renamingOutbound {
			removedPrevious = removeOutboundByTag(configJSON, previousTag)
		}
		outboundResult, err = upsertOutboundInConfig(configJSON, outbound, !renamingOutbound)
		if err != nil {
			return nil, err
		}
		if requestedTag != "" && outboundResult.Tag != "" && stringFromMap(rule, "outboundTag") == requestedTag {
			rule["outboundTag"] = outboundResult.Tag
		}
		if previousTag != "" && outboundResult.Tag != "" && previousTag != outboundResult.Tag {
			outboundResult.RemovedPrevious = removedPrevious || removeOutboundByTag(configJSON, previousTag)
			outboundResult.RoutingRefsUpdated = replaceRoutingOutboundTag(configJSON, previousTag, outboundResult.Tag)
		}
	}
	if outboundTag := stringFromMap(rule, "outboundTag"); outboundTag != "" {
		found := false
		for _, existing := range objectSlice(configJSON["outbounds"]) {
			if stringFromMap(existing, "tag") == outboundTag {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("outbound tag not found: %s", outboundTag)
		}
	}

	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	ruleIndex := intValue(payload["rule_index"])
	updated := false
	if ruleIndex > 0 {
		if ruleIndex > len(rules) {
			return nil, fmt.Errorf("routing rule index out of range: %d", ruleIndex)
		}
		rules[ruleIndex-1] = rule
		rules = moveRoutingRuleToFront(rules, ruleIndex-1)
		ruleIndex = 1
		updated = true
	} else if existingIndex := findEquivalentRoutingRule(rules, rule); existingIndex >= 0 {
		rules[existingIndex] = rule
		rules = moveRoutingRuleToFront(rules, existingIndex)
		ruleIndex = 1
		updated = true
	} else {
		rules = prependRoutingRule(rules, rule)
		ruleIndex = 1
	}
	routing["rules"] = rules
	configJSON["routing"] = routing

	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return nil, err
	}
	if err := c.restartXrayService(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"rule_index":       ruleIndex,
		"updated":          updated,
		"outbound_added":   outboundResult.Added,
		"outbound_updated": outboundResult.Updated,
		"outbound_reused":  outboundResult.Reused,
		"outbound_removed": outboundResult.RemovedPrevious,
		"routing_refs":     outboundResult.RoutingRefsUpdated,
		"restarted":        true,
	}, nil
}

type outboundUpsertResult struct {
	Tag                string
	Added              bool
	Updated            bool
	Reused             bool
	RoutingRefsUpdated int
	RemovedPrevious    bool
}

func upsertOutboundInConfig(configJSON map[string]any, outbound map[string]any, reuseEquivalent ...bool) (outboundUpsertResult, error) {
	normalizeOutboundForXUI(outbound)
	tag := stringFromMap(outbound, "tag")
	if tag == "" {
		return outboundUpsertResult{}, fmt.Errorf("outbound.tag is required")
	}
	if err := validateOutboundConfig(outbound); err != nil {
		return outboundUpsertResult{}, err
	}

	outbounds := objectSlice(configJSON["outbounds"])
	for index, existing := range outbounds {
		if stringFromMap(existing, "tag") == tag {
			outbounds[index] = outbound
			configJSON["outbounds"] = outbounds
			return outboundUpsertResult{Tag: tag, Updated: true}, nil
		}
	}
	allowEquivalentReuse := true
	if len(reuseEquivalent) > 0 {
		allowEquivalentReuse = reuseEquivalent[0]
	}
	if allowEquivalentReuse {
		if existingIndex := findEquivalentOutbound(outbounds, outbound); existingIndex >= 0 {
			existingTag := stringFromMap(outbounds[existingIndex], "tag")
			if existingTag == "" {
				existingTag = tag
			}
			return outboundUpsertResult{Tag: existingTag, Reused: true}, nil
		}
	}
	configJSON["outbounds"] = append(outbounds, outbound)
	return outboundUpsertResult{Tag: tag, Added: true}, nil
}

func removeOutboundByTag(configJSON map[string]any, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	outbounds := objectSlice(configJSON["outbounds"])
	filtered := make([]map[string]any, 0, len(outbounds))
	removed := false
	for _, outbound := range outbounds {
		if strings.TrimSpace(stringFromMap(outbound, "tag")) == tag {
			removed = true
			continue
		}
		filtered = append(filtered, outbound)
	}
	if removed {
		configJSON["outbounds"] = filtered
	}
	return removed
}

func findEquivalentOutbound(outbounds []map[string]any, outbound map[string]any) int {
	target := canonicalOutbound(outbound)
	if target == "" {
		return -1
	}
	for index, existing := range outbounds {
		if canonicalOutbound(existing) == target {
			return index
		}
	}
	return -1
}

func canonicalOutbound(outbound map[string]any) string {
	normalized := make(map[string]any, len(outbound))
	for key, value := range outbound {
		if key == "tag" {
			continue
		}
		normalized[key] = normalizeXUIConfigValue(value)
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(body)
}

func prependRoutingRule(rules []map[string]any, rule map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(rules)+1)
	result = append(result, rule)
	result = append(result, rules...)
	return result
}

func moveRoutingRuleToFront(rules []map[string]any, index int) []map[string]any {
	if index <= 0 || index >= len(rules) {
		return rules
	}
	rule := rules[index]
	copy(rules[1:index+1], rules[0:index])
	rules[0] = rule
	return rules
}

func findEquivalentRoutingRule(rules []map[string]any, rule map[string]any) int {
	target := canonicalRoutingRule(rule)
	if target == "" {
		return -1
	}
	for index, existing := range rules {
		if canonicalRoutingRule(existing) == target {
			return index
		}
	}
	return -1
}

func canonicalRoutingRule(rule map[string]any) string {
	normalized := normalizeRoutingRuleForCompare(rule)
	body, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(body)
}

func normalizeRoutingRuleForCompare(rule map[string]any) map[string]any {
	normalized := make(map[string]any, len(rule)+1)
	hasType := false
	for key, value := range rule {
		if key == "type" {
			hasType = true
			if strings.TrimSpace(stringValue(value)) == "" {
				normalized[key] = "field"
				continue
			}
		}
		normalized[key] = normalizeRoutingRuleValue(value)
	}
	if !hasType {
		normalized["type"] = "field"
	}
	return normalized
}

func normalizeRoutingRuleValue(value any) any {
	return normalizeXUIConfigValue(value)
}

func normalizeXUIConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = normalizeXUIConfigValue(item)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, normalizeXUIConfigValue(item))
		}
		return result
	case []map[string]any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, normalizeXUIConfigValue(item))
		}
		return result
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return typed
	}
}

func replaceRoutingOutboundTag(configJSON map[string]any, oldTag string, newTag string) int {
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if oldTag == "" || newTag == "" || oldTag == newTag {
		return 0
	}
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	updated := 0
	for _, rule := range rules {
		if strings.TrimSpace(stringFromMap(rule, "outboundTag")) == oldTag {
			rule["outboundTag"] = newTag
			updated++
		}
	}
	if updated > 0 {
		routing["rules"] = rules
		configJSON["routing"] = routing
	}
	return updated
}

func replaceRoutingUser(configJSON map[string]any, oldEmail string, newEmail string) int {
	oldEmail = strings.TrimSpace(oldEmail)
	newEmail = strings.TrimSpace(newEmail)
	if oldEmail == "" || newEmail == "" || oldEmail == newEmail {
		return 0
	}
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	updated := 0
	for _, rule := range rules {
		users := stringListValue(rule["user"])
		if len(users) == 0 {
			continue
		}
		changed := false
		for index, user := range users {
			if strings.TrimSpace(user) == oldEmail {
				users[index] = newEmail
				changed = true
			}
		}
		if changed {
			rule["user"] = users
			updated++
		}
	}
	if updated > 0 {
		routing["rules"] = rules
		configJSON["routing"] = routing
	}
	return updated
}

func removeRoutingUser(configJSON map[string]any, email string) int {
	email = strings.TrimSpace(email)
	if email == "" {
		return 0
	}
	routing := objectMap(configJSON["routing"])
	rules := objectSlice(routing["rules"])
	if len(rules) == 0 {
		return 0
	}
	filtered := make([]map[string]any, 0, len(rules))
	updated := 0
	for _, rule := range rules {
		users := stringListValue(rule["user"])
		if len(users) == 0 {
			filtered = append(filtered, rule)
			continue
		}
		remaining := make([]string, 0, len(users))
		removed := false
		for _, user := range users {
			if strings.TrimSpace(user) == email {
				removed = true
				continue
			}
			remaining = append(remaining, user)
		}
		if !removed {
			filtered = append(filtered, rule)
			continue
		}
		updated++
		if len(remaining) > 0 {
			rule["user"] = remaining
			filtered = append(filtered, rule)
			continue
		}
		delete(rule, "user")
		if routingRuleHasMatcher(rule) {
			filtered = append(filtered, rule)
		}
	}
	if updated > 0 {
		routing["rules"] = filtered
		configJSON["routing"] = routing
	}
	return updated
}

func routingRuleHasMatcher(rule map[string]any) bool {
	for _, key := range []string{"inboundTag", "domain", "ip", "port", "sourcePort", "sourceIP", "network", "protocol"} {
		if len(stringListValue(rule[key])) > 0 || strings.TrimSpace(stringValue(rule[key])) != "" {
			return true
		}
	}
	return false
}

func stringListValue(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	default:
		return nil
	}
}

func (c *XUIClient) updateClientExpiry(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	email := strings.TrimSpace(stringFromMap(payload, "email"))
	previousEmail := firstNonEmptyString(
		stringFromMap(payload, "previous_email"),
		stringFromMap(payload, "old_email"),
	)
	lookupEmail := firstNonEmptyString(previousEmail, email)
	expiryTime := int64Value(payload["expiry_time"])
	enabled, hasEnabled := boolPayloadValue(payload, "enabled")
	if value, ok := boolPayloadValue(payload, "enable"); ok {
		enabled = value
		hasEnabled = true
	}
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if expiryTime <= 0 {
		return nil, fmt.Errorf("expiry_time is required")
	}
	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, err
	}
	var inbound map[string]any
	for _, item := range inbounds {
		if inboundID > 0 && intValue(item["id"]) == inboundID {
			inbound = item
			break
		}
		if inboundTag != "" && stringValue(item["tag"]) == inboundTag {
			inbound = item
			break
		}
	}
	if inbound == nil {
		return nil, fmt.Errorf("inbound not found for client %s", email)
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	var updatedClient map[string]any
	for _, client := range clients {
		if strings.TrimSpace(stringValue(client["email"])) == lookupEmail {
			client["expiryTime"] = expiryTime
			if hasEnabled {
				client["enable"] = enabled
			}
			if previousEmail != "" && previousEmail != email {
				client["email"] = email
			}
			updatedClient = client
			break
		}
	}
	if updatedClient == nil {
		return nil, fmt.Errorf("client not found in inbound: %s", email)
	}

	inboundID = intValue(inbound["id"])
	settings["clients"] = clients
	body, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal inbound settings: %w", err)
	}
	if settingsText {
		inbound["settings"] = string(body)
	} else {
		inbound["settings"] = settings
	}
	result, err := c.postJSON(ctx, fmt.Sprintf("/panel/api/inbounds/update/%d", inboundID), inbound)
	if err != nil {
		return nil, err
	}
	routingRefsUpdated, err := c.replaceRoutingUserReferences(ctx, previousEmail, email)
	if err != nil {
		return nil, err
	}
	return map[string]any{"message": result.Msg, "email": email, "expiry_time": expiryTime, "enabled": updatedClient["enable"], "routing_refs": routingRefsUpdated, "restarted": false}, nil
}

func (c *XUIClient) setClientEnabled(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	email := strings.TrimSpace(stringFromMap(payload, "email"))
	clientID := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(payload, "client_id"),
		stringFromMap(payload, "client_uuid"),
		stringFromMap(payload, "auth_uuid"),
	))
	enabled, hasEnabled := boolPayloadValue(payload, "enabled")
	if value, ok := boolPayloadValue(payload, "enable"); ok {
		enabled = value
		hasEnabled = true
	}
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" && clientID == "" {
		return nil, fmt.Errorf("email or client_id is required")
	}
	if !hasEnabled {
		return nil, fmt.Errorf("enabled is required")
	}
	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, err
	}
	var inbound map[string]any
	for _, item := range inbounds {
		if inboundID > 0 && intValue(item["id"]) == inboundID {
			inbound = item
			break
		}
		if inboundTag != "" && stringValue(item["tag"]) == inboundTag {
			inbound = item
			break
		}
	}
	if inbound == nil {
		return nil, fmt.Errorf("inbound not found for client %s", firstNonEmptyString(email, clientID))
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	var updatedClient map[string]any
	for _, client := range clients {
		if clientMatchesDeletePayload(client, email, clientID) {
			client["enable"] = enabled
			if email == "" {
				email = strings.TrimSpace(stringValue(client["email"]))
			}
			if clientID == "" {
				clientID = clientPrimaryID(client)
			}
			updatedClient = client
			break
		}
	}
	if updatedClient == nil {
		return nil, fmt.Errorf("client not found in inbound: %s", firstNonEmptyString(email, clientID))
	}

	inboundID = intValue(inbound["id"])
	settings["clients"] = clients
	body, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal inbound settings: %w", err)
	}
	if settingsText {
		inbound["settings"] = string(body)
	} else {
		inbound["settings"] = settings
	}
	result, err := c.postJSON(ctx, fmt.Sprintf("/panel/api/inbounds/update/%d", inboundID), inbound)
	if err != nil {
		return nil, err
	}
	return map[string]any{"message": result.Msg, "email": email, "client_id": clientID, "inbound_id": inboundID, "enabled": updatedClient["enable"], "restarted": false}, nil
}

func boolPayloadValue(payload map[string]any, key string) (bool, bool) {
	value, ok := payload[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on", "enable", "enabled":
			return true, true
		case "0", "false", "no", "off", "disable", "disabled":
			return false, true
		default:
			return false, false
		}
	case int:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case float64:
		return typed != 0, true
	default:
		return false, false
	}
}

func (c *XUIClient) deleteClient(ctx context.Context, payload map[string]any) (map[string]any, error) {
	inboundID := intValue(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromMap(payload, "inbound_tag"))
	email := strings.TrimSpace(stringFromMap(payload, "email"))
	clientID := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(payload, "client_id"),
		stringFromMap(payload, "client_uuid"),
		stringFromMap(payload, "auth_uuid"),
	))
	if inboundID <= 0 && inboundTag == "" {
		return nil, fmt.Errorf("inbound_id or inbound_tag is required")
	}
	if email == "" && clientID == "" {
		return nil, fmt.Errorf("email or client_id is required")
	}
	if email != "" {
		routingRefsUpdated, err := c.removeRoutingUserReferences(ctx, email)
		if err != nil {
			return nil, err
		}
		if result, err := c.postJSON(ctx, "/panel/api/clients/del/"+url.PathEscape(email), map[string]any{}); err == nil {
			return map[string]any{"message": result.Msg, "email": email, "client_id": clientID, "inbound_id": inboundID, "routing_refs": routingRefsUpdated}, nil
		}
	}

	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, err
	}
	var inbound map[string]any
	for _, item := range inbounds {
		if inboundID > 0 && intValue(item["id"]) == inboundID {
			inbound = item
			break
		}
		if inboundTag != "" && stringValue(item["tag"]) == inboundTag {
			inbound = item
			break
		}
	}
	if inbound == nil {
		return nil, fmt.Errorf("inbound not found for client %s", firstNonEmptyString(email, clientID))
	}

	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	clients := objectSlice(settings["clients"])
	removedIndex := -1
	removedClientID := clientID
	for index, client := range clients {
		if clientMatchesDeletePayload(client, email, clientID) {
			removedIndex = index
			if email == "" {
				email = strings.TrimSpace(stringValue(client["email"]))
			}
			if removedClientID == "" {
				removedClientID = firstNonEmptyString(
					stringValue(client["id"]),
					stringValue(client["password"]),
					stringValue(client["email"]),
				)
			}
			break
		}
	}
	if removedIndex < 0 {
		return nil, fmt.Errorf("client not found in inbound: %s", firstNonEmptyString(email, clientID))
	}

	inboundID = intValue(inbound["id"])
	routingRefsUpdated, err := c.removeRoutingUserReferences(ctx, email)
	if err != nil {
		return nil, err
	}
	if removedClientID != "" {
		path := fmt.Sprintf("/panel/api/inbounds/%d/delClient/%s", inboundID, url.PathEscape(removedClientID))
		if result, err := c.postJSON(ctx, path, map[string]any{}); err == nil {
			return map[string]any{"message": result.Msg, "email": email, "client_id": removedClientID, "inbound_id": inboundID, "routing_refs": routingRefsUpdated, "restarted": false}, nil
		}
	}

	clients = append(clients[:removedIndex], clients[removedIndex+1:]...)
	settings["clients"] = clients
	body, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal inbound settings: %w", err)
	}
	if settingsText {
		inbound["settings"] = string(body)
	} else {
		inbound["settings"] = settings
	}
	result, err := c.postJSON(ctx, fmt.Sprintf("/panel/api/inbounds/update/%d", inboundID), inbound)
	if err != nil {
		return nil, err
	}
	return map[string]any{"message": result.Msg, "email": email, "client_id": removedClientID, "inbound_id": inboundID, "routing_refs": routingRefsUpdated, "restarted": false}, nil
}

func (c *XUIClient) removeRoutingUserReferences(ctx context.Context, email string) (int, error) {
	if strings.TrimSpace(email) == "" {
		return 0, nil
	}
	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return 0, err
	}
	updated := removeRoutingUser(mutableConfig.config, email)
	if updated == 0 {
		return 0, nil
	}
	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return 0, err
	}
	return updated, nil
}

func (c *XUIClient) replaceRoutingUserReferences(ctx context.Context, oldEmail string, newEmail string) (int, error) {
	if strings.TrimSpace(oldEmail) == "" || strings.TrimSpace(newEmail) == "" || strings.TrimSpace(oldEmail) == strings.TrimSpace(newEmail) {
		return 0, nil
	}
	mutableConfig, err := c.getMutableXrayConfig(ctx)
	if err != nil {
		return 0, err
	}
	updated := replaceRoutingUser(mutableConfig.config, oldEmail, newEmail)
	if updated == 0 {
		return 0, nil
	}
	if err := c.updateMutableXrayConfig(ctx, mutableConfig); err != nil {
		return 0, err
	}
	return updated, nil
}

func clientMatchesDeletePayload(client map[string]any, email string, clientID string) bool {
	if email != "" && strings.TrimSpace(stringValue(client["email"])) == email {
		return true
	}
	if clientID == "" {
		return false
	}
	for _, key := range []string{"id", "password", "email"} {
		if strings.TrimSpace(stringValue(client[key])) == clientID {
			return true
		}
	}
	return false
}

func ensureNewInboundClientAuth(client map[string]any, protocol string, usedUUIDs map[string]struct{}) {
	if _, ok := client["enable"]; !ok {
		client["enable"] = true
	}
	if strings.TrimSpace(stringValue(client["subId"])) == "" {
		client["subId"] = randomHexString(8)
	}
	if strings.EqualFold(protocol, "trojan") {
		if strings.TrimSpace(stringValue(client["password"])) == "" {
			client["password"] = randomHexString(16)
		}
		return
	}
	client["id"] = generateUniqueInboundClientUUID(usedUUIDs)
}

func normalizeNewInboundPayloadClients(inbound map[string]any, usedUUIDs map[string]struct{}) {
	protocol := strings.TrimSpace(stringValue(inbound["protocol"]))
	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return
	}
	clients := objectSlice(settings["clients"])
	if len(clients) == 0 {
		return
	}
	for _, client := range clients {
		ensureNewInboundClientAuth(client, protocol, usedUUIDs)
	}
	settings["clients"] = clients
	if settingsText {
		if data, err := json.Marshal(settings); err == nil {
			inbound["settings"] = string(data)
		}
		return
	}
	inbound["settings"] = settings
}

func shouldGenerateInboundClientUUID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || id == "00000000-0000-0000-0000-000000000000" {
		return true
	}
	if strings.HasPrefix(id, "00000000-0000-0000-0000-") {
		suffix := strings.TrimPrefix(id, "00000000-0000-0000-0000-")
		if len(suffix) == 12 {
			allDigits := true
			for _, ch := range suffix {
				if ch < '0' || ch > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return true
			}
		}
	}
	return false
}

func collectInboundClientUUIDs(inbounds []map[string]any) map[string]struct{} {
	used := make(map[string]struct{})
	for _, inbound := range inbounds {
		settings, _, err := decodeInboundSettings(inbound["settings"])
		if err != nil {
			continue
		}
		for _, client := range objectSlice(settings["clients"]) {
			for _, key := range []string{"id", "uuid"} {
				id := strings.ToLower(strings.TrimSpace(stringValue(client[key])))
				if id != "" {
					used[id] = struct{}{}
				}
			}
		}
	}
	return used
}

func generateUniqueInboundClientUUID(used map[string]struct{}) string {
	if used == nil {
		used = make(map[string]struct{})
	}
	for {
		id := randomUUIDString()
		key := strings.ToLower(id)
		if _, exists := used[key]; exists {
			continue
		}
		used[key] = struct{}{}
		return id
	}
}

func clientPrimaryID(client map[string]any) string {
	return firstNonEmptyString(
		stringValue(client["id"]),
		stringValue(client["password"]),
		stringValue(client["email"]),
	)
}

func randomUUIDString() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

func randomHexString(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buf)
}
