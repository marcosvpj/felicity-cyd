#pragma once
#include "felicity.h"
#include <stdint.h>

void displayInit();
void displayReading(const BatteryReading& r, float capacityAh,
                    const uint8_t* hist, uint16_t histHead,
                    uint16_t histCount, uint16_t histSize,
                    uint32_t histSpanSec, const char* timeStr);
void displayStatus(const char* msg, uint16_t color);
