#include "display.h"
#include <TFT_eSPI.h>
#include <Arduino.h>
#include <math.h>
#include <string.h>

static TFT_eSPI tft;

void displayInit() {
    tft.init();
    tft.setRotation(1);
    tft.invertDisplay(true);   // ILI9341_2_DRIVER panel needs inversion, else green/red render as purple
    tft.fillScreen(TFT_BLACK);
    pinMode(TFT_BL, OUTPUT);
    digitalWrite(TFT_BL, HIGH);
}

// Spec §7 palette. No custom condensed font asset yet (CYDSOL-8 note /
// §6.1): built-in TFT_eSPI fonts are used as fallback, so identity is
// approximate until the font is converted and dropped into CYD/.
static uint16_t COL_DIVIDER, COL_LABEL, COL_GOOD, COL_BAD, COL_OK;

static void initPalette() {
    static bool done = false;
    if (done) return;
    COL_DIVIDER = tft.color565(0x33, 0x33, 0x33);
    COL_LABEL   = tft.color565(0x6E, 0x6E, 0x6E);
    COL_GOOD    = tft.color565(0x00, 0xE0, 0x5A);
    COL_BAD     = tft.color565(0xFF, 0x7A, 0x18);
    COL_OK      = TFT_WHITE;
    done = true;
}

// Forecast level -> color (Spec §7): good=green, bad=orange, ok=neutral.
// Never derived from weathercode -- level/kwh_est is the only input.
static uint16_t levelColor(const char* level) {
    if (!strcmp(level, "good")) return COL_GOOD;
    if (!strcmp(level, "bad"))  return COL_BAD;
    return COL_OK;
}

// SoC number color by range (Spec §7). Red is reserved for this -- never
// used for a bad forecast, which stays orange.
static uint16_t socColor(float soc) {
    if (soc < 50) return TFT_RED;
    if (soc < 70) return TFT_YELLOW;
    return COL_GOOD;
}

static void fmtHM(char* buf, size_t sz, float hours) {
    int h = (int)hours;
    int m = (int)((hours - h) * 60);
    if (h > 0) snprintf(buf, sz, "%dh %02dm", h, m);
    else        snprintf(buf, sz, "%dm",       m);
}

// Procedural sky icons (Spec §7: distinct by SHAPE + color, not hue alone --
// the ILI9341's viewing-angle shift can bring green and orange/yellow close
// together from the side).
static void drawSun(int cx, int cy, int size, uint16_t color) {
    int r = size / 3;
    tft.fillCircle(cx, cy, r, color);
    for (int a = 0; a < 8; a++) {
        float ang = a * (PI / 4.0f);
        int x1 = cx + (int)(cosf(ang) * r * 1.4f), y1 = cy + (int)(sinf(ang) * r * 1.4f);
        int x2 = cx + (int)(cosf(ang) * r * 1.9f), y2 = cy + (int)(sinf(ang) * r * 1.9f);
        tft.drawLine(x1, y1, x2, y2, color);
    }
}

static void drawCloud(int cx, int cy, int size, uint16_t color) {
    int r = size / 4;
    tft.fillCircle(cx - r,           cy,           r,                      color);
    tft.fillCircle(cx + (int)(r * 0.8f), cy - r / 2, (int)(r * 1.2f),      color);
    tft.fillCircle(cx + r * 2,       cy,           r,                      color);
    tft.fillRect(cx - r * 2, cy, r * 4, r, color);
}

static void drawIcon(int cx, int cy, int size, const char* level) {
    uint16_t color = levelColor(level);
    if (!strcmp(level, "good")) {
        drawSun(cx, cy, size, color);
    } else if (!strcmp(level, "bad")) {
        drawCloud(cx, cy, size, color);
    } else {
        drawSun(cx - size / 5, cy - size / 6, (int)(size * 0.6f), color);
        drawCloud(cx + size / 6, cy + size / 8, (int)(size * 0.8f), color);
    }
}

void displayReading(const BatteryReading& r, float capacityAh,
                    const uint8_t* hist, uint16_t histHead,
                    uint16_t histCount, uint16_t histSize,
                    uint32_t histSpanSec, const char* timeStr,
                    const ResolvedForecast& outlook) {
    (void)histSpanSec;  // base region title is the fixed "SoC 24h" of Spec §6.1,
                         // not a dynamic span -- v0 accepts a gap post-reboot (§6).
    initPalette();

    tft.fillScreen(TFT_BLACK);
    tft.drawFastVLine(240, 0, 168, COL_DIVIDER);
    tft.drawFastHLine(0, 168, 320, COL_DIVIDER);

    // ── Topo-esquerda: estado atual (0,0)-(239,167) ── Spec §6.1 ──
    tft.setTextColor(COL_LABEL, TFT_BLACK);
    tft.setTextDatum(TL_DATUM);
    tft.drawString("SOLAR", 8, 8, 1);
    tft.setTextDatum(TR_DATUM);
    tft.drawString(timeStr, 232, 8, 1);

    char numBuf[8];
    snprintf(numBuf, sizeof(numBuf), "%.0f", r.socPct);
    tft.setTextColor(socColor(r.socPct), TFT_BLACK);
    tft.setTextDatum(TL_DATUM);
    tft.drawString(numBuf, 8, 20, 7);              // font 7: 48px digits, dominant element
    int numW = tft.textWidth(numBuf, 7);
    tft.setTextDatum(BL_DATUM);
    tft.drawString("%", 8 + numW + 4, 20 + 48, 4);  // ~half height, aligned to number's base

    char estBuf[24];
    if (r.currentA < -0.1f) {
        char t[16];
        fmtHM(t, sizeof(t), (r.socPct / 100.0f) * capacityAh / -r.currentA);
        snprintf(estBuf, sizeof(estBuf), "vazio em %s", t);
    } else if (r.currentA > 0.1f) {
        char t[16];
        fmtHM(t, sizeof(t), ((100.0f - r.socPct) / 100.0f) * capacityAh / r.currentA);
        snprintf(estBuf, sizeof(estBuf), "cheio em %s", t);
    } else {
        snprintf(estBuf, sizeof(estBuf), "ocioso");
    }
    tft.setTextColor(TFT_WHITE, TFT_BLACK);
    tft.setTextDatum(TL_DATUM);
    tft.drawString(estBuf, 8, 118, 2);

    char voltNum[8], ampNum[8];
    snprintf(voltNum, sizeof(voltNum), "%.1f", r.voltageV);
    snprintf(ampNum, sizeof(ampNum), "%+.1f", r.currentA);
    int x = 8;
    tft.setTextDatum(TL_DATUM);
    tft.setTextColor(TFT_WHITE, TFT_BLACK);
    tft.drawString(voltNum, x, 144, 4);
    x += tft.textWidth(voltNum, 4);
    tft.setTextColor(COL_LABEL, TFT_BLACK);
    tft.drawString("V", x, 154, 2);
    x += tft.textWidth("V", 2) + 14;
    tft.setTextColor(TFT_WHITE, TFT_BLACK);
    tft.drawString(ampNum, x, 144, 4);
    x += tft.textWidth(ampNum, 4);
    tft.setTextColor(COL_LABEL, TFT_BLACK);
    tft.drawString("A", x, 154, 2);

    // ── Topo-direita: previsao (240,0)-(319,167), ~80px ── Spec §6.1 ──
    tft.setTextColor(COL_LABEL, TFT_BLACK);
    tft.setTextDatum(TL_DATUM);
    tft.drawString("PREV.", 248, 8, 1);

    const int colCx = 280;
    if (outlook.count > 0) {
        const OutlookDay& tomorrow = outlook.days[0];
        drawIcon(colCx, 26, 30, tomorrow.level);

        char kwhBuf[8];
        snprintf(kwhBuf, sizeof(kwhBuf), "%.1f", tomorrow.kwhEst);
        tft.setTextColor(levelColor(tomorrow.level), TFT_BLACK);
        tft.setTextDatum(TC_DATUM);
        tft.drawString(kwhBuf, colCx, 46, 4);
        tft.setTextColor(COL_LABEL, TFT_BLACK);
        tft.drawString("kWh AMANHA", colCx, 74, 1);

        const int mx[2] = {259, 301};
        for (uint8_t i = 1; i <= 2 && i < outlook.count; i++) {
            const OutlookDay& d = outlook.days[i];
            drawIcon(mx[i - 1], 100, 18, d.level);
            char dBuf[6];
            snprintf(dBuf, sizeof(dBuf), "%.1f", d.kwhEst);
            tft.setTextColor(levelColor(d.level), TFT_BLACK);
            tft.setTextDatum(TC_DATUM);
            tft.drawString(dBuf, mx[i - 1], 112, 2);
        }

        char freshBuf[20];
        snprintf(freshBuf, sizeof(freshBuf), "prev. ha %uh", (unsigned)(outlook.cacheAgeSec / 3600));
        tft.setTextColor(COL_LABEL, TFT_BLACK);
        tft.setTextDatum(BL_DATUM);
        tft.drawString(freshBuf, 248, 159, 1);
    } else if (outlook.degraded) {
        // Spec §5: `degraded` is the authoritative signal (no cached date is
        // still in the future) -- gray "?" instead of an icon. `cacheAgeSec`
        // further distinguishes "had a cache, it went stale" from "never
        // fetched at all" (both are `degraded`, per resolveOutlook).
        tft.setTextColor(COL_LABEL, TFT_BLACK);
        tft.setTextDatum(TC_DATUM);
        tft.drawString("?", colCx, 30, 4);
        if (outlook.cacheAgeSec > 0) {
            tft.drawString("previsao", colCx, 80, 1);
            char staleBuf[16];
            snprintf(staleBuf, sizeof(staleBuf), "antiga %ud", (unsigned)(outlook.cacheAgeSec / 86400));
            tft.drawString(staleBuf, colCx, 92, 1);
        } else {
            tft.drawString("sem prev.", colCx, 80, 1);
        }
    }

    // ── Base: grafico 24h (0,168)-(319,239) ── Spec §6.1 ──
    tft.setTextColor(COL_LABEL, TFT_BLACK);
    tft.setTextDatum(TL_DATUM);
    tft.drawString("SoC 24h", 8, 172, 1);

    const int16_t GX = 8, GY = 188, GW = 304, GH = 44;
    if (histCount >= 2) {
        uint8_t vmax = 0, vmin = 100;
        for (uint16_t i = 0; i < histCount; i++) {
            uint16_t idx = (uint16_t)((histHead - histCount + i + histSize) % histSize);
            uint8_t v = hist[idx];
            if (v > vmax) vmax = v;
            if (v < vmin) vmin = v;
        }
        char scaleBuf[16];
        snprintf(scaleBuf, sizeof(scaleBuf), "%u / %u%%", vmax, vmin);
        tft.setTextDatum(TR_DATUM);
        tft.drawString(scaleBuf, 312, 172, 1);

        // Trace maps to [vmin, vmax], matching the scale label above --
        // otherwise the printed range and the plotted shape disagree.
        uint8_t range = (vmax > vmin) ? (vmax - vmin) : 1;
        int16_t prevX = -1, prevY = -1;
        for (int16_t sx = 0; sx < GW; sx++) {
            uint16_t hi = (uint16_t)(((uint32_t)sx * (histCount - 1)) / (GW - 1));
            if (hi >= histCount) hi = histCount - 1;
            uint16_t idx = (uint16_t)((histHead - histCount + hi + histSize) % histSize);
            int16_t py = GY + GH - 1 - (int16_t)((int32_t)(hist[idx] - vmin) * (GH - 1) / range);
            int16_t px = GX + sx;
            if (prevX >= 0) tft.drawLine(prevX, prevY, px, py, COL_GOOD);
            prevX = px;
            prevY = py;
        }
    }
}

void displayStatus(const char* msg, uint16_t color) {
    tft.fillScreen(TFT_BLACK);
    tft.setTextDatum(MC_DATUM);
    tft.setTextColor(color, TFT_BLACK);
    tft.drawString(msg, 160, 120, 4);
}
