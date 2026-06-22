package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
)

const (
	accessLogMaxReadBytes = 1 << 20
	accessLogMaxBatch     = 300
)

var (
	xrayAccessLogPattern  = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})\s+(\S+)\s+(\S+)\s+(.+)$`)
	accessLogEmailPattern = regexp.MustCompile(`\bemail:\s*([^\s]+)`)
	accessLogTagPattern   = regexp.MustCompile(`\[([^\]]+)\]`)
)

type accessLogTailState struct {
	path        string
	offset      int64
	initialized bool
}

func (a *App) collectAndPushAccessLogs(ctx context.Context, cfg config.XUIConfig) {
	if !cfg.AccessLogEnabled || strings.TrimSpace(cfg.AccessLogPath) == "" {
		a.accessLogState = accessLogTailState{}
		return
	}
	entries, err := a.readAccessLogEntries(cfg.AccessLogPath)
	if err != nil {
		log.Printf("read xray access log failed: %v", err)
		return
	}
	if len(entries) == 0 {
		return
	}
	pushCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	defer cancel()
	if err := a.pushAccessLogs(pushCtx, entries); err != nil {
		log.Printf("push xray access logs failed: %v", err)
	}
}

func (a *App) readAccessLogEntries(path string) ([]model.AccessLogEntry, error) {
	path = strings.TrimSpace(path)
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}
	if a.accessLogState.path != path {
		a.accessLogState = accessLogTailState{path: path, offset: stat.Size(), initialized: true}
		return nil, nil
	}
	if !a.accessLogState.initialized {
		a.accessLogState = accessLogTailState{path: path, offset: stat.Size(), initialized: true}
		return nil, nil
	}
	if stat.Size() < a.accessLogState.offset {
		a.accessLogState.offset = 0
	}
	if stat.Size() == a.accessLogState.offset {
		return nil, nil
	}
	readSize := stat.Size() - a.accessLogState.offset
	offset := a.accessLogState.offset
	if readSize > accessLogMaxReadBytes {
		offset = stat.Size() - accessLogMaxReadBytes
		readSize = accessLogMaxReadBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data := make([]byte, int(readSize))
	n, err := io.ReadFull(file, data)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	a.accessLogState.offset = stat.Size()
	return parseXrayAccessLogEntries(data[:n], accessLogMaxBatch), nil
}

func (a *App) pushAccessLogs(ctx context.Context, entries []model.AccessLogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	body, err := json.Marshal(model.AccessLogBatchRequest{Entries: entries})
	if err != nil {
		return fmt.Errorf("marshal access logs: %w", err)
	}
	url := strings.TrimRight(a.config.ServerURL, "/") + "/api/v1/agents/" + a.config.AgentID + "/access-logs"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build access log request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", firstNonEmpty(a.currentAgentToken(), a.config.AgentToken))
	var response map[string]any
	if err := a.doJSON(req, &response); err != nil {
		return fmt.Errorf("upload access logs: %w", err)
	}
	return nil
}

func parseXrayAccessLogEntries(data []byte, maxEntries int) []model.AccessLogEntry {
	if maxEntries <= 0 {
		maxEntries = accessLogMaxBatch
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	entries := make([]model.AccessLogEntry, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		entry, ok := parseXrayAccessLogLine(line)
		if !ok {
			continue
		}
		entries = append(entries, entry)
		if len(entries) >= maxEntries {
			break
		}
	}
	return entries
}

func parseXrayAccessLogLine(line string) (model.AccessLogEntry, bool) {
	matches := xrayAccessLogPattern.FindStringSubmatch(line)
	if len(matches) != 5 {
		return model.AccessLogEntry{}, false
	}
	createdAt, _ := time.ParseInLocation("2006/01/02 15:04:05", matches[1], time.Local)
	sourceIP, sourcePort := splitAccessLogHostPort(matches[2])
	action := strings.ToLower(strings.TrimSpace(matches[3]))
	detail := strings.TrimSpace(matches[4])
	if action != "accepted" {
		return model.AccessLogEntry{}, false
	}
	network, targetHost, targetPort := parseAccessLogTarget(detail)
	if network == "" && targetHost == "" {
		return model.AccessLogEntry{}, false
	}
	entry := model.AccessLogEntry{
		SourceIP:    sourceIP,
		SourcePort:  sourcePort,
		TargetHost:  targetHost,
		TargetPort:  targetPort,
		Network:     network,
		OutboundTag: firstRegexpGroup(accessLogTagPattern, detail),
		ClientEmail: firstRegexpGroup(accessLogEmailPattern, detail),
		RawSummary:  line,
		CreatedAt:   accessLogTimeUTC(createdAt),
		StartedAt:   accessLogTimeUTC(createdAt),
	}
	if net.ParseIP(entry.TargetHost) != nil {
		entry.TargetIP = entry.TargetHost
		entry.TargetHost = ""
	}
	return entry, true
}

func accessLogTimeUTC(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}

func parseAccessLogTarget(detail string) (string, string, int) {
	fields := strings.Fields(detail)
	if len(fields) == 0 {
		return "", "", 0
	}
	target := fields[0]
	if strings.Contains(target, "://") {
		parts := strings.SplitN(target, "://", 2)
		target = parts[1]
	}
	network := ""
	if idx := strings.Index(target, ":"); idx > 0 {
		prefix := strings.ToLower(target[:idx])
		if prefix == "tcp" || prefix == "udp" {
			network = prefix
			target = target[idx+1:]
		}
	}
	host, port := splitAccessLogHostPort(target)
	return network, host, port
}

func splitAccessLogHostPort(value string) (string, int) {
	value = strings.Trim(value, "[] ")
	if value == "" {
		return "", 0
	}
	if host, portText, err := net.SplitHostPort(value); err == nil {
		port, _ := strconv.Atoi(portText)
		return strings.Trim(host, "[]"), port
	}
	idx := strings.LastIndex(value, ":")
	if idx <= 0 || idx == len(value)-1 {
		return value, 0
	}
	port, err := strconv.Atoi(value[idx+1:])
	if err != nil {
		return value, 0
	}
	return strings.Trim(value[:idx], "[]"), port
}

func firstRegexpGroup(pattern *regexp.Regexp, value string) string {
	matches := pattern.FindStringSubmatch(value)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}
