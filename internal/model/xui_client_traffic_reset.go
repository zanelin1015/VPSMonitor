package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

const XUIClientTrafficResetMinVersion = "0.3.46"

type XUIClientTrafficResetSettings struct {
	Period string
	Day    int
}

func ParseXUIClientTrafficResetPayload(payload map[string]any) (XUIClientTrafficResetSettings, error) {
	settings := XUIClientTrafficResetSettings{Day: 1}
	settings.Period, _ = payload["traffic_reset"].(string)
	settings.Period = strings.TrimSpace(settings.Period)
	switch settings.Period {
	case "never", "hourly", "daily", "weekly", "monthly":
	default:
		return settings, fmt.Errorf("traffic_reset must be never, hourly, daily, weekly or monthly")
	}
	if raw, exists := payload["traffic_reset_day"]; exists {
		var day int
		encoded, err := json.Marshal(raw)
		if err != nil || json.Unmarshal(encoded, &day) != nil || day < 1 || day > 31 {
			return settings, fmt.Errorf("traffic_reset_day must be an integer between 1 and 31")
		}
		settings.Day = day
	}
	return settings, nil
}
