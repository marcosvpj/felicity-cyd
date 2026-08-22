# CYD — firmware

ESP32 firmware for the Cheap Yellow Display. Reads the Felicity battery over
the LAN, pulls a generation forecast from an HTTP endpoint, renders both on a
single non-interactive screen.

Product overview and photos: [root README](../README.md).

## Build

```sh
cp include/secrets.h.example include/secrets.h   # gitignored
make upload
```

| Target | Does |
|---|---|
| `make build` | `pio run` — compile only. |
| `make flash` | Flash the binaries already in `.pio/build/<env>/` via `esptool.py`, no rebuild. |
| `make upload` | `build` + `flash`. |

Defaults: `ENV=cyd`, `PORT=/dev/ttyUSB0`, `BAUD=921600`. Override inline:
`make flash PORT=/dev/ttyACM0`.

> If your PlatformIO core lives somewhere other than `~/.platformio`, pass it in:
> `make flash PIO_CORE=/path/to/.platformio`.

### Updating over the air

Once a device is running firmware with OTA support (i.e. anything built after
this feature landed), later updates can be pushed over WiFi instead of USB —
useful when the board isn't within reach of a cable. The uploading machine
must be on the same LAN as the device:

```sh
CYD_OTA_PASSWORD=<value from secrets.h> make ota
```

The device advertises itself via mDNS as `cyd-solar.local`, so this works
without knowing its current DHCP-assigned IP. The first flash on a fresh
board still has to go over USB (`make upload`) — OTA only updates a device
that's already running this firmware.

If `cyd-solar.local` doesn't resolve — the ESP32's mDNS responder can be
slow to re-advertise after a WiFi reconnect, which happens routinely at a
site where the AP is powered off overnight — fall back to the device's IP
(from your router's DHCP leases, or the serial log at boot):

```sh
CYD_OTA_PASSWORD=<value from secrets.h> make ota IP=192.168.1.42
```

### Board

Assumes **ESP32-2432S028R** — the common CYD variant: ILI9341, 320×240,
resistive touch (unused here). All TFT_eSPI pins are passed as **build flags**
in `platformio.ini`; the library's `User_Setup.h` is never edited, so a clean
`pio` install builds this repo as-is.

Some clones ship an ST7789. If yours does, swap `-DILI9341_2_DRIVER=1` in
`platformio.ini` for the matching driver define.

## Configuration

Everything environment-specific lives in `include/secrets.h` (gitignored):

```c
#define SECRET_WIFI_SSID "..."
#define SECRET_WIFI_PSK  "..."
#define SECRET_HOST      "192.168.1.100"   // dongle's LAN IP
#define SECRET_OUTLOOK_URL "https://your-outlook-host.example/outlook.json"
#define SECRET_BATTERY_CAPACITY_AH 100.0f  // nameplate Ah of your pack
#define SECRET_UTC_OFFSET_SEC (-3 * 3600)  // your timezone
#define SECRET_OTA_PASSWORD "..."          // gates OTA updates, see below
```

`SECRET_OUTLOOK_URL` points at your own forecast service — see
[`API/README.md`](../API/README.md). Running without one is fine too; the
display just falls back to its degraded no-forecast state.

Things still hardcoded in `main.cpp` that you will want to change:

| Constant | Default | Note |
|---|---|---|
| `POLL_INTERVAL_MS` | 10 s | Battery poll. |
| `OUTLOOK_POLL_INTERVAL_MS` | 15 min | Forecast poll. |

## Architecture

Five translation units, each replaceable in isolation:

```
felicity.{h,cpp}   protocol client   host/port  -> BatteryReading
outlook.{h,cpp}    forecast client   URL        -> Outlook -> ResolvedForecast
display.{h,cpp}    renderer          structs    -> pixels
ota.{h,cpp}        firmware updates  ArduinoOTA callbacks -> flash
main.cpp           orchestration     WiFi, NTP, cadence, ring buffer, errors
```

- `felicity` has no display and no `Serial` dependency. Swapping the battery
  source for Modbus RTU means rewriting this file only.
- `outlook` owns fetch, parse, the NVS cache, and date resolution. It has no
  display dependency.
- `display` has no network dependency. Moving to a round GC9A01 means rewriting
  this file only.
- `main` is the only place that knows about time, WiFi, and cadence.

### Two independent data paths

The battery read is over plain TCP on the LAN and depends on nothing else. The
forecast is an HTTPS pull from a VPS. **A failure in the forecast path never
gates or delays the battery path** — that separation is the point, not an
accident of layering. Internet at this site is intermittent by design (Starlink
gets powered down when I go to bed); the battery number must keep updating
regardless.

### Cache and date resolution

The forecast is cached in NVS as **raw JSON bytes**, not as the decoded struct,
so a cache load goes through the exact same `parseOutlook` path as a live fetch.
One parser, one set of bugs.

Entries are keyed by **absolute date** (`YYYY-MM-DD`), never by "tomorrow". A
cache holding "tomorrow = 2.8 kWh" becomes a lie the moment the device is
offline across a midnight boundary. `resolveOutlook()` compares cached dates
against today and returns only strictly-future entries; when none remain it
returns `degraded`, and the display shows that state rather than stale numbers.

`resolveOutlook()` must not be called with an unsynced clock — the 1970 epoch
default would make every cached date look like the future and report a
confidently wrong forecast. `main.cpp` guards this: no NTP sync, no forecast.
The clock coasts on the ESP32's internal timer once synced, which drifts by
seconds per day — irrelevant when the only question is which calendar day it is.

### SoC history

288-slot ring buffer in RAM, sampled every 5 min = 24 h of sparkline. Sampling
runs on its own timer, decoupled from the 10 s battery poll — at poll cadence
the same span would need 8640 slots. History does not survive reboot; that's
accepted for now.

## Protocol

The Felicity WiFi dongle listens on **TCP :53970**, speaks ASCII commands,
answers JSON, and has no authentication. This firmware issues a single command:

```
wifilocalMonitor:get dev real infor
```

The framing rules, field layout, unit conversions and the list of still-undecoded
fields are documented separately in [`docs/PROTOCOL.md`](../docs/PROTOCOL.md) —
that document is useful even if you're not building this display.

Reference implementation: `src/felicity.cpp`.

## Observed quirks

- **Cell #5 runs ~80 mV high at top of charge.** Stable over hours, not
  widening. Read as cosmetic LFP top-of-curve behaviour rather than drift —
  revisit if the spread grows.
- **The ILI9341 shifts colour off-axis.** Green and orange converge when viewed
  from the side, which is exactly how a wall-mounted screen gets read. Levels
  are therefore distinguished by **icon shape as well as hue**, and never by hue
  alone.

## Roadmap

- Forecast icons derived from level (in progress).
- Degraded/stale forecast state needs its own visual treatment, not just an
  absence.
- Persist SoC history across reboots.
- Optional MQTT publish, so the same reading can feed InfluxDB/Grafana without a
  second poller hitting the dongle.
