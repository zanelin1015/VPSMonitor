package store

import (
	"fmt"
	"sort"
	"time"

	"bridge-core/internal/model"
)

type customerTrafficPoint struct {
	assignmentID int64
	reportedAt   time.Time
	upload       uint64
	download     uint64
}

// ListCustomerTrafficUsage derives per-day deltas from the persisted X-UI
// client counters. It intentionally includes history before the requested
// range so the first sample in the range has a baseline, while only deltas
// whose current sample falls inside the range are returned.
func (s *SQLiteStore) ListCustomerTrafficUsage(customerID int64, from, to time.Time) ([]model.CustomerTrafficDailyUsage, error) {
	from = from.UTC()
	to = to.UTC()
	if customerID <= 0 || !from.Before(to) {
		return nil, fmt.Errorf("invalid customer traffic range")
	}
	rows, err := s.db.Query(`
		SELECT a.id, t.reported_at, t.upload_bytes, t.download_bytes
		FROM customer_assignments a
		JOIN customer_traffic_samples t
		  ON t.agent_id = a.agent_id
		 AND t.inbound_id = a.inbound_id
		 AND (a.client_email = '' OR lower(t.client_email) = lower(a.client_email))
		 AND (a.client_id = '' OR t.client_id = a.client_id OR t.client_email = a.client_email)
		WHERE a.customer_id = ?
		  AND a.enabled = 1
		  AND t.reported_at < ?
		ORDER BY a.id ASC, t.reported_at ASC, t.id ASC
	`, customerID, to.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("query customer traffic samples: %w", err)
	}
	defer rows.Close()

	byAssignment := map[int64][]customerTrafficPoint{}
	for rows.Next() {
		var assignmentID int64
		var reportedAtText string
		var upload, download int64
		if err := rows.Scan(&assignmentID, &reportedAtText, &upload, &download); err != nil {
			return nil, fmt.Errorf("scan customer traffic sample: %w", err)
		}
		reportedAt, err := time.Parse(time.RFC3339Nano, reportedAtText)
		if err != nil {
			continue
		}
		byAssignment[assignmentID] = append(byAssignment[assignmentID], customerTrafficPoint{
			assignmentID: assignmentID,
			reportedAt:   reportedAt,
			upload:       nonNegativeTraffic(upload),
			download:     nonNegativeTraffic(download),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customer traffic samples: %w", err)
	}

	type dailyKey struct {
		assignmentID int64
		date         string
	}
	daily := map[dailyKey]*model.CustomerTrafficDailyUsage{}
	for assignmentID, points := range byAssignment {
		if len(points) < 2 {
			continue
		}
		var previous *customerTrafficPoint
		for index := range points {
			current := &points[index]
			if previous != nil && !current.reportedAt.Before(from) && current.reportedAt.Before(to) {
				key := dailyKey{assignmentID: assignmentID, date: current.reportedAt.In(model.TrafficResetLocation).Format("2006-01-02")}
				item := daily[key]
				if item == nil {
					item = &model.CustomerTrafficDailyUsage{AssignmentID: assignmentID, Date: key.date}
					daily[key] = item
				}
				item.UploadBytes += trafficDelta(previous.upload, current.upload)
				item.DownloadBytes += trafficDelta(previous.download, current.download)
			}
			previous = current
		}
	}

	result := make([]model.CustomerTrafficDailyUsage, 0, len(daily))
	for _, item := range daily {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Date != result[j].Date {
			return result[i].Date < result[j].Date
		}
		return result[i].AssignmentID < result[j].AssignmentID
	})
	return result, nil
}

// ListCustomerTrafficRecords returns recent counter deltas at the sampler's
// cadence. It is kept separate from daily aggregation so the customer portal
// can offer a compact "recent records" view without exposing raw counters.
func (s *SQLiteStore) ListCustomerTrafficRecords(customerID int64, from, to time.Time, multipliers map[int64]float64) ([]model.CustomerTrafficRecord, error) {
	from = from.UTC()
	to = to.UTC()
	if customerID <= 0 || !from.Before(to) {
		return nil, fmt.Errorf("invalid customer traffic range")
	}
	rows, err := s.db.Query(`
		SELECT a.id, t.reported_at, t.upload_bytes, t.download_bytes
		FROM customer_assignments a
		JOIN customer_traffic_samples t
		  ON t.agent_id = a.agent_id
		 AND t.inbound_id = a.inbound_id
		 AND (a.client_email = '' OR lower(t.client_email) = lower(a.client_email))
		 AND (a.client_id = '' OR t.client_id = a.client_id OR t.client_email = a.client_email)
		WHERE a.customer_id = ?
		  AND a.enabled = 1
		  AND t.reported_at < ?
		ORDER BY a.id ASC, t.reported_at ASC, t.id ASC
	`, customerID, to.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("query customer traffic records: %w", err)
	}
	defer rows.Close()

	byAssignment := map[int64][]customerTrafficPoint{}
	for rows.Next() {
		var assignmentID int64
		var reportedAtText string
		var upload, download int64
		if err := rows.Scan(&assignmentID, &reportedAtText, &upload, &download); err != nil {
			return nil, fmt.Errorf("scan customer traffic record: %w", err)
		}
		reportedAt, err := time.Parse(time.RFC3339Nano, reportedAtText)
		if err != nil {
			continue
		}
		byAssignment[assignmentID] = append(byAssignment[assignmentID], customerTrafficPoint{
			assignmentID: assignmentID,
			reportedAt:   reportedAt,
			upload:       nonNegativeTraffic(upload),
			download:     nonNegativeTraffic(download),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customer traffic records: %w", err)
	}

	byTimestamp := map[string]*model.CustomerTrafficRecord{}
	for _, points := range byAssignment {
		if len(points) < 2 {
			continue
		}
		var previous *customerTrafficPoint
		for index := range points {
			current := &points[index]
			if previous != nil && !current.reportedAt.Before(from) && current.reportedAt.Before(to) {
				upload := scaleCustomerTrafficRecord(trafficDelta(previous.upload, current.upload), multipliers[points[0].assignmentID])
				download := scaleCustomerTrafficRecord(trafficDelta(previous.download, current.download), multipliers[points[0].assignmentID])
				if upload+download > 0 {
					recordedAt := current.reportedAt.In(model.TrafficResetLocation).Truncate(time.Minute)
					key := recordedAt.Format(time.RFC3339)
					item := byTimestamp[key]
					if item == nil {
						item = &model.CustomerTrafficRecord{RecordedAt: recordedAt}
						byTimestamp[key] = item
					}
					item.UploadBytes += upload
					item.DownloadBytes += download
					item.TotalBytes += upload + download
				}
			}
			previous = current
		}
	}

	result := make([]model.CustomerTrafficRecord, 0, len(byTimestamp))
	for _, item := range byTimestamp {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	return result, nil
}

func scaleCustomerTrafficRecord(value uint64, multiplier float64) uint64 {
	if value == 0 {
		return 0
	}
	if multiplier <= 0 {
		multiplier = 1
	}
	scaled := float64(value) * multiplier
	max := ^uint64(0)
	if scaled >= float64(max) {
		return max
	}
	return uint64(scaled + 0.5)
}

func nonNegativeTraffic(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}
