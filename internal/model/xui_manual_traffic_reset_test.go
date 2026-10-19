package model

import "testing"

func TestManualTrafficResetPayload(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch map[string]any
		valid bool
	}{
		{"valid", nil, true}, {"trim", map[string]any{"email": " alice "}, true},
		{"missing identity", map[string]any{"client_id": ""}, false}, {"missing email", map[string]any{"email": " "}, false},
		{"fractional inbound", map[string]any{"inbound_id": 1.5}, false}, {"negative inbound", map[string]any{"inbound_id": -1}, false},
		{"all clients flag", map[string]any{"all": true}, false}, {"quota mutation", map[string]any{"total_bytes": 0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := map[string]any{"inbound_id": 7, "inbound_tag": "node-7", "email": "alice", "client_id": "stable-id"}
			for k, v := range tc.patch {
				p[k] = v
			}
			target, err := ParseXUIManualTrafficResetPayload(p)
			if (err == nil) != tc.valid {
				t.Fatalf("err=%v valid=%v", err, tc.valid)
			}
			if tc.valid && target.Email != "alice" {
				t.Fatal("email not normalized")
			}
		})
	}
}
