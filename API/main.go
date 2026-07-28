// Generates outlook.json from Open-Meteo and writes it atomically to -out.
// Meant to run as a systemd timer job every 3-6h, with Caddy serving -out
// as a static file. If the fetch fails, the process exits non-zero without
// touching -out, so Caddy keeps serving the last good payload (the VPS-side
// cache layer) until the next timer run succeeds.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"time"

	"cydsolar-api/openmeteo"
	"cydsolar-api/outlook"
	"cydsolar-api/output"
)

const (
	// outlookDays is the "tomorrow + 2" window the display shows.
	outlookDays = 3
	// requestDays includes today so we can drop it: Open-Meteo's "today" is
	// already partially elapsed and isn't part of the outlook.
	requestDays = outlookDays + 1

	saoPauloTZ = "America/Sao_Paulo"
)

func main() {
	outPath := flag.String("out", "outlook.json", "path to write outlook.json; left untouched if this run fails")
	lat := flag.Float64("lat", math.NaN(), "site latitude (required)")
	lon := flag.Float64("lon", math.NaN(), "site longitude (required)")
	flag.Parse()

	if math.IsNaN(*lat) || math.IsNaN(*lon) {
		log.Fatal("-lat and -lon are both required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := openmeteo.NewClient()
	if err := run(ctx, client, *outPath, *lat, *lon, time.Now()); err != nil {
		log.Fatalf("outlook generation failed, %s left untouched: %v", *outPath, err)
	}
}

func run(ctx context.Context, client *openmeteo.Client, outPath string, latitude, longitude float64, now time.Time) error {
	resp, err := client.FetchShortwaveRadiation(ctx, latitude, longitude, requestDays)
	if err != nil {
		return fmt.Errorf("fetching forecast: %w", err)
	}

	readings, err := toHourlyReadings(resp)
	if err != nil {
		return fmt.Errorf("parsing forecast: %w", err)
	}

	loc, err := time.LoadLocation(saoPauloTZ)
	if err != nil {
		return fmt.Errorf("loading timezone: %w", err)
	}
	today := now.In(loc).Format("2006-01-02")

	estimates := outlook.Estimate(readings, outlook.DefaultPanelKwp, outlook.DefaultDerate)
	days := make([]outlook.DayEstimate, 0, outlookDays)
	for _, e := range estimates {
		if e.Date == today {
			continue // today is already partially elapsed, not part of the outlook.
		}
		days = append(days, e)
		if len(days) == outlookDays {
			break
		}
	}
	// requestDays always spans requestDays timezone-aligned days; today's
	// date always matches exactly one of them, so a valid response yields
	// exactly outlookDays here. Anything else (empty/short body from
	// Open-Meteo) is a semantically bad response — refuse to overwrite the
	// last good outlook.json with it (the cache layer covers "API returns
	// junk", not just "API is down").
	if len(days) != outlookDays {
		return fmt.Errorf("got %d outlook days after dropping today, want %d — refusing to overwrite last good payload", len(days), outlookDays)
	}

	payload := output.BuildPayload(days, now)
	if err := output.Write(outPath, payload); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	return nil
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
