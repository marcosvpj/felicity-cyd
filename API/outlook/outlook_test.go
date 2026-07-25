package outlook

import (
	"testing"
	"time"
)

func TestEstimate(t *testing.T) {
	mkTime := func(day string, hour int) time.Time {
		tm, err := time.Parse("2006-01-02T15:04", day+"T00:00")
		if err != nil {
			t.Fatalf("bad fixture time: %v", err)
		}
		return tm.Add(time.Duration(hour) * time.Hour)
	}

	tests := []struct {
		name     string
		readings []HourlyReading
		panelKwp float64
		derate   float64
		want     []DayEstimate
	}{
		{
			name: "single day, two hours",
			readings: []HourlyReading{
				{Time: mkTime("2026-07-25", 10), RadiationWm2: 400},
				{Time: mkTime("2026-07-25", 11), RadiationWm2: 600},
			},
			panelKwp: 1.1,
			derate:   0.7,
			// sum = 1000 Wh/m² -> PSH 1 -> 1 * 1.1 * 0.7 = 0.77
			want: []DayEstimate{{Date: "2026-07-25", KwhEst: 0.77}},
		},
		{
			name: "multiple days preserve order",
			readings: []HourlyReading{
				{Time: mkTime("2026-07-26", 12), RadiationWm2: 500},
				{Time: mkTime("2026-07-25", 12), RadiationWm2: 1000},
			},
			panelKwp: 1.1,
			derate:   0.7,
			want: []DayEstimate{
				{Date: "2026-07-26", KwhEst: 0.385},
				{Date: "2026-07-25", KwhEst: 0.77},
			},
		},
		{
			name:     "no readings",
			readings: nil,
			panelKwp: 1.1,
			derate:   0.7,
			want:     []DayEstimate{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Estimate(tt.readings, tt.panelKwp, tt.derate)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d estimates, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i].Date != tt.want[i].Date {
					t.Errorf("estimate[%d].Date = %q, want %q", i, got[i].Date, tt.want[i].Date)
				}
				if diff := got[i].KwhEst - tt.want[i].KwhEst; diff > 1e-9 || diff < -1e-9 {
					t.Errorf("estimate[%d].KwhEst = %v, want %v", i, got[i].KwhEst, tt.want[i].KwhEst)
				}
			}
		})
	}
}
