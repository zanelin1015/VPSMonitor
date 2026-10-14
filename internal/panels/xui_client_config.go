package panels

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"bridge-core/internal/model"
)

func (c *XUIClient) updateClientConfig(ctx context.Context, payload map[string]any) (map[string]any, error) {
	changes, err := model.ParseXUIClientConfigChanges(payload["changes"])
	if err != nil || changes.Empty() {
		return nil, fmt.Errorf("invalid client configuration changes: %v", err)
	}
	expected, err := model.ParseXUIClientConfigChanges(payload["expected"])
	if err != nil {
		return nil, err
	}
	inboundID := intValue(payload["inbound_id"])
	inboundTag, email := strings.TrimSpace(stringFromMap(payload, "inbound_tag")), strings.TrimSpace(stringFromMap(payload, "email"))
	if (inboundID <= 0 && inboundTag == "") || email == "" {
		return nil, fmt.Errorf("client identity is incomplete")
	}
	target, err := c.loadClientUpdateTarget(ctx, inboundID, inboundTag, email, clientIDForUpdate(payload))
	if err != nil {
		return nil, err
	}
	if changes.TrafficReset != nil {
		_, period := target.client["trafficReset"]
		_, day := target.client["trafficResetDay"]
		if !target.clientAPI || !period || !day {
			return nil, fmt.Errorf("this x-ui panel does not support per-client traffic reset settings")
		}
	}
	fields, expectedFields := changes.Fields(), expected.Fields()
	var keys []string
	for field, desired := range fields {
		previous, ok := expectedFields[field]
		if !ok {
			return nil, fmt.Errorf("missing expected value for %s", field)
		}
		// A retry after an uncertain write may already contain the desired value.
		if !clientConfigValueMatches(target.client, field, previous) && !clientConfigValueMatches(target.client, field, desired) {
			return nil, fmt.Errorf("客户端 %s 的 %s 已在 x-ui 修改，请刷新后重新编辑", email, field)
		}
		keys = append(keys, field)
	}
	sort.Strings(keys)
	for field, value := range fields {
		target.client[field] = value
	}
	return c.saveClientUpdate(ctx, target, keys...)
}

func clientConfigValueMatches(actual map[string]any, field string, desired any) bool {
	if field == "enable" {
		value, ok := boolPayloadValue(actual, field)
		return ok && value == desired
	}
	if field == "trafficReset" {
		value, ok := actual[field].(string)
		return ok && value == desired
	}
	value, err := clientUpdateInt64(actual[field], field)
	return err == nil && value == int64Value(desired)
}
