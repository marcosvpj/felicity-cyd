# Felicity WiFi dongle — local TCP protocol

Reverse-engineered notes on the local network API exposed by the WiFi dongle
that ships with Felicity Solar LiFePO4 battery packs. Everything here was
derived by observing traffic between the vendor app and the dongle on a
**Felicity 24 V / 8S LiFePO4** pack. No vendor documentation was available.

This is what powers [`felicity-cyd`](../README.md), but the protocol is useful
on its own — you can talk to it from anything that opens a socket.

**If your model behaves differently, please open an issue or a PR.** Field
positions and units are the part most likely to vary between packs, and one
confirmed data point from another model is worth more than any amount of
guessing here.

---

## The short version

```sh
printf 'wifilocalMonitor:get dev real infor' | nc 192.168.1.100 53970
```

```json
{"Batt":[[26900],[31],[null]],"Batsoc":[[7300,1000,200000]], ...}
```

Plain TCP. ASCII command in, JSON out. **No authentication, no TLS, no
handshake.** The dongle is a small server sitting on your LAN.

## Finding the dongle

It gets an address from your DHCP server like any other client. Either read it
off the router's lease table, or sweep for the port:

```sh
nmap -p 53970 --open 192.168.1.0/24
```

Give it a static lease once you find it — the whole point of a local API is not
having to rediscover the host.

### A note on security

There is no authentication of any kind. Anyone who can reach the dongle on
TCP 53970 can read your full battery telemetry, and there is a write command
(see below) that appears to change device settings.

That's the vendor's design, not something this document introduces — but treat
it accordingly: keep the dongle on your trusted LAN, not on a guest network you
hand out to visitors, and don't port-forward 53970 to the internet under any
circumstances.

## Framing

This is the part that costs the most time, so it goes first.

**The dongle does not delimit responses and does not close the socket.** You
have to work out where a message ends by yourself. Three rules:

1. **Unknown commands produce silence.** Not an error, not a close — nothing.
   A reader that waits for *some* response before timing out will hang for the
   full timeout on every typo. Budget for it.
2. **A response spans multiple TCP segments.** `real infor` is around 800 bytes
   and arrives in pieces. A single `read()` gets you a fragment of JSON.
3. **So: parse incrementally and stop when the JSON object balances.** Count
   `{` and `}`, skipping braces inside strings and honouring backslash escapes.
   When depth returns to zero, the message is complete.

Reference implementation: [`CYD/src/felicity.cpp`](../CYD/src/felicity.cpp)
(`readJsonResponse`), ~30 lines.

A 4-second timeout has been reliable on a quiet LAN. Reconnect per request; the
dongle handles it fine and it avoids having to reason about a half-consumed
stream after an error.

## Commands

### `wifilocalMonitor:get dev real infor`

Live telemetry. This is the only command needed for a monitor, and the only one
documented in detail here.

**Note the spelling — `infor`, not `info`.** It is not a typo in this document.

### Others

The vendor app issues several other `wifilocalMonitor:get ...` commands
(device identity, configured limits, and what appears to be stored history),
plus a write in the shape of:

```
wifilocalMonitor:set cmd=<JSON>
```

Their exact strings and the per-setting JSON schema for `set` have not been
recovered — doing it properly means decompiling the Android app (`jadx`) rather
than guessing, and none of it is needed for read-only monitoring.

**Don't brute-force the command space against a live pack.** It's a BMS holding
several kWh; an accidentally-successful `set` is not a cheap mistake.

## `real infor` response

Top-level keys, with the meanings established so far. Values are nested arrays
even where a scalar would do — that's the wire format, not a transcription
error.

| Key | Shape | Meaning |
|---|---|---|
| `Batt` | `[[mV], [dA], [W or null]]` | Pack voltage, current, power |
| `Batsoc` | `[[soc, ?, ?]]` | State of charge, plus two undecoded values |
| `BatcelList` | `[[mV × 16]]` | Per-cell voltage, padded |
| `BMaxMin` | `[[maxMv, minMv], [maxIdx, minIdx]]` | Cell extremes and their 1-based indices |
| `BTemp` | `[[dC, dC], [dC, dC]]` | Probe temperatures, then BMS internal |
| `Estate` | integer | Bitmask — undecoded |
| `Bfault` | integer | Bitmask — undecoded |
| `Bwarn` | integer | Bitmask — undecoded |

### Units — read this twice

Nothing is in the unit you'd assume.

| Field | Wire | Convert |
|---|---|---|
| `Batt[0][0]` voltage | millivolts | `÷ 1000` → V |
| `Batt[1][0]` current | **signed deciamps** | `÷ 10` → A |
| `Batsoc[0][0]` SoC | centipercent | `÷ 100` → % |
| `BatcelList[0][n]` | millivolts | as-is |
| `BTemp[..][..]` | decidegrees C | `÷ 10` → °C |

**The current unit is the one that bites.** A wire value of `-1` is −0.1 A, not
−1 A. Read at the wrong scale it stays plausible for a long time — the sign and
the trend are both right, only the magnitude is off by 10× — so it can survive
a lot of casual testing before something downstream (a runtime estimate, a
coulomb count) starts producing nonsense.

Sign convention: **positive is charging**, negative is discharging.

### Cell array padding

`BatcelList[0]` always has **16 slots**. Only the first N are real; the
remainder carry the sentinel **`32767`**. A 24 V / 8S pack fills 8 and pads the
other 8.

Don't infer cell count from array length. Iterate until you hit `32767`, or
count entries within a plausible LFP range (roughly 2500–3800 mV).

### `Batt[2][0]` is `null`, not `0`

The power field is `null` when the pack is idle. A parser that assumes a number
there will throw or silently coerce depending on the language. Deriving
P = V·I downstream is more robust and costs nothing.

### A second, disagreeing pack voltage

Total pack voltage appears to show up twice in the payload — once in
`Batt[0][0]`, and again in a second field elsewhere in the response — with the
two differing by roughly **40 mV**, consistently rather than intermittently.
The exact key for the second field hasn't been re-confirmed against a live
payload since it was first noted, so it isn't named here; treat its existence
as a lead to verify, not a fact to build on.

Working hypothesis: one is an operational/filtered value from the BMS, the
other is a raw sum of the cell readings. Unconfirmed either way. `Batt[0][0]`
is treated as canonical here; if you start persisting metrics and can identify
the second field, log both — the delta itself might turn out to mean
something.

## Not yet decoded

The honest list. Contributions very welcome on any of these.

**`Estate`, `Bfault`, `Bwarn` bitmasks.** Nothing has faulted on this pack yet,
so there has been nothing to correlate bit changes against. Anyone who has
triggered a real over-temperature, over-current or cell-imbalance protection
and captured the payload has data that can't be produced on demand safely.

**The two extra values in `Batsoc`.** Observed as `1000` and `200000` alongside
the SoC. The first plausibly encodes state of health (100.0%?), the second
possibly lifetime amp-hour throughput. Confirming means comparing the same
fields across weeks on a pack that has cycled.

**Everything about the `set` command.** See above.

## Porting to other hardware

The client used here is one file with a struct-shaped output:

```c
bool readBattery(const char* host, uint16_t port, BatteryReading& out);
```

Everything downstream — display, caching, estimates — depends on
`BatteryReading` and not on this protocol. Swapping in Modbus RTU over RS485,
or another vendor's API, means rewriting that one file.

If you write a client in another language, a link here is welcome.
