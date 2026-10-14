package model

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var xrayShareVersionPattern = regexp.MustCompile(`(?:^|[^0-9.])v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:$|[^0-9.])`)

// XrayRequiresRealityMLKEM reports whether the serving REALITY core requires
// an X25519MLKEM768 key share. Unknown versions keep the legacy export.
func XrayRequiresRealityMLKEM(version string) bool {
	match := xrayShareVersionPattern.FindStringSubmatch(version)
	if len(match) != 4 {
		return false
	}
	minimum := [3]int{26, 9, 8}
	var parts [3]int
	for i := range minimum {
		part, err := strconv.Atoi(match[i+1])
		if err != nil {
			return false
		}
		parts[i] = part
	}
	for i := range minimum {
		if parts[i] != minimum[i] {
			return parts[i] > minimum[i]
		}
	}
	return true
}

// ShadowrocketImportURL is a client-specific companion to the generic link.
// Shadowrocket's "chrome" profile is legacy; "chrome140" offers ML-KEM.
// Other clients use different profile names, so never replace the generic URL.
func ShadowrocketImportURL(rawURL, xrayVersion string) string {
	if !XrayRequiresRealityMLKEM(xrayVersion) {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "vless" || parsed.User == nil || parsed.Hostname() == "" {
		return ""
	}
	query := parsed.Query()
	if !strings.EqualFold(query.Get("security"), "reality") {
		return ""
	}
	query.Set("fp", "chrome140")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
