package server

import (
	"strings"

	"bridge-core/internal/model"
)

func renderMihomoSubscription(proxies []mihomoProxy, selectableProxies []mihomoProxy, frontGroups []mihomoProxyGroup) string {
	var proxyYAML strings.Builder
	if len(proxies) == 0 {
		proxyYAML.WriteString("  []")
	} else {
		for _, proxy := range proxies {
			proxy.writeYAML(&proxyYAML)
		}
	}

	proxyNames := renderMihomoProxyNames(selectableProxies)
	proxyNameLists := map[string]string{
		mihomoProxyNameMarker:   proxyNames,
		mihomoHKProxyNameMarker: renderMihomoProxyNames(filterMihomoProxies(selectableProxies, mihomoProxyRegionHK)),
		mihomoJPProxyNameMarker: renderMihomoProxyNames(filterMihomoProxies(selectableProxies, mihomoProxyRegionJP)),
		mihomoUSProxyNameMarker: renderMihomoProxyNames(filterMihomoProxies(selectableProxies, mihomoProxyRegionUS)),
		mihomoSGProxyNameMarker: renderMihomoProxyNames(filterMihomoProxies(selectableProxies, mihomoProxyRegionSG)),
		mihomoTWProxyNameMarker: renderMihomoProxyNames(filterMihomoProxies(selectableProxies, mihomoProxyRegionTW)),
		mihomoKRProxyNameMarker: renderMihomoProxyNames(filterMihomoProxies(selectableProxies, mihomoProxyRegionKR)),
		mihomoNetflixNameMarker: renderMihomoProxyNames(filterMihomoProxies(selectableProxies, mihomoProxyRegionNetflix)),
	}

	var frontGroupYAML strings.Builder
	for _, group := range frontGroups {
		group.writeYAML(&frontGroupYAML)
	}
	content := strings.Replace(mihomoSubscriptionTemplate, mihomoProxyMarker, strings.TrimSuffix(proxyYAML.String(), "\n"), 1)
	content = strings.Replace(content, mihomoFrontProxyGroupMarker, frontGroupYAML.String(), 1)
	for marker, names := range proxyNameLists {
		content = strings.ReplaceAll(content, marker, names)
	}
	return content
}

type mihomoProxyRegion string

const (
	mihomoProxyRegionHK      mihomoProxyRegion = "hk"
	mihomoProxyRegionJP      mihomoProxyRegion = "jp"
	mihomoProxyRegionUS      mihomoProxyRegion = "us"
	mihomoProxyRegionSG      mihomoProxyRegion = "sg"
	mihomoProxyRegionTW      mihomoProxyRegion = "tw"
	mihomoProxyRegionKR      mihomoProxyRegion = "kr"
	mihomoProxyRegionNetflix mihomoProxyRegion = "netflix"
)

func renderMihomoProxyNames(proxies []mihomoProxy) string {
	if len(proxies) == 0 {
		return "      - DIRECT"
	}
	var names strings.Builder
	for index, proxy := range proxies {
		if index > 0 {
			names.WriteByte('\n')
		}
		names.WriteString("      - ")
		names.WriteString(yamlString(proxy.Name))
	}
	return names.String()
}

func filterMihomoProxies(proxies []mihomoProxy, region mihomoProxyRegion) []mihomoProxy {
	filtered := make([]mihomoProxy, 0, len(proxies))
	for _, proxy := range proxies {
		if mihomoProxyMatchesRegion(proxy.Name, region) {
			filtered = append(filtered, proxy)
		}
	}
	return filtered
}

func mihomoProxyMatchesRegion(name string, region mihomoProxyRegion) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	var keywords []string
	switch region {
	case mihomoProxyRegionHK:
		keywords = []string{"港", "hk", "hong kong", "hongkong"}
	case mihomoProxyRegionJP:
		keywords = []string{"日本", "川日", "东京", "大阪", "泉日", "埼玉", "沪日", "深日", "jp", "japan"}
	case mihomoProxyRegionUS:
		keywords = []string{"美", "波特兰", "达拉斯", "俄勒冈", "凤凰城", "费利蒙", "硅谷", "拉斯维加斯", "洛杉矶", "圣何塞", "圣克拉拉", "西雅图", "芝加哥", "us", "united states"}
	case mihomoProxyRegionSG:
		keywords = []string{"新加坡", "坡", "狮城", "sg", "singapore"}
	case mihomoProxyRegionTW:
		keywords = []string{"台", "新北", "彰化", "tw", "taiwan"}
	case mihomoProxyRegionKR:
		keywords = []string{"kr", "korea", "kor", "首尔", "韩", "韓"}
	case mihomoProxyRegionNetflix:
		keywords = []string{"nf", "奈飞", "解锁", "netflix", "media"}
	default:
		return false
	}
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

type mihomoProxy struct {
	Name        string
	DialerProxy string
	Fields      []mihomoField
	Objects     []mihomoObject
}

type mihomoField struct {
	Key   string
	Value any
}

type mihomoObject struct {
	Key    string
	Fields []mihomoField
}

func configureMihomoRealityHandshake(proxy *mihomoProxy, xrayVersion string) {
	// Xray 26.9.8 introduced a REALITY server check requiring the
	// X25519MLKEM768 key share. Mihomo removes it by default for old servers.
	// Keep that default when the serving core is old or unknown.
	if !model.XrayRequiresRealityMLKEM(xrayVersion) {
		return
	}
	for i := range proxy.Objects {
		if proxy.Objects[i].Key != "reality-opts" {
			continue
		}
		proxy.Objects[i].Fields = append(proxy.Objects[i].Fields, mihomoField{"support-x25519mlkem768", true})
		// Chrome's current uTLS fingerprint includes the required key share.
		for j := range proxy.Fields {
			if proxy.Fields[j].Key == "client-fingerprint" {
				proxy.Fields[j].Value = "chrome"
				return
			}
		}
		proxy.Fields = append(proxy.Fields, mihomoField{"client-fingerprint", "chrome"})
		return
	}
}

type mihomoProxyGroup struct {
	Name    string
	Proxies []string
}

func (g mihomoProxyGroup) writeYAML(b *strings.Builder) {
	if strings.TrimSpace(g.Name) == "" || len(g.Proxies) == 0 {
		return
	}
	b.WriteString("  - name: ")
	b.WriteString(yamlString(g.Name))
	b.WriteString("\n")
	b.WriteString("    type: select\n")
	b.WriteString("    proxies:\n")
	for _, proxy := range g.Proxies {
		if strings.TrimSpace(proxy) == "" {
			continue
		}
		b.WriteString("      - ")
		b.WriteString(yamlString(proxy))
		b.WriteString("\n")
	}
}

func (p mihomoProxy) writeYAML(b *strings.Builder) {
	b.WriteString("  - name: ")
	b.WriteString(yamlString(p.Name))
	b.WriteString("\n")
	if p.DialerProxy != "" {
		b.WriteString("    dialer-proxy: ")
		b.WriteString(yamlString(p.DialerProxy))
		b.WriteString("\n")
	}
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
