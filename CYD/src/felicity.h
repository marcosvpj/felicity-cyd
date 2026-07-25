#pragma once
#include <stdint.h>

// One snapshot of battery telemetry decoded into engineering units.
// Lives across loop iterations so display can keep showing last-known
// values during transient read failures.
struct BatteryReading {
    bool      valid       = false;   // false = struct content meaningless
    uint32_t  timestampMs = 0;       // millis() at successful read

    float     voltageV    = 0;       // total pack voltage
    float     currentA    = 0;       // signed: + charging, - discharging
    float     socPct      = 0;       // 0..100

    uint8_t   numCells    = 0;       // populated cells (8 for 24V LFP)
    uint16_t  cells[16]   = {0};     // per-cell mV
    uint16_t  cellMaxMv   = 0;
    uint16_t  cellMinMv   = 0;
    uint8_t   cellMaxIdx  = 0;       // 1-based from BMS
    uint8_t   cellMinIdx  = 0;

    float     tempMinC    = 0;
    float     tempMaxC    = 0;

    uint16_t  fault       = 0;       // Bfault bitmask
    uint16_t  warning     = 0;       // Bwarn bitmask
    uint16_t  state       = 0;       // Estate bitmask
};

// Blocking read. Returns true on success and fills `out`.
// On failure, `out` is left untouched (caller may keep showing stale data).
// Timeout is hard-capped at ~5s.
bool readBattery(const char* host, uint16_t port, BatteryReading& out);
