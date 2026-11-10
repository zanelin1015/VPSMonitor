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
	byAssignment, err := s.listCustomerTrafficPoints(customerID, from, to)
	if err != nil {
		return nil, err
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
				key := dailyKey{assignmentID: assignmentID, date: current.reportedAt.In(model.BeijingLocation).Format("2006-01-02")}
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
	byAssignment, err := s.listCustomerTrafficPoints(customerID, from, to)
	if err != nil {
		return nil, err
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
					recordedAt := current.reportedAt.In(model.BeijingLocation).Truncate(time.Minute)
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

// Customer identity branches are separate so SQLite can seek by email or ID
// before scanning the requested time range. Combining them with OR made the
// planner scan an agent's entire history for every assignment. Baselines use
// an indexed top-one lookup rather than ranking every historical sample.
const customerTrafficPointsQuery = `
	WITH enabled_assignments AS (
		SELECT id AS assignment_id, agent_id, inbound_id, client_id, client_email
		FROM customer_assignments
		WHERE customer_id = ? AND enabled = 1
	), baseline_ids AS (
		SELECT a.assignment_id, (
			SELECT t.id FROM customer_traffic_samples t
			WHERE t.agent_id = a.agent_id AND t.inbound_id = a.inbound_id
			  AND t.client_email COLLATE NOCASE = a.client_email
			  AND (a.client_id = '' OR t.client_id = a.client_id OR t.client_email = a.client_email)
			  AND t.reported_at < ?
			ORDER BY t.reported_at DESC, t.id DESC LIMIT 1
		) AS sample_id
		FROM enabled_assignments a WHERE a.client_email <> ''
		UNION ALL
		SELECT a.assignment_id, (
			SELECT t.id FROM customer_traffic_samples t
			WHERE t.agent_id = a.agent_id AND t.inbound_id = a.inbound_id
			  AND t.client_id = a.client_id AND t.reported_at < ?
			ORDER BY t.reported_at DESC, t.id DESC LIMIT 1
		) AS sample_id
		FROM enabled_assignments a WHERE a.client_email = '' AND a.client_id <> ''
	), selected_points AS (
		SELECT b.assignment_id, t.id AS sample_id, t.reported_at, t.upload_bytes, t.download_bytes
		FROM baseline_ids b JOIN customer_traffic_samples t ON t.id = b.sample_id
		UNION ALL
		SELECT a.assignment_id, t.id AS sample_id, t.reported_at, t.upload_bytes, t.download_bytes
		FROM enabled_assignments a JOIN customer_traffic_samples t
		  ON t.agent_id = a.agent_id AND t.inbound_id = a.inbound_id
		 AND t.client_email COLLATE NOCASE = a.client_email
		 AND (a.client_id = '' OR t.client_id = a.client_id OR t.client_email = a.client_email)
		WHERE a.client_email <> '' AND t.reported_at >= ? AND t.reported_at < ?
		UNION ALL
		SELECT a.assignment_id, t.id AS sample_id, t.reported_at, t.upload_bytes, t.download_bytes
		FROM enabled_assignments a JOIN customer_traffic_samples t
		  ON t.agent_id = a.agent_id AND t.inbound_id = a.inbound_id AND t.client_id = a.client_id
		WHERE a.client_email = '' AND a.client_id <> '' AND t.reported_at >= ? AND t.reported_at < ?
	)
	SELECT assignment_id, sample_id, reported_at, upload_bytes, download_bytes
	FROM selected_points ORDER BY assignment_id, reported_at, sample_id
`

// listCustomerTrafficPoints reads the requested range and one preceding sample
// per enabled assignment, preserving the baseline needed to calculate deltas.
func (s *SQLiteStore) listCustomerTrafficPoints(customerID int64, from, to time.Time) (map[int64][]customerTrafficPoint, error) {
	fromText := from.UTC().Format(time.RFC3339Nano)
	toText := to.UTC().Format(time.RFC3339Nano)
	rows, err := s.db.Query(customerTrafficPointsQuery, customerID, fromText, fromText, fromText, toText, fromText, toText)
	if err != nil {
		return nil, fmt.Errorf("query customer traffic points: %w", err)
	}
	defer rows.Close()

	byAssignment := make(map[int64][]customerTrafficPoint)
	for rows.Next() {
		var assignmentID int64
		var reportedAtText string
		var upload, download int64
		if err := rows.Scan(&assignmentID, new(int64), &reportedAtText, &upload, &download); err != nil {
			return nil, fmt.Errorf("scan customer traffic point: %w", err)
		}
		reportedAt, parseErr := time.Parse(time.RFC3339Nano, reportedAtText)
		if parseErr != nil {
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
		return nil, fmt.Errorf("iterate customer traffic points: %w", err)
	}
	return byAssignment, nil
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
