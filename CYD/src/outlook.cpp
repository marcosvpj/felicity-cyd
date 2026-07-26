#include "outlook.h"
#include <HTTPClient.h>
#include <WiFiClientSecure.h>

// Bounds how long a stalled fetch can delay the next SoC read in loop()
// (sequential, single-core) — kept short rather than generous.
static const uint32_t TIMEOUT_MS = 5000;

bool fetchOutlook(const char* url, String& out) {
    WiFiClientSecure client;
    client.setInsecure();  // public read-only payload, no cert pinning (Spec §9)

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
