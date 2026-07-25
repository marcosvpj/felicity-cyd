// Walking skeleton (CYDSOL-2): fetch Open-Meteo, compute kWh_est per day, print.
// No serving/serialization yet — that's CYDSOL-3+.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"cydsolar-api/openmeteo"
	"cydsolar-api/outlook"
)

// Spec §9: propriedade em Alfredo Wagner - SC.
const (
	latitude  = -27.7898
	longitude = -49.2854

	// outlookDays is the "amanhã + 2" window the display shows (Spec §4/§6).
	outlookDays = 3
	// requestDays includes today so we can drop it: Open-Meteo's "today" is
	// already partially elapsed and isn't part of the outlook.
	requestDays = outlookDays + 1
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := openmeteo.NewClient()
	resp, err := client.FetchShortwaveRadiation(ctx, latitude, longitude, requestDays)
	if err != nil {
		log.Fatalf("fetching forecast: %v", err)
	}

	readings, err := toHourlyReadings(resp)
	if err != nil {
		log.Fatalf("parsing forecast: %v", err)
	}

	estimates := outlook.Estimate(readings, outlook.DefaultPanelKwp, outlook.DefaultDerate)
	if len(estimates) > 0 {
		estimates = estimates[1:] // drop today, keep amanhã + 2
	}
	for _, e := range estimates {
		fmt.Printf("%s: %.2f kWh_est\n", e.Date, e.KwhEst)
	}
}

// toHourlyReadings adapts the infra response into domain value objects.
func toHourlyReadings(resp *openmeteo.Response) ([]outlook.HourlyReading, error) {
	times := resp.Hourly.Time
	radiation := resp.Hourly.ShortwaveRadiation
	if len(times) != len(radiation) {
		return nil, fmt.Errorf("hourly time/radiation length mismatch: %d vs %d", len(times), len(radiation))
	}

	readings := make([]outlook.HourlyReading, 0, len(times))
	for i, t := range times {
		parsed, err := time.Parse("2006-01-02T15:04", t)
		if err != nil {
			return nil, fmt.Errorf("parsing hourly time %q: %w", t, err)
		}
		readings = append(readings, outlook.HourlyReading{
			Time:         parsed,
			RadiationWm2: radiation[i],
		})
	}
	return readings, nil
}
