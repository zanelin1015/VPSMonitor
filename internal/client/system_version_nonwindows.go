//go:build !windows

package client

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func detectSystemVersion() string {
	switch runtime.GOOS {
	case "linux":
		return detectLinuxSystemVersion()
	case "darwin":
		return detectDarwinSystemVersion()
	default:
		return humanizeRuntimeOS(runtime.GOOS)
	}
}

func detectLinuxSystemVersion() string {
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if version := readLinuxSystemVersion(path); version != "" {
			return version
		}
	}
	return "Linux"
}

func readLinuxSystemVersion(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rawValue, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[strings.TrimSpace(key)] = parseOSReleaseValue(rawValue)
	}

	name := normalizeLinuxSystemName(values["NAME"])
	versionID := strings.TrimSpace(values["VERSION_ID"])
	prettyName := normalizeLinuxPrettyName(values["PRETTY_NAME"])
	version := strings.TrimSpace(values["VERSION"])

	switch {
	case name != "" && versionID != "":
		return strings.TrimSpace(name + " " + versionID)
	case prettyName != "":
		return prettyName
	case name != "" && version != "":
		return strings.TrimSpace(name + " " + strings.TrimSpace(version))
	case name != "":
		return name
	default:
		return ""
	}
}

func parseOSReleaseValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return strings.TrimSpace(unquoted)
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func normalizeLinuxSystemName(name string) string {
	switch strings.TrimSpace(name) {
	case "Debian GNU/Linux":
		return "Debian"
	case "Alpine Linux":
		return "Alpine"
	case "CentOS Linux":
		return "CentOS"
	case "Oracle Linux Server":
		return "Oracle Linux"
	case "Red Hat Enterprise Linux Server":
		return "Red Hat Enterprise Linux"
	default:
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), " GNU/Linux"))
	}
}

func normalizeLinuxPrettyName(name string) string {
	name = strings.TrimSpace(name)
	switch {
	case strings.HasPrefix(name, "Debian GNU/Linux "):
		return strings.Replace(name, "Debian GNU/Linux", "Debian", 1)
	case strings.HasPrefix(name, "Alpine Linux "):
		return strings.Replace(name, "Alpine Linux", "Alpine", 1)
	case strings.HasPrefix(name, "CentOS Linux "):
		return strings.Replace(name, "CentOS Linux", "CentOS", 1)
	default:
		return name
	}
}

func humanizeRuntimeOS(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Unknown OS"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func detectDarwinSystemVersion() string {
	name, nameErr := exec.Command("sw_vers", "-productName").Output()
	version, versionErr := exec.Command("sw_vers", "-productVersion").Output()
	if nameErr != nil || versionErr != nil {
		return "macOS"
	}
	return strings.TrimSpace(strings.TrimSpace(string(name)) + " " + strings.TrimSpace(string(version)))
}
