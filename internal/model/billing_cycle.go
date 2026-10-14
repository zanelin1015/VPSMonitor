package model

import (
	"fmt"
	"time"
)

var BeijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func BillingCycleMonths(cycle string) (int, error) {
	switch cycle {
	case "month":
		return 1, nil
	case "quarter":
		return 3, nil
	case "semiannual":
		return 6, nil
	case "year":
		return 12, nil
	default:
		return 0, fmt.Errorf("unsupported billing cycle: %s", cycle)
	}
}

// Derive from the original day to avoid permanent end-of-month drift.
func ClientBillingCycleBoundary(start time.Time, months int) time.Time {
	start = start.In(BeijingLocation)
	first := time.Date(start.Year(), start.Month()+time.Month(months), 1, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), BeijingLocation)
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, BeijingLocation).Day()
	day := start.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(first.Year(), first.Month(), day, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), BeijingLocation)
}
