package client

import (
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestBuildSummaryIncludesXUIDiskUsage(t *testing.T) {
	summary := buildSummary(model.AgentSnapshot{
		ReportedAt: time.Now().UTC(),
		XUI: &model.XUISnapshot{
			ServerStatus: model.XUIServerStatus{
				Disk: model.XUIUsage{Current: 42, Total: 100},
			},
		},
	})
	if summary.DiskUsed != 42 || summary.DiskTotal != 100 {
		t.Fatalf("expected disk usage from x-ui status, got %#v", summary)
	}
}
