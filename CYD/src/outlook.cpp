#include "outlook.h"
#include <ArduinoJson.h>
#include <HTTPClient.h>
#include <Preferences.h>
#include <WiFiClientSecure.h>

// Bounds how long a stalled fetch can delay the next SoC read in loop()
// (sequential, single-core) — kept short rather than generous.
static const uint32_t TIMEOUT_MS = 5000;

bool fetchOutlook(const char* url, String& out) {
    WiFiClientSecure client;
    client.setInsecure();  // public read-only payload, no cert pinning

    HTTPClient http;
    http.setConnectTimeout(TIMEOUT_MS);
    http.setTimeout(TIMEOUT_MS);
    if (!http.begin(client, url)) return false;

    int code = http.GET();
    if (code != HTTP_CODE_OK) {
        http.end();
        return false;
    }

    out = http.getString();
    http.end();
    return true;
}

bool parseOutlook(const String& json, Outlook& out) {
    JsonDocument doc;
    if (deserializeJson(doc, json) != DeserializationError::Ok) return false;

    Outlook o = {};
    snprintf(o.fetched, sizeof(o.fetched), "%s", doc["fetched"] | "");

    JsonArrayConst days = doc["days"].as<JsonArrayConst>();
    for (JsonObjectConst d : days) {
        if (o.count >= 3) break;

        const char* date  = d["date"]  | "";
        const char* level = d["level"] | "";
        if (!date[0] || !level[0]) return false;  // malformed entry — don't cache garbage

        OutlookDay& day = o.days[o.count];
        snprintf(day.date, sizeof(day.date), "%s", date);
        snprintf(day.level, sizeof(day.level), "%s", level);
        day.kwhEst = d["kwh_est"] | 0.0f;
        o.count++;
    }
    if (o.count == 0) return false;

    out = o;
    return true;
}

static const char* NVS_NAMESPACE = "outlook";
static const char* NVS_KEY_JSON  = "json";

bool saveOutlookCache(const String& rawJson) {
    Preferences prefs;
    if (!prefs.begin(NVS_NAMESPACE, false)) return false;
    size_t written = prefs.putString(NVS_KEY_JSON, rawJson);
    prefs.end();
    return written == rawJson.length();
}

bool loadOutlookCache(Outlook& out) {
    Preferences prefs;
    if (!prefs.begin(NVS_NAMESPACE, true)) return false;
    String raw = prefs.getString(NVS_KEY_JSON, "");
    prefs.end();
    if (raw.length() == 0) return false;
    return parseOutlook(raw, out);
}

// Days since 1970-01-01 for a proleptic-Gregorian Y-M-D, per Howard
// Hinnant's civil_from_days algorithm. Used instead of timegm()/mktime()
// to convert `fetched` (always UTC) to epoch seconds without pulling in
// libc TZ behavior — mktime() in particular would wrongly apply this
// device's local UTC_OFFSET_SEC to an already-UTC timestamp.
static int64_t daysFromCivil(int y, int m, int d) {
    y -= m <= 2;
    int64_t era = (y >= 0 ? y : y - 399) / 400;
    unsigned yoe = (unsigned)(y - era * 400);
    unsigned doy = (153 * (m + (m > 2 ? -3 : 9)) + 2) / 5 + d - 1;
    unsigned doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    return era * 146097 + (int64_t)doe - 719468;
}

// Parses "YYYY-MM-DDTHH:MM:SSZ" (RFC3339 UTC, as emitted by API/output.go)
// into epoch seconds. Returns 0 on malformed input.
static time_t parseIsoUtc(const char* s) {
    int y, mo, d, h, mi, se;
    if (sscanf(s, "%d-%d-%dT%d:%d:%dZ", &y, &mo, &d, &h, &mi, &se) != 6) return 0;
    int64_t days = daysFromCivil(y, mo, d);
    return (time_t)(days * 86400 + h * 3600 + mi * 60 + se);
}

ResolvedForecast resolveOutlook(const Outlook& cache, const char* todayDate, time_t nowEpoch) {
    ResolvedForecast r = {};

    time_t fetchedEpoch = parseIsoUtc(cache.fetched);
    r.cacheAgeSec = (fetchedEpoch > 0 && nowEpoch >= fetchedEpoch)
                        ? (uint32_t)(nowEpoch - fetchedEpoch)
                        : 0;

    // Cache days arrive in chronological order from the VPS — no re-sort
    // needed for up to 3 entries.
    for (uint8_t i = 0; i < cache.count && r.count < 3; i++) {
        if (strcmp(cache.days[i].date, todayDate) > 0) {
            r.days[r.count++] = cache.days[i];
        }
    }
    r.degraded = (r.count == 0);
    return r;
}
