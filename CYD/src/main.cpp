#include <Arduino.h>
#include <WiFi.h>
#include <TFT_eSPI.h>
#include <time.h>
#include "felicity.h"
#include "display.h"
#include "outlook.h"

// --- Config: fill in for your environment ----------------------------------
// Provide real values in include/secrets.h (gitignored) — see README.
#include "secrets.h"
static const char*    WIFI_SSID          = SECRET_WIFI_SSID;
static const char*    WIFI_PSK           = SECRET_WIFI_PSK;
static const char*    HOST               = SECRET_HOST;    // dongle on LAN
static const uint16_t PORT              = 53970;
static const uint32_t POLL_INTERVAL_MS  = 10000;           // 10 s
static const float    BATTERY_CAPACITY_AH = 100.0f;        // nameplate Ah of your pack
static const long     UTC_OFFSET_SEC    = -3 * 3600;       // BRT = UTC-3; adjust for your zone

// Forecast pull (Spec §3: second, independent path — never gates the SoC path).
static const char*    OUTLOOK_URL             = "https://dashboard-solar.marcosvpj.xyz/outlook.json";
static const uint32_t OUTLOOK_POLL_INTERVAL_MS = 15UL * 60 * 1000;  // 15 min
// ---------------------------------------------------------------------------

// --- SOC history (circular buffer, one entry per successful poll) ----------
static const uint16_t HIST_SIZE = 8640;  // 8640 × 10 s = 24 h of history
static uint8_t  socHist[HIST_SIZE];
static uint16_t histHead      = 0;
static uint16_t histCount     = 0;
static uint32_t histFirstMs   = 0;

static void pushSoc(float soc) {
    if (histCount == 0) {
        histFirstMs = millis();
    } else if (histCount >= HIST_SIZE) {
        histFirstMs += POLL_INTERVAL_MS;   // oldest slot is being overwritten
    }
    socHist[histHead] = (uint8_t)constrain((int)roundf(soc), 0, 100);
    histHead = (histHead + 1) % HIST_SIZE;
    if (histCount < HIST_SIZE) histCount++;
}

static void getTimeStr(char* buf, size_t sz) {
    struct tm ti;
    if (getLocalTime(&ti, 0)) snprintf(buf, sz, "%02d:%02d", ti.tm_hour, ti.tm_min);
    else                       snprintf(buf, sz, "--:--");
}

// Full date+time for the outlook fetch log — the point of NTP here is date
// resolution (Spec §5), so the log must show the date, not just HH:MM.
static void getTimestampStr(char* buf, size_t sz) {
    struct tm ti;
    if (getLocalTime(&ti, 0)) strftime(buf, sz, "%Y-%m-%dT%H:%M:%S", &ti);
    else                       snprintf(buf, sz, "unsynced");
}
// ---------------------------------------------------------------------------

static BatteryReading lastGood;
static uint32_t       lastOutlookFetchMs = 0;
static bool           outlookFetchedOnce = false;

static bool connectWifi(uint32_t timeoutMs = 15000) {
    WiFi.mode(WIFI_STA);
    WiFi.begin(WIFI_SSID, WIFI_PSK);
    uint32_t start = millis();
    while (WiFi.status() != WL_CONNECTED) {
        if (millis() - start > timeoutMs) return false;
        delay(250);
    }
    return true;
}

void setup() {
    Serial.begin(115200);
    delay(200);

    displayInit();
    displayStatus("Connecting WiFi...", TFT_WHITE);

    if (!connectWifi()) {
        displayStatus("WiFi FAILED", TFT_RED);
        delay(3000);
        ESP.restart();
    }
    Serial.printf("WiFi OK, IP: %s\n", WiFi.localIP().toString().c_str());

    configTime(UTC_OFFSET_SEC, 0, "pool.ntp.org", "time.cloudflare.com");
    displayStatus("Syncing time...", TFT_WHITE);
    // Wait up to 2 s for NTP; display will show "--:--" if it doesn't arrive
    struct tm ti;
    for (int i = 0; i < 8 && !getLocalTime(&ti, 0); i++) delay(250);

    displayStatus("Connecting to battery...", TFT_WHITE);
}

void loop() {
    if (WiFi.status() != WL_CONNECTED) {
        displayStatus("WiFi lost, retrying", TFT_YELLOW);
        WiFi.reconnect();
        delay(3000);
        return;
    }

    char timeStr[8];
    getTimeStr(timeStr, sizeof(timeStr));

    // Forecast pull — a separate connection from the battery TCP client
    // (Spec §3), so a fetch failure never corrupts or blocks SoC data.
    // It does run synchronously here before the read below, so a slow
    // fetch can delay this cycle's SoC refresh by up to TIMEOUT_MS
    // (outlook.cpp); harmless at a 15-min cadence for v0.
    //
    // Wait for NTP before the first attempt so the log's timestamp is
    // always real (Spec §5 needs dates, not "unsynced").
    struct tm nowTm;
    bool timeSynced = getLocalTime(&nowTm, 0);
    bool outlookDue = timeSynced &&
        (!outlookFetchedOnce || millis() - lastOutlookFetchMs >= OUTLOOK_POLL_INTERVAL_MS);
    if (outlookDue) {
        outlookFetchedOnce = true;
        lastOutlookFetchMs = millis();

        String outlookJson;
        char ts[24];
        getTimestampStr(ts, sizeof(ts));
        if (fetchOutlook(OUTLOOK_URL, outlookJson)) {
            Serial.printf("[outlook] %s %s\n", ts, outlookJson.c_str());
        } else {
            Serial.printf("[outlook] %s fetch failed\n", ts);
        }
    }

    BatteryReading r;
    if (readBattery(HOST, PORT, r)) {
        lastGood = r;
        pushSoc(r.socPct);
        uint32_t spanSec = histCount > 0 ? (millis() - histFirstMs) / 1000 : 0;
        displayReading(r, BATTERY_CAPACITY_AH,
                       socHist, histHead, histCount, HIST_SIZE,
                       spanSec, timeStr);
        Serial.printf("OK  V=%.2f I=%.1f SOC=%.1f\n",
                      r.voltageV, r.currentA, r.socPct);
    } else {
        Serial.println("read failed");
        if (lastGood.valid) {
            uint32_t spanSec = histCount > 0 ? (millis() - histFirstMs) / 1000 : 0;
            displayReading(lastGood, BATTERY_CAPACITY_AH,
                           socHist, histHead, histCount, HIST_SIZE,
                           spanSec, timeStr);
        } else {
            displayStatus("No data", TFT_RED);
        }
    }

    delay(POLL_INTERVAL_MS);
}
