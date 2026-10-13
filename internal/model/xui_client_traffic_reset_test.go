package model

import "testing"

func TestParseXUIClientTrafficResetPayload(t *testing.T) {
	for _, period := range []string{"never", "hourly", "daily", "weekly", "monthly"} {
		settings, err := ParseXUIClientTrafficResetPayload(map[string]any{"traffic_reset": period})
		if err != nil || settings.Period != period || settings.Day != 1 {
			t.Fatalf("invalid default settings for %s: %+v %v", period, settings, err)
		}
	}
	for _, day := range []any{1, float64(15), 31} {
		if _, err := ParseXUIClientTrafficResetPayload(map[string]any{"traffic_reset": "monthly", "traffic_reset_day": day}); err != nil {
			t.Fatal(err)
		}
	}
	for _, payload := range []map[string]any{
		nil, {"traffic_reset": "month"}, {"traffic_reset": true},
		{"traffic_reset": "monthly", "traffic_reset_day": nil},
		{"traffic_reset": "monthly", "traffic_reset_day": 0},
		{"traffic_reset": "monthly", "traffic_reset_day": 32},
		{"traffic_reset": "monthly", "traffic_reset_day": 1.5},
		{"traffic_reset": "monthly", "traffic_reset_day": "15"},
		{"traffic_reset": "monthly", "traffic_reset_day": false},
	} {
		if _, err := ParseXUIClientTrafficResetPayload(payload); err == nil {
			t.Fatalf("invalid reset settings accepted: %v", payload)
		}
	}
}
