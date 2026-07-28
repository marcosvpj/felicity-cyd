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

	"cydsolar-api/openmeteo"
	"cydsolar-api/output"
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
	client := openmeteo.NewClientWithBaseURL(server.URL)

	if err := run(context.Background(), client, outPath, today); err != nil {
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

func TestRun_OpenMeteoDown_LeavesLastGoodPayloadUntouched(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	goodServer := fakeOpenMeteo(t, today)
	defer goodServer.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "outlook.json")
	goodClient := openmeteo.NewClientWithBaseURL(goodServer.URL)

	if err := run(context.Background(), goodClient, outPath, today); err != nil {
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

	err = run(context.Background(), downClient, outPath, today.Add(4*time.Hour))
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
	goodClient := openmeteo.NewClientWithBaseURL(goodServer.URL)

	if err := run(context.Background(), goodClient, outPath, today); err != nil {
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

	err = run(context.Background(), emptyClient, outPath, today.Add(4*time.Hour))
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
