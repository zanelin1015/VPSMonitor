package panels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bridge-core/internal/model"

	_ "modernc.org/sqlite"
)

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func decodeInboundSettings(raw any) (map[string]any, bool, error) {
	switch value := raw.(type) {
	case string:
		var settings map[string]any
		if strings.TrimSpace(value) == "" {
			return map[string]any{"clients": []map[string]any{}}, true, nil
		}
		if err := json.Unmarshal([]byte(value), &settings); err != nil {
			return nil, true, fmt.Errorf("decode inbound settings: %w", err)
		}
		return settings, true, nil
	case map[string]any:
		return value, false, nil
	default:
		return objectMap(raw), false, nil
	}
}

func (c *XUIClient) collectLocal(ctx context.Context, snapshot *model.XUISnapshot) error {
	dbPath, explicit, err := c.resolveLocalDBPath()
	if err != nil {
		if explicit {
			return fmt.Errorf("x-ui local db: %w", err)
		}
		return err
	}

	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(dbPath))
	if err != nil {
		return fmt.Errorf("open x-ui local db %s: %w", dbPath, err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("open x-ui local db %s: %w", dbPath, err)
	}

	inbounds, err := readLocalInbounds(ctx, db)
	if err != nil {
		return fmt.Errorf("read x-ui local inbounds: %w", err)
	}
	configJSON, err := readLocalXrayConfig(ctx, db)
	if err != nil {
		return fmt.Errorf("read x-ui local xray config: %w", err)
	}
	configJSON = c.enrichLocalXrayConfig(ctx, configJSON)
	outboundTraffic, err := readLocalOutboundTraffic(ctx, db)
	if err != nil {
		return fmt.Errorf("read x-ui local outbound traffic: %w", err)
	}

	snapshot.ServerStatus = c.localServerStatus()
	snapshot.Inbounds = inbounds
	snapshot.RawConfig = configJSON
	snapshot.Outbounds = extractObjectList(configJSON["outbounds"])
	snapshot.RoutingRules = extractRoutingRules(configJSON["routing"])
	snapshot.OutboundTraffic = outboundTraffic
	return nil
}

func (c *XUIClient) enrichLocalXrayConfig(ctx context.Context, localConfig map[string]any) map[string]any {
	if !xrayConfigNeedsFallback(localConfig) || c.baseURL == "" || (!c.hasAPIToken() && c.config.Username == "" && c.config.Password == "") {
		return localConfig
	}
	if err := c.ensureLogin(ctx); err != nil {
		return localConfig
	}
	remoteConfig, err := c.collectXrayConfig(ctx)
	if isXUIAuthError(err) {
		c.invalidateSession()
		if loginErr := c.login(ctx); loginErr == nil {
			remoteConfig, err = c.collectXrayConfig(ctx)
		}
	}
	if err != nil {
		return localConfig
	}
	return mergeRicherXrayConfig(localConfig, remoteConfig)
}

func (c *XUIClient) resolveLocalDBPath() (string, bool, error) {
	if path := strings.TrimSpace(c.config.DBPath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return path, true, err
		}
		return path, true, nil
	}
	if path := strings.TrimSpace(os.Getenv("XUI_DB_PATH")); path != "" {
		if _, err := os.Stat(path); err != nil {
			return path, true, err
		}
		return path, true, nil
	}
	if folder := strings.TrimSpace(os.Getenv("XUI_DB_FOLDER")); folder != "" {
		path := filepath.Join(folder, "x-ui.db")
		if _, err := os.Stat(path); err == nil {
			return path, false, nil
		}
	}
	for _, path := range defaultXUIDBPaths {
		if _, err := os.Stat(path); err == nil {
			return path, false, nil
		}
	}
	return "", false, errXUILocalDBNotFound
}

func sqliteReadOnlyDSN(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	return u.String()
}

func readLocalInbounds(ctx context.Context, db *sql.DB) ([]map[string]any, error) {
	columns, err := sqliteTableColumns(ctx, db, "inbounds")
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s,
		       %s, %s, %s, %s, %s,
		       %s, %s, %s, %s
		FROM inbounds
		ORDER BY id`,
		sqliteColumnExpr(columns, "id", "0", "id"),
		sqliteColumnExpr(columns, "user_id", "0", "user_id"),
		sqliteColumnExpr(columns, "up", "0", "up"),
		sqliteColumnExpr(columns, "down", "0", "down"),
		sqliteColumnExpr(columns, "total", "0", "total"),
		sqliteColumnExpr(columns, "all_time", "0", "all_time"),
		sqliteColumnExpr(columns, "remark", "''", "remark"),
		sqliteColumnExpr(columns, "enable", "1", "enable"),
		sqliteColumnExpr(columns, "expiry_time", "0", "expiry_time"),
		sqliteColumnExpr(columns, "traffic_reset", "''", "traffic_reset"),
		sqliteColumnExpr(columns, "last_traffic_reset_time", "0", "last_traffic_reset_time"),
		sqliteColumnExpr(columns, "listen", "''", "listen"),
		sqliteColumnExpr(columns, "port", "0", "port"),
		sqliteColumnExpr(columns, "protocol", "''", "protocol"),
		sqliteColumnExpr(columns, "settings", "'{}'", "settings"),
		sqliteColumnExpr(columns, "stream_settings", "'{}'", "stream_settings"),
		sqliteColumnExpr(columns, "tag", "''", "tag"),
		sqliteColumnExpr(columns, "sniffing", "'{}'", "sniffing"),
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inbounds []map[string]any
	for rows.Next() {
		var id, userID, port int
		var up, down, total, allTime, expiryTime, lastTrafficResetTime int64
		var enable bool
		var remark, trafficReset, listen, protocol, settings, streamSettings, tag, sniffing sql.NullString
		if err := rows.Scan(
			&id, &userID, &up, &down, &total, &allTime, &remark, &enable, &expiryTime,
			&trafficReset, &lastTrafficResetTime, &listen, &port, &protocol,
			&settings, &streamSettings, &tag, &sniffing,
		); err != nil {
			return nil, err
		}
		inbounds = append(inbounds, map[string]any{
			"id":                   id,
			"userId":               userID,
			"up":                   up,
			"down":                 down,
			"total":                total,
			"allTime":              allTime,
			"remark":               nullString(remark),
			"enable":               enable,
			"expiryTime":           expiryTime,
			"trafficReset":         nullString(trafficReset),
			"lastTrafficResetTime": lastTrafficResetTime,
			"listen":               nullString(listen),
			"port":                 port,
			"protocol":             nullString(protocol),
			"settings":             nullString(settings),
			"streamSettings":       nullString(streamSettings),
			"tag":                  nullString(tag),
			"sniffing":             nullString(sniffing),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stats, err := readLocalClientStats(ctx, db)
	if err != nil {
		return nil, err
	}
	clientsByInbound, err := readLocalClients(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		id := intValue(inbound["id"])
		inbound["clientStats"] = stats[id]
		for _, client := range clientsByInbound[id] {
			appendClientToInbound(inbound, client)
		}
	}
	return inbounds, nil
}

func readLocalClientStats(ctx context.Context, db *sql.DB) (map[int][]map[string]any, error) {
	columns, err := sqliteTableColumns(ctx, db, "client_traffics")
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s
		FROM client_traffics
		ORDER BY id`,
		sqliteColumnExpr(columns, "id", "0", "id"),
		sqliteColumnExpr(columns, "inbound_id", "0", "inbound_id"),
		sqliteColumnExpr(columns, "enable", "1", "enable"),
		sqliteColumnExpr(columns, "email", "''", "email"),
		sqliteColumnExpr(columns, "up", "0", "up"),
		sqliteColumnExpr(columns, "down", "0", "down"),
		sqliteColumnExpr(columns, "all_time", "0", "all_time"),
		sqliteColumnExpr(columns, "expiry_time", "0", "expiry_time"),
		sqliteColumnExpr(columns, "total", "0", "total"),
		sqliteColumnExpr(columns, "reset", "0", "reset"),
		sqliteColumnExpr(columns, "last_online", "0", "last_online"),
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int][]map[string]any)
	for rows.Next() {
		var id, inboundID, reset int
		var enable bool
		var email sql.NullString
		var up, down, allTime, expiryTime, total, lastOnline int64
		if err := rows.Scan(&id, &inboundID, &enable, &email, &up, &down, &allTime, &expiryTime, &total, &reset, &lastOnline); err != nil {
			return nil, err
		}
		result[inboundID] = append(result[inboundID], map[string]any{
			"id":         id,
			"inboundId":  inboundID,
			"enable":     enable,
			"email":      nullString(email),
			"up":         up,
			"down":       down,
			"allTime":    allTime,
			"expiryTime": expiryTime,
			"total":      total,
			"reset":      reset,
			"lastOnline": lastOnline,
		})
	}
	return result, rows.Err()
}

func readLocalClients(ctx context.Context, db *sql.DB) (map[int][]map[string]any, error) {
	columns, err := sqliteTableColumns(ctx, db, "clients")
	if err != nil {
		return map[int][]map[string]any{}, nil
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s,
		       %s, %s, %s, %s, %s, %s, %s, %s, %s, %s,
		       %s, %s, %s
		FROM clients
		ORDER BY id`,
		sqliteColumnExpr(columns, "id", "0", "row_id"),
		sqliteColumnExpr(columns, "email", "''", "email"),
		sqliteColumnExpr(columns, "uuid", "''", "uuid"),
		sqliteColumnExpr(columns, "password", "''", "password"),
		sqliteColumnExpr(columns, "auth", "''", "auth"),
		sqliteColumnExpr(columns, "flow", "''", "flow"),
		sqliteColumnExpr(columns, "security", "''", "security"),
		sqliteColumnExpr(columns, "enable", "1", "enable"),
		sqliteColumnExpr(columns, "total", "0", "total"),
		sqliteColumnExpr(columns, "expiry_time", "0", "expiry_time"),
		sqliteColumnExpr(columns, "reset", "0", "reset"),
		sqliteColumnExpr(columns, "limit_ip", "0", "limit_ip"),
		sqliteColumnExpr(columns, "tg_id", "0", "tg_id"),
		sqliteColumnExpr(columns, "sub_id", "''", "sub_id"),
		sqliteColumnExpr(columns, "group_name", "''", "group_name"),
		sqliteColumnExpr(columns, "comment", "''", "comment"),
		sqliteColumnExpr(columns, "up", "0", "up"),
		sqliteColumnExpr(columns, "down", "0", "down"),
		sqliteColumnExpr(columns, "all_time", "0", "all_time"),
		sqliteColumnExpr(columns, "last_online", "0", "last_online"),
		sqliteColumnExpr(columns, "created_at", "0", "created_at"),
		sqliteColumnExpr(columns, "updated_at", "0", "updated_at"),
		sqliteColumnExpr(columns, "reverse", "''", "reverse"),
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	inboundMap, err := readLocalClientInboundMap(ctx, db, columns)
	if err != nil {
		return nil, err
	}
	result := make(map[int][]map[string]any)
	for rows.Next() {
		var rowID, reset, limitIP int
		var enable bool
		var total, expiryTime, tgID, up, down, allTime, lastOnline, createdAt, updatedAt int64
		var email, uuid, password, auth, flow, security, subID, group, comment, reverse sql.NullString
		if err := rows.Scan(
			&rowID, &email, &uuid, &password, &auth, &flow, &security, &enable, &total, &expiryTime,
			&reset, &limitIP, &tgID, &subID, &group, &comment, &up, &down, &allTime, &lastOnline,
			&createdAt, &updatedAt, &reverse,
		); err != nil {
			return nil, err
		}
		client := map[string]any{
			"rowId":      rowID,
			"email":      nullString(email),
			"id":         nullString(uuid),
			"password":   nullString(password),
			"auth":       nullString(auth),
			"flow":       nullString(flow),
			"security":   nullString(security),
			"enable":     enable,
			"totalGB":    total,
			"expiryTime": expiryTime,
			"reset":      reset,
			"limitIp":    limitIP,
			"tgId":       tgID,
			"subId":      nullString(subID),
			"group":      nullString(group),
			"comment":    nullString(comment),
			"up":         up,
			"down":       down,
			"allTime":    allTime,
			"lastOnline": lastOnline,
			"createdAt":  createdAt,
			"updatedAt":  updatedAt,
		}
		if rawReverse := strings.TrimSpace(nullString(reverse)); rawReverse != "" {
			var parsed any
			if err := json.Unmarshal([]byte(rawReverse), &parsed); err == nil {
				client["reverse"] = parsed
			} else {
				client["reverse"] = rawReverse
			}
		}
		for _, inboundID := range inboundMap[rowID] {
			result[inboundID] = append(result[inboundID], client)
		}
	}
	return result, rows.Err()
}

func readLocalClientInboundMap(ctx context.Context, db *sql.DB, clientColumns map[string]struct{}) (map[int][]int, error) {
	result := make(map[int][]int)
	if _, ok := clientColumns["inbound_id"]; ok {
		rows, err := db.QueryContext(ctx, `SELECT id, inbound_id FROM clients WHERE inbound_id > 0`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var clientID, inboundID int
			if err := rows.Scan(&clientID, &inboundID); err != nil {
				return nil, err
			}
			result[clientID] = append(result[clientID], inboundID)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	linkColumns, err := sqliteTableColumns(ctx, db, "client_inbounds")
	if err != nil {
		return result, nil
	}
	query := fmt.Sprintf(`SELECT %s, %s FROM client_inbounds ORDER BY client_id, inbound_id`,
		sqliteColumnExpr(linkColumns, "client_id", "0", "client_id"),
		sqliteColumnExpr(linkColumns, "inbound_id", "0", "inbound_id"),
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var clientID, inboundID int
		if err := rows.Scan(&clientID, &inboundID); err != nil {
			return nil, err
		}
		if clientID > 0 && inboundID > 0 {
			result[clientID] = append(result[clientID], inboundID)
		}
	}
	return result, rows.Err()
}

func sqliteTableColumns(ctx context.Context, db *sql.DB, table string) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		if name != "" {
			columns[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s has no columns", table)
	}
	return columns, nil
}

func sqliteColumnExpr(columns map[string]struct{}, column, fallback, alias string) string {
	if _, ok := columns[column]; ok {
		return column
	}
	return fallback + " AS " + alias
}

func readLocalXrayConfig(ctx context.Context, db *sql.DB) (map[string]any, error) {
	var raw sql.NullString
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'xrayTemplateConfig'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	configJSON, err := decodeLocalXrayTemplate(nullString(raw))
	if err != nil {
		return nil, err
	}
	return configJSON, nil
}

func writeLocalXrayConfig(ctx context.Context, db *sql.DB, configJSON map[string]any) error {
	body, err := json.Marshal(configJSON)
	if err != nil {
		return fmt.Errorf("marshal local xray template: %w", err)
	}
	result, err := db.ExecContext(ctx, `UPDATE settings SET value = ? WHERE key = 'xrayTemplateConfig'`, string(body))
	if err != nil {
		return fmt.Errorf("update local xray template: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read local xray template update count: %w", err)
	}
	if affected > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('xrayTemplateConfig', ?)`, string(body)); err != nil {
		return fmt.Errorf("insert local xray template: %w", err)
	}
	return nil
}

func decodeLocalXrayTemplate(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var current any
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return nil, err
	}
	for i := 0; i < 5; i++ {
		switch value := current.(type) {
		case string:
			if strings.TrimSpace(value) == "" {
				return map[string]any{}, nil
			}
			var next any
			if err := json.Unmarshal([]byte(value), &next); err != nil {
				return nil, err
			}
			current = next
		case map[string]any:
			if wrapped, ok := value["xraySetting"]; ok {
				current = wrapped
				continue
			}
			return value, nil
		default:
			return map[string]any{}, nil
		}
	}
	return nil, fmt.Errorf("xray template nested too deeply")
}

func readLocalOutboundTraffic(ctx context.Context, db *sql.DB) ([]map[string]any, error) {
	columns, err := sqliteTableColumns(ctx, db, "outbound_traffics")
	if err != nil {
		return nil, nil
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s
		FROM outbound_traffics
		ORDER BY id`,
		sqliteColumnExpr(columns, "id", "0", "id"),
		sqliteColumnExpr(columns, "tag", "''", "tag"),
		sqliteFirstColumnExpr(columns, []string{"up", "uplink", "upload", "sent", "tx"}, "0", "up"),
		sqliteFirstColumnExpr(columns, []string{"down", "downlink", "download", "recv", "rx"}, "0", "down"),
		sqliteFirstColumnExpr(columns, []string{"total", "all_time", "allTime", "traffic"}, "0", "total"),
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var id int
		var tag sql.NullString
		var up, down, total int64
		if err := rows.Scan(&id, &tag, &up, &down, &total); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"id":    id,
			"tag":   nullString(tag),
			"up":    up,
			"down":  down,
			"total": total,
		})
	}
	return result, rows.Err()
}

func sqliteFirstColumnExpr(columns map[string]struct{}, candidates []string, fallback, alias string) string {
	for _, column := range candidates {
		if _, ok := columns[column]; ok {
			if column == alias {
				return column
			}
			return column + " AS " + alias
		}
	}
	return fallback + " AS " + alias
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func (c *XUIClient) localServerStatus() model.XUIServerStatus {
	var status model.XUIServerStatus
	status.CPU = c.localStatus.cpuPercent()
	status.Uptime = localUptime()
	if used, total, ok := localMemUsage(); ok {
		status.Mem.Current = used
		status.Mem.Total = total
	}
	if netCounters, ok := localNetUsage(); ok {
		now := time.Now()
		status.NetTraffic.Recv = netCounters.rx
		status.NetTraffic.Sent = netCounters.tx
		if c.localStatus.hasNet && netCounters.rx >= c.localStatus.lastNet.rx && netCounters.tx >= c.localStatus.lastNet.tx {
			elapsed := now.Sub(c.localStatus.lastNetTime).Seconds()
			if elapsed > 0 {
				status.NetIO.Down = uint64(float64(netCounters.rx-c.localStatus.lastNet.rx) / elapsed)
				status.NetIO.Up = uint64(float64(netCounters.tx-c.localStatus.lastNet.tx) / elapsed)
			}
		}
		c.localStatus.lastNet = netCounters
		c.localStatus.lastNetTime = now
		c.localStatus.hasNet = true
	}
	status.PublicIP = publicIPFromBaseURL(c.baseURL)
	if localXrayRunning() {
		status.Xray.State = "running"
	}
	return status
}

func (s *localStatusSampler) cpuPercent() float64 {
	cpu, ok := localCPUTotal()
	if !ok {
		return 0
	}
	defer func() {
		s.lastCPU = cpu
		s.hasCPU = true
	}()
	if !s.hasCPU || cpu.total <= s.lastCPU.total {
		return 0
	}
	totalDelta := cpu.total - s.lastCPU.total
	idleDelta := uint64(0)
	if cpu.idle > s.lastCPU.idle {
		idleDelta = cpu.idle - s.lastCPU.idle
	}
	if totalDelta == 0 || idleDelta > totalDelta {
		return 0
	}
	return (1 - float64(idleDelta)/float64(totalDelta)) * 100
}

func localCPUTotal() (localCPUCounters, bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return localCPUCounters{}, false
	}
	lines := strings.SplitN(string(data), "\n", 2)
	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return localCPUCounters{}, false
	}
	var total uint64
	var idle uint64
	for index, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return localCPUCounters{}, false
		}
		total += value
		if index == 3 || index == 4 {
			idle += value
		}
	}
	return localCPUCounters{idle: idle, total: total}, total > 0
}

func localUptime() uint64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return uint64(value)
}

func localMemUsage() (uint64, uint64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	var totalKB uint64
	var availableKB uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			totalKB = value
		case "MemAvailable":
			availableKB = value
		}
	}
	if totalKB == 0 {
		return 0, 0, false
	}
	if availableKB > totalKB {
		availableKB = totalKB
	}
	return (totalKB - availableKB) * 1024, totalKB * 1024, true
}

func localNetUsage() (localNetCounters, bool) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return localNetCounters{}, false
	}
	var total localNetCounters
	var fallback localNetCounters
	for lineNo, line := range strings.Split(string(data), "\n") {
		if lineNo < 2 {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		fallback.rx += rx
		fallback.tx += tx
		if name == "lo" || strings.HasPrefix(name, "lo:") {
			continue
		}
		total.rx += rx
		total.tx += tx
	}
	if total.rx == 0 && total.tx == 0 {
		total = fallback
	}
	return total, total.rx > 0 || total.tx > 0
}

func publicIPFromBaseURL(baseURL string) model.XUIPublicIP {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return model.XUIPublicIP{}
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil {
		return model.XUIPublicIP{}
	}
	if ip.To4() != nil {
		return model.XUIPublicIP{IPv4: ip.String()}
	}
	return model.XUIPublicIP{IPv6: ip.String()}
}

func localXrayRunning() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(string(comm)))
		if name == "xray" || strings.HasPrefix(name, "xray-") {
			return true
		}
	}
	return false
}
