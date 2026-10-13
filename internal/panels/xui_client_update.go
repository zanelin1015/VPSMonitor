package panels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type xuiClientUpdateTarget struct {
	inbound      map[string]any
	settings     map[string]any
	settingsText bool
	client       map[string]any
	clientIndex  int
	clientAPI    bool
}

func clientUpdateInt64(raw any, field string) (int64, error) {
	value := int64Value(raw)
	valid := true
	switch typed := raw.(type) {
	case int, int64:
	case float64:
		valid = float64(value) == typed
	case json.Number:
		_, err := typed.Int64()
		valid = err == nil
	case string:
		var err error
		value, err = strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		valid = err == nil
	default:
		valid = false
	}
	if !valid {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	return value, nil
}

func clientIDForUpdate(payload map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(payload, "client_id"),
		stringFromMap(payload, "client_uuid"),
		stringFromMap(payload, "auth_uuid"),
	))
}

func (c *XUIClient) loadClientUpdateTarget(ctx context.Context, inboundID int, inboundTag, email, clientID string) (*xuiClientUpdateTarget, error) {
	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, err
	}
	inbound := findInboundForAction(inbounds, inboundID, inboundTag)
	if inbound == nil {
		return nil, fmt.Errorf("inbound not found for client %s", firstNonEmptyString(email, clientID))
	}
	settings, settingsText, err := decodeInboundSettings(inbound["settings"])
	if err != nil {
		return nil, err
	}
	target := &xuiClientUpdateTarget{inbound: inbound, settings: settings, settingsText: settingsText, clientIndex: -1}
	for index, client := range objectSlice(settings["clients"]) {
		if (email != "" && strings.TrimSpace(stringValue(client["email"])) == email) ||
			(email == "" && clientMatchesDeletePayload(client, "", clientID)) {
			target.client = normalizeClientUpdateRecord(client)
			target.clientIndex = index
			if email == "" {
				email = strings.TrimSpace(stringValue(client["email"]))
			}
			break
		}
	}
	if email != "" {
		client, readErr := c.getClientUpdateRecord(ctx, email, intValue(inbound["id"]))
		switch {
		case readErr == nil:
			target.client = client
			target.clientAPI = true
		case isXUIHTTPStatus(readErr, http.StatusNotFound):
			// Older panels have no clients API. Keep their full-inbound update
			// path, which preserves credentials on versions with UUID-reset bugs.
		default:
			return nil, readErr
		}
	}
	if target.client == nil {
		return nil, fmt.Errorf("client not found in inbound: %s", firstNonEmptyString(email, clientID))
	}
	if clientID != "" && !clientMatchesDeletePayload(target.client, "", clientID) {
		return nil, fmt.Errorf("client identity does not match requested client")
	}
	return target, nil
}

func normalizeClientUpdateRecord(raw map[string]any) map[string]any {
	client := normalizeXUIV3ClientForInbound(raw)
	// A v3 ClientRecord has a numeric database id and a separate uuid.
	// The update request's id is the credential, never that database id.
	if _, ok := client["id"].(string); !ok {
		delete(client, "id")
	}
	return client
}

func (c *XUIClient) getClientUpdateRecord(ctx context.Context, email string, inboundID int) (map[string]any, error) {
	response, err := c.getJSONObject(ctx, "/panel/api/clients/get/"+url.PathEscape(email))
	if err != nil {
		return nil, err
	}
	raw := response
	if nested, exists := response["client"]; exists {
		raw = objectMap(nested)
	}
	client := normalizeClientUpdateRecord(raw)
	if tunnelIPs, exists := response["tunnelAllowedIPs"]; exists {
		// Tunnel addresses belong to each inbound, not just the shared row.
		client["allowedIPsByInbound"] = tunnelIPs
	}
	if strings.TrimSpace(stringValue(client["email"])) != email {
		return nil, fmt.Errorf("x-ui client response does not match requested email")
	}
	ids := append(xuiClientInboundIDs(response), xuiClientInboundIDs(raw)...)
	for _, id := range ids {
		if id == inboundID {
			return client, nil
		}
	}
	return nil, fmt.Errorf("client %s is not attached to inbound %d", email, inboundID)
}

// The GET endpoint returns a database ClientRecord. POST accepts the Client
// configuration schema, whose allowedIPs and timestamp fields differ.
func clientUpdateAPIPayload(client map[string]any) (map[string]any, error) {
	payload := make(map[string]any, len(client))
	for key, value := range client {
		payload[key] = value
	}
	copyAlias(payload, "created_at", "createdAt")
	copyAlias(payload, "updated_at", "updatedAt")
	for _, key := range []string{"uuid", "createdAt", "updatedAt"} {
		delete(payload, key)
	}
	if raw, exists := payload["allowedIPs"]; exists {
		ips, err := clientUpdateAllowedIPs(raw)
		if err != nil {
			return nil, err
		}
		payload["allowedIPs"] = ips
	}
	if raw, exists := payload["allowedIPsByInbound"]; exists && raw != nil {
		addresses, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("x-ui client allowedIPsByInbound must be an object")
		}
		byInbound := make(map[string][]string, len(addresses))
		for id, value := range addresses {
			ips, err := clientUpdateAllowedIPs(value)
			if err != nil {
				return nil, fmt.Errorf("x-ui client allowedIPsByInbound for inbound %s: %w", id, err)
			}
			byInbound[id] = ips
		}
		payload["allowedIPsByInbound"] = byInbound
	}
	if raw, ok := payload["reverse"].(string); ok {
		var reverse map[string]any
		if strings.TrimSpace(raw) != "" {
			if err := json.Unmarshal([]byte(raw), &reverse); err != nil {
				return nil, fmt.Errorf("decode x-ui client reverse configuration: %w", err)
			}
		}
		payload["reverse"] = reverse
	}
	return payload, nil
}

func clientUpdateAllowedIPs(raw any) ([]string, error) {
	var values []string
	switch value := raw.(type) {
	case nil:
	case string:
		text := strings.TrimSpace(value)
		if strings.HasPrefix(text, "[") {
			if err := json.Unmarshal([]byte(text), &values); err != nil {
				return nil, fmt.Errorf("decode x-ui client allowedIPs: %w", err)
			}
		} else {
			values = strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
		}
	case []string:
		values = value
	case []any:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("x-ui client allowedIPs must contain only strings")
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("x-ui client allowedIPs must be text or an array of strings")
	}
	ips := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			ips = append(ips, text)
		}
	}
	return ips, nil
}

func (c *XUIClient) saveClientUpdate(ctx context.Context, target *xuiClientUpdateTarget, fields ...string) (map[string]any, error) {
	inboundID := intValue(target.inbound["id"])
	email := stringValue(target.client["email"])
	path := "/panel/api/clients/update/" + url.PathEscape(email)
	body := target.client
	method := "client_api"
	if target.clientAPI {
		var err error
		body, err = clientUpdateAPIPayload(target.client)
		if err != nil {
			return nil, err
		}
	} else {
		clients := objectSlice(target.settings["clients"])
		clients[target.clientIndex] = target.client
		target.settings["clients"] = clients
		if err := encodeInboundSettings(target.inbound, target.settings, target.settingsText); err != nil {
			return nil, err
		}
		path = fmt.Sprintf("/panel/api/inbounds/update/%d", inboundID)
		body = target.inbound
		method = "inbound_api"
	}
	envelope, err := c.postJSON(ctx, path, body)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"message": envelope.Msg, "email": email, "inbound_id": inboundID,
		"update_method": method,
		"verified":      false, "restarted": false,
	}
	var actual map[string]any
	if target.clientAPI {
		actual, err = c.getClientUpdateRecord(ctx, email, inboundID)
	} else {
		actual, err = c.getLegacyClientUpdateRecord(ctx, inboundID, email)
	}
	if err != nil {
		// Return a result after a successful write, so ExecuteAction does not
		// replay it if authentication expires during the verification read.
		return result, fmt.Errorf("x-ui reported success but client verification failed: %w", err)
	}
	for _, field := range fields {
		if err := verifyClientUpdate(target.client, actual, field); err != nil {
			return result, err
		}
	}
	result["verified"] = true
	if enabled, exists := boolPayloadValue(actual, "enable"); exists {
		result["enabled"] = enabled
	}
	return result, nil
}

func (c *XUIClient) getLegacyClientUpdateRecord(ctx context.Context, inboundID int, email string) (map[string]any, error) {
	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, err
	}
	inbound := findInboundForAction(inbounds, inboundID, "")
	if inbound != nil {
		settings, _, err := decodeInboundSettings(inbound["settings"])
		if err != nil {
			return nil, err
		}
		for _, client := range objectSlice(settings["clients"]) {
			if strings.TrimSpace(stringValue(client["email"])) == email {
				return normalizeClientUpdateRecord(client), nil
			}
		}
	}
	return nil, fmt.Errorf("client %s missing from inbound %d after update", email, inboundID)
}

func verifyClientUpdate(expected, actual map[string]any, field string) error {
	value, exists := actual[field]
	actualValue, valueErr := clientUpdateInt64(value, field)
	matched := exists && valueErr == nil && actualValue == int64Value(expected[field])
	if field == "enable" {
		enabled, valid := boolPayloadValue(actual, field)
		matched = valid && enabled == expected[field]
	}
	if field == "trafficReset" {
		period, valid := value.(string)
		matched = exists && valid && period == expected[field]
	}
	if !matched {
		return fmt.Errorf("x-ui client update did not persist for %s: %s expected %v, got %v", stringValue(expected["email"]), field, expected[field], value)
	}
	for _, key := range []string{"id", "password", "auth", "subId", "secret", "privateKey", "publicKey", "preSharedKey"} {
		if original := stringValue(expected[key]); original != "" && stringValue(actual[key]) != original {
			return fmt.Errorf("x-ui client %s changed during update; verification failed", key)
		}
	}
	return nil
}
