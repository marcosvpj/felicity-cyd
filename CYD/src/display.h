#pragma once
#include "felicity.h"
#include "outlook.h"
#include <stdint.h>

void displayInit();
void displayReading(const BatteryReading& r, float capacityAh,
                    const uint8_t* hist, uint16_t histHead,
                    uint16_t histCount, uint16_t histSize,
                    uint32_t histSpanSec, const char* timeStr,
                    const ResolvedForecast& outlook);
void displayStatus(const char* msg, uint16_t color);
