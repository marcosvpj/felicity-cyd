# felicity-cyd-v0

Read-only monitor for Felicity LFP battery, running on ESP32 Cheap Yellow
Display. Connects to the dongle's local TCP API (port 53970), polls every
10 s, renders SOC / V / A / cell delta / temps on the TFT.

No internet required. No cloud. No MQTT (yet).

## Setup

1. Install PlatformIO (VSCode extension or CLI).
2. Copy `include/secrets.h.example` to `include/secrets.h` and fill in
   `SECRET_WIFI_SSID`, `SECRET_WIFI_PSK`, `SECRET_HOST`. This file is
   gitignored — never commit it.
3. Plug CYD into USB, run `make upload`.

## Makefile

| Target        | Does |
|---------------|------|
| `make build`  | Compile the firmware (`pio run`), no upload. |
| `make flash`  | Flash the binaries already in `.pio/build/<env>/` to the board via `esptool.py`, without rebuilding. |
| `make upload` | `build` + `flash` — compile then flash in one step. |

Defaults: `ENV=cyd`, `PORT=/dev/ttyUSB0`, `BAUD=921600`. Override on the
command line, e.g. `make flash PORT=/dev/ttyACM0`.

## CYD model assumed

ESP32-2432S028R — the most common variant. ILI9341 driver, 320x240,
resistive touch (touch not used here), USB-C or micro-USB depending on rev.

If your board uses a different driver (some clones ship ST7789), change
`-DILI9341_2_DRIVER=1` in `platformio.ini` accordingly.

## Architecture

Three modules, each replaceable:

- `felicity.{h,cpp}` — protocol client. Pure: takes host/port, returns
  `BatteryReading`. No display, no Serial dependency. To swap for Modbus
  later, only this file changes.
- `display.{h,cpp}` — renderer. Pure: takes a `BatteryReading`, renders.
  No network dependency. To change layout or move to round display
  (GC9A01), only this file changes.
- `main.cpp` — orchestration: WiFi, polling cadence, error handling.

## Pitfalls observed during development

- **The dongle stays silent on unknown commands** instead of returning an
  error. The reader uses JSON brace-counting to detect end-of-response
  rather than relying on socket close.
- **Response can span multiple TCP segments** (~800 bytes for `real infor`).
  Read accumulates until JSON is balanced.
- **Cell array has 16 slots, only the first N are valid**, marked by
  sentinel `32767` for the rest. 24V LFP = 8 cells.
- **Current unit is 0.1 A signed**, not 1 A. `-1` in the wire means -0.1 A.
- **Power slot (`Batt[2][0]`) is `null` when idle**, not 0. Derive P = V·I
  in the consumer if you need it.
- **Total voltage in `Batt[0]` and `BattList[0]` differ by ~40 mV.**
  Hypothesis: one is operational, one is raw cell sum. Treat `Batt[0]` as
  canonical for display; log both if you start persisting.
- **Cell #5 runs ~80 mV high at top of charge.** Not progressing over
  hours. Likely cosmetic LFP top-of-curve behavior; revisit if it widens.

## Known unknowns

- Bit meanings of `Estate`, `Bfault`, `Bwarn` not decoded. Observe what
  changes when load/charge state shifts.
- Two extra slots in `Batsoc` (`1000`, `200000`) likely SOH-equivalent
  and lifetime Ah throughput. Compare values across days to confirm.
- The `wifilocalMonitor:set cmd=<JSON>` write API needs further APK
  reverse engineering (jadx) to discover the JSON schema for each
  setting. Not relevant for read-only v0.

## Next milestones

- v0.1: persist last reading to NVS so display survives WiFi blips
  without a "No data" flash on boot.
- v0.2: add touch interaction — tap to cycle between SOC view,
  per-cell bar chart, raw flag decode.
- v1:   move polling into the Debian notebook (Go service), CYD becomes
  an MQTT subscriber, multiple displays possible, persistent metrics
  via InfluxDB.
