package panels

import (
	"encoding/json"
	"fmt"
	"strings"

	"bridge-core/internal/model"
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

func decodeEnvelopeObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func injectLocalCertificate(inbound map[string]any, payload map[string]any, localCertificates []model.XUILocalCertificate) (map[string]any, error) {
	streamSettings, encoded, err := jsonObjectField(inbound["streamSettings"])
	if err != nil {
		return nil, fmt.Errorf("decode inbound streamSettings: %w", err)
	}
	security := strings.ToLower(stringFromMap(streamSettings, "security"))
	if security != "tls" {
		writeJSONField(inbound, "streamSettings", streamSettings, encoded)
		return nil, nil
	}

	selector := objectMap(payload["tls_certificate"])
	if len(selector) == 0 {
		writeJSONField(inbound, "streamSettings", streamSettings, encoded)
		return nil, nil
	}

	certificateFile, keyFile, resolved, err := resolveLocalCertificate(selector, streamSettings, localCertificates)
	if err != nil {
		return nil, err
	}
	if certificateFile == "" || keyFile == "" {
		writeJSONField(inbound, "streamSettings", streamSettings, encoded)
		return nil, nil
	}

	tlsSettings := objectMap(streamSettings["tlsSettings"])
	tlsSettings["certificates"] = []map[string]any{
		{
			"certificateFile": certificateFile,
			"keyFile":         keyFile,
		},
	}
	streamSettings["tlsSettings"] = tlsSettings
	writeJSONField(inbound, "streamSettings", streamSettings, encoded)
	return resolved, nil
}

func jsonObjectField(raw any) (map[string]any, bool, error) {
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return map[string]any{}, true, nil
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return nil, true, err
		}
		return decoded, true, nil
	case map[string]any:
		return value, false, nil
	default:
		return objectMap(raw), false, nil
	}
}

func writeJSONField(target map[string]any, key string, value map[string]any, encoded bool) {
	if !encoded {
		target[key] = value
		return
	}
	body, err := json.Marshal(value)
	if err != nil {
		target[key] = value
		return
	}
	target[key] = string(body)
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

func resolveLocalCertificate(selector map[string]any, streamSettings map[string]any, inventory []model.XUILocalCertificate) (string, string, map[string]any, error) {
	mode := strings.ToLower(stringFromMap(selector, "mode"))
	switch mode {
	case "", "none":
		return "", "", nil, nil
	case "manual":
		certificateFile := stringFromMap(selector, "certificate_file")
		keyFile := stringFromMap(selector, "key_file")
		if certificateFile == "" || keyFile == "" {
			return "", "", nil, fmt.Errorf("manual tls certificate requires certificate_file and key_file")
		}
		return certificateFile, keyFile, map[string]any{
			"mode":             mode,
			"certificate_file": certificateFile,
			"key_file":         keyFile,
		}, nil
	case "inventory":
		inventoryID := stringFromMap(selector, "inventory_id")
		if inventoryID == "" {
			return "", "", nil, fmt.Errorf("inventory tls certificate requires inventory_id")
		}
		for _, cert := range inventory {
			if cert.ID == inventoryID {
				return cert.CertPath, cert.KeyPath, localCertificateResult(mode, cert), nil
			}
		}
		return "", "", nil, fmt.Errorf("local tls certificate not found: %s", inventoryID)
	case "domain_auto":
		serverName := strings.TrimSpace(stringFromMap(selector, "domain"))
		if serverName == "" {
			tlsSettings := objectMap(streamSettings["tlsSettings"])
			serverName = strings.TrimSpace(stringFromMap(tlsSettings, "serverName"))
		}
		if serverName == "" {
			return "", "", nil, fmt.Errorf("auto tls certificate matching requires a server name")
		}
		for _, cert := range inventory {
			if localCertificateMatchesDomain(cert, serverName) {
				return cert.CertPath, cert.KeyPath, localCertificateResult(mode, cert), nil
			}
		}
		return "", "", nil, fmt.Errorf("no local tls certificate matches domain %q", serverName)
	default:
		return "", "", nil, fmt.Errorf("unsupported tls certificate mode: %s", mode)
	}
}

func localCertificateResult(mode string, cert model.XUILocalCertificate) map[string]any {
	return map[string]any{
		"mode":      mode,
		"id":        cert.ID,
		"name":      cert.Name,
		"subject":   cert.Subject,
		"cert_path": cert.CertPath,
		"key_path":  cert.KeyPath,
	}
}

func localCertificateMatchesDomain(cert model.XUILocalCertificate, domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	if matchesCertificatePattern(strings.ToLower(cert.Subject), domain) {
		return true
	}
	for _, name := range cert.DNSNames {
		if matchesCertificatePattern(strings.ToLower(name), domain) {
			return true
		}
	}
	return false
}

func matchesCertificatePattern(pattern, domain string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == domain {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(domain, suffix)
	}
	return false
}
