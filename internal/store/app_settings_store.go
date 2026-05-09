package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const clientInstallSettingsKey = "client_install"

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
	return normalizeClientInstallSettings(settings), true, nil
}

func (s *SQLiteStore) SaveClientInstallSettings(settings model.ClientInstallSettingsRequest) (model.ClientInstallSettingsRequest, error) {
	settings = normalizeClientInstallSettings(settings)
	data, err := json.Marshal(settings)
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

func normalizeClientInstallSettings(settings model.ClientInstallSettingsRequest) model.ClientInstallSettingsRequest {
	settings.ServerURL = strings.TrimSpace(settings.ServerURL)
	settings.InstallScriptURL = strings.TrimSpace(settings.InstallScriptURL)
	settings.PollInterval = strings.TrimSpace(settings.PollInterval)
	if settings.RequestTimeoutSeconds < 0 {
		settings.RequestTimeoutSeconds = 0
	}
	return settings
}
