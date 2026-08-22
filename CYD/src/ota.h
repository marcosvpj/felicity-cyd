#pragma once

// Wraps ArduinoOTA so main.cpp doesn't need to know about its callback API.
// Requires the device to already be on the LAN (see connectWifi in main.cpp);
// updates are pushed from a machine on the same network, not pulled from a
// remote server.
void otaInit(const char* password);
void otaHandle();
