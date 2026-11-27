package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"bridge-core/internal/model"
)

var ErrProtectedAgentTag = errors.New("Client role tags are managed in Client 角色")

type agentTagSettings struct {
	Tags    []string `json:"tags"`
	Deleted []string `json:"deleted,omitempty"`
}

func getAgentTagSettings(db queryer) (agentTagSettings, bool, error) {
	var raw string
	err := db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, tagSettingsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return agentTagSettings{}, false, nil
	}
	if err != nil {
		return agentTagSettings{}, false, fmt.Errorf("load tag settings: %w", err)
	}
	var settings agentTagSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return settings, false, fmt.Errorf("decode tag settings: %w", err)
	}
	settings.Tags = normalizeTags(settings.Tags)
	return settings, true, nil
}

func (settings agentTagSettings) active(tags []string) []string {
	if len(settings.Deleted) == 0 {
		return tags
	}
	return slices.DeleteFunc(slices.Clone(tags), func(tag string) bool {
		return containsStringFold(settings.Deleted, tag)
	})
}

func (s *SQLiteStore) GetTagSettings() ([]string, bool, error) {
	settings, found, err := getAgentTagSettings(s.db)
	return settings.active(settings.Tags), found, err
}

func (s *SQLiteStore) SaveTagSettings(tags []string) ([]string, error) {
	return s.changeTagSettings(tags, false)
}

func (s *SQLiteStore) AddTagSettings(tags []string) ([]string, error) {
	return s.changeTagSettings(tags, true)
}

func (s *SQLiteStore) changeTagSettings(tags []string, add bool) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	settings, _, err := getAgentTagSettings(tx)
	if err != nil {
		return nil, err
	}
	tags = normalizeTags(tags)
	if add {
		settings.Deleted = slices.DeleteFunc(settings.Deleted, func(tag string) bool { return containsStringFold(tags, tag) })
		tags = normalizeTags(append(settings.Tags, tags...))
	}
	settings.Tags = settings.active(tags)
	if err := saveAgentTagSettings(tx, settings); err != nil {
		return nil, err
	}
	return settings.Tags, tx.Commit()
}

func saveAgentTagSettings(tx *sql.Tx, settings agentTagSettings) error {
	_, err := tx.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, tagSettingsKey, mustJSON(settings), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

type agentTagsRow struct {
	id       string
	tags     []string
	metadata map[string]json.RawMessage
}

func listAgentTagsRows(db interface {
	Query(string, ...any) (*sql.Rows, error)
}) ([]agentTagsRow, error) {
	rows, err := db.Query(`SELECT agent_id, agent_tags_json FROM agents ORDER BY agent_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]agentTagsRow, 0)
	for rows.Next() {
		var item agentTagsRow
		var raw string
		if err := rows.Scan(&item.id, &raw); err != nil {
			return nil, err
		}
		if strings.HasPrefix(strings.TrimSpace(raw), "{") {
			if err := json.Unmarshal([]byte(raw), &item.metadata); err != nil {
				return nil, fmt.Errorf("decode tags for %s: %w", item.id, err)
			}
			raw = string(item.metadata["tags"])
		}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &item.tags); err != nil {
				return nil, fmt.Errorf("decode tags for %s: %w", item.id, err)
			}
		}
		item.tags = normalizeTags(item.tags)
		result = append(result, item)
	}
	return result, rows.Err()
}

func isProtectedAgentTag(tag string) bool {
	return tag == model.AgentRoleLine || tag == model.AgentRoleLanding
}

func buildAgentTagInventory(settings agentTagSettings, agents []agentTagsRow) []model.AgentTagInventoryItem {
	inventory := make(map[string]model.AgentTagInventoryItem)
	for _, tag := range append([]string{model.AgentRoleLine, model.AgentRoleLanding}, settings.active(settings.Tags)...) {
		inventory[strings.ToLower(tag)] = model.AgentTagInventoryItem{Tag: tag, Protected: isProtectedAgentTag(tag)}
	}
	for _, agent := range agents {
		for _, tag := range settings.active(agent.tags) {
			key := strings.ToLower(tag)
			item := inventory[key]
			if item.Tag == "" {
				item.Tag = tag
			}
			item.AgentCount++
			inventory[key] = item
		}
	}
	items := make([]model.AgentTagInventoryItem, 0, len(inventory))
	for _, item := range inventory {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Tag < items[j].Tag })
	return items
}

func (s *SQLiteStore) ListAgentTagInventory() ([]model.AgentTagInventoryItem, error) {
	settings, _, err := getAgentTagSettings(s.db)
	if err != nil {
		return nil, err
	}
	agents, err := listAgentTagsRows(s.db)
	if err != nil {
		return nil, err
	}
	return buildAgentTagInventory(settings, agents), nil
}

func (s *SQLiteStore) DeleteAgentTag(tag, actor string) ([]model.AgentTagInventoryItem, int, error) {
	tag = strings.TrimSpace(tag)
	if isProtectedAgentTag(tag) {
		return nil, 0, ErrProtectedAgentTag
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	settings, _, err := getAgentTagSettings(tx)
	if err != nil {
		return nil, 0, err
	}
	agents, err := listAgentTagsRows(tx)
	if err != nil {
		return nil, 0, err
	}
	matches := func(value string) bool { return strings.EqualFold(value, tag) }
	exists := containsStringFold(settings.Tags, tag)
	removed := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range agents {
		agent := &agents[i]
		if !containsStringFold(agent.tags, tag) {
			continue
		}
		before := agent.tags
		agent.tags = slices.DeleteFunc(slices.Clone(before), matches)
		body := mustJSON(agent.tags)
		if agent.metadata != nil {
			agent.metadata["tags"] = json.RawMessage(body)
			body = mustJSON(agent.metadata)
		}
		if _, err := tx.Exec(`UPDATE agents SET agent_tags_json = ?, updated_at = ? WHERE agent_id = ?`, body, now, agent.id); err != nil {
			return nil, 0, err
		}
		if _, err := tx.Exec(`INSERT INTO config_audit_logs (agent_id, actor, before_json, after_json, created_at) VALUES (?, ?, ?, ?, ?)`,
			agent.id, actor, mustJSON(model.TagSettingsResponse{Tags: before}), mustJSON(model.TagSettingsResponse{Tags: agent.tags}), now); err != nil {
			return nil, 0, err
		}
		removed++
	}
	if exists || removed > 0 {
		settings.Tags = slices.DeleteFunc(settings.Tags, matches)
		settings.Deleted = normalizeTags(append(settings.Deleted, tag))
		if err := saveAgentTagSettings(tx, settings); err != nil {
			return nil, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return buildAgentTagInventory(settings, agents), removed, nil
}
