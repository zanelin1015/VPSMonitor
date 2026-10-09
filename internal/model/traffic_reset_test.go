package model

import (
	"strings"
	"testing"
	"time"
)

func TestTrafficResetPasswordIdentityDoesNotExposePassword(t *testing.T) {
	id := TrafficResetClientIdentity("shadowsocks", "", "secret-password")
	if id == "" || strings.Contains(id, "secret-password") || id != TrafficResetClientIdentity("shadowsocks", "", "secret-password") || id == TrafficResetClientIdentity("shadowsocks", "", "changed-password") {
		t.Fatal("unsafe or unstable password identity")
	}
}

func TestTrafficResetCalendarBoundaries(t *testing.T) {
	parse := func(value string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04:05", value, TrafficResetLocation)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct{ start, cycle, now, last, next string }{
		{"2026-01-31 00:00:00", "month", "2026-03-01 12:00:00", "2026-02-28 00:00:00", "2026-03-31 00:00:00"},
		{"2024-02-29 00:00:00", "year", "2027-03-01 00:00:00", "2027-02-28 00:00:00", "2028-02-29 00:00:00"},
		{"2026-07-02 00:00:00", "semiannual", "2026-10-09 00:00:00", "", "2027-01-02 00:00:00"},
		{"2026-01-02 00:00:00", "quarter", "2026-10-02 00:00:00", "2026-10-02 00:00:00", "2027-01-02 00:00:00"},
		{"2026-01-02 00:00:00", "month", "2026-10-01 23:55:00", "2026-09-02 00:00:00", "2026-10-02 00:00:00"},
	} {
		t.Run(tc.start+tc.cycle+tc.now, func(t *testing.T) {
			last, next, err := TrafficResetBoundaries(parse(tc.start).UnixMilli(), tc.cycle, parse(tc.now))
			if err != nil {
				t.Fatal(err)
			}
			if (tc.last == "" && !last.IsZero()) || (tc.last != "" && !last.Equal(parse(tc.last))) || !next.Equal(parse(tc.next)) {
				t.Fatalf("got %v / %v", last, next)
			}
		})
	}
	if _, _, err := TrafficResetBoundaries(1, "week", time.Now()); err == nil {
		t.Fatal("unsupported cycle accepted")
	}
}
