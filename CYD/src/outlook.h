#pragma once
#include <Arduino.h>

// Fetch outlook.json from the VPS (Spec §3/§9). Independent path from the
// battery TCP client — failures here must never affect SoC readings.
//
// Payload is small, public and read-only, so TLS is used without cert
// pinning (setInsecure()). Returns true and fills `out` with the raw JSON
// body on success; `out` is left untouched on failure.
bool fetchOutlook(const char* url, String& out);
