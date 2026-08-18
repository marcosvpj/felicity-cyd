# API — forecast service

A small Go binary that turns an irradiance forecast into "how many kWh will the
panels make tomorrow", writes it to a JSON file, and gets out of the way. Caddy
serves the file statically; the CYD pulls it every 15 minutes.

Product overview: [root README](../README.md).

## Why a file and not a server

The payload is three numbers that change every few hours. Anything more than a
static file would be ceremony:

- No process is listening, so nothing can crash at 3 a.m.
- A web server that already exists (Caddy) does the serving, TLS, and caching.
- "Serve the last good value when upstream is down" costs zero code — it's just
  *not overwriting the file*.

The binary runs from a systemd timer every ~4 h, exits, and is gone.

## The contract

`outlook.json`:

```json
{
  "fetched": "2026-07-24T18:00:00Z",
  "days": [
    { "date": "2026-07-25", "level": "good", "kwh_est": 4.8 },
    { "date": "2026-07-26", "level": "ok",   "kwh_est": 3.1 },
    { "date": "2026-07-27", "level": "bad",  "kwh_est": 1.1 }
  ]
}
```

**Dates are absolute.** Never "tomorrow", never day indices. The consumer is a
device that may be offline across midnight; a relative label goes stale
silently, an absolute date can be checked. This single decision is what makes
the CYD's degraded state possible at all.

`fetched` is when the *VPS* produced the payload, which is what lets the display
show "forecast is 6 h old".

Today is deliberately **excluded**: it's already half-elapsed, so its estimate
answers a question nobody is asking.

### Levels

| Level | kWh estimate |
|---|---|
| `bad` | < 2.5 |
| `ok` | 2.5 – 4.5 |
| `good` | ≥ 4.5 |

Derived from a year of Open-Meteo archive data for this specific site, for a
1.1 kWp array. **They are site- and array-specific** — recalibrate for yours, in
`outlook/outlook.go`.

`level` is re-derived from the *rounded* `kwh_est`, not the raw float. Otherwise
an unrounded `2.4999` would publish `level: "bad"` next to `kwh_est: 2.5` and
the two fields would visibly disagree at the boundary.

## The estimate

```
sum(hourly shortwave_radiation W/m²) over a local day  =  Wh/m²
Wh/m² ÷ 1000                                           =  PSH (peak sun hours)
kwh_est = PSH × panel_kWp × derate
```

Defaults: `panel_kWp = 1.1`, `derate = 0.7`.

The derate absorbs everything the model doesn't know: panel temperature, dust,
MPPT efficiency, cable loss, module tilt versus solar angle, and the fact that
these panels sit at 900 m in a valley that holds morning fog. It is an
empirical fudge factor, not physics. Tune it against your own production data.

Open-Meteo is queried with `timezone=America/Sao_Paulo` — without it, hourly
buckets are UTC and daily aggregation smears across local midnight.

## Layout

```
main.go            wiring: fetch -> adapt -> estimate -> serialize -> write
openmeteo/         infra: HTTP client for the forecast API
outlook/           domain: irradiance -> daily kWh estimate, level thresholds
output/            infra: the outlook.json wire contract + atomic write
history/           infra: the forecast-history.ndjson append-only log
deploy/            systemd units, Caddy snippet, deployment notes
```

`outlook/` imports neither `net/http` nor `encoding/json`. That's the whole
architecture: the part that decides what a good solar day is doesn't know the
internet exists, and can be tested without one. Everything else is plumbing.
Small tool, no DDD ceremony beyond that line.

## Failure behaviour

If Open-Meteo is unreachable, or returns a malformed body, or returns fewer days
than expected, the process **exits non-zero without touching the output file**.
Caddy keeps serving the last good payload, and the next timer run tries again.

This covers "the API returned junk" and not just "the API is down" — a semantic
check on the response shape (`got N outlook days, want 3`) guards against
quietly publishing an empty `days` array.

Writes are atomic: temp file in the same directory, `chmod 0644`, then rename.
Caddy can never observe a half-written JSON.

Covered by `TestRun_OpenMeteoDown_LeavesLastGoodPayloadUntouched` in
`main_test.go`.

## Forecast history

Every run also appends one NDJSON line to `-history` (default
`forecast-history.ndjson`), recording the whole run — not just `outlook.json`'s
published fields:

```json
{"run_at":"2026-07-24T18:00:00Z","kwp":1.1,"derate":0.7,
 "days":[{"date":"2026-07-25","psh":4.9,"kwh_est":3.8,"level":"ok"}]}
```

`psh` (peak sun hours) is stored raw, separate from `kwh_est`: it's the
quantity independent of the `kwp`/`derate` model, so a future derate
recalibration can still reconstruct what would have been published, which a
`kwh_est`-only history couldn't. The run — all forecast days in one line — is
the archived unit, not the day, so forecast accuracy can later be compared by
horizon (a next-day estimate vs. a two-day-out one).

Archiving a run is secondary to publishing it: a failure to append (e.g. the
history file's directory doesn't exist) is logged and does not fail the run
or affect `outlook.json`. `outlook.json` is always written first.

There's no reader for this file yet — it exists purely to stop losing
forecast history to each run overwriting the last one. `jq` is enough to pull
it into a spreadsheet once there's a use for it.

## Configuration

Site latitude and longitude are **required command-line flags** (`-lat`,
`-lon`) — the binary refuses to start without them. `-out` and `-history`
default to relative paths for local runs; production passes absolute ones
explicitly (see `deploy/cydsolar-outlook.service`). Panel size, derate, and the
level thresholds are **compile-time constants** (`outlook/outlook.go`). Fine
for a single-site tool; if you're running this for your own property, edit and
rebuild for those, or pass your own `-lat`/`-lon` at the call site.

## Build and deploy

```sh
go test ./...
go build -o cydsolar-api .
./cydsolar-api -out /var/www/dashboard-solar/outlook.json \
  -history /var/lib/cydsolar/forecast-history.ndjson \
  -lat <your-lat> -lon <your-lon>
```

CI ships it on every push to `main` touching `API/**`. Full deployment
instructions — systemd units, Caddy snippet, first-run bootstrap — in
[`deploy/README.md`](deploy/README.md).
