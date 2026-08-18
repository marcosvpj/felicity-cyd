package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cydsolar-api/history"
	"cydsolar-api/openmeteo"
	"cydsolar-api/outlook"
	"cydsolar-api/output"
)

// Arbitrary coordinates for tests -- fakeOpenMeteo ignores the query string
// entirely, so any values exercise the same code path.
const (
	testLat = 0.0
	testLon = 0.0
)

// fakeOpenMeteo serves a fixed hourly response: "today" plus 3 forecast days,
// each with a constant W/m² value chosen so its kwh_est lands in a known
// level tier (bad/ok/good), so the test also pins down the rounding and
// level boundaries end to end.
func fakeOpenMeteo(t *testing.T, today time.Time) *httptest.Server {
	t.Helper()

	type hourly struct {
		Time               []string  `json:"time"`
		ShortwaveRadiation []float64 `json:"shortwave_radiation"`
	}
	var h hourly
	dayValues := []float64{250, 100, 200, 300} // today(dropped), bad, ok, good
	for d, wm2 := range dayValues {
		date := today.AddDate(0, 0, d)
		for hour := range 24 {
			h.Time = append(h.Time, fmt.Sprintf("%sT%02d:00", date.Format("2006-01-02"), hour))
			h.ShortwaveRadiation = append(h.ShortwaveRadiation, wm2)
		}
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Hourly hourly `json:"hourly"`
		}{Hourly: h})
	}))
}

func TestRun_WritesContractPayload(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	server := fakeOpenMeteo(t, today)
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "outlook.json")
	historyPath := filepath.Join(dir, "forecast-history.ndjson")
	client := openmeteo.NewClientWithBaseURL(server.URL)

	if err := run(context.Background(), client, outPath, historyPath, testLat, testLon, today); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading %s: %v", outPath, err)
	}
	var got output.Payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshaling payload: %v", err)
	}

	if len(got.Days) != 3 {
		t.Fatalf("got %d days, want 3 (today dropped): %+v", len(got.Days), got.Days)
	}
	wantDates := []string{"2026-07-26", "2026-07-27", "2026-07-28"}
	wantLevels := []string{"bad", "ok", "good"}
	for i, d := range got.Days {
		if d.Date != wantDates[i] {
			t.Errorf("Days[%d].Date = %q, want %q", i, d.Date, wantDates[i])
		}
		if d.Level != wantLevels[i] {
			t.Errorf("Days[%d].Level = %q, want %q", i, d.Level, wantLevels[i])
		}
	}
}

func TestRun_ArchivesOneHistoryLinePerRun(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	server := fakeOpenMeteo(t, today)
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "outlook.json")
	historyPath := filepath.Join(dir, "forecast-history.ndjson")
	client := openmeteo.NewClientWithBaseURL(server.URL)

	if err := run(context.Background(), client, outPath, historyPath, testLat, testLon, today); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := run(context.Background(), client, outPath, historyPath, testLat, testLon, today.Add(4*time.Hour)); err != nil {
		t.Fatalf("second run: %v", err)
	}

	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("reading %s: %v", historyPath, err)
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d history lines, want 2 (one per run): %s", len(lines), data)
	}

	var rec history.Record
	if err := json.Unmarshal(lines[0], &rec); err != nil {
		t.Fatalf("unmarshaling first history line: %v", err)
	}
	if rec.RunAt != today.UTC().Format(time.RFC3339) {
		t.Errorf("RunAt = %q, want %q", rec.RunAt, today.UTC().Format(time.RFC3339))
	}
	if rec.Kwp != outlook.DefaultPanelKwp || rec.Derate != outlook.DefaultDerate {
		t.Errorf("Kwp/Derate = %v/%v, want %v/%v", rec.Kwp, rec.Derate, outlook.DefaultPanelKwp, outlook.DefaultDerate)
	}
	wantDates := []string{"2026-07-26", "2026-07-27", "2026-07-28"}
	wantLevels := []string{"bad", "ok", "good"}
	if len(rec.Days) != 3 {
		t.Fatalf("got %d days, want 3: %+v", len(rec.Days), rec.Days)
	}
	for i, d := range rec.Days {
		if d.Date != wantDates[i] {
			t.Errorf("Days[%d].Date = %q, want %q", i, d.Date, wantDates[i])
		}
		if d.Level != wantLevels[i] {
			t.Errorf("Days[%d].Level = %q, want %q", i, d.Level, wantLevels[i])
		}
		if d.Psh <= 0 {
			t.Errorf("Days[%d].Psh = %v, want > 0", i, d.Psh)
		}
	}
}

// TestRun_HistoryPshRoundedToOneDecimal uses non-round decimal radiation
// values (as real Open-Meteo data has) instead of fakeOpenMeteo's integer
// fixture: summing 24 non-exact binary floats accumulates noise (e.g.
// 4.883000000000001), and psh must come out rounded in the archived line —
// the same policy output.BuildPayload already applies to kwh_est.
func TestRun_HistoryPshRoundedToOneDecimal(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	type hourly struct {
		Time               []string  `json:"time"`
		ShortwaveRadiation []float64 `json:"shortwave_radiation"`
	}
	var h hourly
	// 24 hours of 187.3 W/m² -> sum 4495.2 Wh/m² -> psh 4.4952, which rounds
	// to 4.5. Repeated float addition of 187.3 does not land on 4495.2
	// exactly in binary, so this also exercises the noisy-sum path.
	const wm2 = 187.3
	for d := range 4 { // today + 3 forecast days
		date := today.AddDate(0, 0, d)
		for hour := range 24 {
			h.Time = append(h.Time, fmt.Sprintf("%sT%02d:00", date.Format("2006-01-02"), hour))
			h.ShortwaveRadiation = append(h.ShortwaveRadiation, wm2)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Hourly hourly `json:"hourly"`
		}{Hourly: h})
	}))
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "outlook.json")
	historyPath := filepath.Join(dir, "forecast-history.ndjson")
	client := openmeteo.NewClientWithBaseURL(server.URL)

	if err := run(context.Background(), client, outPath, historyPath, testLat, testLon, today); err != nil {
		t.Fatalf("run: %v", err)
	}

	line, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("reading %s: %v", historyPath, err)
	}
	var rec history.Record
	if err := json.Unmarshal(line, &rec); err != nil {
		t.Fatalf("unmarshaling history line: %v", err)
	}
	for i, d := range rec.Days {
		if d.Psh != 4.5 {
			t.Errorf("Days[%d].Psh = %v, want 4.5 (rounded, no float noise)", i, d.Psh)
		}
	}
	if bytes.Contains(line, []byte("000000000")) {
		t.Errorf("history line has unrounded float noise: %s", line)
	}
}

func TestRun_HistoryWriteFailureDoesNotFailRun(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	server := fakeOpenMeteo(t, today)
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "outlook.json")
	// A history path under a nonexistent parent directory: os.OpenFile
	// fails, but that must not affect outlook.json or run()'s return value.
	historyPath := filepath.Join(dir, "no-such-dir", "forecast-history.ndjson")
	client := openmeteo.NewClientWithBaseURL(server.URL)

	if err := run(context.Background(), client, outPath, historyPath, testLat, testLon, today); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.ReadFile(outPath); err != nil {
		t.Errorf("outlook.json missing despite history archive failure: %v", err)
	}
}

func TestRun_OpenMeteoDown_LeavesLastGoodPayloadUntouched(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	goodServer := fakeOpenMeteo(t, today)
	defer goodServer.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "outlook.json")
	historyPath := filepath.Join(dir, "forecast-history.ndjson")
	goodClient := openmeteo.NewClientWithBaseURL(goodServer.URL)

	if err := run(context.Background(), goodClient, outPath, historyPath, testLat, testLon, today); err != nil {
		t.Fatalf("seeding good run: %v", err)
	}
	before, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading seeded payload: %v", err)
	}

	downServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer downServer.Close()
	downClient := openmeteo.NewClientWithBaseURL(downServer.URL)

	err = run(context.Background(), downClient, outPath, historyPath, testLat, testLon, today.Add(4*time.Hour))
	if err == nil {
		t.Fatal("run with Open-Meteo down: got nil error, want an error")
	}

	after, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading payload after failed run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("outlook.json changed after a failed run:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestRun_OpenMeteoEmptyBody_LeavesLastGoodPayloadUntouched(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	goodServer := fakeOpenMeteo(t, today)
	defer goodServer.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "outlook.json")
	historyPath := filepath.Join(dir, "forecast-history.ndjson")
	goodClient := openmeteo.NewClientWithBaseURL(goodServer.URL)

	if err := run(context.Background(), goodClient, outPath, historyPath, testLat, testLon, today); err != nil {
		t.Fatalf("seeding good run: %v", err)
	}
	before, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading seeded payload: %v", err)
	}

	// A 200 OK with a semantically empty body (no "hourly" data) is worse
	// than the API being down: it decodes without error, so this must be
	// caught by the days-count guard in run(), not by the HTTP-error path.
	emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer emptyServer.Close()
	emptyClient := openmeteo.NewClientWithBaseURL(emptyServer.URL)

	err = run(context.Background(), emptyClient, outPath, historyPath, testLat, testLon, today.Add(4*time.Hour))
	if err == nil {
		t.Fatal("run with empty Open-Meteo body: got nil error, want an error")
	}

	after, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading payload after empty-body run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("outlook.json changed after an empty-body run:\nbefore: %s\nafter:  %s", before, after)
	}
}
