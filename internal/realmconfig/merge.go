package realmconfig

import (
	"fmt"
	"strings"

	"bridge-core/internal/model"
)

// MergeSnapshotIntoEntry treats the realm config collected from the VPS as the
// source of truth, while keeping operator-only metadata such as target_agent_id.
func MergeSnapshotIntoEntry(entry model.AgentEntryConfig, snapshot *model.RealmSnapshot) model.AgentEntryConfig {
	entry.PortForwarding = MergeSnapshotIntoForwardConfig(entry.PortForwarding, snapshot)
	return entry
}

func MergeSnapshotIntoForwardConfig(cfg model.RealmForwardConfig, snapshot *model.RealmSnapshot) model.RealmForwardConfig {
	if snapshot == nil || len(snapshot.Rules) == 0 || strings.EqualFold(strings.TrimSpace(cfg.Backend), "none") {
		return cfg
	}
	merged := cfg
	merged.Enabled = true
	merged.Backend = "realm"
	if strings.TrimSpace(snapshot.ConfigPath) != "" {
		merged.ConfigPath = strings.TrimSpace(snapshot.ConfigPath)
	}
	if strings.TrimSpace(snapshot.ServiceName) != "" {
		merged.ServiceName = strings.TrimSpace(snapshot.ServiceName)
	}
	if strings.TrimSpace(snapshot.BinaryPath) != "" {
		merged.BinaryPath = strings.TrimSpace(snapshot.BinaryPath)
	}

	existingByListen := make(map[string]model.RealmForwardRule, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		normalized, ok := normalizeRule(rule)
		if !ok {
			continue
		}
		existingByListen[listenKey(normalized)] = normalized
	}

	rules := make([]model.RealmForwardRule, 0, len(snapshot.Rules)+len(cfg.Rules))
	seenListen := make(map[string]struct{}, len(snapshot.Rules)+len(cfg.Rules))
	appendRule := func(rule model.RealmForwardRule) {
		normalized, ok := normalizeRule(rule)
		if !ok {
			return
		}
		key := listenKey(normalized)
		if _, exists := seenListen[key]; exists {
			return
		}
		seenListen[key] = struct{}{}
		rules = append(rules, normalized)
	}

	for _, rule := range snapshot.Rules {
		normalized, ok := normalizeRule(rule)
		if !ok {
			continue
		}
		if existing, exists := existingByListen[listenKey(normalized)]; exists {
			normalized.Name = firstNonEmpty(existing.Name, normalized.Name)
			normalized.TargetAgentID = firstNonEmpty(normalized.TargetAgentID, existing.TargetAgentID)
			normalized.Note = firstNonEmpty(normalized.Note, existing.Note)
			if isGeneratedRealmRuleID(normalized.ID) && strings.TrimSpace(existing.ID) != "" {
				normalized.ID = strings.TrimSpace(existing.ID)
			}
		}
		appendRule(normalized)
	}
	for _, rule := range cfg.Rules {
		appendRule(rule)
	}
	merged.Rules = rules
	return merged
}

func normalizeRule(rule model.RealmForwardRule) (model.RealmForwardRule, bool) {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.ListenAddress = strings.TrimSpace(rule.ListenAddress)
	rule.TargetAgentID = strings.TrimSpace(rule.TargetAgentID)
	rule.TargetAddress = strings.TrimSpace(rule.TargetAddress)
	rule.Network = normalizeNetwork(rule.Network)
	rule.Note = strings.TrimSpace(rule.Note)
	if rule.ListenAddress == "" {
		rule.ListenAddress = "0.0.0.0"
	}
	if rule.ListenPort <= 0 || rule.ListenPort > 65535 || rule.TargetPort <= 0 || rule.TargetPort > 65535 {
		return model.RealmForwardRule{}, false
	}
	if rule.TargetAddress == "" && rule.TargetAgentID == "" {
		return model.RealmForwardRule{}, false
	}
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("realm-%d-%d-%s", rule.ListenPort, rule.TargetPort, rule.Network)
	}
	rule.Enabled = true
	return rule, true
}

func listenKey(rule model.RealmForwardRule) string {
	return fmt.Sprintf("%s:%d", strings.ToLower(strings.TrimSpace(rule.ListenAddress)), rule.ListenPort)
}

func normalizeNetwork(network string) string {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "udp":
		return "udp"
	case "both", "tcp+udp", "all":
		return "both"
	default:
		return "tcp"
	}
}

func isGeneratedRealmRuleID(id string) bool {
	id = strings.TrimSpace(id)
	return id == "" || strings.HasPrefix(id, "auto-realm-") || strings.HasPrefix(id, "realm-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
