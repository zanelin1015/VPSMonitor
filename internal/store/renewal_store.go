package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"bridge-core/internal/model"
)

func updateRenewalTrafficBaselineTx(tx *sql.Tx, agentID string, trafficSent uint64, trafficRecv uint64, trafficTotal uint64, now time.Time) error {
	if trafficTotal == 0 {
		trafficTotal = trafficSent + trafficRecv
	}
	if trafficTotal == 0 && trafficSent == 0 && trafficRecv == 0 {
		return nil
	}
	var renewalJSON string
	err := tx.QueryRow(`SELECT renewal_config_json FROM agents WHERE agent_id = ?`, agentID).Scan(&renewalJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	var cfg model.VPSRenewalConfig
	if renewalJSON != "" {
		_ = json.Unmarshal([]byte(renewalJSON), &cfg)
	}
	cfg = normalizeRenewalConfig(cfg)
	if cfg.TrafficLimitBytes == 0 || !cfg.AutoRenew || cfg.Cycle == "" {
		return nil
	}

	periodStart, ok := currentRenewalPeriodStart(cfg, now)
	if !ok {
		return nil
	}
	periodKey := periodStart.Format("2006-01-02")
	changed := false
	if cfg.TrafficBaselinePeriodStart == "" {
		cfg.TrafficBaselinePeriodStart = periodKey
		cfg.TrafficBaselineBytes = trafficTotal
		cfg.TrafficSentBaselineBytes = trafficSent
		cfg.TrafficRecvBaselineBytes = trafficRecv
		changed = true
	} else if cfg.TrafficBaselinePeriodStart != periodKey {
		cfg.TrafficBaselinePeriodStart = periodKey
		cfg.TrafficBaselineBytes = trafficTotal
		cfg.TrafficSentBaselineBytes = trafficSent
		cfg.TrafficRecvBaselineBytes = trafficRecv
		changed = true
	}
	if cfg.TrafficBaselineBytes > trafficTotal {
		cfg.TrafficBaselineBytes = trafficTotal
		changed = true
	}
	if cfg.TrafficSentBaselineBytes > trafficSent {
		cfg.TrafficSentBaselineBytes = trafficSent
		changed = true
	}
	if cfg.TrafficRecvBaselineBytes > trafficRecv {
		cfg.TrafficRecvBaselineBytes = trafficRecv
		changed = true
	}
	if cfg.TrafficBaselineBytes == 0 && trafficTotal > 0 {
		cfg.TrafficBaselineBytes = trafficTotal
		changed = true
	}
	if cfg.TrafficSentBaselineBytes == 0 && trafficSent > 0 {
		cfg.TrafficSentBaselineBytes = trafficSent
		changed = true
	}
	if cfg.TrafficRecvBaselineBytes == 0 && trafficRecv > 0 {
		cfg.TrafficRecvBaselineBytes = trafficRecv
		changed = true
	}
	if !changed {
		return nil
	}

	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE agents SET renewal_config_json = ?, updated_at = ? WHERE agent_id = ?`, string(body), time.Now().UTC().Format(time.RFC3339Nano), agentID)
	return err
}

func currentRenewalPeriodStart(cfg model.VPSRenewalConfig, now time.Time) (time.Time, bool) {
	nowDay := renewalDate(now)
	if cfg.StartDate != "" {
		start, ok := parseRenewalDate(cfg.StartDate)
		if !ok || nowDay.Before(start) {
			return time.Time{}, false
		}
		periodStart := start
		periodEnd := addRenewalPeriod(periodStart, cfg.Cycle)
		for !periodEnd.After(nowDay) {
			periodStart = periodEnd
			periodEnd = addRenewalPeriod(periodStart, cfg.Cycle)
		}
		return periodStart, true
	}
	if cfg.ExpireDate != "" {
		periodEnd, ok := parseRenewalDate(cfg.ExpireDate)
		if !ok {
			return time.Time{}, false
		}
		for !periodEnd.After(nowDay) {
			periodEnd = addRenewalPeriod(periodEnd, cfg.Cycle)
		}
		return subtractRenewalPeriod(periodEnd, cfg.Cycle), true
	}
	return time.Time{}, false
}

func renewalDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func parseRenewalDate(value string) (time.Time, bool) {
	date, err := time.Parse("2006-01-02", value)
	return date, err == nil
}

func addRenewalPeriod(date time.Time, cycle string) time.Time {
	switch cycle {
	case "week":
		return date.AddDate(0, 0, 7)
	case "year":
		return date.AddDate(1, 0, 0)
	default:
		return date.AddDate(0, 1, 0)
	}
}

func subtractRenewalPeriod(date time.Time, cycle string) time.Time {
	switch cycle {
	case "week":
		return date.AddDate(0, 0, -7)
	case "year":
		return date.AddDate(-1, 0, 0)
	default:
		return date.AddDate(0, -1, 0)
	}
}
