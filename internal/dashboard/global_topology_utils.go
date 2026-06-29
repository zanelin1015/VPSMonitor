package dashboard

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

func uniqueNormalizedDomains(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeHost(value); normalized != "" {
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result
}

func uniqueNormalizedIPs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeIP(value); normalized != "" {
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result
}

func mergeStringSets(sets ...[]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, values := range sets {
		for _, value := range values {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func normalizeHost(value string) string {
	value = extractEndpointHost(value)
	if normalizeIP(value) != "" {
		return ""
	}
	return value
}

func extractEndpointHost(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if strings.Contains(value, "/") {
		value = strings.Split(value, "/")[0]
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	return value
}

func matchesDomainPatterns(domain string, patterns []string) bool {
	for _, pattern := range patterns {
		if domainMatchesPattern(domain, pattern) {
			return true
		}
	}
	return false
}

func domainMatchesPattern(domain string, pattern string) bool {
	if domain == "" || pattern == "" {
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

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return ""
}

func inboundTopologyKey(agentID string, inboundID int, inboundTag string) string {
	return fmt.Sprintf("%s::%d::%s", agentID, inboundID, inboundTag)
}

func outboundTopologyKey(agentID string, outboundTag string) string {
	return agentID + "::" + outboundTag
}

func clientChainKey(agentID string, inboundID int, email string) string {
	return fmt.Sprintf("%s::%d::%s", agentID, inboundID, email)
}

func filterEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}
