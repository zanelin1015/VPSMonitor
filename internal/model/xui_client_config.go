package model

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const XUIClientConfigMinVersion = "0.3.49"

type XUIClientConfigChanges struct {
	Enabled         *bool   `json:"enabled,omitempty"`
	TotalBytes      *int64  `json:"total_bytes,omitempty"`
	ExpiryTime      *int64  `json:"expiry_time,omitempty"`
	TrafficReset    *string `json:"traffic_reset,omitempty"`
	TrafficResetDay *int    `json:"traffic_reset_day,omitempty"`
}

func (p XUIClientConfigChanges) Empty() bool {
	return p.Enabled == nil && p.TotalBytes == nil && p.ExpiryTime == nil && p.TrafficReset == nil && p.TrafficResetDay == nil
}

func (p XUIClientConfigChanges) Validate() error {
	if p.TotalBytes != nil && (*p.TotalBytes < 0 || *p.TotalBytes > 9007199254740991) {
		return fmt.Errorf("total_bytes must be a non-negative safe integer")
	}
	if p.ExpiryTime != nil && (*p.ExpiryTime < 0 || *p.ExpiryTime > 8640000000000000) {
		return fmt.Errorf("expiry_time must be a valid non-negative timestamp")
	}
	if (p.TrafficReset == nil) != (p.TrafficResetDay == nil) {
		return fmt.Errorf("traffic_reset and traffic_reset_day must be supplied together")
	}
	if p.TrafficReset != nil {
		_, err := ParseXUIClientTrafficResetPayload(map[string]any{"traffic_reset": *p.TrafficReset, "traffic_reset_day": *p.TrafficResetDay})
		return err
	}
	return nil
}

func (p XUIClientConfigChanges) Fields() map[string]any {
	fields := map[string]any{}
	if p.Enabled != nil {
		fields["enable"] = *p.Enabled
	}
	if p.TotalBytes != nil {
		fields["totalGB"] = *p.TotalBytes
	}
	if p.ExpiryTime != nil {
		fields["expiryTime"] = *p.ExpiryTime
	}
	if p.TrafficReset != nil {
		fields["trafficReset"] = *p.TrafficReset
	}
	if p.TrafficResetDay != nil {
		fields["trafficResetDay"] = *p.TrafficResetDay
	}
	return fields
}

func ParseXUIClientConfigChanges(raw any) (XUIClientConfigChanges, error) {
	var patch XUIClientConfigChanges
	data, err := json.Marshal(raw)
	if err != nil {
		return patch, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		return patch, err
	}
	return patch, patch.Validate()
}

type XUIClientConfigRequest struct {
	RequestID       string                  `json:"request_id"`
	ClientID        string                  `json:"client_id,omitempty"`
	InboundID       int                     `json:"inbound_id"`
	InboundTag      string                  `json:"inbound_tag,omitempty"`
	Email           string                  `json:"email"`
	Billing         XUIClientBillingConfig  `json:"billing"`
	PreviousBilling *XUIClientBillingConfig `json:"previous_billing"`
	Changes         XUIClientConfigChanges  `json:"changes"`
	Expected        XUIClientConfigChanges  `json:"expected"`
}

func (r XUIClientConfigRequest) Validate() error {
	if len(r.RequestID) < 8 || len(r.RequestID) > 128 || strings.ContainsAny(r.RequestID, " \t\r\n") {
		return fmt.Errorf("request_id must be an opaque identifier of 8–128 characters")
	}
	if r.InboundID <= 0 || strings.TrimSpace(r.Email) == "" {
		return fmt.Errorf("client identity is incomplete")
	}
	b := r.Billing
	if math.IsNaN(b.RevenueAmount) || math.IsInf(b.RevenueAmount, 0) || b.RevenueAmount < 0 || math.IsNaN(b.TrafficMultiplier) || math.IsInf(b.TrafficMultiplier, 0) || b.TrafficMultiplier < 0.1 || b.TrafficMultiplier > 100 {
		return fmt.Errorf("invalid revenue amount or traffic multiplier")
	}
	if b.RevenueCurrency != "CNY" && b.RevenueCurrency != "USDT" {
		return fmt.Errorf("unsupported revenue currency")
	}
	switch b.RevenueCycle {
	case "month", "quarter", "semiannual", "year":
	default:
		return fmt.Errorf("unsupported revenue cycle")
	}
	if b.StartTime < 0 || b.ExpireTime < 0 || b.StartTime > 8640000000000000 || b.ExpireTime > 8640000000000000 || b.ExpireAutoRenew {
		return fmt.Errorf("invalid billing dates or automatic renewal")
	}
	if b.StartTime > 0 && b.ExpireTime > 0 && b.ExpireTime <= b.StartTime {
		return fmt.Errorf("到期时间必须晚于开始日期")
	}
	if err := r.Changes.Validate(); err != nil {
		return err
	}
	if err := r.Expected.Validate(); err != nil {
		return err
	}
	expected := r.Expected.Fields()
	for field := range r.Changes.Fields() {
		if _, ok := expected[field]; !ok {
			return fmt.Errorf("missing expected value for %s", field)
		}
	}
	return nil
}

type XUIClientConfigSave struct {
	RequestID    string                 `json:"request_id"`
	ClientID     string                 `json:"client_id,omitempty"`
	InboundID    int                    `json:"inbound_id"`
	InboundTag   string                 `json:"inbound_tag,omitempty"`
	Email        string                 `json:"email"`
	Billing      XUIClientBillingConfig `json:"billing"`
	BillingSaved bool                   `json:"billing_saved"`
	Action       *XUIAction             `json:"action,omitempty"`
}

type XUIClientConfigTarget struct {
	AgentID    string
	InboundID  int
	InboundTag string
	ClientID   string
	Email      string
}
