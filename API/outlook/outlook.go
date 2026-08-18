// Package outlook is the domain layer: turns hourly irradiance readings into
// a daily generation estimate. No infra dependencies (no net/http, no encoding/json).
package outlook

import "time"

const (
	DefaultPanelKwp = 1.1
	DefaultDerate   = 0.7
)

// Level is the good/ok/bad generation tier for a day.
type Level string

const (
	LevelBad  Level = "bad"
	LevelOk   Level = "ok"
	LevelGood Level = "good"
)

// Thresholds derived from a year of Open-Meteo archive data for this site
// and array — they're specific to this installation's panel size and
// location, not a general rule of thumb.
const (
	thresholdBadOk  = 2.5 // kwh_est below this is bad
	thresholdOkGood = 4.5 // kwh_est at or above this is good
)

// LevelFor derives the good/ok/bad tier for an estimated daily generation.
func LevelFor(kwhEst float64) Level {
	switch {
	case kwhEst < thresholdBadOk:
		return LevelBad
	case kwhEst < thresholdOkGood:
		return LevelOk
	default:
		return LevelGood
	}
}

// HourlyReading is one hour of measured shortwave radiation (W/m²).
type HourlyReading struct {
	Time         time.Time
	RadiationWm2 float64
}

// DayEstimate is the estimated generation for a single calendar day. It
// carries no Level: the wire-contract rounding of kwh_est (output.BuildPayload)
// happens after this, and the level must be derived from the rounded value
// so the two published fields can't disagree at a threshold boundary.
type DayEstimate struct {
	Date   string // YYYY-MM-DD
	KwhEst float64
	// Psh (peak sun hours) is kept alongside KwhEst because it's independent
	// of panelKwp/derate: a future derate recalibration can't reconstruct
	// kwh_est from a KwhEst-only history, but it can from Psh.
	Psh float64
}

// Estimate groups hourly readings by calendar day and derives kWh_est per day:
// sum(W/m²) over the day's hours = Wh/m² (PSH*1000) -> ÷1000 = PSH (peak sun hours)
// -> kWh_est = PSH × panelKwp × derate.
func Estimate(readings []HourlyReading, panelKwp, derate float64) []DayEstimate {
	whPerM2ByDay := make(map[string]float64)
	var order []string

	for _, r := range readings {
		day := r.Time.Format("2006-01-02")
		if _, seen := whPerM2ByDay[day]; !seen {
			order = append(order, day)
		}
		whPerM2ByDay[day] += r.RadiationWm2
	}

	estimates := make([]DayEstimate, 0, len(order))
	for _, day := range order {
		psh := whPerM2ByDay[day] / 1000
		estimates = append(estimates, DayEstimate{
			Date:   day,
			KwhEst: psh * panelKwp * derate,
			Psh:    psh,
		})
	}
	return estimates
}
