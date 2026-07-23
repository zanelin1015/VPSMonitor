package server

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

const (
	mihomoProxyMarker     = "{{VPSMONITOR_PROXIES}}"
	mihomoProxyNameMarker = "{{VPSMONITOR_PROXY_NAMES}}"
)

//go:embed customer_subscription_acl4ssr.yaml.tmpl
var mihomoSubscriptionTemplate string

func (a *App) handleCustomerSubscription(w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if len(parts) != 2 || !isMihomoSubscriptionFile(parts[1]) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	token, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(token) == "" {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	user, found, err := a.store.GetCustomerBySubscriptionToken(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	overview, err := a.customerOverview(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	content := buildMihomoSubscription(user, overview.Links)
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", customerSubscriptionContentDisposition(user.Username))
	_, _ = w.Write([]byte(content))
}

func customerSubscriptionContentDisposition(username string) string {
	title := strings.TrimSpace(username)
	if title == "" {
		title = "vpsmonitor"
	}
	return fmt.Sprintf(
		"inline; filename=%s; filename*=UTF-8''%s",
		safeSubscriptionFilename(title),
		url.PathEscape(title),
	)
}

func isMihomoSubscriptionFile(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mihomo.yaml", "mihomo.yml", "clash.yaml", "clash.yml":
		return true
	default:
		return false
	}
}

func buildMihomoSubscription(user model.CustomerUser, links []model.CustomerLinkView) string {
	names := make(map[string]int, len(links))
	proxies := make([]mihomoProxy, 0, len(links))
	for _, link := range links {
		if !link.Resolved || strings.TrimSpace(link.ImportURL) == "" {
			continue
		}
		name := uniqueSubscriptionName(customerSubscriptionNodeName(link), names)
		proxy, ok := parseMihomoProxy(link.ImportURL, name)
		if !ok {
			continue
		}
		proxies = append(proxies, proxy)
	}
	sort.SliceStable(proxies, func(i, j int) bool {
		return proxies[i].Name < proxies[j].Name
	})

	return renderMihomoSubscription(proxies)
}

func renderMihomoSubscription(proxies []mihomoProxy) string {
	var proxyYAML strings.Builder
	if len(proxies) == 0 {
		proxyYAML.WriteString("  []")
	} else {
		for _, proxy := range proxies {
			proxy.writeYAML(&proxyYAML)
		}
	}

	var proxyNames strings.Builder
	if len(proxies) == 0 {
		proxyNames.WriteString("      - DIRECT")
	} else {
		for index, proxy := range proxies {
			if index > 0 {
				proxyNames.WriteByte('\n')
			}
			proxyNames.WriteString("      - ")
			proxyNames.WriteString(yamlString(proxy.Name))
		}
	}

	content := strings.Replace(mihomoSubscriptionTemplate, mihomoProxyMarker, strings.TrimSuffix(proxyYAML.String(), "\n"), 1)
	return strings.ReplaceAll(content, mihomoProxyNameMarker, proxyNames.String())
}

type mihomoProxy struct {
	Name    string
	Fields  []mihomoField
	Objects []mihomoObject
}

type mihomoField struct {
	Key   string
	Value any
}

type mihomoObject struct {
	Key    string
	Fields []mihomoField
}

func (p mihomoProxy) writeYAML(b *strings.Builder) {
	b.WriteString("  - name: ")
	b.WriteString(yamlString(p.Name))
	b.WriteString("\n")
	for _, field := range p.Fields {
		writeMihomoField(b, "    ", field)
	}
	for _, object := range p.Objects {
		if len(object.Fields) == 0 {
			continue
		}
		b.WriteString("    ")
		b.WriteString(object.Key)
		b.WriteString(":\n")
		for _, field := range object.Fields {
			writeMihomoField(b, "      ", field)
		}
	}
}

func writeMihomoField(b *strings.Builder, indent string, field mihomoField) {
	b.WriteString(indent)
	b.WriteString(field.Key)
	b.WriteString(": ")
	b.WriteString(yamlValue(field.Value))
	b.WriteString("\n")
}

func parseMihomoProxy(rawURL, name string) (mihomoProxy, bool) {
	scheme := strings.ToLower(strings.TrimSpace(strings.SplitN(rawURL, ":", 2)[0]))
	switch scheme {
	case "vless":
		return parseVLESSMihomoProxy(rawURL, name)
	case "vmess":
		return parseVMessMihomoProxy(rawURL, name)
	case "ss":
		return parseSSMihomoProxy(rawURL, name)
	case "trojan":
		return parseTrojanMihomoProxy(rawURL, name)
	case "socks", "socks5":
		return parseSocksMihomoProxy(rawURL, name)
	case "http", "https":
		return parseHTTPMihomoProxy(rawURL, name)
	default:
		return mihomoProxy{}, false
	}
}

func parseVLESSMihomoProxy(rawURL, name string) (mihomoProxy, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User == nil || parsed.Hostname() == "" {
		return mihomoProxy{}, false
	}
	port, ok := parseProxyPort(parsed.Port())
	if !ok {
		return mihomoProxy{}, false
	}
	q := parsed.Query()
	network := firstNonEmptyString(q.Get("type"), q.Get("network"))
	security := strings.ToLower(firstNonEmptyString(q.Get("security"), q.Get("tls")))
	tlsEnabled := security == "tls" || security == "reality"
	fields := []mihomoField{
		{"type", "vless"},
		{"server", parsed.Hostname()},
		{"port", port},
		{"uuid", parsed.User.Username()},
		{"udp", true},
	}
	if tlsEnabled {
		fields = append(fields, mihomoField{"tls", true})
		if serverName := firstNonEmptyString(q.Get("sni"), q.Get("servername"), q.Get("peer")); serverName != "" {
			fields = append(fields, mihomoField{"servername", serverName})
		}
	}
	if shouldSkipCertVerify(q) {
		fields = append(fields, mihomoField{"skip-cert-verify", true})
	}
	if flow := q.Get("flow"); flow != "" {
		fields = append(fields, mihomoField{"flow", flow})
	}
	if fp := q.Get("fp"); fp != "" {
		fields = append(fields, mihomoField{"client-fingerprint", fp})
	}
	if network != "" && network != "tcp" {
		fields = append(fields, mihomoField{"network", network})
	}
	if alpn := splitList(q.Get("alpn")); len(alpn) > 0 {
		fields = append(fields, mihomoField{"alpn", alpn})
	}
	objects := mihomoTransportObjects(network, q)
	if security == "reality" {
		reality := []mihomoField{}
		if publicKey := q.Get("pbk"); publicKey != "" {
			reality = append(reality, mihomoField{"public-key", publicKey})
		}
		if shortID := q.Get("sid"); shortID != "" {
			reality = append(reality, mihomoField{"short-id", shortID})
		}
		if spiderX := firstNonEmptyString(q.Get("spx"), q.Get("spiderX")); spiderX != "" {
			reality = append(reality, mihomoField{"spider-x", spiderX})
		}
		objects = append(objects, mihomoObject{Key: "reality-opts", Fields: reality})
	}
	return mihomoProxy{Name: name, Fields: fields, Objects: objects}, true
}

func parseTrojanMihomoProxy(rawURL, name string) (mihomoProxy, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User == nil || parsed.Hostname() == "" {
		return mihomoProxy{}, false
	}
	port, ok := parseProxyPort(parsed.Port())
	if !ok {
		return mihomoProxy{}, false
	}
	q := parsed.Query()
	network := firstNonEmptyString(q.Get("type"), q.Get("network"))
	fields := []mihomoField{
		{"type", "trojan"},
		{"server", parsed.Hostname()},
		{"port", port},
		{"password", parsed.User.Username()},
		{"udp", true},
	}
	if sni := firstNonEmptyString(q.Get("sni"), q.Get("servername"), q.Get("peer")); sni != "" {
		fields = append(fields, mihomoField{"sni", sni})
	}
	if shouldSkipCertVerify(q) {
		fields = append(fields, mihomoField{"skip-cert-verify", true})
	}
	if network != "" && network != "tcp" {
		fields = append(fields, mihomoField{"network", network})
	}
	return mihomoProxy{Name: name, Fields: fields, Objects: mihomoTransportObjects(network, q)}, true
}

type vmessShare struct {
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port any    `json:"port"`
	ID   string `json:"id"`
	AID  any    `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	ALPN string `json:"alpn"`
	FP   string `json:"fp"`
}

func parseVMessMihomoProxy(rawURL, name string) (mihomoProxy, bool) {
	payload := strings.TrimSpace(strings.TrimPrefix(rawURL, "vmess://"))
	payload = strings.TrimSpace(strings.SplitN(payload, "#", 2)[0])
	data, ok := decodeShareBase64(payload)
	if !ok {
		return mihomoProxy{}, false
	}
	var share vmessShare
	if err := json.Unmarshal(data, &share); err != nil || share.Add == "" || share.ID == "" {
		return mihomoProxy{}, false
	}
	port, ok := parseAnyPort(share.Port)
	if !ok {
		return mihomoProxy{}, false
	}
	aid, _ := parseAnyInt(share.AID)
	cipher := firstNonEmptyString(share.Scy, "auto")
	fields := []mihomoField{
		{"type", "vmess"},
		{"server", share.Add},
		{"port", port},
		{"uuid", share.ID},
		{"alterId", aid},
		{"cipher", cipher},
		{"udp", true},
	}
	tlsEnabled := share.TLS != "" && !strings.EqualFold(share.TLS, "none")
	if tlsEnabled {
		fields = append(fields, mihomoField{"tls", true})
		if serverName := firstNonEmptyString(share.SNI, share.Host); serverName != "" {
			fields = append(fields, mihomoField{"servername", serverName})
		}
	}
	if share.FP != "" {
		fields = append(fields, mihomoField{"client-fingerprint", share.FP})
	}
	if share.Net != "" && share.Net != "tcp" {
		fields = append(fields, mihomoField{"network", share.Net})
	}
	if alpn := splitList(share.ALPN); len(alpn) > 0 {
		fields = append(fields, mihomoField{"alpn", alpn})
	}
	objects := []mihomoObject{}
	switch share.Net {
	case "ws":
		wsFields := []mihomoField{}
		if share.Path != "" {
			wsFields = append(wsFields, mihomoField{"path", share.Path})
		}
		if share.Host != "" {
			wsFields = append(wsFields, mihomoField{"headers", map[string]string{"Host": share.Host}})
		}
		objects = append(objects, mihomoObject{Key: "ws-opts", Fields: wsFields})
	case "grpc":
		if share.Path != "" {
			objects = append(objects, mihomoObject{Key: "grpc-opts", Fields: []mihomoField{{"grpc-service-name", share.Path}}})
		}
	}
	return mihomoProxy{Name: name, Fields: fields, Objects: objects}, true
}

func parseSocksMihomoProxy(rawURL, name string) (mihomoProxy, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return mihomoProxy{}, false
	}
	port, ok := parseProxyPort(parsed.Port())
	if !ok {
		return mihomoProxy{}, false
	}
	fields := []mihomoField{
		{"type", "socks5"},
		{"server", parsed.Hostname()},
		{"port", port},
		{"udp", true},
	}
	if parsed.User != nil {
		if username := parsed.User.Username(); username != "" {
			fields = append(fields, mihomoField{"username", username})
		}
		if password, ok := parsed.User.Password(); ok {
			fields = append(fields, mihomoField{"password", password})
		}
	}
	return mihomoProxy{Name: name, Fields: fields}, true
}

func parseHTTPMihomoProxy(rawURL, name string) (mihomoProxy, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return mihomoProxy{}, false
	}
	port, ok := parseProxyPort(parsed.Port())
	if !ok {
		return mihomoProxy{}, false
	}
	fields := []mihomoField{
		{"type", "http"},
		{"server", parsed.Hostname()},
		{"port", port},
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		fields = append(fields, mihomoField{"tls", true})
	}
	if parsed.User != nil {
		if username := parsed.User.Username(); username != "" {
			fields = append(fields, mihomoField{"username", username})
		}
		if password, ok := parsed.User.Password(); ok {
			fields = append(fields, mihomoField{"password", password})
		}
	}
	return mihomoProxy{Name: name, Fields: fields}, true
}

func parseSSMihomoProxy(rawURL, name string) (mihomoProxy, bool) {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.User != nil && parsed.Hostname() != "" {
		method, password, ok := parseSSUserInfo(parsed.User.String())
		port, portOK := parseProxyPort(parsed.Port())
		if ok && portOK {
			return mihomoProxy{Name: name, Fields: []mihomoField{
				{"type", "ss"},
				{"server", parsed.Hostname()},
				{"port", port},
				{"cipher", method},
				{"password", password},
				{"udp", true},
			}}, true
		}
	}
	decoded, ok := decodeSSWholeURL(rawURL)
	if !ok {
		return mihomoProxy{}, false
	}
	at := strings.LastIndex(decoded, "@")
	if at <= 0 || at >= len(decoded)-1 {
		return mihomoProxy{}, false
	}
	method, password, ok := splitMethodPassword(decoded[:at])
	if !ok {
		return mihomoProxy{}, false
	}
	host, portText, err := net.SplitHostPort(decoded[at+1:])
	if err != nil {
		return mihomoProxy{}, false
	}
	port, ok := parseProxyPort(portText)
	if !ok || host == "" {
		return mihomoProxy{}, false
	}
	return mihomoProxy{Name: name, Fields: []mihomoField{
		{"type", "ss"},
		{"server", host},
		{"port", port},
		{"cipher", method},
		{"password", password},
		{"udp", true},
	}}, true
}

func parseSSUserInfo(raw string) (string, string, bool) {
	value, _ := url.QueryUnescape(raw)
	if method, password, ok := splitMethodPassword(value); ok {
		return method, password, true
	}
	if decoded, ok := decodeShareBase64(value); ok {
		return splitMethodPassword(string(decoded))
	}
	return "", "", false
}

func splitMethodPassword(value string) (string, string, bool) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), parts[1], true
}

func decodeSSWholeURL(rawURL string) (string, bool) {
	value := strings.TrimPrefix(strings.TrimSpace(rawURL), "ss://")
	for _, sep := range []string{"#", "?"} {
		if index := strings.Index(value, sep); index >= 0 {
			value = value[:index]
		}
	}
	decoded, ok := decodeShareBase64(value)
	return string(decoded), ok
}

func mihomoTransportObjects(network string, q url.Values) []mihomoObject {
	switch network {
	case "ws":
		fields := []mihomoField{}
		if path := q.Get("path"); path != "" {
			fields = append(fields, mihomoField{"path", path})
		}
		if host := q.Get("host"); host != "" {
			fields = append(fields, mihomoField{"headers", map[string]string{"Host": host}})
		}
		return []mihomoObject{{Key: "ws-opts", Fields: fields}}
	case "grpc":
		serviceName := firstNonEmptyString(q.Get("serviceName"), q.Get("service_name"), q.Get("path"))
		if serviceName == "" {
			return nil
		}
		return []mihomoObject{{Key: "grpc-opts", Fields: []mihomoField{{"grpc-service-name", serviceName}}}}
	default:
		return nil
	}
}

func customerSubscriptionNodeName(link model.CustomerLinkView) string {
	return firstNonEmptyString(strings.TrimSpace(link.Remark), link.EntryClientName, link.ClientEmail, link.InboundTag, "授权链路")
}

func uniqueSubscriptionName(name string, seen map[string]int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "授权链路"
	}
	count := seen[name]
	seen[name] = count + 1
	if count == 0 {
		return name
	}
	return fmt.Sprintf("%s %d", name, count+1)
}

func yamlValue(value any) string {
	switch typed := value.(type) {
	case string:
		return yamlString(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case []string:
		if len(typed) == 0 {
			return "[]"
		}
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, yamlString(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]string:
		if len(typed) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, yamlString(key)+": "+yamlString(typed[key]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return yamlString(fmt.Sprint(value))
	}
}

func yamlString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return strconv.Quote(value)
	}
	return string(data)
}

func parseProxyPort(value string) (int, bool) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	return port, err == nil && port > 0 && port <= 65535
}

func parseAnyPort(value any) (int, bool) {
	number, ok := parseAnyInt(value)
	return number, ok && number > 0 && number <= 65535
}

func parseAnyInt(value any) (int, bool) {
	switch typed := value.(type) {
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		return number, err == nil
	case float64:
		number := int(typed)
		return number, typed == float64(number)
	case int:
		return typed, true
	case int64:
		return int(typed), typed <= int64(^uint(0)>>1)
	default:
		return 0, false
	}
}

func splitList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func shouldSkipCertVerify(q url.Values) bool {
	for _, key := range []string{"allowInsecure", "allow_insecure", "skip-cert-verify", "skip_cert_verify", "insecure"} {
		if isTruthy(q.Get(key)) {
			return true
		}
	}
	return false
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func decodeShareBase64(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\r", "")
	encodings := []*base64.Encoding{
		base64.RawStdEncoding,
		base64.StdEncoding,
		base64.RawURLEncoding,
		base64.URLEncoding,
	}
	for _, encoding := range encodings {
		if data, err := encoding.DecodeString(value); err == nil {
			return data, true
		}
	}
	padded := value
	if rem := len(padded) % 4; rem != 0 {
		padded += strings.Repeat("=", 4-rem)
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		if data, err := encoding.DecodeString(padded); err == nil {
			return data, true
		}
	}
	return nil, false
}

func safeSubscriptionFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "vpsmonitor"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "vpsmonitor"
	}
	return b.String()
}
