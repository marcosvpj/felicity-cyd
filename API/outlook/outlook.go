// Package outlook is the domain layer: turns hourly irradiance readings into
// a daily generation estimate. No infra dependencies (no net/http, no encoding/json).
package outlook

import "time"

const (
	DefaultPanelKwp = 1.1
	DefaultDerate   = 0.7
)

// HourlyReading is one hour of measured shortwave radiation (W/m²).
type HourlyReading struct {
	Time         time.Time
	RadiationWm2 float64
}

// DayEstimate is the estimated generation for a single calendar day.
type DayEstimate struct {
	Date   string // YYYY-MM-DD
	KwhEst float64
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
		})
	}
	return estimates
}
