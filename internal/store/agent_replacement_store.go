package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"bridge-core/internal/model"
)

type AgentReplacementConflictError struct {
	message string
}

func (e *AgentReplacementConflictError) Error() string {
	return e.message
}

func newAgentReplacementConflict(format string, args ...any) error {
	return &AgentReplacementConflictError{message: fmt.Sprintf(format, args...)}
}

// ReplaceAgentReferences atomically moves live authorization records and
// forwarding references to a replacement Client. The source Client and all of
// its historical records remain untouched so the admin can verify the result
// before deleting it separately.
func (s *SQLiteStore) ReplaceAgentReferences(sourceAgentID, replacementAgentID, actor string) (model.AgentReplacementResult, error) {
	sourceAgentID = strings.TrimSpace(sourceAgentID)
	replacementAgentID = strings.TrimSpace(replacementAgentID)
	result := model.AgentReplacementResult{
		Status:             "replaced",
		SourceAgentID:      sourceAgentID,
		ReplacementAgentID: replacementAgentID,
	}
	if sourceAgentID == "" || replacementAgentID == "" {
		return result, fmt.Errorf("source and replacement agent ids are required")
	}
	if sourceAgentID == replacementAgentID {
		return result, fmt.Errorf("replacement agent must be different from source agent")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system:agent-replacement"
	}

	tx, err := s.db.Begin()
	if err != nil {
		return result, fmt.Errorf("begin agent replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, found, err := s.loadAgentTx(tx, sourceAgentID); err != nil {
		return result, err
	} else if !found {
		return result, fmt.Errorf("source agent not found")
	}
	replacement, found, err := s.loadAgentTx(tx, replacementAgentID)
	if err != nil {
		return result, err
	}
	if !found {
		return result, fmt.Errorf("replacement agent not found")
	}
	replacementAddress := preferredReplacementAgentAddress(replacement)
	if replacementAddress == "" {
		if snapshot, ok, snapshotErr := loadLatestAgentSnapshotTx(tx, replacementAgentID); snapshotErr == nil && ok {
			replacement.Summary = snapshot.Summary
			replacementAddress = preferredReplacementAgentAddress(replacement)
		}
	}

	agentIDs, err := listAgentIDsTx(tx)
	if err != nil {
		return result, err
	}
	for _, agentID := range agentIDs {
		if agentID == sourceAgentID {
			continue
		}
		record, found, err := s.loadAgentTx(tx, agentID)
		if err != nil {
			return result, err
		}
		if !found {
			continue
		}
		before, err := cloneReplacementManagedConfig(record.Config)
		if err != nil {
			return result, err
		}
		realmUpdates, haProxyUpdates, err := replaceAgentConfigReferences(
			&record.Config.Entry,
			sourceAgentID,
			replacementAgentID,
			replacementAddress,
			replacement.Config.Entry.PortForwarding,
		)
		if err != nil {
			return result, fmt.Errorf("update references in agent %s: %w", agentID, err)
		}
		if realmUpdates == 0 && haProxyUpdates == 0 {
			continue
		}
		if agentID == replacementAgentID {
			return result, newAgentReplacementConflict("replacement agent config references the source agent; remove that self-reference before replacement")
		}
		entryJSON, err := json.Marshal(record.Config.Entry)
		if err != nil {
			return result, fmt.Errorf("marshal replacement entry config for %s: %w", agentID, err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.Exec(`
			UPDATE agents
			SET entry_config_json = ?, updated_at = ?
			WHERE agent_id = ?
		`, string(entryJSON), now, agentID); err != nil {
			return result, fmt.Errorf("save replacement entry config for %s: %w", agentID, err)
		}
		if err := insertReplacementConfigAuditTx(tx, agentID, actor+":replace:"+sourceAgentID+"->"+replacementAgentID, before, record.Config); err != nil {
			return result, err
		}
		result.RealmReferencesUpdated += realmUpdates
		result.HAProxyReferencesUpdated += haProxyUpdates
		result.UpdatedConfigAgentIDs = append(result.UpdatedConfigAgentIDs, agentID)
	}

	if err := migrateAgentReplacementPermissionsTx(tx, sourceAgentID, replacementAgentID, &result); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit agent replacement: %w", err)
	}
	sort.Strings(result.UpdatedConfigAgentIDs)
	return result, nil
}

func loadLatestAgentSnapshotTx(tx *sql.Tx, agentID string) (model.AgentSnapshot, bool, error) {
	var snapshotJSON string
	err := tx.QueryRow(`SELECT snapshot_json FROM latest_snapshots WHERE agent_id = ?`, agentID).Scan(&snapshotJSON)
	if err == sql.ErrNoRows {
		return model.AgentSnapshot{}, false, nil
	}
	if err != nil {
		return model.AgentSnapshot{}, false, fmt.Errorf("load latest snapshot for replacement agent %s: %w", agentID, err)
	}
	var snapshot model.AgentSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return model.AgentSnapshot{}, false, fmt.Errorf("decode latest snapshot for replacement agent %s: %w", agentID, err)
	}
	return snapshot, true, nil
}

func listAgentIDsTx(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT agent_id FROM agents ORDER BY agent_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list agents for replacement: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return nil, fmt.Errorf("scan agent for replacement: %w", err)
		}
		ids = append(ids, agentID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents for replacement: %w", err)
	}
	return ids, nil
}

func preferredReplacementAgentAddress(agent model.AgentRecord) string {
	if value := strings.TrimSpace(agent.Config.Entry.ImportDomain); value != "" {
		return value
	}
	for _, address := range agent.Config.Entry.Addresses {
		if value := strings.TrimSpace(address); value != "" {
			return value
		}
	}
	for _, value := range []string{
		agent.Summary.ObservedIP,
		agent.PublicIPv4,
		agent.Summary.PublicIPv4,
		agent.PublicIPv6,
		agent.Summary.PublicIPv6,
	} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func replaceAgentConfigReferences(
	entry *model.AgentEntryConfig,
	sourceAgentID string,
	replacementAgentID string,
	replacementAddress string,
	replacementRealm model.RealmForwardConfig,
) (int, int, error) {
	if entry == nil {
		return 0, 0, nil
	}
	realmUpdates := 0
	for index := range entry.PortForwarding.Rules {
		rule := &entry.PortForwarding.Rules[index]
		if strings.TrimSpace(rule.TargetAgentID) != sourceAgentID {
			continue
		}
		if replacementAddress == "" {
			return 0, 0, newAgentReplacementConflict("replacement agent has no main domain or public address for Realm target")
		}
		rule.TargetAgentID = replacementAgentID
		rule.TargetAddress = replacementAddress
		realmUpdates++
	}

	haProxyUpdates := 0
	for ruleIndex := range entry.HAProxy.Rules {
		rule := &entry.HAProxy.Rules[ruleIndex]
		updated, err := replaceHAProxyAgentTarget(&rule.Primary, sourceAgentID, replacementAgentID, replacementAddress, replacementRealm)
		if err != nil {
			return 0, 0, fmt.Errorf("HAProxy rule %s primary: %w", replacementRuleLabel(*rule), err)
		}
		if updated {
			haProxyUpdates++
		}
		for backupIndex := range rule.Backups {
			updated, err := replaceHAProxyAgentTarget(&rule.Backups[backupIndex], sourceAgentID, replacementAgentID, replacementAddress, replacementRealm)
			if err != nil {
				return 0, 0, fmt.Errorf("HAProxy rule %s backup %d: %w", replacementRuleLabel(*rule), backupIndex+1, err)
			}
			if updated {
				haProxyUpdates++
			}
		}
		if err := validateReplacementHAProxyTargets(*rule); err != nil {
			return 0, 0, err
		}
	}
	return realmUpdates, haProxyUpdates, nil
}

func replaceHAProxyAgentTarget(
	target *model.HAProxyRealmTarget,
	sourceAgentID string,
	replacementAgentID string,
	replacementAddress string,
	replacementRealm model.RealmForwardConfig,
) (bool, error) {
	if target == nil || strings.TrimSpace(target.AgentID) != sourceAgentID {
		return false, nil
	}
	if replacementAddress == "" {
		return false, newAgentReplacementConflict("replacement agent has no main domain or public address")
	}
	if !replacementRealm.Enabled || strings.EqualFold(strings.TrimSpace(replacementRealm.Backend), "none") {
		return false, newAgentReplacementConflict("replacement agent does not have Realm enabled")
	}
	rule, ok := findReplacementRealmRule(replacementRealm.Rules, target.RealmRuleID, target.Port)
	if !ok {
		return false, newAgentReplacementConflict("replacement agent has no enabled Realm rule for port %d", target.Port)
	}
	if strings.EqualFold(strings.TrimSpace(rule.Network), "udp") {
		return false, newAgentReplacementConflict("replacement Realm rule for port %d is UDP-only", rule.ListenPort)
	}
	target.AgentID = replacementAgentID
	target.RealmRuleID = rule.ID
	target.Address = replacementAddress
	target.Port = rule.ListenPort
	return true, nil
}

func findReplacementRealmRule(rules []model.RealmForwardRule, ruleID string, listenPort int) (model.RealmForwardRule, bool) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID != "" {
		for _, rule := range rules {
			if rule.Enabled && strings.EqualFold(strings.TrimSpace(rule.ID), ruleID) {
				return rule, true
			}
		}
	}
	if listenPort > 0 {
		for _, rule := range rules {
			if rule.Enabled && rule.ListenPort == listenPort {
				return rule, true
			}
		}
	}
	return model.RealmForwardRule{}, false
}

func validateReplacementHAProxyTargets(rule model.HAProxyRule) error {
	seen := make(map[string]struct{}, 1+len(rule.Backups))
	targets := append([]model.HAProxyRealmTarget{rule.Primary}, rule.Backups...)
	for _, target := range targets {
		key := strings.ToLower(strings.TrimSpace(target.AgentID)) + "\x00" + strings.ToLower(strings.TrimSpace(target.RealmRuleID))
		if strings.TrimSpace(target.RealmRuleID) == "" {
			key = strings.ToLower(strings.TrimSpace(target.AgentID)) + fmt.Sprintf("\x00port:%d", target.Port)
		}
		if strings.TrimSpace(target.AgentID) == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			return newAgentReplacementConflict("HAProxy rule %s would contain duplicate replacement targets", replacementRuleLabel(rule))
		}
		seen[key] = struct{}{}
	}
	return nil
}

func replacementRuleLabel(rule model.HAProxyRule) string {
	if value := strings.TrimSpace(rule.Name); value != "" {
		return value
	}
	if value := strings.TrimSpace(rule.ID); value != "" {
		return value
	}
	return fmt.Sprintf("port %d", rule.ListenPort)
}

func cloneReplacementManagedConfig(cfg model.ManagedAgentConfig) (model.ManagedAgentConfig, error) {
	body, err := json.Marshal(cfg)
	if err != nil {
		return model.ManagedAgentConfig{}, fmt.Errorf("marshal config before agent replacement: %w", err)
	}
	var cloned model.ManagedAgentConfig
	if err := json.Unmarshal(body, &cloned); err != nil {
		return model.ManagedAgentConfig{}, fmt.Errorf("clone config before agent replacement: %w", err)
	}
	return cloned, nil
}

func insertReplacementConfigAuditTx(tx *sql.Tx, agentID, actor string, before, after model.ManagedAgentConfig) error {
	beforeJSON, err := json.Marshal(redactManagedAgentConfig(before))
	if err != nil {
		return fmt.Errorf("marshal replacement config audit before: %w", err)
	}
	afterJSON, err := json.Marshal(redactManagedAgentConfig(after))
	if err != nil {
		return fmt.Errorf("marshal replacement config audit after: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO config_audit_logs (agent_id, actor, before_json, after_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, agentID, actor, string(beforeJSON), string(afterJSON), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert replacement config audit for %s: %w", agentID, err)
	}
	return nil
}

func migrateAgentReplacementPermissionsTx(tx *sql.Tx, sourceAgentID, replacementAgentID string, result *model.AgentReplacementResult) error {
	var err error
	if result.AreaManagerAgentsMigrated, err = countAgentRowsTx(tx, "area_manager_agents", sourceAgentID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO area_manager_agents (manager_id, agent_id, created_at)
		SELECT manager_id, ?, created_at FROM area_manager_agents WHERE agent_id = ?
	`, replacementAgentID, sourceAgentID); err != nil {
		return fmt.Errorf("migrate area manager agents: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM area_manager_agents WHERE agent_id = ?`, sourceAgentID); err != nil {
		return fmt.Errorf("remove source area manager agents: %w", err)
	}

	if result.AreaManagerTagsMigrated, err = migrateAreaManagerAgentTagsTx(tx, sourceAgentID, replacementAgentID); err != nil {
		return err
	}
	if result.AreaAssignmentsMigrated, err = migrateAreaManagerAssignmentsTx(tx, sourceAgentID, replacementAgentID); err != nil {
		return err
	}
	if result.CustomerAssignmentsMigrated, err = migrateCustomerAssignmentsTx(tx, sourceAgentID, replacementAgentID); err != nil {
		return err
	}

	if result.OutboundGrantsMigrated, err = countAgentRowsTx(tx, "area_manager_outbound_grants", sourceAgentID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO area_manager_outbound_grants (manager_id, agent_id, outbound_tag, created_at)
		SELECT manager_id, ?, outbound_tag, created_at FROM area_manager_outbound_grants WHERE agent_id = ?
	`, replacementAgentID, sourceAgentID); err != nil {
		return fmt.Errorf("migrate area manager outbound grants: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM area_manager_outbound_grants WHERE agent_id = ?`, sourceAgentID); err != nil {
		return fmt.Errorf("remove source area manager outbound grants: %w", err)
	}
	return nil
}

func countAgentRowsTx(tx *sql.Tx, table, agentID string) (int, error) {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE agent_id = ?`, agentID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s replacement rows: %w", table, err)
	}
	return count, nil
}

func migrateAreaManagerAgentTagsTx(tx *sql.Tx, sourceAgentID, replacementAgentID string) (int, error) {
	rows, err := tx.Query(`
		SELECT manager_id, tags_json
		FROM area_manager_agent_tags
		WHERE agent_id = ?
	`, sourceAgentID)
	if err != nil {
		return 0, fmt.Errorf("list area manager tags for replacement: %w", err)
	}
	type tagRow struct {
		managerID int64
		tagsJSON  string
	}
	items := make([]tagRow, 0)
	for rows.Next() {
		var item tagRow
		if err := rows.Scan(&item.managerID, &item.tagsJSON); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan area manager tags for replacement: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close area manager tags for replacement: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate area manager tags for replacement: %w", err)
	}
	for _, item := range items {
		var sourceTags []string
		_ = json.Unmarshal([]byte(item.tagsJSON), &sourceTags)
		var targetJSON string
		var targetTags []string
		err := tx.QueryRow(`
			SELECT tags_json FROM area_manager_agent_tags
			WHERE manager_id = ? AND agent_id = ?
		`, item.managerID, replacementAgentID).Scan(&targetJSON)
		if err != nil && err != sql.ErrNoRows {
			return 0, fmt.Errorf("load replacement area manager tags: %w", err)
		}
		if err == nil {
			_ = json.Unmarshal([]byte(targetJSON), &targetTags)
		}
		mergedJSON, err := json.Marshal(normalizeTags(append(targetTags, sourceTags...)))
		if err != nil {
			return 0, fmt.Errorf("merge area manager tags: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO area_manager_agent_tags (manager_id, agent_id, tags_json, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(manager_id, agent_id) DO UPDATE SET
				tags_json = excluded.tags_json,
				updated_at = excluded.updated_at
		`, item.managerID, replacementAgentID, string(mergedJSON), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return 0, fmt.Errorf("save replacement area manager tags: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM area_manager_agent_tags WHERE agent_id = ?`, sourceAgentID); err != nil {
		return 0, fmt.Errorf("remove source area manager tags: %w", err)
	}
	return len(items), nil
}

func migrateAreaManagerAssignmentsTx(tx *sql.Tx, sourceAgentID, replacementAgentID string) (int, error) {
	rows, err := tx.Query(`
		SELECT manager_id, inbound_id, inbound_tag, client_email, public_client_name, enabled, created_at, updated_at
		FROM area_manager_assignments
		WHERE agent_id = ?
	`, sourceAgentID)
	if err != nil {
		return 0, fmt.Errorf("list area manager assignments for replacement: %w", err)
	}
	type assignmentRow struct {
		managerID                          int64
		inboundID, enabled                 int
		inboundTag, clientEmail            string
		publicClientName, created, updated string
	}
	items := make([]assignmentRow, 0)
	for rows.Next() {
		var item assignmentRow
		if err := rows.Scan(&item.managerID, &item.inboundID, &item.inboundTag, &item.clientEmail, &item.publicClientName, &item.enabled, &item.created, &item.updated); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan area manager assignment for replacement: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close area manager assignments for replacement: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate area manager assignments for replacement: %w", err)
	}
	for _, item := range items {
		if _, err := tx.Exec(`
			INSERT INTO area_manager_assignments (
				manager_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
				enabled, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(manager_id, agent_id, inbound_id, inbound_tag, client_email) DO UPDATE SET
				public_client_name = CASE
					WHEN area_manager_assignments.public_client_name = '' THEN excluded.public_client_name
					ELSE area_manager_assignments.public_client_name
				END,
				enabled = MAX(area_manager_assignments.enabled, excluded.enabled),
				updated_at = excluded.updated_at
		`, item.managerID, replacementAgentID, item.inboundID, item.inboundTag, item.clientEmail, item.publicClientName, item.enabled, item.created, item.updated); err != nil {
			return 0, fmt.Errorf("save replacement area manager assignment: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM area_manager_assignments WHERE agent_id = ?`, sourceAgentID); err != nil {
		return 0, fmt.Errorf("remove source area manager assignments: %w", err)
	}
	return len(items), nil
}

func migrateCustomerAssignmentsTx(tx *sql.Tx, sourceAgentID, replacementAgentID string) (int, error) {
	rows, err := tx.Query(`
		SELECT customer_id, inbound_id, inbound_tag, client_email, public_client_name, customer_remark,
		       enabled, created_at, updated_at
		FROM customer_assignments
		WHERE agent_id = ?
	`, sourceAgentID)
	if err != nil {
		return 0, fmt.Errorf("list customer assignments for replacement: %w", err)
	}
	type assignmentRow struct {
		customerID                                int64
		inboundID, enabled                        int
		inboundTag, clientEmail, publicClientName string
		customerRemark, created, updated          string
	}
	items := make([]assignmentRow, 0)
	for rows.Next() {
		var item assignmentRow
		if err := rows.Scan(&item.customerID, &item.inboundID, &item.inboundTag, &item.clientEmail, &item.publicClientName, &item.customerRemark, &item.enabled, &item.created, &item.updated); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan customer assignment for replacement: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close customer assignments for replacement: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate customer assignments for replacement: %w", err)
	}
	for _, item := range items {
		if _, err := tx.Exec(`
			INSERT INTO customer_assignments (
				customer_id, agent_id, inbound_id, inbound_tag, client_email, public_client_name,
				customer_remark, enabled, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(customer_id, agent_id, inbound_id, inbound_tag, client_email) DO UPDATE SET
				public_client_name = CASE
					WHEN customer_assignments.public_client_name = '' THEN excluded.public_client_name
					ELSE customer_assignments.public_client_name
				END,
				customer_remark = CASE
					WHEN customer_assignments.customer_remark = '' THEN excluded.customer_remark
					ELSE customer_assignments.customer_remark
				END,
				enabled = MAX(customer_assignments.enabled, excluded.enabled),
				updated_at = excluded.updated_at
		`, item.customerID, replacementAgentID, item.inboundID, item.inboundTag, item.clientEmail, item.publicClientName, item.customerRemark, item.enabled, item.created, item.updated); err != nil {
			return 0, fmt.Errorf("save replacement customer assignment: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM customer_assignments WHERE agent_id = ?`, sourceAgentID); err != nil {
		return 0, fmt.Errorf("remove source customer assignments: %w", err)
	}
	return len(items), nil
}
