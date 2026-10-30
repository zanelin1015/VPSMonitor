package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"bridge-core/internal/model"
)

func buildSingBoxCustomerLink(assignment model.CustomerAssignment, snapshots []model.AgentSnapshot, agents []model.AgentRecord) model.CustomerLinkView {
	link := model.CustomerLinkView{
		AssignmentID:    assignment.ID,
		SourceType:      "singbox",
		EntryClientName: firstNonEmptyString(assignment.PublicClientName, assignment.SingBoxInboundTag, assignment.AgentID),
		InboundTag:      assignment.SingBoxInboundTag,
		ClientEmail:     assignment.SingBoxClientKey,
		Remark:          assignment.Remark,
		Resolved:        false,
		Steps:           []model.CustomerLinkStep{{Role: "entry", Label: firstNonEmptyString(assignment.PublicClientName, assignment.SingBoxInboundTag, assignment.AgentID)}},
		UpdatedAt:       assignment.UpdatedAt,
	}
	var snapshot *model.AgentSnapshot
	for index := range snapshots {
		if snapshots[index].AgentID == assignment.AgentID {
			snapshot = &snapshots[index]
			break
		}
	}
	if snapshot == nil || snapshot.SingBox == nil {
		link.UnresolvedReason = "当前没有可用的 sing-box 配置快照"
		link.Summary = link.EntryClientName + " 暂无 sing-box 快照"
		return link
	}
	var inbound map[string]any
	for _, item := range snapshot.SingBox.Clients {
		if strings.EqualFold(strings.TrimSpace(stringValueAny(item["tag"])), strings.TrimSpace(assignment.SingBoxInboundTag)) {
			inbound = item
			break
		}
	}
	if inbound == nil {
		link.UnresolvedReason = "sing-box 入站节点不存在或尚未采集"
		link.Summary = link.EntryClientName + " 找不到入站节点"
		return link
	}
	host := ""
	for _, agent := range agents {
		if agent.AgentID != assignment.AgentID {
			continue
		}
		host = firstNonEmptyString(agent.PublicIPv4, agent.PublicIPv6, agent.Summary.PublicIPv4, agent.Summary.PublicIPv6, agent.Hostname)
		break
	}
	if host == "" {
		host = firstNonEmptyString(snapshot.Summary.PublicIPv4, snapshot.Summary.PublicIPv6, snapshot.Summary.ObservedIP, snapshot.Summary.Hostname)
	}
	if host == "" {
		link.UnresolvedReason = "sing-box 入站没有可用的公网地址"
		link.Summary = link.EntryClientName + " 缺少公网地址"
		return link
	}
	outbound := cloneSingBoxObject(inbound)
	outbound["server"] = host
	outbound["server_port"] = intValueAny(inbound["listen_port"])
	delete(outbound, "listen")
	delete(outbound, "listen_port")
	delete(outbound, "users")
	outbound["tag"] = firstNonEmptyString(assignment.PublicClientName, assignment.SingBoxInboundTag)
	if users, ok := inbound["users"].([]any); ok && assignment.SingBoxClientKey != "" {
		for _, raw := range users {
			user, ok := raw.(map[string]any)
			if !ok || !singBoxCustomerUserMatches(user, assignment.SingBoxClientKey) {
				continue
			}
			for _, key := range []string{"uuid", "password", "userkey", "username", "email", "flow"} {
				if value, exists := user[key]; exists {
					outbound[key] = value
				}
			}
			break
		}
	}
	link.SingBoxConfig = map[string]any{
		"log":       map[string]any{"level": "info"},
		"outbounds": []any{outbound},
		"route":     map[string]any{"final": outbound["tag"]},
	}
	link.Resolved = true
	link.Summary = link.EntryClientName + " · sing-box"
	link.Steps = append(link.Steps, model.CustomerLinkStep{Role: "singbox", Label: fmt.Sprintf("%s:%d", host, intValueAny(inbound["listen_port"]))})
	return link
}

func buildSingBoxSubscription(links []model.CustomerLinkView) ([]byte, error) {
	outbounds := make([]any, 0, len(links))
	for _, link := range links {
		if link.SourceType != "singbox" || !link.Resolved || link.SingBoxConfig == nil {
			continue
		}
		items, _ := link.SingBoxConfig["outbounds"].([]any)
		outbounds = append(outbounds, items...)
	}
	config := map[string]any{
		"log":       map[string]any{"level": "info"},
		"outbounds": outbounds,
	}
	if len(outbounds) > 0 {
		first, _ := outbounds[0].(map[string]any)
		config["route"] = map[string]any{"final": first["tag"]}
	}
	return json.MarshalIndent(config, "", "  ")
}

func singBoxCustomerUserMatches(user map[string]any, key string) bool {
	if separator := strings.IndexByte(key, ':'); separator > 0 {
		field := key[:separator]
		value := key[separator+1:]
		return strings.TrimSpace(stringValueAny(user[field])) == value
	}
	for _, field := range []string{"name", "userkey", "username", "email", "uuid", "password"} {
		if strings.TrimSpace(stringValueAny(user[field])) == key {
			return true
		}
	}
	return false
}

func cloneSingBoxObject(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func stringValueAny(value any) string {
	result, _ := value.(string)
	return result
}

func intValueAny(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	case json.Number:
		parsed, _ := number.Int64()
		return int(parsed)
	default:
		return 0
	}
}
