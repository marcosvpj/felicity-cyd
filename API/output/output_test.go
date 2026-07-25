package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cydsolar-api/outlook"
)

func TestBuildPayload(t *testing.T) {
	fetched := time.Date(2026, 7, 24, 18, 0, 0, 0, time.UTC)
	estimates := []outlook.DayEstimate{
		{Date: "2026-07-25", KwhEst: 2.7734999999999999},
		{Date: "2026-07-26", KwhEst: 1.1},
	}

	got := BuildPayload(estimates, fetched)

	if got.Fetched != "2026-07-24T18:00:00Z" {
		t.Errorf("Fetched = %q, want %q", got.Fetched, "2026-07-24T18:00:00Z")
	}
	if len(got.Days) != 2 {
		t.Fatalf("got %d days, want 2", len(got.Days))
	}
	if got.Days[0].KwhEst != 2.8 {
		t.Errorf("Days[0].KwhEst = %v, want 2.8 (rounded to 1 decimal)", got.Days[0].KwhEst)
	}
	if got.Days[0].Level != "ok" {
		t.Errorf("Days[0].Level = %q, want %q", got.Days[0].Level, "ok")
	}
	if got.Days[1].Level != "bad" {
		t.Errorf("Days[1].Level = %q, want %q", got.Days[1].Level, "bad")
	}
}

// TestBuildPayload_LevelMatchesRoundedKwhEst pins down that level is derived
// from the rounded kwh_est, not the raw estimate: an unrounded value just
// under the bad/ok boundary must not ship as level=bad next to kwh_est=2.5.
func TestBuildPayload_LevelMatchesRoundedKwhEst(t *testing.T) {
	estimates := []outlook.DayEstimate{
		{Date: "2026-07-25", KwhEst: 2.4999},
	}

	got := BuildPayload(estimates, time.Now())

	if got.Days[0].KwhEst != 2.5 {
		t.Fatalf("KwhEst = %v, want 2.5 (rounded)", got.Days[0].KwhEst)
	}
	if got.Days[0].Level != "ok" {
		t.Errorf("Level = %q, want %q (must match the rounded 2.5, not raw 2.4999)", got.Days[0].Level, "ok")
	}
}

func TestWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outlook.json")

	payload := BuildPayload([]outlook.DayEstimate{
		{Date: "2026-07-25", KwhEst: 2.8},
	}, time.Now())

	if err := Write(path, payload); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	var got Payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshaling written file: %v", err)
	}
	if len(got.Days) != 1 || got.Days[0].Date != "2026-07-25" {
		t.Errorf("written payload = %+v, want the built payload", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("written file mode = %o, want 0644 (must be world-readable for Caddy)", perm)
	}

	// No leftover temp files after a successful write.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries after Write, want 1 (no leftover temp file): %v", len(entries), entries)
	}
}

func TestWrite_OverwritesPreviousGoodPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outlook.json")

	first := BuildPayload([]outlook.DayEstimate{{Date: "2026-07-25", KwhEst: 2.8}}, time.Now())
	if err := Write(path, first); err != nil {
		t.Fatalf("first Write: %v", err)
	}

	second := BuildPayload([]outlook.DayEstimate{{Date: "2026-07-26", KwhEst: 5.5}}, time.Now())
	if err := Write(path, second); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	var got Payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshaling written file: %v", err)
	}
	if len(got.Days) != 1 || got.Days[0].Date != "2026-07-26" {
		t.Errorf("written payload = %+v, want the second payload to have replaced the first", got)
	}
}
