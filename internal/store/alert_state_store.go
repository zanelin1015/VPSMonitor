package store

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *SQLiteStore) ShouldSendAlert(alertKey, fingerprint, message string, cooldown time.Duration) (bool, error) {
	if alertKey == "" {
		return false, nil
	}
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	var (
		storedFingerprint string
		state             string
		lastSentText      string
	)
	err := s.db.QueryRow(`
		SELECT fingerprint, state, last_sent_at
		FROM alert_states
		WHERE alert_key = ?
	`, alertKey).Scan(&storedFingerprint, &state, &lastSentText)
	if err == sql.ErrNoRows {
		_, err = s.db.Exec(`
			INSERT INTO alert_states (alert_key, fingerprint, state, first_seen_at, last_seen_at, last_sent_at, resolved_at, message)
			VALUES (?, ?, 'active', ?, ?, ?, '', ?)
		`, alertKey, fingerprint, nowText, nowText, nowText, message)
		if err != nil {
			return false, fmt.Errorf("create alert state: %w", err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("load alert state: %w", err)
	}

	lastSentAt := parseTime(lastSentText)
	shouldSend := state != "active" || storedFingerprint != fingerprint || lastSentAt.IsZero() || (cooldown > 0 && now.Sub(lastSentAt) >= cooldown)
	lastSentUpdate := lastSentText
	if shouldSend {
		lastSentUpdate = nowText
	}
	_, err = s.db.Exec(`
		UPDATE alert_states
		SET fingerprint = ?, state = 'active', last_seen_at = ?, last_sent_at = ?, resolved_at = '', message = ?
		WHERE alert_key = ?
	`, fingerprint, nowText, lastSentUpdate, message, alertKey)
	if err != nil {
		return false, fmt.Errorf("update alert state: %w", err)
	}
	return shouldSend, nil
}

func (s *SQLiteStore) ResolveAlert(alertKey string) error {
	if alertKey == "" {
		return nil
	}
	nowText := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`
		UPDATE alert_states
		SET state = 'resolved', resolved_at = ?, last_seen_at = ?
		WHERE alert_key = ? AND state = 'active'
	`, nowText, nowText, alertKey); err != nil {
		return fmt.Errorf("resolve alert state: %w", err)
	}
	return nil
}
