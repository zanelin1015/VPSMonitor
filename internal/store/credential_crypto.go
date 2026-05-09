package store

import (
	"crypto/aes"
	gocipher "crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bridge-core/internal/config"
)

const (
	credentialCipherKeyBytes = 32
	encryptedValuePrefix     = "enc:v1:"
)

type CredentialCipher struct {
	aead gocipher.AEAD
}

func LoadOrCreateCredentialCipher(path string) (*CredentialCipher, error) {
	key, err := loadOrCreateCredentialKey(path)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init credential cipher: %w", err)
	}
	aead, err := gocipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init credential gcm: %w", err)
	}
	return &CredentialCipher{aead: aead}, nil
}

func loadOrCreateCredentialKey(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("credential key path is required")
	}
	if data, err := os.ReadFile(path); err == nil {
		key, decodeErr := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if decodeErr != nil {
			return nil, fmt.Errorf("decode credential key %s: %w", path, decodeErr)
		}
		if len(key) != credentialCipherKeyBytes {
			return nil, fmt.Errorf("credential key %s must be %d bytes after base64 decode", path, credentialCipherKeyBytes)
		}
		return key, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read credential key %s: %w", path, err)
	}

	key, err := randomBytes(credentialCipherKeyBytes)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create credential key dir: %w", err)
	}
	data := []byte(base64.RawStdEncoding.EncodeToString(key) + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write credential key %s: %w", path, err)
	}
	return key, nil
}

func (c *CredentialCipher) EncryptString(value string) (string, error) {
	if c == nil || value == "" || strings.HasPrefix(value, encryptedValuePrefix) {
		return value, nil
	}
	nonce, err := randomBytes(c.aead.NonceSize())
	if err != nil {
		return "", err
	}
	ciphertext := c.aead.Seal(nil, nonce, []byte(value), nil)
	payload := append(nonce, ciphertext...)
	return encryptedValuePrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

func (c *CredentialCipher) DecryptString(value string) (string, error) {
	if c == nil || value == "" || !strings.HasPrefix(value, encryptedValuePrefix) {
		return value, nil
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encryptedValuePrefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted value: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", fmt.Errorf("encrypted value is too short")
	}
	plaintext, err := c.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt encrypted value: %w", err)
	}
	return string(plaintext), nil
}

func (s *SQLiteStore) storedXUIConfigJSON(cfg config.XUIConfig) (string, error) {
	stored := cfg
	password, err := s.secrets.EncryptString(stored.Password)
	if err != nil {
		return "", fmt.Errorf("encrypt x-ui password: %w", err)
	}
	stored.Password = password
	return mustJSON(stored), nil
}

func (s *SQLiteStore) decryptXUIConfig(cfg config.XUIConfig) (config.XUIConfig, error) {
	password, err := s.secrets.DecryptString(cfg.Password)
	if err != nil {
		return config.XUIConfig{}, err
	}
	cfg.Password = password
	return cfg, nil
}

func (s *SQLiteStore) encryptPlaintextCredentials() error {
	if s.secrets == nil {
		return nil
	}
	if err := s.encryptPlaintextXUICredentials(); err != nil {
		return err
	}
	if err := s.encryptPlaintextTelegramTokens(); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) encryptPlaintextXUICredentials() error {
	rows, err := s.db.Query(`SELECT agent_id, xui_config_json FROM agents`)
	if err != nil {
		return fmt.Errorf("query stored credentials: %w", err)
	}
	defer rows.Close()

	type update struct {
		agentID string
		xuiJSON string
	}
	var updates []update
	for rows.Next() {
		var agentID, rawJSON string
		if err := rows.Scan(&agentID, &rawJSON); err != nil {
			return fmt.Errorf("scan stored credentials: %w", err)
		}
		var cfg config.XUIConfig
		if rawJSON == "" || json.Unmarshal([]byte(rawJSON), &cfg) != nil {
			continue
		}
		if cfg.Password == "" || strings.HasPrefix(cfg.Password, encryptedValuePrefix) {
			continue
		}
		xuiJSON, err := s.storedXUIConfigJSON(cfg)
		if err != nil {
			return err
		}
		updates = append(updates, update{agentID: agentID, xuiJSON: xuiJSON})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate stored credentials: %w", err)
	}
	for _, item := range updates {
		if _, err := s.db.Exec(`UPDATE agents SET xui_config_json = ? WHERE agent_id = ?`, item.xuiJSON, item.agentID); err != nil {
			return fmt.Errorf("encrypt stored credentials for %s: %w", item.agentID, err)
		}
	}
	return nil
}

func (s *SQLiteStore) encryptPlaintextTelegramTokens() error {
	rows, err := s.db.Query(`SELECT id, bot_token FROM telegram_bots`)
	if err != nil {
		return fmt.Errorf("query stored telegram tokens: %w", err)
	}
	defer rows.Close()

	type update struct {
		id    int64
		token string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var token string
		if err := rows.Scan(&id, &token); err != nil {
			return fmt.Errorf("scan stored telegram token: %w", err)
		}
		if token == "" || strings.HasPrefix(token, encryptedValuePrefix) {
			continue
		}
		encrypted, err := s.secrets.EncryptString(token)
		if err != nil {
			return fmt.Errorf("encrypt telegram token: %w", err)
		}
		updates = append(updates, update{id: id, token: encrypted})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate stored telegram tokens: %w", err)
	}
	for _, item := range updates {
		if _, err := s.db.Exec(`UPDATE telegram_bots SET bot_token = ? WHERE id = ?`, item.token, item.id); err != nil {
			return fmt.Errorf("encrypt stored telegram token %d: %w", item.id, err)
		}
	}
	return nil
}
