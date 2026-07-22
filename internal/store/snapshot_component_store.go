package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const (
	snapshotComponentXUI           = "xui"
	snapshotComponentRealm         = "realm"
	snapshotComponentNetworkPolicy = "network_policy"
)

type snapshotComponentState struct {
	name    string
	present bool
	payload any
}

type snapshotXUIConfiguration struct {
	BaseURL      string                      `json:"base_url,omitempty"`
	AppVersion   string                      `json:"app_version,omitempty"`
	Inbounds     []map[string]any            `json:"inbounds,omitempty"`
	Outbounds    []map[string]any            `json:"outbounds,omitempty"`
	RoutingRules []map[string]any            `json:"routing_rules,omitempty"`
	Certificates []model.XUILocalCertificate `json:"certificates,omitempty"`
}

type snapshotRealmConfiguration struct {
	ConfigPath  string                   `json:"config_path,omitempty"`
	ServiceName string                   `json:"service_name,omitempty"`
	BinaryPath  string                   `json:"binary_path,omitempty"`
	Rules       []model.RealmForwardRule `json:"rules,omitempty"`
}

type snapshotNetworkPolicyConfiguration struct {
	Interface        string                        `json:"interface,omitempty"`
	FirewallBackend  string                        `json:"firewall_backend,omitempty"`
	RateLimitBackend string                        `json:"rate_limit_backend,omitempty"`
	Rules            []model.NetworkPortPolicyRule `json:"rules,omitempty"`
}

func (s *SQLiteStore) saveSnapshotComponentEventsTx(tx *sql.Tx, snapshot model.AgentSnapshot) error {
	for _, state := range snapshotComponentStates(snapshot) {
		payload := []byte("null")
		if state.present {
			encoded, err := json.Marshal(state.payload)
			if err != nil {
				return fmt.Errorf("marshal %s snapshot component: %w", state.name, err)
			}
			payload = encoded
		}
		digest := sha256.Sum256(payload)
		contentHash := hex.EncodeToString(digest[:])

		var previousHash string
		err := tx.QueryRow(`
			SELECT content_hash
			FROM snapshot_component_events
			WHERE agent_id = ? AND component = ?
			ORDER BY id DESC
			LIMIT 1
		`, snapshot.AgentID, state.name).Scan(&previousHash)
		if err == sql.ErrNoRows && !state.present {
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("load latest %s snapshot component: %w", state.name, err)
		}
		if previousHash == contentHash {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO snapshot_component_events (agent_id, component, observed_at, content_hash, payload_json)
			VALUES (?, ?, ?, ?, ?)
		`, snapshot.AgentID, state.name, snapshot.ReportedAt.Format(time.RFC3339Nano), contentHash, string(payload)); err != nil {
			return fmt.Errorf("save %s snapshot component event: %w", state.name, err)
		}
	}
	return nil
}

func snapshotComponentStates(snapshot model.AgentSnapshot) []snapshotComponentState {
	states := make([]snapshotComponentState, 0, 3)
	if snapshot.XUI == nil {
		states = append(states, snapshotComponentState{name: snapshotComponentXUI})
	} else if snapshot.XUI.Error == "" {
		states = append(states, snapshotComponentState{
			name:    snapshotComponentXUI,
			present: true,
			payload: snapshotXUIConfiguration{
				BaseURL:      snapshot.XUI.BaseURL,
				AppVersion:   snapshot.XUI.AppVersion,
				Inbounds:     sanitizeXUIConfigurationMaps(snapshot.XUI.Inbounds),
				Outbounds:    sanitizeXUIConfigurationMaps(snapshot.XUI.Outbounds),
				RoutingRules: sanitizeXUIConfigurationMaps(snapshot.XUI.RoutingRules),
				Certificates: append([]model.XUILocalCertificate(nil), snapshot.XUI.Certificates...),
			},
		})
	}
	if snapshot.Realm == nil {
		states = append(states, snapshotComponentState{name: snapshotComponentRealm})
	} else if snapshot.Realm.Error == "" {
		states = append(states, snapshotComponentState{
			name:    snapshotComponentRealm,
			present: true,
			payload: snapshotRealmConfiguration{
				ConfigPath:  snapshot.Realm.ConfigPath,
				ServiceName: snapshot.Realm.ServiceName,
				BinaryPath:  snapshot.Realm.BinaryPath,
				Rules:       append([]model.RealmForwardRule(nil), snapshot.Realm.Rules...),
			},
		})
	}
	if snapshot.NetworkPolicy == nil {
		states = append(states, snapshotComponentState{name: snapshotComponentNetworkPolicy})
	} else if snapshot.NetworkPolicy.Error == "" {
		states = append(states, snapshotComponentState{
			name:    snapshotComponentNetworkPolicy,
			present: true,
			payload: snapshotNetworkPolicyConfiguration{
				Interface:        snapshot.NetworkPolicy.Interface,
				FirewallBackend:  snapshot.NetworkPolicy.FirewallBackend,
				RateLimitBackend: snapshot.NetworkPolicy.RateLimitBackend,
				Rules:            append([]model.NetworkPortPolicyRule(nil), snapshot.NetworkPolicy.Rules...),
			},
		})
	}
	return states
}

func sanitizeXUIConfigurationMaps(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		sanitized, _ := sanitizeXUIConfigurationValue(item).(map[string]any)
		result = append(result, sanitized)
	}
	return result
}

func sanitizeXUIConfigurationValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isXUIRuntimeField(key) {
				continue
			}
			result[key] = sanitizeXUIConfigurationValue(child)
		}
		return result
	case []map[string]any:
		result := make([]map[string]any, 0, len(typed))
		for _, child := range typed {
			sanitized, _ := sanitizeXUIConfigurationValue(child).(map[string]any)
			result = append(result, sanitized)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, sanitizeXUIConfigurationValue(child))
		}
		return result
	default:
		return value
	}
}

func isXUIRuntimeField(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "up", "down", "alltime", "lastonline", "online", "upload", "download", "uplink", "downlink", "sent", "recv", "tx", "rx":
		return true
	default:
		return false
	}
}
