package store

import (
	"time"

	"bridge-core/internal/model"
)

// CreateCustomerEntranceAssignments grants selected entry listeners together.
// Existing grants, including disabled grants, are preserved on retries.
func (s *SQLiteStore) CreateCustomerEntranceAssignments(customerID int64, requests []model.CustomerAssignmentRequest) (int64, error) {
	if len(requests) == 0 {
		return 0, nil
	}
	grants := make([]model.CustomerAssignmentRequest, 0, len(requests))
	for _, request := range requests {
		grant, err := s.normalizeCustomerAssignmentRequest(request, true)
		if err != nil {
			return 0, err
		}
		grants = append(grants, grant)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var created int64
	for _, grant := range grants {
		result, err := tx.Exec(`
			INSERT INTO customer_assignments (
				customer_id, agent_id, inbound_id, inbound_tag, client_id, client_email, public_client_name, created_at, updated_at
			)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM customer_assignments
				WHERE customer_id = ? AND agent_id = ? AND inbound_id = ?
				  AND ((client_id <> '' AND client_id = ?) OR (client_email <> '' AND client_email COLLATE NOCASE = ?))
			)`, customerID, grant.AgentID, grant.InboundID, grant.InboundTag, grant.ClientID, grant.ClientEmail, grant.PublicClientName, now, now,
			customerID, grant.AgentID, grant.InboundID, grant.ClientID, grant.ClientEmail)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		created += count
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return created, nil
}
