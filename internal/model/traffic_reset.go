package model

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

var TrafficResetLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Password-based identities must not expose credentials in policy/action logs.
func TrafficResetClientIdentity(protocol, id, password string) string {
	if id != "" {
		return id
	}
	switch strings.ToLower(protocol) {
	case "shadowsocks", "trojan":
		if password != "" {
			return fmt.Sprintf("secret:%x", sha256.Sum256([]byte(protocol+"\x00"+password)))
		}
	}
	return ""
}

type TrafficResetSettings struct {
	Enabled        bool `json:"enabled"`
	CatchUpMinutes int  `json:"catch_up_minutes"`
}
type TrafficResetPolicy struct {
	ID            string `json:"id"`
	AgentID       string `json:"agent_id"`
	InboundID     int    `json:"inbound_id"`
	ClientID      string `json:"client_id"`
	Email         string `json:"email"`
	Enabled       bool   `json:"enabled"`
	FollowBilling bool   `json:"follow_billing"`
	StartTime     int64  `json:"start_time"`
	Cycle         string `json:"cycle"`
	UpdatedAt     string `json:"updated_at"`
}
type TrafficResetJob struct {
	ID         string         `json:"id"`
	PolicyID   string         `json:"policy_id"`
	AgentID    string         `json:"agent_id"`
	AgentName  string         `json:"agent_name,omitempty"`
	ClientName string         `json:"client_name,omitempty"`
	InboundID  int            `json:"inbound_id"`
	ClientID   string         `json:"client_id"`
	Email      string         `json:"email"`
	Boundary   int64          `json:"boundary"`
	RunAt      int64          `json:"run_at"`
	Deadline   int64          `json:"deadline"`
	Signature  string         `json:"signature"`
	Status     string         `json:"status"`
	ActionID   int64          `json:"action_id"`
	Message    string         `json:"message"`
	Result     map[string]any `json:"result,omitempty"`
	UpdatedAt  string         `json:"updated_at"`
}

// Audit entries are append-only, separate from the mutable period job state.
type TrafficResetLog struct {
	ID                  int64  `json:"id"`
	JobID               string `json:"job_id"`
	ActionID            int64  `json:"action_id"`
	AgentID             string `json:"agent_id"`
	AgentName           string `json:"agent_name"`
	ClientID            string `json:"client_id"`
	ClientName          string `json:"client_name"`
	Email               string `json:"email"`
	InboundID           int    `json:"inbound_id"`
	PlannedAt           int64  `json:"planned_at"`
	StartedAt           string `json:"started_at,omitempty"`
	TimeSource          string `json:"time_source"`
	CompletedAt         string `json:"completed_at,omitempty"`
	CompletedTimeSource string `json:"completed_time_source"`
	RecordedAt          string `json:"recorded_at"`
	Operation           string `json:"operation"`
	Outcome             string `json:"outcome"`
	Message             string `json:"message"`
}

func TrafficCycleMonths(cycle string) (int, error) {
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
		return 0, fmt.Errorf("unsupported traffic cycle: %s", cycle)
	}
}

// Derive from the original day to avoid permanent end-of-month drift.
func ClientCycleBoundary(start time.Time, months int) time.Time {
	start = start.In(TrafficResetLocation)
	first := time.Date(start.Year(), start.Month()+time.Month(months), 1, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), TrafficResetLocation)
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, TrafficResetLocation).Day()
	day := start.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(first.Year(), first.Month(), day, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), TrafficResetLocation)
}

// Traffic is reset monthly, independently of the billing cycle. The billing
// cycle controls price and expiry; the start day anchors each monthly reset.
// Only the relevant completed boundary and the next one are returned, never an old backlog.
func TrafficResetBoundaries(startMillis int64, cycle string, now time.Time) (time.Time, time.Time, error) {
	if _, err := TrafficCycleMonths(cycle); err != nil || startMillis <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("valid start time and cycle are required")
	}
	months := 1
	start := time.UnixMilli(startMillis).In(TrafficResetLocation)
	now = now.In(TrafficResetLocation)
	index := ((now.Year()-start.Year())*12 + int(now.Month()-start.Month())) / months
	if index < 1 {
		index = 1
	}
	boundary := ClientCycleBoundary(start, index*months)
	for boundary.After(now) && index > 1 {
		index--
		boundary = ClientCycleBoundary(start, index*months)
	}
	if boundary.After(now) {
		return time.Time{}, boundary, nil
	}
	return boundary, ClientCycleBoundary(start, (index+1)*months), nil
}
