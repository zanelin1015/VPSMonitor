package server

import (
	"regexp"
	"slices"
	"strings"

	"bridge-core/internal/model"
)

var legacyCustomerEntranceCityPattern = regexp.MustCompile(`(?i)^(?:CN[-_\s]+)?([\p{Han}]{2,16})入口(?:$|[-_\s])`)

func customerEntranceCity(agent model.DashboardAgentView, names ...string) string {
	if classification := agent.Entry.Classification; classification != nil {
		if city := strings.TrimSuffix(strings.TrimSpace(classification.City), "市"); city != "" {
			return city
		}
	}
	for _, name := range names {
		match := legacyCustomerEntranceCityPattern.FindStringSubmatch(strings.TrimSpace(name))
		if len(match) == 0 {
			continue
		}
		city := strings.TrimSuffix(match[1], "市")
		switch city {
		case "国内", "中国", "中国大陆", "专线":
			continue
		default:
			return city
		}
	}
	return ""
}

func customerIEPLExitName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, "落地")
	name = strings.TrimSuffix(name, "出口")
	return strings.TrimSpace(name)
}

func customerIEPLName(city, exit string) string {
	return city + "-IEPL-" + exit
}

func groupedCustomerIEPLName(entrances []model.CustomerLinkEntrance) string {
	if len(entrances) == 0 || entrances[0].IEPLExit == "" {
		return ""
	}
	exit := entrances[0].IEPLExit
	cities := make([]string, 0, len(entrances))
	for _, entrance := range entrances {
		if entrance.IEPLCity == "" || entrance.IEPLExit != exit {
			return ""
		}
		if !slices.Contains(cities, entrance.IEPLCity) {
			cities = append(cities, entrance.IEPLCity)
		}
	}
	return customerIEPLName(strings.Join(cities, "/"), exit)
}
