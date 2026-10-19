package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const XUIManualTrafficResetMinVersion = "0.3.51"

type XUIManualTrafficResetPayload struct {
	InboundID  int    `json:"inbound_id"`
	InboundTag string `json:"inbound_tag"`
	Email      string `json:"email"`
	ClientID   string `json:"client_id"`
}

func ParseXUIManualTrafficResetPayload(payload map[string]any) (XUIManualTrafficResetPayload, error) {
	var target XUIManualTrafficResetPayload
	data, err := json.Marshal(payload)
	if err != nil {
		return target, fmt.Errorf("invalid manual traffic reset payload: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil {
		return target, fmt.Errorf("invalid manual traffic reset payload: %w", err)
	}
	target.Email = strings.TrimSpace(target.Email)
	target.ClientID = strings.TrimSpace(target.ClientID)
	target.InboundTag = strings.TrimSpace(target.InboundTag)
	if target.InboundID <= 0 || target.Email == "" || target.ClientID == "" {
		return target, fmt.Errorf("inbound_id, email and client_id are required for manual traffic reset")
	}
	return target, nil
}
