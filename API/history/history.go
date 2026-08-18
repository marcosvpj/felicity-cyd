// Package history appends one NDJSON line per forecast run to a file, so
// forecast accuracy can be evaluated later against real production once
// that data exists. Each line carries PSH (the model-independent quantity)
// alongside the published kwh_est/level and the kwp/derate that were in
// effect, so a later derate recalibration doesn't strand the record.
package history

import (
	"encoding/json"
	"fmt"
	"os"
)

// Day is one calendar day within a run's forecast window.
type Day struct {
	Date   string  `json:"date"`
	Psh    float64 `json:"psh"`
	KwhEst float64 `json:"kwh_est"`
	Level  string  `json:"level"`
}

// Record is one run of the outlook generator: the whole multi-day forecast
// window it produced, archived as a single line. The run, not the day, is
// the natural unit -- it's what lets a per-horizon degradation (D-1 vs D-2
// vs D-3 accuracy) be measured later.
type Record struct {
	RunAt  string  `json:"run_at"`
	Kwp    float64 `json:"kwp"`
	Derate float64 `json:"derate"`
	Days   []Day   `json:"days"`
}

// Append serializes record as one JSON line and appends it to path,
// creating the file if it doesn't exist. O_APPEND makes each call's write
// atomic with respect to other appenders at this size.
func Append(path string, record Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshaling history record: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening history file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing history record: %w", err)
	}
	return nil
}
