package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"bridge-core/internal/model"
)

type windowsInterfacePayload struct {
	BootID     string   `json:"boot_id"`
	Preferred  []string `json:"preferred"`
	Interfaces []struct {
		Name          string `json:"name"`
		Index         int    `json:"index"`
		MAC           string `json:"mac"`
		Kind          string `json:"kind"`
		Up            bool   `json:"up"`
		Loopback      bool   `json:"loopback"`
		ReceivedBytes uint64 `json:"received_bytes"`
		SentBytes     uint64 `json:"sent_bytes"`
		Error         string `json:"error"`
	} `json:"interfaces"`
}

func decodeWindowsNetworkInterfaces(body []byte) *model.NetworkInterfacesSnapshot {
	result := &model.NetworkInterfacesSnapshot{CollectedAt: time.Now().UTC(), Interfaces: []model.NetworkInterfaceCounter{}}
	var payload windowsInterfacePayload
	if err := json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(body), []byte{0xef, 0xbb, 0xbf}), &payload); err != nil {
		result.Error = "解析 Windows 网卡计数器失败: " + err.Error()
		return result
	}
	if payload.BootID == "" {
		result.Error = "无法识别 Windows 启动代次"
		return result
	}
	for _, item := range payload.Interfaces {
		if item.Error != "" || item.Name == "" || item.Index <= 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s 的统计读取失败: %s", item.Name, firstNonEmpty(item.Error, "接口元数据无效")))
			continue
		}
		result.Interfaces = append(result.Interfaces, model.NetworkInterfaceCounter{Name: item.Name, Index: item.Index, MAC: item.MAC, Kind: item.Kind,
			Up: item.Up, Loopback: item.Loopback, ReceivedBytes: item.ReceivedBytes, SentBytes: item.SentBytes})
	}
	if len(payload.Interfaces) > 0 && len(result.Interfaces) == 0 {
		result.Error = "所有 Windows 网卡的统计读取失败"
		return result
	}
	result.Interfaces = finalizeNetworkInterfaces(result.Interfaces, payload.BootID, payload.Preferred)
	return result
}
