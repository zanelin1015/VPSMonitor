package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *SQLiteStore) ListFrontProxyNodes() ([]model.FrontProxyNode, error) {
	rows, err := s.db.Query(`
		SELECT id, name, protocol, share_url, enabled, remark, created_at, updated_at
		FROM front_proxy_nodes
		ORDER BY enabled DESC, name COLLATE NOCASE ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list front proxy nodes: %w", err)
	}
	defer rows.Close()
	return scanFrontProxyNodeRows(rows)
}

func (s *SQLiteStore) CreateFrontProxyNode(req model.FrontProxyNodeRequest) (model.FrontProxyNode, error) {
	normalized, err := normalizeFrontProxyNodeRequest(req, true)
	if err != nil {
		return model.FrontProxyNode{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`
		INSERT INTO front_proxy_nodes (name, protocol, share_url, enabled, remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, normalized.Name, frontProxyProtocolFromShareURL(normalized.ShareURL), normalized.ShareURL, boolInt(*normalized.Enabled), normalized.Remark, now, now)
	if err != nil {
		return model.FrontProxyNode{}, fmt.Errorf("create front proxy node: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.FrontProxyNode{}, fmt.Errorf("read front proxy node id: %w", err)
	}
	item, found, err := s.GetFrontProxyNode(id)
	if err != nil {
		return model.FrontProxyNode{}, err
	}
	if !found {
		return model.FrontProxyNode{}, fmt.Errorf("created front proxy node not found")
	}
	return item, nil
}

func (s *SQLiteStore) UpdateFrontProxyNode(id int64, req model.FrontProxyNodeRequest) (model.FrontProxyNode, error) {
	if id <= 0 {
		return model.FrontProxyNode{}, fmt.Errorf("invalid front proxy node id")
	}
	current, found, err := s.GetFrontProxyNode(id)
	if err != nil {
		return model.FrontProxyNode{}, err
	}
	if !found {
		return model.FrontProxyNode{}, fmt.Errorf("front proxy node not found")
	}
	normalized, err := normalizeFrontProxyNodeRequest(req, false)
	if err != nil {
		return model.FrontProxyNode{}, err
	}
	if normalized.Enabled == nil {
		normalized.Enabled = &current.Enabled
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`
		UPDATE front_proxy_nodes
		SET name = ?, protocol = ?, share_url = ?, enabled = ?, remark = ?, updated_at = ?
		WHERE id = ?
	`, normalized.Name, frontProxyProtocolFromShareURL(normalized.ShareURL), normalized.ShareURL, boolInt(*normalized.Enabled), normalized.Remark, now, id)
	if err != nil {
		return model.FrontProxyNode{}, fmt.Errorf("update front proxy node: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return model.FrontProxyNode{}, fmt.Errorf("front proxy node not found")
	}
	item, found, err := s.GetFrontProxyNode(id)
	if err != nil {
		return model.FrontProxyNode{}, err
	}
	if !found {
		return model.FrontProxyNode{}, fmt.Errorf("front proxy node not found")
	}
	return item, nil
}

func (s *SQLiteStore) DeleteFrontProxyNode(id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid front proxy node id")
	}
	result, err := s.db.Exec(`DELETE FROM front_proxy_nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete front proxy node: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("front proxy node not found")
	}
	return nil
}

func (s *SQLiteStore) GetFrontProxyNode(id int64) (model.FrontProxyNode, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, name, protocol, share_url, enabled, remark, created_at, updated_at
		FROM front_proxy_nodes
		WHERE id = ?
	`, id)
	item, err := scanFrontProxyNode(row)
	if err == sql.ErrNoRows {
		return model.FrontProxyNode{}, false, nil
	}
	if err != nil {
		return model.FrontProxyNode{}, false, fmt.Errorf("load front proxy node: %w", err)
	}
	return item, true, nil
}

func (s *SQLiteStore) ReplaceFrontProxyGrants(granteeType string, granteeID int64, nodeIDs []int64) error {
	granteeType = normalizeFrontProxyGranteeType(granteeType)
	if granteeType == "" || granteeID <= 0 {
		return fmt.Errorf("invalid front proxy grantee")
	}
	nodeIDs, err := s.normalizeFrontProxyNodeIDs(nodeIDs, false)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin front proxy grants: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.validateFrontProxyGranteeTx(tx, granteeType, granteeID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM front_proxy_grants WHERE grantee_type = ? AND grantee_id = ?`, granteeType, granteeID); err != nil {
		return fmt.Errorf("clear front proxy grants: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, nodeID := range nodeIDs {
		if _, err := tx.Exec(`
			INSERT INTO front_proxy_grants (node_id, grantee_type, grantee_id, created_at)
			VALUES (?, ?, ?, ?)
		`, nodeID, granteeType, granteeID, now); err != nil {
			return fmt.Errorf("save front proxy grant: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit front proxy grants: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListFrontProxyNodesForGrantee(granteeType string, granteeID int64) ([]model.FrontProxyNode, error) {
	granteeType = normalizeFrontProxyGranteeType(granteeType)
	if granteeType == "" || granteeID <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT n.id, n.name, n.protocol, n.share_url, n.enabled, n.remark, n.created_at, n.updated_at
		FROM front_proxy_nodes n
		INNER JOIN front_proxy_grants g ON g.node_id = n.id
		WHERE g.grantee_type = ? AND g.grantee_id = ? AND n.enabled = 1
		ORDER BY n.name COLLATE NOCASE ASC, n.id ASC
	`, granteeType, granteeID)
	if err != nil {
		return nil, fmt.Errorf("list front proxy nodes for grantee: %w", err)
	}
	defer rows.Close()
	return scanFrontProxyNodeRows(rows)
}

func (s *SQLiteStore) ListFrontProxyNodeViewsForGrantee(granteeType string, granteeID int64) ([]model.FrontProxyNodeView, error) {
	nodes, err := s.ListFrontProxyNodesForGrantee(granteeType, granteeID)
	if err != nil {
		return nil, err
	}
	return frontProxyNodeViews(nodes), nil
}

func (s *SQLiteStore) ReplaceCustomerAssignmentFrontProxyNodes(assignmentID int64, nodeIDs []int64, actorType string, actorID int64) error {
	if assignmentID <= 0 {
		return fmt.Errorf("invalid assignment id")
	}
	actorType = normalizeFrontProxyActorType(actorType)
	nodeIDs, err := s.normalizeFrontProxyNodeIDs(nodeIDs, true)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin customer assignment front proxies: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := validateCustomerAssignmentExistsTx(tx, assignmentID); err != nil {
		return err
	}
	if actorType != model.AdminRoleRoot {
		if actorID <= 0 {
			return fmt.Errorf("invalid front proxy actor")
		}
		for _, nodeID := range nodeIDs {
			if err := validateFrontProxyGrantTx(tx, nodeID, actorType, actorID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM customer_assignment_front_proxies WHERE assignment_id = ?`, assignmentID); err != nil {
		return fmt.Errorf("clear customer assignment front proxies: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for index, nodeID := range nodeIDs {
		if _, err := tx.Exec(`
			INSERT INTO customer_assignment_front_proxies (assignment_id, node_id, sort_order, created_at)
			VALUES (?, ?, ?, ?)
		`, assignmentID, nodeID, index+1, now); err != nil {
			return fmt.Errorf("save customer assignment front proxy: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit customer assignment front proxies: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListFrontProxyNodesForAssignments(assignmentIDs []int64) (map[int64][]model.FrontProxyNode, error) {
	result := make(map[int64][]model.FrontProxyNode)
	ids := uniquePositiveInt64s(assignmentIDs)
	if len(ids) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT p.assignment_id, n.id, n.name, n.protocol, n.share_url, n.enabled, n.remark, n.created_at, n.updated_at
		FROM customer_assignment_front_proxies p
		INNER JOIN front_proxy_nodes n ON n.id = p.node_id
		WHERE p.assignment_id IN (%s) AND n.enabled = 1
		ORDER BY p.assignment_id ASC, p.sort_order ASC, n.name COLLATE NOCASE ASC, n.id ASC
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("list front proxy nodes for assignments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var assignmentID int64
		var item model.FrontProxyNode
		var enabled int
		var createdAtText, updatedAtText string
		if err := rows.Scan(&assignmentID, &item.ID, &item.Name, &item.Protocol, &item.ShareURL, &enabled, &item.Remark, &createdAtText, &updatedAtText); err != nil {
			return nil, fmt.Errorf("scan assignment front proxy: %w", err)
		}
		item.Enabled = enabled != 0
		item.CreatedAt = parseTime(createdAtText)
		item.UpdatedAt = parseTime(updatedAtText)
		result[assignmentID] = append(result[assignmentID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assignment front proxies: %w", err)
	}
	return result, nil
}

func (s *SQLiteStore) attachFrontProxyViewsToCustomerAssignments(items []model.CustomerAssignment) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	byAssignment, err := s.ListFrontProxyNodesForAssignments(ids)
	if err != nil {
		return err
	}
	for index := range items {
		items[index].FrontProxies = frontProxyNodeViews(byAssignment[items[index].ID])
	}
	return nil
}

func frontProxyNodeViews(nodes []model.FrontProxyNode) []model.FrontProxyNodeView {
	if len(nodes) == 0 {
		return nil
	}
	views := make([]model.FrontProxyNodeView, 0, len(nodes))
	for _, node := range nodes {
		views = append(views, model.FrontProxyNodeView{
			ID:       node.ID,
			Name:     node.Name,
			Protocol: node.Protocol,
			Enabled:  node.Enabled,
		})
	}
	return views
}

func (s *SQLiteStore) normalizeFrontProxyNodeIDs(raw []int64, enabledOnly bool) ([]int64, error) {
	seen := make(map[int64]struct{}, len(raw))
	ids := make([]int64, 0, len(raw))
	for _, id := range raw {
		if id <= 0 {
			return nil, fmt.Errorf("invalid front proxy node id")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		var exists int
		query := `SELECT 1 FROM front_proxy_nodes WHERE id = ?`
		if enabledOnly {
			query += ` AND enabled = 1`
		}
		if err := s.db.QueryRow(query, id).Scan(&exists); err == sql.ErrNoRows {
			return nil, fmt.Errorf("front proxy node %d not found", id)
		} else if err != nil {
			return nil, fmt.Errorf("load front proxy node %d: %w", id, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func normalizeFrontProxyNodeRequest(req model.FrontProxyNodeRequest, creating bool) (model.FrontProxyNodeRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.ShareURL = strings.TrimSpace(req.ShareURL)
	req.Remark = strings.TrimSpace(req.Remark)
	if req.Name == "" {
		return req, fmt.Errorf("front proxy name is required")
	}
	if len(req.Name) > 160 {
		return req, fmt.Errorf("front proxy name is too long")
	}
	if req.ShareURL == "" {
		return req, fmt.Errorf("front proxy share_url is required")
	}
	if frontProxyProtocolFromShareURL(req.ShareURL) == "" {
		return req, fmt.Errorf("unsupported front proxy share_url")
	}
	if len(req.ShareURL) > 4096 {
		return req, fmt.Errorf("front proxy share_url is too long")
	}
	if len(req.Remark) > 1000 {
		return req, fmt.Errorf("front proxy remark is too long")
	}
	if req.Enabled == nil && creating {
		enabled := true
		req.Enabled = &enabled
	}
	return req, nil
}

func frontProxyProtocolFromShareURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
	case "vless", "vmess", "trojan", "ss", "socks", "socks5", "http", "https":
		return strings.ToLower(strings.TrimSpace(parsed.Scheme))
	default:
		return ""
	}
}

func normalizeFrontProxyGranteeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.FrontProxyGranteeAreaManager:
		return model.FrontProxyGranteeAreaManager
	case model.FrontProxyGranteeCustomer:
		return model.FrontProxyGranteeCustomer
	default:
		return ""
	}
}

func normalizeFrontProxyActorType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.AdminRoleAreaManager:
		return model.AdminRoleAreaManager
	case model.FrontProxyGranteeCustomer:
		return model.FrontProxyGranteeCustomer
	default:
		return model.AdminRoleRoot
	}
}

func (s *SQLiteStore) validateFrontProxyGranteeTx(tx *sql.Tx, granteeType string, granteeID int64) error {
	switch granteeType {
	case model.FrontProxyGranteeAreaManager:
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM area_manager_accounts WHERE id = ?`, granteeID).Scan(&exists); err == sql.ErrNoRows {
			return fmt.Errorf("area manager not found")
		} else if err != nil {
			return fmt.Errorf("load area manager: %w", err)
		}
	case model.FrontProxyGranteeCustomer:
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM customer_accounts WHERE id = ?`, granteeID).Scan(&exists); err == sql.ErrNoRows {
			return fmt.Errorf("customer not found")
		} else if err != nil {
			return fmt.Errorf("load customer: %w", err)
		}
	default:
		return fmt.Errorf("invalid front proxy grantee type")
	}
	return nil
}

func validateFrontProxyGrantTx(tx *sql.Tx, nodeID int64, granteeType string, granteeID int64) error {
	var exists int
	if err := tx.QueryRow(`
		SELECT 1
		FROM front_proxy_grants g
		INNER JOIN front_proxy_nodes n ON n.id = g.node_id
		WHERE g.node_id = ? AND g.grantee_type = ? AND g.grantee_id = ? AND n.enabled = 1
	`, nodeID, granteeType, granteeID).Scan(&exists); err == sql.ErrNoRows {
		return fmt.Errorf("front proxy node %d is not granted to this account", nodeID)
	} else if err != nil {
		return fmt.Errorf("check front proxy grant: %w", err)
	}
	return nil
}

func validateCustomerAssignmentExistsTx(tx *sql.Tx, assignmentID int64) error {
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM customer_assignments WHERE id = ?`, assignmentID).Scan(&exists); err == sql.ErrNoRows {
		return fmt.Errorf("assignment not found")
	} else if err != nil {
		return fmt.Errorf("load customer assignment: %w", err)
	}
	return nil
}

func scanFrontProxyNodeRows(rows *sql.Rows) ([]model.FrontProxyNode, error) {
	items := make([]model.FrontProxyNode, 0)
	for rows.Next() {
		item, err := scanFrontProxyNode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan front proxy node: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate front proxy nodes: %w", err)
	}
	return items, nil
}

func scanFrontProxyNode(scanner rowScanner) (model.FrontProxyNode, error) {
	var item model.FrontProxyNode
	var enabled int
	var createdAtText, updatedAtText string
	if err := scanner.Scan(&item.ID, &item.Name, &item.Protocol, &item.ShareURL, &enabled, &item.Remark, &createdAtText, &updatedAtText); err != nil {
		return model.FrontProxyNode{}, err
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(createdAtText)
	item.UpdatedAt = parseTime(updatedAtText)
	return item, nil
}

func uniquePositiveInt64s(raw []int64) []int64 {
	seen := make(map[int64]struct{}, len(raw))
	result := make([]int64, 0, len(raw))
	for _, value := range raw {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
