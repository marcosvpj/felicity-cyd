#include "ota.h"
#include <ArduinoOTA.h>
#include <TFT_eSPI.h>
#include "display.h"

// mDNS name the device advertises OTA under (matches platformio.ini's
// upload_port for the `cyd_ota` environment). Not a secret, so it's fixed
// here rather than in secrets.h.
static const char* OTA_HOSTNAME = "cyd-solar";

void otaInit(const char* password) {
    ArduinoOTA.setHostname(OTA_HOSTNAME);
    ArduinoOTA.setPassword(password);

    ArduinoOTA.onStart([]() {
        Serial.println("[ota] update starting");
        displayStatus("OTA update...", TFT_WHITE);
    });
    ArduinoOTA.onEnd([]() {
        Serial.println("[ota] update complete, rebooting");
    });
    ArduinoOTA.onProgress([](unsigned int done, unsigned int total) {
        Serial.printf("[ota] progress: %u%%\n", (done * 100) / total);
    });
    ArduinoOTA.onError([](ota_error_t error) {
        Serial.printf("[ota] error [%u]\n", error);
        displayStatus("OTA update failed", TFT_RED);
    });

    ArduinoOTA.begin();
    Serial.printf("[ota] ready, hostname=%s.local\n", OTA_HOSTNAME);
}

void otaHandle() {
    ArduinoOTA.handle();
}
