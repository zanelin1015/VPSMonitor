package store

import (
	"database/sql"
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

type customerTrafficAssignment struct {
	id          int64
	agentID     string
	inboundID   int
	clientID    string
	clientEmail string
}

// listCustomerTrafficPoints reads only the requested range plus one baseline
// sample per assignment. The old join returned every retained sample up to
// the end of the range, so a 30-day customer view repeatedly scanned the full
// traffic history and made the portal wait for the frontend timeout.
func (s *SQLiteStore) listCustomerTrafficPoints(customerID int64, from, to time.Time) (map[int64][]customerTrafficPoint, error) {
	fromText := from.UTC().Format(time.RFC3339Nano)
	toText := to.UTC().Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT id, agent_id, inbound_id, client_id, client_email
		FROM customer_assignments
		WHERE customer_id = ? AND enabled = 1
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("query customer traffic assignments: %w", err)
	}
	assignments := make([]customerTrafficAssignment, 0)
	for rows.Next() {
		var item customerTrafficAssignment
		if err := rows.Scan(&item.id, &item.agentID, &item.inboundID, &item.clientID, &item.clientEmail); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan customer traffic assignment: %w", err)
		}
		assignments = append(assignments, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate customer traffic assignments: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close customer traffic assignments: %w", err)
	}

	byAssignment := make(map[int64][]customerTrafficPoint, len(assignments))
	for _, assignment := range assignments {
		matchCondition := "client_email COLLATE NOCASE = ?"
		matchArgs := []any{assignment.clientEmail}
		if assignment.clientEmail != "" && assignment.clientID != "" {
			// Preserve the legacy match rule: a client ID is accepted when it
			// matches, or when the stored email has the exact original casing.
			matchCondition += " AND (client_id = ? OR client_email = ?)"
			matchArgs = append(matchArgs, assignment.clientID, assignment.clientEmail)
		} else if assignment.clientEmail == "" {
			if assignment.clientID == "" {
				continue
			}
			matchCondition = "client_id = ?"
			matchArgs = []any{assignment.clientID}
		}
		if len(matchArgs) == 0 {
			continue
		}
		baseArgs := append([]any{assignment.agentID, assignment.inboundID}, matchArgs...)
		baselineQuery := `
			SELECT id, reported_at, upload_bytes, download_bytes
			FROM customer_traffic_samples
			WHERE agent_id = ? AND inbound_id = ? AND ` + matchCondition + `
			  AND reported_at < ?
			ORDER BY reported_at DESC, id DESC
			LIMIT 1
		`
		var baseline customerTrafficPoint
		var baselineReportedAt string
		var baselineUpload, baselineDownload int64
		baselineErr := s.db.QueryRow(baselineQuery, append(baseArgs, fromText)...).Scan(
			new(int64), &baselineReportedAt, &baselineUpload, &baselineDownload,
		)
		if baselineErr != nil && baselineErr != sql.ErrNoRows {
			return nil, fmt.Errorf("query customer traffic baseline: %w", baselineErr)
		}
		if baselineErr == nil {
			if reportedAt, parseErr := time.Parse(time.RFC3339Nano, baselineReportedAt); parseErr == nil {
				baseline.assignmentID = assignment.id
				baseline.reportedAt = reportedAt
				baseline.upload = nonNegativeTraffic(baselineUpload)
				baseline.download = nonNegativeTraffic(baselineDownload)
				byAssignment[assignment.id] = append(byAssignment[assignment.id], baseline)
			}
		}

		rangeQuery := `
			SELECT id, reported_at, upload_bytes, download_bytes
			FROM customer_traffic_samples
			WHERE agent_id = ? AND inbound_id = ? AND ` + matchCondition + `
			  AND reported_at >= ? AND reported_at < ?
			ORDER BY reported_at ASC, id ASC
		`
		rangeRows, err := s.db.Query(rangeQuery, append(baseArgs, fromText, toText)...)
		if err != nil {
			return nil, fmt.Errorf("query customer traffic range: %w", err)
		}
		for rangeRows.Next() {
			var point customerTrafficPoint
			var reportedAtText string
			var upload, download int64
			if err := rangeRows.Scan(new(int64), &reportedAtText, &upload, &download); err != nil {
				_ = rangeRows.Close()
				return nil, fmt.Errorf("scan customer traffic range: %w", err)
			}
			reportedAt, parseErr := time.Parse(time.RFC3339Nano, reportedAtText)
			if parseErr != nil {
				continue
			}
			point.assignmentID = assignment.id
			point.reportedAt = reportedAt
			point.upload = nonNegativeTraffic(upload)
			point.download = nonNegativeTraffic(download)
			byAssignment[assignment.id] = append(byAssignment[assignment.id], point)
		}
		if err := rangeRows.Err(); err != nil {
			_ = rangeRows.Close()
			return nil, fmt.Errorf("iterate customer traffic range: %w", err)
		}
		if err := rangeRows.Close(); err != nil {
			return nil, fmt.Errorf("close customer traffic range: %w", err)
		}
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
