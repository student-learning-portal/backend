package domain

import (
	"testing"
	"time"
)

func TestClassifyRisk(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	cfg := RiskThresholds{MinProgressPercent: 40, MaxInactiveDays: 7}

	day := func(n int) *time.Time {
		ts := now.Add(time.Duration(-n) * 24 * time.Hour)
		return &ts
	}

	cases := []struct {
		name       string
		p          StudentProgress
		wantStatus string
		wantDays   int
	}{
		{"on track", StudentProgress{ProgressPercent: 80, LastActivity: day(2)}, RiskOnTrack, 2},
		{"low progress, recent", StudentProgress{ProgressPercent: 10, LastActivity: day(1)}, RiskAtRisk, 1},
		{"good progress, inactive", StudentProgress{ProgressPercent: 90, LastActivity: day(20)}, RiskAtRisk, 20},
		{"enrolled, never active", StudentProgress{ProgressPercent: 0, LastActivity: nil}, RiskAtRisk, 0},
		{"progress at threshold is on track", StudentProgress{ProgressPercent: 40, LastActivity: day(2)}, RiskOnTrack, 2},
		{"inactivity at threshold is on track", StudentProgress{ProgressPercent: 90, LastActivity: day(7)}, RiskOnTrack, 7},
		{"inactivity one day past threshold", StudentProgress{ProgressPercent: 90, LastActivity: day(8)}, RiskAtRisk, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, days := ClassifyRisk(tc.p, now, cfg)
			if status != tc.wantStatus {
				t.Errorf("status = %q, want %q", status, tc.wantStatus)
			}
			if days != tc.wantDays {
				t.Errorf("daysInactive = %d, want %d", days, tc.wantDays)
			}
		})
	}
}
