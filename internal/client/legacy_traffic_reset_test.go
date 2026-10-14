package client

import (
	"context"
	"testing"

	"bridge-core/internal/model"
)

func TestLegacyTrafficResetIsRejectedBeforePanelAccess(t *testing.T) {
	a := &App{}
	result := a.executeXUIAction(context.Background(), model.ManagedAgentConfig{}, nil, nil, model.XUIAction{Kind: model.XUIActionResetClientTraffic})
	if result.Status != model.XUIActionStatusFailed || result.Error != model.LegacyTrafficResetRemovedMessage {
		t.Fatalf("legacy reset was not rejected: %+v", result)
	}
}
