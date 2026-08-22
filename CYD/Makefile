ENV  ?= cyd
PORT ?= /dev/ttyUSB0
BAUD ?= 921600

# PlatformIO core dir — override if you use a non-default PLATFORMIO_CORE_DIR,
# e.g. `make flash PIO_CORE=/path/to/core`.
PIO_CORE ?= $(HOME)/.platformio

BUILD_DIR   := .pio/build/$(ENV)
ESPTOOL     := $(PIO_CORE)/packages/tool-esptoolpy/esptool.py
BOOT_APP0   := $(PIO_CORE)/packages/framework-arduinoespressif32/tools/partitions/boot_app0.bin

.PHONY: build flash upload ota

build:
	pio run -e $(ENV)

# Flash the binaries already in $(BUILD_DIR) without rebuilding.
flash:
	@test -f "$(ESPTOOL)" || { echo "esptool not found at $(ESPTOOL) -- set PIO_CORE"; exit 1; }
	@test -f "$(BOOT_APP0)" || { echo "boot_app0.bin not found at $(BOOT_APP0) -- set PIO_CORE"; exit 1; }
	python3 "$(ESPTOOL)" --chip esp32 --port "$(PORT)" --baud $(BAUD) \
		--before default_reset --after hard_reset write_flash -z \
		--flash_mode dio --flash_freq 40m --flash_size detect \
		0x1000  $(BUILD_DIR)/bootloader.bin \
		0x8000  $(BUILD_DIR)/partitions.bin \
		0xe000  $(BOOT_APP0) \
		0x10000 $(BUILD_DIR)/firmware.bin

upload: build flash

# Push a new build over the LAN instead of USB. Needs CYD_OTA_PASSWORD set
# (see platformio.ini) and the device already running firmware with OTA
# support -- see CYD/README.md.
ota:
	pio run -e cyd_ota -t upload
