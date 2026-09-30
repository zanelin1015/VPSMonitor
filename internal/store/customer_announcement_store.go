package store

import (
	"fmt"
	"strings"
	"time"
)

func (s *SQLiteStore) ListCustomerAnnouncementReads(customerID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT announcement_id
		FROM customer_announcement_reads
		WHERE customer_id = ?
		ORDER BY read_at ASC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list customer announcement reads: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan customer announcement read: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customer announcement reads: %w", err)
	}
	return ids, nil
}

func (s *SQLiteStore) MarkCustomerAnnouncementsRead(customerID int64, announcementIDs []string) error {
	if customerID <= 0 {
		return fmt.Errorf("customer id is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin customer announcement reads: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, rawID := range announcementIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, err = tx.Exec(`
			INSERT INTO customer_announcement_reads (customer_id, announcement_id, read_at)
			VALUES (?, ?, ?)
			ON CONFLICT(customer_id, announcement_id) DO UPDATE SET read_at = excluded.read_at
		`, customerID, id, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("save customer announcement read: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit customer announcement reads: %w", err)
	}
	return nil
}
