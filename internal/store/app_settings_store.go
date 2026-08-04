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
	exchangeRatesCacheKey    = "exchange_rates_cache"
	tagSettingsKey           = "tag_settings"
	frontendSettingsKey      = "frontend_settings"
	scheduledTasksKey        = "scheduled_tasks"
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
	settings.RealmVersion = strings.TrimSpace(settings.RealmVersion)
	settings.RealmDownloadBaseURL = strings.TrimRight(strings.TrimSpace(settings.RealmDownloadBaseURL), "/")
	settings.XUIUsername = strings.TrimSpace(settings.XUIUsername)
	settings.XUIPassword = strings.TrimSpace(settings.XUIPassword)
	settings.XUIWebPath = normalizeXUIBootstrapWebPath(settings.XUIWebPath)
	settings.XUIInstallScriptURL = strings.TrimSpace(settings.XUIInstallScriptURL)
	if settings.HAProxyAutoInstall {
		settings.RealmAutoInstall = false
	}
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

func (s *SQLiteStore) GetExchangeRatesCache() (model.ExchangeRatesResponse, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, exchangeRatesCacheKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return model.ExchangeRatesResponse{}, false, nil
	}
	if err != nil {
		return model.ExchangeRatesResponse{}, false, fmt.Errorf("load exchange rates cache: %w", err)
	}
	var cached model.ExchangeRatesResponse
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		return model.ExchangeRatesResponse{}, false, fmt.Errorf("decode exchange rates cache: %w", err)
	}
	cached = normalizeExchangeRatesCache(cached)
	if cached.FetchedAt.IsZero() || len(cached.Rates) == 0 {
		return model.ExchangeRatesResponse{}, false, nil
	}
	return cached, true, nil
}

func (s *SQLiteStore) SaveExchangeRatesCache(rates model.ExchangeRatesResponse) error {
	rates = normalizeExchangeRatesCache(rates)
	rates.Stale = false
	rates.Error = ""
	if rates.FetchedAt.IsZero() {
		rates.FetchedAt = time.Now().UTC()
	}
	data, err := json.Marshal(rates)
	if err != nil {
		return fmt.Errorf("encode exchange rates cache: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, exchangeRatesCacheKey, string(data), now)
	if err != nil {
		return fmt.Errorf("save exchange rates cache: %w", err)
	}
	return nil
}

func normalizeExchangeRatesCache(rates model.ExchangeRatesResponse) model.ExchangeRatesResponse {
	rates.Base = strings.ToUpper(strings.TrimSpace(rates.Base))
	if rates.Base == "" {
		rates.Base = "EUR"
	}
	rates.Date = strings.TrimSpace(rates.Date)
	rates.Source = strings.TrimSpace(rates.Source)
	normalized := map[string]float64{}
	for currency, rate := range rates.Rates {
		currency = strings.ToUpper(strings.TrimSpace(currency))
		if len(currency) == 3 && rate > 0 {
			normalized[currency] = rate
		}
	}
	if rates.Base == "EUR" {
		normalized["EUR"] = 1
	}
	rates.Rates = normalized
	return rates
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
	settings.Announcements = normalizeCustomerAnnouncements(settings.Announcements, false)
	return settings, true, nil
}

func (s *SQLiteStore) SaveFrontendSettings(settings model.FrontendSettings) (model.FrontendSettings, error) {
	settings.Announcements = normalizeCustomerAnnouncements(settings.Announcements, true)
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

func normalizeCustomerAnnouncements(items []model.CustomerAnnouncement, generateID bool) []model.CustomerAnnouncement {
	if len(items) == 0 {
		return nil
	}
	normalized := make([]model.CustomerAnnouncement, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" && generateID {
			if generated, err := randomTokenBytes(12); err == nil {
				item.ID = generated
			}
		}
		if item.ID == "" {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		item.Title = strings.TrimSpace(item.Title)
		item.Content = strings.TrimSpace(item.Content)
		item.LinkLabel = strings.TrimSpace(item.LinkLabel)
		item.LinkURL = strings.TrimSpace(item.LinkURL)
		item.StartsAt = normalizeAnnouncementTime(item.StartsAt)
		item.EndsAt = normalizeAnnouncementTime(item.EndsAt)
		switch strings.ToLower(strings.TrimSpace(item.Level)) {
		case "success", "warning", "error":
			item.Level = strings.ToLower(strings.TrimSpace(item.Level))
		default:
			item.Level = "info"
		}
		if item.Title == "" && item.Content == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeAnnouncementTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339)
}

func (s *SQLiteStore) GetScheduledTaskSettings() (model.ScheduledTaskSettings, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, scheduledTasksKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return defaultScheduledTaskSettings(), false, nil
	}
	if err != nil {
		return model.ScheduledTaskSettings{}, false, fmt.Errorf("load scheduled task settings: %w", err)
	}
	var settings model.ScheduledTaskSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.ScheduledTaskSettings{}, false, fmt.Errorf("decode scheduled task settings: %w", err)
	}
	return normalizeScheduledTaskSettings(settings), true, nil
}

func (s *SQLiteStore) SaveScheduledTaskSettings(settings model.ScheduledTaskSettings) (model.ScheduledTaskSettings, error) {
	settings = normalizeScheduledTaskSettings(settings)
	data, err := json.Marshal(settings)
	if err != nil {
		return model.ScheduledTaskSettings{}, fmt.Errorf("encode scheduled task settings: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, scheduledTasksKey, string(data), now)
	if err != nil {
		return model.ScheduledTaskSettings{}, fmt.Errorf("save scheduled task settings: %w", err)
	}
	return settings, nil
}

func defaultScheduledTaskSettings() model.ScheduledTaskSettings {
	return model.ScheduledTaskSettings{
		AlertSweep: model.ScheduledTaskConfig{
			Enabled:         true,
			IntervalMinutes: 5,
		},
		DailyTrafficReport: model.ScheduledTaskConfig{
			Enabled:      true,
			TimeOfDay:    "09:00",
			IntervalDays: 1,
		},
	}
}

func normalizeScheduledTaskSettings(settings model.ScheduledTaskSettings) model.ScheduledTaskSettings {
	defaults := defaultScheduledTaskSettings()
	if settings.AlertSweep.IntervalMinutes <= 0 {
		settings.AlertSweep.IntervalMinutes = defaults.AlertSweep.IntervalMinutes
	}
	if settings.AlertSweep.IntervalMinutes > 24*60 {
		settings.AlertSweep.IntervalMinutes = 24 * 60
	}
	if settings.DailyTrafficReport.IntervalDays <= 0 {
		settings.DailyTrafficReport.IntervalDays = defaults.DailyTrafficReport.IntervalDays
	}
	if settings.DailyTrafficReport.IntervalDays > 365 {
		settings.DailyTrafficReport.IntervalDays = 365
	}
	settings.DailyTrafficReport.TimeOfDay = normalizeTaskTimeOfDay(settings.DailyTrafficReport.TimeOfDay, defaults.DailyTrafficReport.TimeOfDay)
	return settings
}

func normalizeTaskTimeOfDay(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return fallback
	}
	return parsed.Format("15:04")
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
