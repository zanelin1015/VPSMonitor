package client

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const haProxyRuntimeReadLimit = 4 << 20

type haProxyRuntimeStat struct {
	Status            string
	CurrentSessions   int64
	TotalSessions     uint64
	CheckStatus       string
	CheckDescription  string
	CheckDurationMS   int64
	LastChangeSeconds int64
	DowntimeSeconds   int64
}

func haProxyRuntimeSocketPath(cfg model.HAProxyConfig) string {
	serviceName := sanitizeHAProxyName(firstNonEmpty(strings.TrimSpace(cfg.ServiceName), defaultHAProxyServiceName))
	return "/run/" + serviceName + ".sock"
}

func collectHAProxySnapshot(ctx context.Context, cfg model.HAProxyConfig) *model.HAProxySnapshot {
	cfg = normalizeClientHAProxyConfig(cfg)
	if !cfg.Enabled {
		return nil
	}
	snapshot := newHAProxySnapshot(cfg)
	if haProxyRuntimeGOOS != "linux" {
		snapshot.Error = "HAProxy runtime status is only available on Linux"
		return snapshot
	}
	if len(snapshot.Rules) == 0 {
		return snapshot
	}

	body, err := queryHAProxyRuntimeStats(ctx, snapshot.SocketPath)
	if err != nil {
		snapshot.Error = fmt.Sprintf("read HAProxy runtime status: %v", err)
		return snapshot
	}
	stats, err := parseHAProxyRuntimeStats(body)
	if err != nil {
		snapshot.Error = fmt.Sprintf("parse HAProxy runtime status: %v", err)
		return snapshot
	}
	populateHAProxySnapshot(cfg, snapshot, stats)
	return snapshot
}

func newHAProxySnapshot(cfg model.HAProxyConfig) *model.HAProxySnapshot {
	snapshot := &model.HAProxySnapshot{
		CollectedAt: time.Now().UTC(),
		SocketPath:  haProxyRuntimeSocketPath(cfg),
		Rules:       make([]model.HAProxyRuleRuntimeStatus, 0),
	}
	for _, rule := range activeHAProxyRules(cfg.Rules) {
		targets := make([]model.HAProxyTargetRuntimeStatus, 0, 1+len(rule.Backups))
		targets = append(targets, newHAProxyTargetRuntimeStatus("primary", 0, "primary", rule.Primary))
		for index, target := range rule.Backups {
			targets = append(targets, newHAProxyTargetRuntimeStatus("backup", index+1, fmt.Sprintf("backup_%d", index+1), target))
		}
		snapshot.Rules = append(snapshot.Rules, model.HAProxyRuleRuntimeStatus{
			RuleID:     rule.ID,
			Name:       rule.Name,
			ListenPort: rule.ListenPort,
			Status:     "unknown",
			Targets:    targets,
		})
	}
	return snapshot
}

func newHAProxyTargetRuntimeStatus(role string, backupIndex int, prefix string, target model.HAProxyRealmTarget) model.HAProxyTargetRuntimeStatus {
	return model.HAProxyTargetRuntimeStatus{
		Role:        role,
		BackupIndex: backupIndex,
		AgentID:     target.AgentID,
		RealmRuleID: target.RealmRuleID,
		Address:     target.Address,
		Port:        target.Port,
		ServerName:  haProxyTargetRuntimeName(prefix, target),
		Status:      "UNKNOWN",
	}
}

func queryHAProxyRuntimeStats(ctx context.Context, socketPath string) ([]byte, error) {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(1500 * time.Millisecond)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(conn, "show stat\n"); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(conn, haProxyRuntimeReadLimit))
	if err != nil && len(body) == 0 {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty response from %s", socketPath)
	}
	return body, nil
}

func parseHAProxyRuntimeStats(body []byte) (map[string]haProxyRuntimeStat, error) {
	reader := csv.NewReader(strings.NewReader(string(body)))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("missing CSV header")
	}

	header := make(map[string]int, len(records[0]))
	for index, name := range records[0] {
		name = strings.TrimSpace(strings.TrimPrefix(name, "#"))
		header[name] = index
	}
	if _, ok := header["pxname"]; !ok {
		return nil, fmt.Errorf("missing pxname column")
	}
	if _, ok := header["svname"]; !ok {
		return nil, fmt.Errorf("missing svname column")
	}

	stats := make(map[string]haProxyRuntimeStat)
	for _, record := range records[1:] {
		backend := haProxyCSVValue(record, header, "pxname")
		server := haProxyCSVValue(record, header, "svname")
		if backend == "" || server == "" || server == "FRONTEND" || server == "BACKEND" {
			continue
		}
		if rowType := haProxyCSVValue(record, header, "type"); rowType != "" && rowType != "2" {
			continue
		}
		stats[haProxyRuntimeStatKey(backend, server)] = haProxyRuntimeStat{
			Status:            firstNonEmpty(haProxyCSVValue(record, header, "status"), "UNKNOWN"),
			CurrentSessions:   parseHAProxyInt64(haProxyCSVValue(record, header, "scur")),
			TotalSessions:     parseHAProxyUint64(haProxyCSVValue(record, header, "stot")),
			CheckStatus:       haProxyCSVValue(record, header, "check_status"),
			CheckDescription:  firstNonEmpty(haProxyCSVValue(record, header, "last_chk"), haProxyCSVValue(record, header, "check_desc")),
			CheckDurationMS:   parseHAProxyInt64(haProxyCSVValue(record, header, "check_duration")),
			LastChangeSeconds: parseHAProxyInt64(haProxyCSVValue(record, header, "lastchg")),
			DowntimeSeconds:   parseHAProxyInt64(haProxyCSVValue(record, header, "downtime")),
		}
	}
	return stats, nil
}

func populateHAProxySnapshot(cfg model.HAProxyConfig, snapshot *model.HAProxySnapshot, stats map[string]haProxyRuntimeStat) {
	rules := activeHAProxyRules(cfg.Rules)
	for ruleIndex := range snapshot.Rules {
		if ruleIndex >= len(rules) {
			break
		}
		ruleStatus := &snapshot.Rules[ruleIndex]
		backendName := haProxyRuleRuntimeName(rules[ruleIndex], ruleIndex) + "_backend"
		allKnown := len(ruleStatus.Targets) > 0
		activeTargetIndex := -1
		for targetIndex := range ruleStatus.Targets {
			target := &ruleStatus.Targets[targetIndex]
			stat, ok := stats[haProxyRuntimeStatKey(backendName, target.ServerName)]
			if !ok {
				allKnown = false
				continue
			}
			target.Status = strings.ToUpper(strings.TrimSpace(stat.Status))
			target.Healthy = haProxyRuntimeStatusHealthy(target.Status)
			target.CurrentSessions = stat.CurrentSessions
			target.TotalSessions = stat.TotalSessions
			target.CheckStatus = stat.CheckStatus
			target.CheckDescription = stat.CheckDescription
			target.CheckDurationMS = stat.CheckDurationMS
			target.LastChangeSeconds = stat.LastChangeSeconds
			target.TotalDowntimeSecs = stat.DowntimeSeconds
			if activeTargetIndex < 0 && target.Healthy {
				activeTargetIndex = targetIndex
			}
		}

		if activeTargetIndex >= 0 {
			active := &ruleStatus.Targets[activeTargetIndex]
			active.Active = true
			ruleStatus.ActiveRole = active.Role
			ruleStatus.ActiveBackupIndex = active.BackupIndex
			ruleStatus.ActiveAgentID = active.AgentID
			if active.Role == "primary" {
				ruleStatus.Status = "primary"
			} else {
				ruleStatus.Status = "backup"
			}
		} else if allKnown {
			ruleStatus.Status = "unavailable"
		}
	}
}

func haProxyRuntimeStatKey(backend, server string) string {
	return strings.ToLower(strings.TrimSpace(backend)) + "\x00" + strings.ToLower(strings.TrimSpace(server))
}

func haProxyRuntimeStatusHealthy(status string) bool {
	status = strings.ToUpper(strings.TrimSpace(status))
	return status == "UP" || strings.HasPrefix(status, "UP ")
}

func haProxyCSVValue(record []string, header map[string]int, name string) string {
	index, ok := header[name]
	if !ok || index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func parseHAProxyInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func parseHAProxyUint64(value string) uint64 {
	parsed, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return parsed
}
