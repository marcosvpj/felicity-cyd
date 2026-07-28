#pragma once
#include <Arduino.h>
#include <time.h>

// Fetch outlook.json from the VPS. Independent path from the battery TCP
// client — failures here must never affect SoC readings.
//
// Payload is small, public and read-only, so TLS is used without cert
// pinning (setInsecure()). Returns true and fills `out` with the raw JSON
// body on success; `out` is left untouched on failure.
bool fetchOutlook(const char* url, String& out);

// One day of the outlook.json wire contract, decoded.
struct OutlookDay {
    char  date[11];   // "YYYY-MM-DD"
    char  level[8];   // "good" | "ok" | "bad"
    float kwhEst;
};

// Decoded outlook.json. `count` is 0..3; `fetched` is the
// RFC3339 UTC instant the VPS produced this payload (e.g.
// "2026-07-24T18:00:00Z").
struct Outlook {
    char       fetched[25];
    OutlookDay days[3];
    uint8_t    count;
};

// Parses a raw outlook.json body into `out`. Returns false (and leaves
// `out` untouched) on malformed JSON or an unexpected shape — the caller
// must not cache or act on a partial/garbage result.
bool parseOutlook(const String& json, Outlook& out);

// NVS-backed cache: survives reboot / power cut, so the CYD keeps showing
// the last known forecast when the internet is down.
// Persists the raw JSON bytes (not the decoded struct) so load always goes
// through the same `parseOutlook` path as a live fetch. Returns false if
// the NVS write failed (e.g. `Preferences::begin` couldn't open the
// namespace) — the caller should retry on a later fetch rather than
// assuming the cache is now up to date.
bool saveOutlookCache(const String& rawJson);

// Loads the cached outlook.json from NVS into `out`. Returns false (and
// leaves `out` untouched) if nothing has ever been cached.
bool loadOutlookCache(Outlook& out);

// Result of resolving a cached forecast against the current date. `count`
// is the number of cache entries whose date is strictly after `todayDate`
// (i.e. "tomorrow" and beyond), in cache order.
struct ResolvedForecast {
    bool       degraded;     // true: no future date in the cache — show the degraded state
    uint32_t   cacheAgeSec;  // now - cache.fetched, for the on-screen freshness indicator
    OutlookDay days[3];
    uint8_t    count;
};

// `todayDate` is "YYYY-MM-DD" in the same local zone the outlook.json
// dates are in — the caller derives it from the resolved wall clock
// (NTP-synced, or coasting on the ESP32 timer once synced once).
// `nowEpoch` is UTC epoch seconds (`time(nullptr)`), used only for the
// cache-age calculation.
//
// Caller must not call this with an unsynced clock: an arbitrary
// `todayDate` (e.g. the 1970 epoch default) would make every cached date
// look like it's in the future and report a bogus fresh forecast instead
// of degrading.
ResolvedForecast resolveOutlook(const Outlook& cache, const char* todayDate, time_t nowEpoch);
