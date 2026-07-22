#include "felicity.h"
#include <WiFi.h>
#include <ArduinoJson.h>

static const char* CMD_REAL_INFOR = "wifilocalMonitor:get dev real infor";
static const uint32_t TIMEOUT_MS  = 4000;
static const size_t   BUF_RESERVE = 2048;  // typical response ~800 bytes

// Read until the JSON object is balanced (depth returns to 0) or timeout.
// The Felicity dongle:
//   - sends the response possibly across several TCP segments
//   - keeps the socket open silently if it doesn't recognize the command
// So we must detect end-of-message ourselves; cannot rely on connection close.
static bool readJsonResponse(WiFiClient& client, String& out) {
    out.reserve(BUF_RESERVE);
    int depth = 0;
    bool inStr = false;
    bool escape = false;
    uint32_t start = millis();

    while (millis() - start < TIMEOUT_MS) {
        while (client.available()) {
            char c = client.read();
            out += c;

            if (escape) { escape = false; continue; }
            if (inStr) {
                if (c == '\\')      escape = true;
                else if (c == '"')  inStr = false;
            } else {
                if (c == '"')       inStr = true;
                else if (c == '{')  depth++;
                else if (c == '}') {
                    depth--;
                    if (depth == 0) return true;
                }
            }
        }
        if (!client.connected() && !client.available()) break;
        delay(5);
    }
    return false;
}

bool readBattery(const char* host, uint16_t port, BatteryReading& out) {
    WiFiClient client;
    client.setTimeout(TIMEOUT_MS / 1000);  // Arduino WiFiClient uses seconds

    if (!client.connect(host, port, TIMEOUT_MS)) return false;
    client.print(CMD_REAL_INFOR);

    String raw;
    bool gotResponse = readJsonResponse(client, raw);
    client.stop();
    if (!gotResponse) return false;

    JsonDocument doc;
    if (deserializeJson(doc, raw) != DeserializationError::Ok) return false;

    BatteryReading r;
    r.timestampMs = millis();

    // --- Top-level scalars ----------------------------------------------
    // Batt:    [[voltage_mV], [current_dA], [power_W or null]]
    // Batsoc:  [[soc_centipct, ?, ?]]
    r.voltageV = doc["Batt"][0][0].as<int32_t>()  / 1000.0f;
    r.currentA = doc["Batt"][1][0].as<int32_t>()  / 10.0f;
    r.socPct   = doc["Batsoc"][0][0].as<int32_t>() / 100.0f;

    r.fault    = doc["Bfault"].as<uint16_t>();
    r.warning  = doc["Bwarn"].as<uint16_t>();
    r.state    = doc["Estate"].as<uint16_t>();

    // --- Cells ----------------------------------------------------------
    // BatcelList[0] is 16 slots; 32767 marks "no cell here".
    JsonArrayConst cellArr = doc["BatcelList"][0].as<JsonArrayConst>();
    r.numCells = 0;
    for (JsonVariantConst v : cellArr) {
        uint16_t mv = v.as<uint16_t>();
        if (mv == 32767) break;
        if (r.numCells < 16) r.cells[r.numCells++] = mv;
    }
    r.cellMaxMv  = doc["BMaxMin"][0][0].as<uint16_t>();
    r.cellMinMv  = doc["BMaxMin"][0][1].as<uint16_t>();
    r.cellMaxIdx = doc["BMaxMin"][1][0].as<uint8_t>();
    r.cellMinIdx = doc["BMaxMin"][1][1].as<uint8_t>();

    // --- Temperature ----------------------------------------------------
    // BTemp: [[probe1_dC, probe2_dC], [bms_temp1_dC, bms_temp2_dC]]
    JsonArrayConst temps = doc["BTemp"][0].as<JsonArrayConst>();
    bool first = true;
    for (JsonVariantConst v : temps) {
        float t = v.as<int32_t>() / 10.0f;
        if (first) { r.tempMinC = r.tempMaxC = t; first = false; }
        else {
            if (t < r.tempMinC) r.tempMinC = t;
            if (t > r.tempMaxC) r.tempMaxC = t;
        }
    }

    r.valid = true;
    out = r;
    return true;
}
