// Package output serializes outlook estimates into the Spec §4 outlook.json
// contract and writes them to disk. This is the infra/serialization layer —
// the outlook package stays free of encoding/json and os.
package output

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"cydsolar-api/outlook"
)

// Day is one entry of the Spec §4 outlook.json "days" array.
type Day struct {
	Date   string  `json:"date"`
	Level  string  `json:"level"`
	KwhEst float64 `json:"kwh_est"`
}

// Payload is the Spec §4 outlook.json contract.
type Payload struct {
	Fetched string `json:"fetched"`
	Days    []Day  `json:"days"`
}

// BuildPayload converts domain estimates into the wire contract. kwh_est is
// rounded to 1 decimal to match the Spec §4 example and avoid raw float
// noise (e.g. 2.7734999999999999). Level is re-derived from the rounded
// value so the two fields can never disagree at a threshold boundary (e.g.
// an unrounded 2.4999 must not emit level=bad next to kwh_est=2.5).
func BuildPayload(estimates []outlook.DayEstimate, fetched time.Time) Payload {
	days := make([]Day, 0, len(estimates))
	for _, e := range estimates {
		rounded := math.Round(e.KwhEst*10) / 10
		days = append(days, Day{
			Date:   e.Date,
			Level:  string(outlook.LevelFor(rounded)),
			KwhEst: rounded,
		})
	}
	return Payload{
		Fetched: fetched.UTC().Format(time.RFC3339),
		Days:    days,
	}
}

// Write serializes payload and writes it to path atomically: it writes to a
// temp file in the same directory, then renames it into place, so a reader
// (Caddy) never observes a partial write. Write is only ever called after a
// successful Open-Meteo fetch — if the caller returns early on a fetch
// error, path is left untouched, which is the Spec §3 "camada 1" VPS cache
// (last good response keeps being served).
func Write(path string, payload Payload) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}
	return writeAtomic(path, data)
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".outlook-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	// os.CreateTemp mode is 0600; Caddy runs as a different user and needs
	// to read this file, so widen it before it becomes the real file.
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp file into place: %w", err)
	}
	return nil
}
