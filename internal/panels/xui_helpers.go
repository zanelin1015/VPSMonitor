package panels

import (
	"encoding/json"
	"fmt"
	"strings"
)

func extractObjectList(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

func extractRoutingRules(raw any) []map[string]any {
	routing, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return extractObjectList(routing["rules"])
}

func payloadObject(payload map[string]any, key string) (map[string]any, error) {
	if payload == nil {
		return nil, fmt.Errorf("%s payload is required", key)
	}
	if raw, ok := payload[key]; ok {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		return obj, nil
	}
	if _, hasRestart := payload["restart"]; hasRestart {
		return nil, fmt.Errorf("%s is required", key)
	}
	return payload, nil
}

func objectMap(raw any) map[string]any {
	obj, ok := raw.(map[string]any)
	if !ok || obj == nil {
		return map[string]any{}
	}
	return obj
}

func objectSlice(raw any) []map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		return items
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if obj, ok := item.(map[string]any); ok {
				result = append(result, obj)
			}
		}
		return result
	default:
		return []map[string]any{}
	}
}

func stringFromMap(obj map[string]any, key string) string {
	return stringValue(obj[key])
}

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

func intValue(raw any) int {
	return int(int64Value(raw))
}

func int64Value(raw any) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}

func validateOutboundConfig(outbound map[string]any) error {
	protocol := strings.ToLower(strings.TrimSpace(stringFromMap(outbound, "protocol")))
	if err := validateOutboundRealitySettings(outbound); err != nil {
		return err
	}
	switch protocol {
	case "vless":
		settings := objectMap(outbound["settings"])
		if validEndpoint(settings, "address", "port") {
			return nil
		}
		for _, item := range objectSlice(settings["vnext"]) {
			if validEndpoint(item, "address", "port") {
				return nil
			}
		}
		return fmt.Errorf("%s outbound requires a valid address and port", protocol)
	case "vmess":
		settings := objectMap(outbound["settings"])
		for _, item := range objectSlice(settings["vnext"]) {
			if validEndpoint(item, "address", "port") {
				return nil
			}
		}
		return fmt.Errorf("%s outbound requires a valid address and port", protocol)
	case "trojan", "shadowsocks", "http", "socks", "socks5":
		settings := objectMap(outbound["settings"])
		for _, item := range objectSlice(settings["servers"]) {
			if validEndpoint(item, "address", "port") {
				return nil
			}
		}
		return fmt.Errorf("%s outbound requires a valid address and port", protocol)
	default:
		return nil
	}
}

func normalizeOutboundForXUI(outbound map[string]any) {
	if strings.ToLower(strings.TrimSpace(stringFromMap(outbound, "protocol"))) != "vless" {
		return
	}
	settings := objectMap(outbound["settings"])
	if validEndpoint(settings, "address", "port") {
		if strings.TrimSpace(stringFromMap(settings, "encryption")) == "" {
			settings["encryption"] = "none"
		}
		outbound["settings"] = settings
		return
	}
	for _, item := range objectSlice(settings["vnext"]) {
		if !validEndpoint(item, "address", "port") {
			continue
		}
		settings["address"] = stringFromMap(item, "address")
		settings["port"] = intValue(item["port"])
		if users := objectSlice(item["users"]); len(users) > 0 {
			user := users[0]
			if id := strings.TrimSpace(stringFromMap(user, "id")); id != "" {
				settings["id"] = id
			}
			if flow := strings.TrimSpace(stringFromMap(user, "flow")); flow != "" {
				settings["flow"] = flow
			}
			if encryption := strings.TrimSpace(stringFromMap(user, "encryption")); encryption != "" {
				settings["encryption"] = encryption
			}
		}
		if strings.TrimSpace(stringFromMap(settings, "encryption")) == "" {
			settings["encryption"] = "none"
		}
		delete(settings, "vnext")
		outbound["settings"] = settings
		return
	}
}

func validateOutboundRealitySettings(outbound map[string]any) error {
	streamSettings := objectMap(outbound["streamSettings"])
	if strings.ToLower(strings.TrimSpace(stringFromMap(streamSettings, "security"))) != "reality" {
		return nil
	}
	realitySettings := objectMap(streamSettings["realitySettings"])
	if isPlaceholderValue(stringFromMap(realitySettings, "serverName")) || strings.TrimSpace(stringFromMap(realitySettings, "serverName")) == "" {
		return fmt.Errorf("reality outbound requires streamSettings.realitySettings.serverName")
	}
	if isPlaceholderValue(stringFromMap(realitySettings, "publicKey")) || strings.TrimSpace(stringFromMap(realitySettings, "publicKey")) == "" {
		return fmt.Errorf("reality outbound requires streamSettings.realitySettings.publicKey")
	}
	return nil
}

func validEndpoint(item map[string]any, addressKey, portKey string) bool {
	address := strings.TrimSpace(stringFromMap(item, addressKey))
	if address == "" || isPlaceholderValue(address) {
		return false
	}
	port := intValue(item[portKey])
	return port > 0 && port <= 65535
}

func isPlaceholderValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "undefined", "null", "nan":
		return true
	default:
		return false
	}
}
