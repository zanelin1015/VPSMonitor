package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const (
	clientInstallSettingsKey = "client_install"
	tagSettingsKey           = "tag_settings"
	frontendSettingsKey      = "frontend_settings"
	outboundLinkLibraryKey   = "outbound_link_library"
	topologyLookupCacheKey   = "topology_lookup_cache"
)

func (s *SQLiteStore) GetClientInstallSettings() (model.ClientInstallSettingsRequest, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, clientInstallSettingsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return model.ClientInstallSettingsRequest{}, false, nil
	}
	if err != nil {
		return model.ClientInstallSettingsRequest{}, false, fmt.Errorf("load client install settings: %w", err)
	}
	var settings model.ClientInstallSettingsRequest
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.ClientInstallSettingsRequest{}, false, fmt.Errorf("decode client install settings: %w", err)
	}
	settings, err = s.decryptClientInstallSettings(settings)
	if err != nil {
		return model.ClientInstallSettingsRequest{}, false, fmt.Errorf("decrypt client install settings: %w", err)
	}
	return normalizeClientInstallSettings(settings), true, nil
}

func (s *SQLiteStore) SaveClientInstallSettings(settings model.ClientInstallSettingsRequest) (model.ClientInstallSettingsRequest, error) {
	settings = normalizeClientInstallSettings(settings)
	stored, err := s.encryptClientInstallSettings(settings)
	if err != nil {
		return model.ClientInstallSettingsRequest{}, err
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return model.ClientInstallSettingsRequest{}, fmt.Errorf("encode client install settings: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, clientInstallSettingsKey, string(data), now)
	if err != nil {
		return model.ClientInstallSettingsRequest{}, fmt.Errorf("save client install settings: %w", err)
	}
	return settings, nil
}

func (s *SQLiteStore) encryptClientInstallSettings(settings model.ClientInstallSettingsRequest) (model.ClientInstallSettingsRequest, error) {
	if s.secrets == nil || settings.XUIPassword == "" || strings.HasPrefix(settings.XUIPassword, encryptedValuePrefix) {
		return settings, nil
	}
	encrypted, err := s.secrets.EncryptString(settings.XUIPassword)
	if err != nil {
		return model.ClientInstallSettingsRequest{}, fmt.Errorf("encrypt x-ui bootstrap password: %w", err)
	}
	settings.XUIPassword = encrypted
	return settings, nil
}

func (s *SQLiteStore) decryptClientInstallSettings(settings model.ClientInstallSettingsRequest) (model.ClientInstallSettingsRequest, error) {
	if s.secrets == nil || settings.XUIPassword == "" || !strings.HasPrefix(settings.XUIPassword, encryptedValuePrefix) {
		return settings, nil
	}
	decrypted, err := s.secrets.DecryptString(settings.XUIPassword)
	if err != nil {
		return model.ClientInstallSettingsRequest{}, err
	}
	settings.XUIPassword = decrypted
	return settings, nil
}

func normalizeClientInstallSettings(settings model.ClientInstallSettingsRequest) model.ClientInstallSettingsRequest {
	settings.ServerURL = strings.TrimSpace(settings.ServerURL)
	settings.InstallScriptURL = strings.TrimSpace(settings.InstallScriptURL)
	settings.PollInterval = strings.TrimSpace(settings.PollInterval)
	settings.XUIUsername = strings.TrimSpace(settings.XUIUsername)
	settings.XUIPassword = strings.TrimSpace(settings.XUIPassword)
	settings.XUIWebPath = normalizeXUIBootstrapWebPath(settings.XUIWebPath)
	settings.XUIInstallScriptURL = strings.TrimSpace(settings.XUIInstallScriptURL)
	if settings.RequestTimeoutSeconds < 0 {
		settings.RequestTimeoutSeconds = 0
	}
	if settings.XUIPanelPort < 0 || settings.XUIPanelPort > 65535 {
		settings.XUIPanelPort = 0
	}
	return settings
}

func normalizeXUIBootstrapWebPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = "/" + strings.Trim(value, "/")
	if value == "/" {
		return ""
	}
	return value + "/"
}

func (s *SQLiteStore) GetTagSettings() ([]string, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, tagSettingsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load tag settings: %w", err)
	}
	var payload model.TagSettingsResponse
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false, fmt.Errorf("decode tag settings: %w", err)
	}
	return normalizeTags(payload.Tags), true, nil
}

func (s *SQLiteStore) SaveTagSettings(tags []string) ([]string, error) {
	normalized := normalizeTags(tags)
	data, err := json.Marshal(model.TagSettingsResponse{Tags: normalized})
	if err != nil {
		return nil, fmt.Errorf("encode tag settings: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, tagSettingsKey, string(data), now)
	if err != nil {
		return nil, fmt.Errorf("save tag settings: %w", err)
	}
	return normalized, nil
}

func (s *SQLiteStore) GetFrontendSettings() (model.FrontendSettings, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, frontendSettingsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return model.FrontendSettings{}, false, nil
	}
	if err != nil {
		return model.FrontendSettings{}, false, fmt.Errorf("load frontend settings: %w", err)
	}
	var settings model.FrontendSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.FrontendSettings{}, false, fmt.Errorf("decode frontend settings: %w", err)
	}
	return settings, true, nil
}

func (s *SQLiteStore) SaveFrontendSettings(settings model.FrontendSettings) (model.FrontendSettings, error) {
	data, err := json.Marshal(settings)
	if err != nil {
		return model.FrontendSettings{}, fmt.Errorf("encode frontend settings: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, frontendSettingsKey, string(data), now)
	if err != nil {
		return model.FrontendSettings{}, fmt.Errorf("save frontend settings: %w", err)
	}
	return settings, nil
}

func (s *SQLiteStore) GetTopologyLookupCache() (model.TopologyLookupCache, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, topologyLookupCacheKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return model.TopologyLookupCache{}, false, nil
	}
	if err != nil {
		return model.TopologyLookupCache{}, false, fmt.Errorf("load topology lookup cache: %w", err)
	}
	var cache model.TopologyLookupCache
	if err := json.Unmarshal([]byte(raw), &cache); err != nil {
		return model.TopologyLookupCache{}, false, fmt.Errorf("decode topology lookup cache: %w", err)
	}
	return normalizeTopologyLookupCache(cache), true, nil
}

func (s *SQLiteStore) SaveTopologyLookupCache(cache model.TopologyLookupCache) error {
	cache = normalizeTopologyLookupCache(cache)
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("encode topology lookup cache: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, topologyLookupCacheKey, string(data), now)
	if err != nil {
		return fmt.Errorf("save topology lookup cache: %w", err)
	}
	return nil
}

func normalizeTopologyLookupCache(cache model.TopologyLookupCache) model.TopologyLookupCache {
	now := time.Now().UTC()
	if len(cache.Hosts) > 0 {
		hosts := make(map[string]model.TopologyHostCacheEntry, len(cache.Hosts))
		for key, entry := range cache.Hosts {
			key = strings.TrimSpace(strings.ToLower(key))
			if key == "" || entry.ExpiresAt.Before(now) {
				continue
			}
			hosts[key] = entry
		}
		cache.Hosts = hosts
	}
	if len(cache.Geos) > 0 {
		geos := make(map[string]model.TopologyGeoCacheEntry, len(cache.Geos))
		for key, entry := range cache.Geos {
			key = strings.TrimSpace(key)
			if key == "" || entry.ExpiresAt.Before(now) {
				continue
			}
			if entry.Geo.IP == "" {
				entry.Geo.IP = key
			}
			geos[key] = entry
		}
		cache.Geos = geos
	}
	return cache
}

type outboundLinkLibraryPayload struct {
	Items []model.OutboundLinkLibraryItem `json:"items"`
}

func (s *SQLiteStore) ListOutboundLinkLibraryItems() ([]model.OutboundLinkLibraryItem, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, outboundLinkLibraryKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return []model.OutboundLinkLibraryItem{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load outbound link library: %w", err)
	}
	var payload outboundLinkLibraryPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("decode outbound link library: %w", err)
	}
	return normalizeOutboundLinkLibraryItems(payload.Items), nil
}

func (s *SQLiteStore) SaveOutboundLinkLibraryItem(item model.OutboundLinkLibraryItem) (model.OutboundLinkLibraryItem, error) {
	items, err := s.ListOutboundLinkLibraryItems()
	if err != nil {
		return model.OutboundLinkLibraryItem{}, err
	}
	now := time.Now().UTC()
	item = normalizeOutboundLinkLibraryItem(item)
	if item.ID == "" {
		item.ID = randomSettingID()
		item.CreatedAt = now
	} else {
		for _, existing := range items {
			if existing.ID == item.ID {
				item.CreatedAt = existing.CreatedAt
				break
			}
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = now
		}
	}
	item.UpdatedAt = now
	replaced := false
	for index := range items {
		if items[index].ID == item.ID {
			items[index] = item
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, item)
	}
	if err := s.saveOutboundLinkLibraryItems(items); err != nil {
		return model.OutboundLinkLibraryItem{}, err
	}
	return item, nil
}

func (s *SQLiteStore) DeleteOutboundLinkLibraryItem(id string) error {
	id = strings.TrimSpace(id)
	items, err := s.ListOutboundLinkLibraryItems()
	if err != nil {
		return err
	}
	next := items[:0]
	for _, item := range items {
		if item.ID != id {
			next = append(next, item)
		}
	}
	return s.saveOutboundLinkLibraryItems(next)
}

func (s *SQLiteStore) saveOutboundLinkLibraryItems(items []model.OutboundLinkLibraryItem) error {
	payload := outboundLinkLibraryPayload{Items: normalizeOutboundLinkLibraryItems(items)}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode outbound link library: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, outboundLinkLibraryKey, string(data), now)
	if err != nil {
		return fmt.Errorf("save outbound link library: %w", err)
	}
	return nil
}

func normalizeOutboundLinkLibraryItems(items []model.OutboundLinkLibraryItem) []model.OutboundLinkLibraryItem {
	result := make([]model.OutboundLinkLibraryItem, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = normalizeOutboundLinkLibraryItem(item)
		if item.ID == "" || item.Tag == "" || item.Protocol == "" || item.Outbound == nil {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func normalizeOutboundLinkLibraryItem(item model.OutboundLinkLibraryItem) model.OutboundLinkLibraryItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Tag = strings.TrimSpace(item.Tag)
	item.Protocol = strings.ToLower(strings.TrimSpace(item.Protocol))
	if item.Outbound != nil {
		if item.Tag == "" {
			if tag, _ := item.Outbound["tag"].(string); tag != "" {
				item.Tag = strings.TrimSpace(tag)
			}
		}
		if item.Protocol == "" {
			if protocol, _ := item.Outbound["protocol"].(string); protocol != "" {
				item.Protocol = strings.ToLower(strings.TrimSpace(protocol))
			}
		}
	}
	if item.Name == "" {
		item.Name = firstNonEmpty(item.Tag, item.Protocol, "outbound")
	}
	return item
}

func randomSettingID() string {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buf)
}
