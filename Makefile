ENV  ?= cyd
PORT ?= /dev/ttyUSB0
BAUD ?= 921600

BUILD_DIR   := .pio/build/$(ENV)
ESPTOOL     := /home/marcosvpjr/.platformio/packages/tool-esptoolpy/esptool.py
BOOT_APP0   := /home/marcosvpjr/.platformio/packages/framework-arduinoespressif32/tools/partitions/boot_app0.bin

.PHONY: build flash upload

build:
	pio run -e $(ENV)

# Flash the binaries already in $(BUILD_DIR) without rebuilding.
flash:
	python3 "$(ESPTOOL)" --chip esp32 --port "$(PORT)" --baud $(BAUD) \
		--before default_reset --after hard_reset write_flash -z \
		--flash_mode dio --flash_freq 40m --flash_size detect \
		0x1000  $(BUILD_DIR)/bootloader.bin \
		0x8000  $(BUILD_DIR)/partitions.bin \
		0xe000  $(BOOT_APP0) \
		0x10000 $(BUILD_DIR)/firmware.bin

upload: build flash
