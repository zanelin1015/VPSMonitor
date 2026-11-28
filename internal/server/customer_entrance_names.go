package server

import (
	"slices"
	"strings"

	"bridge-core/internal/model"
)

func customerIEPLExitName(exit customerExitDetails, landing model.DashboardAgentView, client model.XUIClientView) string {
	if exit.landingAgentID == "" {
		return firstNonEmptyString(exit.name, client.Email, client.Comment, "出口")
	}
	name := firstNonEmptyString(landing.CustomerDisplayName, landing.AgentName, exit.name, exit.landingAgentID)
	name = strings.TrimSuffix(name, "落地")
	name = strings.TrimSuffix(name, "出口")
	name = firstNonEmptyString(name, exit.landingAgentID)
	_, landingCode, landingCountry := customerGeoParts("", landing.Geo)
	_, targetCode, targetCountry := customerGeoParts("", exit.landingTargetGeo)
	country := firstNonEmptyString(exit.classifiedCountry, landingCode, targetCode, landingCountry, targetCountry)
	if country == "" || strings.HasPrefix(strings.ToUpper(name), strings.ToUpper(country)+"-") {
		return name
	}
	return country + "-" + name
}

func customerIEPLName(entry, exit string) string {
	return "IEPL-" + entry + "-" + exit
}

func groupedCustomerIEPLName(entrances []model.CustomerLinkEntrance) string {
	if len(entrances) == 0 || entrances[0].IEPLExit == "" {
		return ""
	}
	exit := entrances[0].IEPLExit
	names := make([]string, 0, len(entrances))
	for _, entrance := range entrances {
		if entrance.IEPLEntry == "" || entrance.IEPLExit != exit {
			return ""
		}
		if !slices.Contains(names, entrance.IEPLEntry) {
			names = append(names, entrance.IEPLEntry)
		}
	}
	return customerIEPLName(strings.Join(names, "/"), exit)
}
