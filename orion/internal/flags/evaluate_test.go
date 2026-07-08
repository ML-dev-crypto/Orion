package flags

import (
	"testing"
	"time"
)

func TestEvaluateAt_ScheduleWindow(t *testing.T) {
	fixedNow := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	past := fixedNow.Add(-24 * time.Hour)
	future := fixedNow.Add(24 * time.Hour)

	tests := []struct {
		name string
		f    Flag
		want bool
	}{
		{
			name: "enabled false always loses, even with a passed schedule",
			f:    Flag{Enabled: false, RolloutPct: 100, Schedule: &Schedule{StartAt: &past}},
			want: false,
		},
		{
			name: "enabled true, schedule already started, no end -> active",
			f:    Flag{Enabled: true, RolloutPct: 100, Schedule: &Schedule{StartAt: &past}},
			want: true,
		},
		{
			name: "enabled true, schedule starts in the future -> not yet active",
			f:    Flag{Enabled: true, RolloutPct: 100, Schedule: &Schedule{StartAt: &future}},
			want: false,
		},
		{
			name: "enabled true, end_at already passed -> expired",
			f:    Flag{Enabled: true, RolloutPct: 100, Schedule: &Schedule{EndAt: &past}},
			want: false,
		},
		{
			name: "enabled true, currently inside start/end window -> active",
			f:    Flag{Enabled: true, RolloutPct: 100, Schedule: &Schedule{StartAt: &past, EndAt: &future}},
			want: true,
		},
		{
			name: "no schedule at all -> unrestricted",
			f:    Flag{Enabled: true, RolloutPct: 100},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.f.Key = "test_flag"
			got := evaluateAt(tt.f, EvalRequest{UserID: "alice"}, fixedNow)
			if got != tt.want {
				t.Errorf("evaluateAt() = %v, want %v", got, tt.want)
			}
		})
	}
}