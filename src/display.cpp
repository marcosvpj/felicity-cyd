#include "display.h"
#include <TFT_eSPI.h>
#include <Arduino.h>

static TFT_eSPI tft;

void displayInit() {
    tft.init();
    tft.setRotation(1);
    tft.invertDisplay(true);   // ILI9341_2_DRIVER panel needs inversion, else green/red render as purple
    tft.fillScreen(TFT_BLACK);
    pinMode(TFT_BL, OUTPUT);
    digitalWrite(TFT_BL, HIGH);
}

// Muted status colors (RGB565) — softer than the stock TFT_GREEN/YELLOW/RED
// primaries, which read as too bright/saturated filling the whole screen.
static const uint16_t BG_GOOD     = 0x0D01;  // muted green   #0ca30c
static const uint16_t BG_WARNING  = 0xF583;  // muted amber   #fab219
static const uint16_t BG_CRITICAL = 0xC9E7;  // muted red     #d03b3b

static uint16_t bgForSoc(float soc) {
    if (soc >= 50) return BG_GOOD;
    if (soc >= 20) return BG_WARNING;
    return BG_CRITICAL;
}

static uint16_t darken(uint16_t c) {
    uint16_t r = ((c >> 11) & 0x1F) / 2;
    uint16_t g = ((c >>  5) & 0x3F) / 2;
    uint16_t b = ( c        & 0x1F) / 2;
    return (r << 11) | (g << 5) | b;
}

static void fmtHM(char* buf, size_t sz, float hours) {
    int h = (int)hours;
    int m = (int)((hours - h) * 60);
    if (h > 0) snprintf(buf, sz, "%dh %02dm", h, m);
    else        snprintf(buf, sz, "%dm",       m);
}

void displayReading(const BatteryReading& r, float capacityAh,
                    const uint8_t* hist, uint16_t histHead,
                    uint16_t histCount, uint16_t histSize,
                    uint32_t histSpanSec, const char* timeStr) {

    const uint16_t bg  = bgForSoc(r.socPct);
    const uint16_t fg  = TFT_BLACK;
    const uint16_t dim = darken(bg);
    tft.fillScreen(bg);

    // ── Header ─────────────────────────────────────── y 0..22 ──
    tft.setTextColor(fg, bg);
    tft.setTextDatum(ML_DATUM);
    tft.drawString("TREND", 8, 11, 2);
    tft.setTextDatum(MR_DATUM);
    tft.drawString(timeStr, 305, 11, 2);
    if (r.fault || r.warning)
        tft.fillCircle(314, 11, 5, r.fault ? TFT_RED : TFT_ORANGE);
    tft.drawFastHLine(0, 22, 320, dim);

    // ── Big SOC ────────────────────────────────────── y 23..103 ─
    char numBuf[8];
    snprintf(numBuf, sizeof(numBuf), "%.0f", r.socPct);
    tft.setTextColor(fg, bg);
    tft.setTextDatum(MR_DATUM);
    tft.drawString(numBuf, 175, 65, 7);   // font 7 = 48px 7-seg digits
    tft.setTextDatum(ML_DATUM);
    tft.drawString("%", 178, 42, 4);      // % in smaller font, superscript

    // ── Time estimate ──────────────────────────────── y 104..121 ─
    char estBuf[36] = "Idle";
    if (r.currentA > 0.1f) {
        char t[16];
        fmtHM(t, sizeof(t), ((100.0f - r.socPct) / 100.0f) * capacityAh / r.currentA);
        snprintf(estBuf, sizeof(estBuf), "+ Full in %s", t);
    } else if (r.currentA < -0.1f) {
        char t[16];
        fmtHM(t, sizeof(t), (r.socPct / 100.0f) * capacityAh / -r.currentA);
        snprintf(estBuf, sizeof(estBuf), "- Empty in %s", t);
    }
    tft.setTextColor(fg, bg);
    tft.setTextDatum(ML_DATUM);
    tft.drawString(estBuf, 8, 112, 2);

    tft.drawFastHLine(0, 122, 320, dim);

    // ── Chart header ───────────────────────────────── y 123..140 ─
    char spanBuf[20];
    if      (histSpanSec < 60)   snprintf(spanBuf, sizeof(spanBuf), "LAST %us",  (unsigned)histSpanSec);
    else if (histSpanSec < 3600) snprintf(spanBuf, sizeof(spanBuf), "LAST %um",  (unsigned)(histSpanSec / 60));
    else                         snprintf(spanBuf, sizeof(spanBuf), "LAST %uh",  (unsigned)(histSpanSec / 3600));
    tft.setTextColor(fg, bg);
    tft.setTextDatum(ML_DATUM);
    tft.drawString(spanBuf, 8, 131, 2);

    if (histCount >= 2) {
        uint16_t oldIdx = (uint16_t)((histHead - histCount + histSize) % histSize);
        uint16_t newIdx = (uint16_t)((histHead + histSize - 1) % histSize);
        int delta = (int)hist[newIdx] - (int)hist[oldIdx];
        char deltaBuf[12];
        snprintf(deltaBuf, sizeof(deltaBuf), "%+d%%", delta);
        tft.setTextDatum(MR_DATUM);
        tft.drawString(deltaBuf, 312, 131, 2);
    }

    // ── Area chart ─────────────────────────────────── y 141..193 ─
    const int16_t CY = 141, CW = 320, CH = 52;
    if (histCount >= 2) {
        for (int16_t sx = 0; sx < CW; sx++) {
            uint16_t hi  = (uint16_t)(((uint32_t)sx * (histCount - 1)) / (CW - 1));
            if (hi >= histCount) hi = histCount - 1;
            uint16_t idx = (uint16_t)((histHead - histCount + hi + histSize) % histSize);
            int16_t  py  = CY + CH - 1 - (int16_t)(hist[idx] * (CH - 2) / 100);
            tft.drawFastVLine(sx, py, CY + CH - py, dim);
        }
        for (int16_t sx = 0; sx < CW - 1; sx++) {
            uint16_t hi1 = (uint16_t)(((uint32_t)sx       * (histCount - 1)) / (CW - 1));
            uint16_t hi2 = (uint16_t)(((uint32_t)(sx + 1) * (histCount - 1)) / (CW - 1));
            if (hi1 >= histCount) hi1 = histCount - 1;
            if (hi2 >= histCount) hi2 = histCount - 1;
            uint16_t i1  = (uint16_t)((histHead - histCount + hi1 + histSize) % histSize);
            uint16_t i2  = (uint16_t)((histHead - histCount + hi2 + histSize) % histSize);
            int16_t py1  = CY + CH - 1 - (int16_t)(hist[i1] * (CH - 2) / 100);
            int16_t py2  = CY + CH - 1 - (int16_t)(hist[i2] * (CH - 2) / 100);
            tft.drawLine(sx, py1, sx + 1, py2, fg);
        }
    }

    tft.drawFastHLine(0, 194, 320, dim);

    // ── Bottom stats ───────────────────────────────── y 195..240 ─
    // VOLT
    tft.setTextColor(dim, bg);
    tft.setTextDatum(ML_DATUM);
    tft.drawString("VOLT", 10, 205, 2);
    tft.setTextColor(fg, bg);
    char voltBuf[12];
    snprintf(voltBuf, sizeof(voltBuf), "%.2f", r.voltageV);
    tft.drawString(voltBuf, 10, 225, 4);

    // AMP
    tft.setTextColor(dim, bg);
    tft.setTextDatum(MC_DATUM);
    tft.drawString("AMP", 160, 205, 2);
    tft.setTextColor(fg, bg);
    char ampBuf[12];
    snprintf(ampBuf, sizeof(ampBuf), "%+.1f", r.currentA);
    tft.drawString(ampBuf, 160, 225, 4);

    // TEMP
    tft.setTextColor(dim, bg);
    tft.setTextDatum(MR_DATUM);
    tft.drawString("TEMP", 310, 205, 2);
    tft.setTextColor(fg, bg);
    char tempBuf[14];
    snprintf(tempBuf, sizeof(tempBuf), "%.0f/%.0f\xb0", r.tempMinC, r.tempMaxC);
    tft.drawString(tempBuf, 310, 225, 4);
}

void displayStatus(const char* msg, uint16_t color) {
    tft.fillScreen(TFT_BLACK);
    tft.setTextDatum(MC_DATUM);
    tft.setTextColor(color, TFT_BLACK);
    tft.drawString(msg, 160, 120, 4);
}
