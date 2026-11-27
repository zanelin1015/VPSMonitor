package store

import (
	"fmt"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) UpdateAgentClassifications(ids []string, patch model.AgentClassificationPatch, actor string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		record, found, err := s.loadAgentTx(tx, id)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("Client %q not found", id)
		}
		before := record.Config
		next, err := patch.Apply(before.Entry.Classification)
		if err != nil {
			return err
		}
		record.Config.Entry.Classification = next
		if _, err = tx.Exec(`UPDATE agents SET entry_config_json = json_set(entry_config_json, '$.classification', json(?)), updated_at = ? WHERE agent_id = ?`,
			mustJSON(next), time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
			return err
		}
		if err = insertReplacementConfigAuditTx(tx, id, actor, before, record.Config); err != nil {
			return err
		}
	}
	return tx.Commit()
}
