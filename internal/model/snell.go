package model

import (
	"errors"
	"strconv"
	"strings"
)

// SnellConfig is a manually managed Snell endpoint. It is metadata for
// customer subscriptions; the VPSMonitor Client does not install or mutate a
// Snell service on the target host.
type SnellConfig struct {
	Enabled      bool               `json:"enabled,omitempty"`
	Name         string             `json:"name,omitempty"`
	Server       string             `json:"server,omitempty"`
	Port         int                `json:"port,omitempty"`
	PSK          string             `json:"psk,omitempty"`
	Version      int                `json:"version,omitempty"`
	ObfsMode     string             `json:"obfs_mode,omitempty"`
	ObfsHost     string             `json:"obfs_host,omitempty"`
	ObfsPath     string             `json:"obfs_path,omitempty"`
	ForwardRules []SnellForwardRule `json:"forward_rules,omitempty"`
}

type SnellForwardRule struct {
	ID                string `json:"id,omitempty"`
	Name              string `json:"name,omitempty"`
	Enabled           bool   `json:"enabled,omitempty"`
	TargetAgentID     string `json:"target_agent_id,omitempty"`
	TargetInboundID   int    `json:"target_inbound_id,omitempty"`
	TargetInboundTag  string `json:"target_inbound_tag,omitempty"`
	TargetClientID    string `json:"target_client_id,omitempty"`
	TargetClientEmail string `json:"target_client_email,omitempty"`
}

func NormalizeSnellConfig(cfg SnellConfig) SnellConfig {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Server = strings.TrimSpace(cfg.Server)
	cfg.PSK = strings.TrimSpace(cfg.PSK)
	cfg.ObfsMode = strings.ToLower(strings.TrimSpace(cfg.ObfsMode))
	cfg.ObfsHost = strings.TrimSpace(cfg.ObfsHost)
	cfg.ObfsPath = strings.TrimSpace(cfg.ObfsPath)
	for index := range cfg.ForwardRules {
		rule := &cfg.ForwardRules[index]
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			rule.ID = "snell-forward-" + strconv.Itoa(index+1)
		}
		rule.Name = strings.TrimSpace(rule.Name)
		rule.TargetAgentID = strings.TrimSpace(rule.TargetAgentID)
		rule.TargetInboundTag = strings.TrimSpace(rule.TargetInboundTag)
		rule.TargetClientID = strings.TrimSpace(rule.TargetClientID)
		rule.TargetClientEmail = strings.TrimSpace(rule.TargetClientEmail)
	}
	if cfg.Name == "" {
		cfg.Name = "Snell"
	}
	if cfg.Version < 1 || cfg.Version > 5 {
		cfg.Version = 5
	}
	if cfg.ObfsMode != "http" {
		cfg.ObfsMode = "none"
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		cfg.Port = 0
	}
	return cfg
}

func ValidateSnellConfig(cfg SnellConfig) error {
	cfg = NormalizeSnellConfig(cfg)
	if !cfg.Enabled {
		return nil
	}
	if cfg.Server == "" {
		return errors.New("Snell 需要填写服务器地址")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return errors.New("Snell 端口必须为 1–65535")
	}
	if cfg.PSK == "" {
		return errors.New("Snell 需要填写 PSK")
	}
	seen := make(map[string]struct{}, len(cfg.ForwardRules))
	for _, rule := range cfg.ForwardRules {
		if !rule.Enabled {
			continue
		}
		if rule.TargetAgentID == "" || (rule.TargetInboundID <= 0 && rule.TargetInboundTag == "") {
			return errors.New("Snell 转发需要填写目标 Client 和入站")
		}
		if rule.TargetClientID == "" && rule.TargetClientEmail == "" {
			return errors.New("Snell 转发需要选择目标 X-UI 客户端")
		}
		clientKey := rule.TargetClientID
		if clientKey == "" {
			clientKey = strings.ToLower(rule.TargetClientEmail)
		}
		key := strings.Join([]string{rule.TargetAgentID, strconv.Itoa(rule.TargetInboundID), rule.TargetInboundTag, clientKey}, "\x00")
		if _, exists := seen[key]; exists {
			return errors.New("Snell 转发目标不能重复")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func HasSnellConfig(cfg SnellConfig) bool {
	cfg = NormalizeSnellConfig(cfg)
	return cfg.Enabled || cfg.Server != "" || cfg.Port != 0 || cfg.PSK != "" || cfg.ObfsHost != "" || cfg.ObfsPath != "" || len(cfg.ForwardRules) > 0
}
