package server

import (
	"testing"

	"bridge-core/internal/model"
)

func TestCustomerTrafficNeverFallsBackToHistoricalAllTime(t *testing.T) {
	if got := customerClientTrafficUsed(model.XUIClientView{Up: 0, Down: 0, TrafficTotal: 0, AllTime: 999}); got != 0 {
		t.Fatalf("historical usage resurrected: %d", got)
	}
}
