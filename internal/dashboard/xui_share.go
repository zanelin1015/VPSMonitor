package dashboard

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

func chooseSingleNodeImportHost(entry model.AgentEntryConfig, certificates []model.XUILocalCertificate, snapshot model.AgentSnapshot) string {
	if host := normalizeImportHost(entry.ImportDomain); host != "" {
		return host
	}
	if domain := firstCertificateDomain(certificates); domain != "" {
		return domain
	}
	for _, address := range entry.Addresses {
		if host := normalizeImportHost(address); host != "" {
			return host
		}
	}
	for _, value := range []string{
		snapshot.Summary.PublicIPv4,
		snapshot.Summary.PublicIPv6,
		snapshot.Summary.ObservedIP,
		xuiBaseURLHost(snapshot.XUI),
	} {
		if host := normalizeUsableImportIP(value); host != "" {
			return host
		}
	}
	return ""
}

func filterDomainCertificates(certificates []model.XUILocalCertificate) []model.XUILocalCertificate {
	result := make([]model.XUILocalCertificate, 0, len(certificates))
	for _, cert := range certificates {
		domains := certificateDomains(cert)
		if len(domains) == 0 {
			continue
		}
		cert.DNSNames = domains
		result = append(result, cert)
	}
	return result
}

func firstCertificateDomain(certificates []model.XUILocalCertificate) string {
	for _, cert := range certificates {
		for _, domain := range cert.DNSNames {
			if normalized := normalizeImportDomain(domain); normalized != "" {
				return normalized
			}
		}
		if normalized := normalizeImportDomain(cert.Subject); normalized != "" {
			return normalized
		}
	}
	return ""
}

func certificateDomains(cert model.XUILocalCertificate) []string {
	values := append([]string{}, cert.DNSNames...)
	values = append(values, cert.Subject)
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeCertificateDomain(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizeCertificateDomain(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	if strings.HasPrefix(value, "*.") {
		suffix := strings.TrimPrefix(value, "*.")
		if suffix == "" || strings.Contains(suffix, " ") || net.ParseIP(suffix) != nil {
			return ""
		}
		return "*." + suffix
	}
	return normalizeImportDomain(value)
}

func normalizeImportDomain(value string) string {
	host := normalizeImportHost(value)
	if net.ParseIP(host) != nil {
		return ""
	}
	return host
}

func normalizeImportHost(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if slash := strings.Index(value, "/"); slash >= 0 {
		value = value[:slash]
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	if value == "" || strings.Contains(value, " ") || strings.Contains(value, "*") || strings.Contains(value, ":") {
		return ""
	}
	return value
}

func normalizeUsableImportIP(value string) string {
	host := normalizeImportHost(value)
	ip := net.ParseIP(host)
	if ip == nil || !isUsableImportIP(ip) {
		return ""
	}
	return ip.String()
}

func isUsableImportIP(ip net.IP) bool {
	return ip != nil && !ip.IsUnspecified() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
}

func xuiBaseURLHost(snapshot *model.XUISnapshot) string {
	if snapshot == nil {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(snapshot.BaseURL))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
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
