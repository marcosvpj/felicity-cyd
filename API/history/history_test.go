package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppend_CreatesFileAndWritesOneLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forecast-history.ndjson")
	record := Record{
		RunAt:  "2026-08-18T18:00:00Z",
		Kwp:    1.1,
		Derate: 0.7,
		Days: []Day{
			{Date: "2026-08-19", Psh: 4.9, KwhEst: 3.8, Level: "ok"},
		},
	}

	if err := Append(path, record); err != nil {
		t.Fatalf("Append: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(lines), data)
	}

	var got Record
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("unmarshaling line: %v", err)
	}
	if got.RunAt != record.RunAt || got.Kwp != record.Kwp || got.Derate != record.Derate {
		t.Errorf("got %+v, want %+v", got, record)
	}
	if len(got.Days) != 1 || got.Days[0] != record.Days[0] {
		t.Errorf("got days %+v, want %+v", got.Days, record.Days)
	}
}

func TestAppend_SecondCallAddsSecondLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forecast-history.ndjson")

	if err := Append(path, Record{RunAt: "2026-08-18T18:00:00Z", Days: []Day{{Date: "2026-08-19"}}}); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if err := Append(path, Record{RunAt: "2026-08-18T22:00:00Z", Days: []Day{{Date: "2026-08-19"}}}); err != nil {
		t.Fatalf("second Append: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), data)
	}
	for i, line := range lines {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Errorf("line %d is not valid JSON: %v (%q)", i, err, line)
		}
	}
}

func TestAppend_UnwritablePathReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "forecast-history.ndjson")

	if err := Append(path, Record{RunAt: "2026-08-18T18:00:00Z"}); err == nil {
		t.Fatal("Append with missing parent dir: got nil error, want an error")
	}
}
