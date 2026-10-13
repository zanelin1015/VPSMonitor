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

func (c *XUIClient) saveClientUpdate(ctx context.Context, target *xuiClientUpdateTarget, field string) (map[string]any, error) {
	inboundID := intValue(target.inbound["id"])
	email := stringValue(target.client["email"])
	path := "/panel/api/clients/update/" + url.PathEscape(email)
	body := target.client
	method := "client_api"
	if !target.clientAPI {
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
	if err := verifyClientUpdate(target.client, actual, field); err != nil {
		return result, err
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
	if !matched {
		return fmt.Errorf("x-ui client update did not persist for %s: %s expected %v, got %v", stringValue(expected["email"]), field, expected[field], value)
	}
	for _, key := range []string{"id", "password", "auth", "subId"} {
		if original := stringValue(expected[key]); original != "" && stringValue(actual[key]) != original {
			return fmt.Errorf("x-ui client %s changed during update; verification failed", key)
		}
	}
	return nil
}
