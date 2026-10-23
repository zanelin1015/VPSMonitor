package store

import (
	"database/sql"
	"encoding/json"
	"strings"

	"bridge-core/internal/model"
)

func areaPermissionBool(v bool) *bool { return &v }
func uniqueAreaManagerAgentIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}
func (s *SQLiteStore) hydrateAreaManagerPermissions(v *model.AreaManagerAdminView) error {
	v.RealmEnabled = true
	v.LineEnabled = true
	v.LandingEnabled = true
	v.FrontLinkEnabled = true
	var line, landing string
	err := s.db.QueryRow(`SELECT realm_enabled, line_enabled, landing_enabled, front_link_enabled, line_agent_ids_json, landing_agent_ids_json FROM area_manager_permissions WHERE manager_id = ?`, v.ID).Scan(&v.RealmEnabled, &v.LineEnabled, &v.LandingEnabled, &v.FrontLinkEnabled, &line, &landing)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(line), &v.LineAgentIDs); err != nil {
		return err
	}
	return json.Unmarshal([]byte(landing), &v.LandingAgentIDs)
}
func (s *SQLiteStore) replaceAreaManagerPermissionsTx(tx *sql.Tx, id int64, n normalizedAreaManagerRequest) error {
	value := func(p *bool) bool { return p == nil || *p }
	_, err := tx.Exec(`INSERT INTO area_manager_permissions(manager_id, realm_enabled, line_enabled, landing_enabled, front_link_enabled, line_agent_ids_json, landing_agent_ids_json) VALUES(?,?,?,?,?,?,?) ON CONFLICT(manager_id) DO UPDATE SET realm_enabled=excluded.realm_enabled,line_enabled=excluded.line_enabled,landing_enabled=excluded.landing_enabled,front_link_enabled=excluded.front_link_enabled,line_agent_ids_json=excluded.line_agent_ids_json,landing_agent_ids_json=excluded.landing_agent_ids_json`, id, value(n.realmEnabled), value(n.lineEnabled), value(n.landingEnabled), value(n.frontLinkEnabled), mustJSON(uniqueAreaManagerAgentIDs(n.lineAgentIDs)), mustJSON(uniqueAreaManagerAgentIDs(n.landingAgentIDs)))
	if err != nil {
		return err
	}
	if !value(n.lineEnabled) {
		if _, err = tx.Exec(`DELETE FROM area_manager_outbound_grants WHERE manager_id=?`, id); err != nil {
			return err
		}
	}
	if !value(n.realmEnabled) {
		if _, err = tx.Exec(`DELETE FROM area_manager_assignments WHERE manager_id=? AND (inbound_tag LIKE 'realm:%' OR inbound_tag LIKE 'haproxy:%')`, id); err != nil {
			return err
		}
	}
	if !value(n.lineEnabled) && !value(n.landingEnabled) {
		if _, err = tx.Exec(`DELETE FROM area_manager_assignments WHERE manager_id=? AND inbound_tag NOT LIKE 'realm:%' AND inbound_tag NOT LIKE 'haproxy:%'`, id); err != nil {
			return err
		}
	}
	if !value(n.frontLinkEnabled) {
		_, err = tx.Exec(`DELETE FROM front_proxy_grants WHERE grantee_type='area_manager' AND grantee_id=?`, id)
	}
	if err != nil {
		return err
	}
	if !value(n.lineEnabled) || !value(n.landingEnabled) {
		allowed := []string{}
		if value(n.lineEnabled) {
			allowed = append(allowed, n.lineAgentIDs...)
		}
		if value(n.landingEnabled) {
			allowed = append(allowed, n.landingAgentIDs...)
		}
		query := `DELETE FROM area_manager_assignments WHERE manager_id=? AND inbound_tag NOT LIKE 'realm:%' AND inbound_tag NOT LIKE 'haproxy:%'`
		args := []any{id}
		if len(allowed) > 0 {
			query += ` AND agent_id NOT IN (` + strings.TrimRight(strings.Repeat("?,", len(allowed)), ",") + `)`
			for _, agentID := range allowed {
				args = append(args, agentID)
			}
		}
		if _, err = tx.Exec(query, args...); err != nil {
			return err
		}
	}
	return err
}
