package panels

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"bridge-core/internal/model"
)

// CollectClientTraffic reads only the cumulative per-client counters needed by
// the realtime dashboard. Local SQLite is preferred so native and mounted
// 1Panel installations do not need an API request every two seconds.
func (c *XUIClient) CollectClientTraffic(ctx context.Context) ([]model.XUIRealtimeClientTraffic, error) {
	localTraffic, localErr := c.collectLocalClientTraffic(ctx)
	if localErr == nil {
		return localTraffic, nil
	}
	if !c.canCollectAuthenticated() {
		return nil, localErr
	}
	if err := c.ensureLogin(ctx); err != nil {
		return nil, err
	}
	traffic, err := c.collectAuthenticatedClientTraffic(ctx)
	if err == nil {
		return traffic, nil
	}
	if isXUIAuthError(err) && !c.hasAPIToken() {
		c.invalidateSession()
		if loginErr := c.login(ctx); loginErr == nil {
			return c.collectAuthenticatedClientTraffic(ctx)
		}
	}
	return nil, err
}

func (c *XUIClient) collectAuthenticatedClientTraffic(ctx context.Context) ([]model.XUIRealtimeClientTraffic, error) {
	inbounds, err := c.getJSONList(ctx, "/panel/api/inbounds/list")
	if err != nil {
		return nil, err
	}
	if clients, clientsErr := c.getJSONList(ctx, "/panel/api/clients/list"); clientsErr == nil {
		inbounds = mergeXUIClientsIntoInbounds(inbounds, clients)
	}
	return flattenXUIClientTraffic(inbounds), nil
}

func (c *XUIClient) collectLocalClientTraffic(ctx context.Context) ([]model.XUIRealtimeClientTraffic, error) {
	dbPath, _, err := c.resolveLocalDBPath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open x-ui local db %s: %w", dbPath, err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("open x-ui local db %s: %w", dbPath, err)
	}

	tags, err := readLocalInboundTags(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("read x-ui local inbound tags: %w", err)
	}
	stats, err := readLocalClientStats(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("read x-ui local client traffic: %w", err)
	}
	traffic := make([]model.XUIRealtimeClientTraffic, 0)
	for inboundID, clients := range stats {
		for _, client := range clients {
			email := strings.TrimSpace(stringValue(client["email"]))
			if email == "" {
				continue
			}
			traffic = append(traffic, model.XUIRealtimeClientTraffic{
				InboundID:  inboundID,
				InboundTag: tags[inboundID],
				Email:      email,
				Up:         int64Value(client["up"]),
				Down:       int64Value(client["down"]),
			})
		}
	}
	sortXUIRealtimeTraffic(traffic)
	return traffic, nil
}

func readLocalInboundTags(ctx context.Context, db *sql.DB) (map[int]string, error) {
	columns, err := sqliteTableColumns(ctx, db, "inbounds")
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT %s, %s FROM inbounds ORDER BY id`,
		sqliteColumnExpr(columns, "id", "0", "id"),
		sqliteColumnExpr(columns, "tag", "''", "tag"),
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]string)
	for rows.Next() {
		var id int
		var tag sql.NullString
		if err := rows.Scan(&id, &tag); err != nil {
			return nil, err
		}
		if id > 0 {
			result[id] = nullString(tag)
		}
	}
	return result, rows.Err()
}

func flattenXUIClientTraffic(inbounds []map[string]any) []model.XUIRealtimeClientTraffic {
	traffic := make([]model.XUIRealtimeClientTraffic, 0)
	seen := make(map[string]struct{})
	for _, inbound := range inbounds {
		inboundID := intValue(inbound["id"])
		inboundTag := strings.TrimSpace(stringValue(inbound["tag"]))
		for _, client := range objectSlice(inbound["clientStats"]) {
			email := strings.TrimSpace(stringValue(client["email"]))
			if email == "" {
				continue
			}
			key := fmt.Sprintf("%d\x00%s\x00%s", inboundID, strings.ToLower(inboundTag), strings.ToLower(email))
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			traffic = append(traffic, model.XUIRealtimeClientTraffic{
				InboundID:  inboundID,
				InboundTag: inboundTag,
				Email:      email,
				Up:         int64Value(client["up"]),
				Down:       int64Value(client["down"]),
			})
		}
	}
	sortXUIRealtimeTraffic(traffic)
	return traffic
}

func sortXUIRealtimeTraffic(traffic []model.XUIRealtimeClientTraffic) {
	sort.Slice(traffic, func(i, j int) bool {
		if traffic[i].InboundID != traffic[j].InboundID {
			return traffic[i].InboundID < traffic[j].InboundID
		}
		if traffic[i].InboundTag != traffic[j].InboundTag {
			return traffic[i].InboundTag < traffic[j].InboundTag
		}
		return traffic[i].Email < traffic[j].Email
	})
}
