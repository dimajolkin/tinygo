package builder

// This file implements support for writing ESP image files. These image files
// are read by the ROM bootloader so have to be in a particular format.
//
// In the future, it may be necessary to implement support for other image
// formats, such as the ESP8266 image formats (again, used by the ROM bootloader
// to load the firmware).

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

type espImageSegment struct {
	addr uint32
	data []byte
}

// makeESPFirmareImage converts an input ELF file to an image file for an ESP32 or
// ESP8266 chip. This is a special purpose image format just for the ESP chip
// family, and is parsed by the on-chip mask ROM bootloader.
//
// The following documentation has been used:
// https://github.com/espressif/esptool/wiki/Firmware-Image-Format
// https://github.com/espressif/esp-idf/blob/8fbb63c2a701c22ccf4ce249f43aded73e134a34/components/bootloader_support/include/esp_image_format.h#L58
// https://github.com/espressif/esptool/blob/master/esptool.py
func makeESPFirmareImage(infile, outfile, format string) error {
	inf, err := elf.Open(infile)
	if err != nil {
		return err
	}
	defer inf.Close()

	// Load all segments to be written to the image. These are actually ELF
	// sections, not true ELF segments (similar to how esptool does it).
	var segments []*espImageSegment
	for _, section := range inf.Sections {
		if section.Type != elf.SHT_PROGBITS || section.Size == 0 || section.Flags&elf.SHF_ALLOC == 0 {
			continue
		}
		data, err := section.Data()
		if err != nil {
			return fmt.Errorf("failed to read section data: %w", err)
		}
		for len(data)%4 != 0 {
			// Align segment to 4 bytes.
			data = append(data, 0)
		}
		if uint64(uint32(section.Addr)) != section.Addr {
			return fmt.Errorf("section address too big: 0x%x", section.Addr)
		}
		segments = append(segments, &espImageSegment{
			addr: uint32(section.Addr),
			data: data,
		})
	}

	// Separate esp32 and esp32-img. The -img suffix indicates we should make an
	// image, not just a binary to be flashed at 0x1000 for example.
	chip := format
	makeImage := false
	if strings.HasSuffix(format, "-img") {
		makeImage = true
		chip = format[:len(format)-len("-img")]
	}

	// Add ESP App Descriptor for ESP32 family chips (required by bootloader)
	if chip == "esp32" || chip == "esp32c3" || chip == "esp32s3" {
		appDesc := createESPAppDescriptor(chip)
		// Find rodata segment or create one
		roDataFound := false
		for _, segment := range segments {
			// Look for DRAM segment that could hold rodata
			if segment.addr >= 0x3FC88000 && segment.addr < 0x3FD00000 {
				// Prepend app descriptor to DRAM segment
				newData := make([]byte, len(appDesc)+len(segment.data))
				copy(newData, appDesc)
				copy(newData[len(appDesc):], segment.data)
				segment.data = newData
				roDataFound = true
				break
			}
		}
		if !roDataFound {
			// Create new DRAM segment for app descriptor
			segments = append([]*espImageSegment{{
				addr: 0x3FC89000, // DRAM address
				data: appDesc,
			}}, segments...)
		}
	}

	// Sort the segments by address. This is what esptool does too.
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].addr < segments[j].addr })

	// Calculate checksum over the segment data. This is used in the image
	// footer.
	checksum := uint8(0xef)
	for _, segment := range segments {
		for _, b := range segment.data {
			checksum ^= b
		}
	}

	// Write first to an in-memory buffer, primarily so that we can easily
	// calculate a hash over the entire image.
	// An added benefit is that we don't need to check for errors all the time.
	outf := &bytes.Buffer{}

	if makeImage {
		// The bootloader starts at 0x1000, or 4096.
		// TinyGo doesn't use a separate bootloader and runs the entire
		// application in the bootloader location.
		outf.Write(make([]byte, 4096))
	}

	// Chip IDs. Source:
	// https://github.com/espressif/esp-idf/blob/v4.3/components/bootloader_support/include/esp_app_format.h#L22
	chip_id := map[string]uint16{
		"esp32":   0x0000,
		"esp32c3": 0x0005,
		"esp32s3": 0x0009,
	}[chip]

	// Image header.
	switch chip {
	case "esp32", "esp32c3", "esp32s3":
		// Header format:
		// https://github.com/espressif/esp-idf/blob/v4.3/components/bootloader_support/include/esp_app_format.h#L71
		// Note: not adding a SHA256 hash as the binary is modified by
		// esptool.py while flashing and therefore the hash won't be valid
		// anymore.
		binary.Write(outf, binary.LittleEndian, struct {
			magic          uint8
			segment_count  uint8
			spi_mode       uint8
			spi_speed_size uint8
			entry_addr     uint32
			wp_pin         uint8
			spi_pin_drv    [3]uint8
			chip_id        uint16
			min_chip_rev   uint8
			reserved       [8]uint8
			hash_appended  bool
		}{
			magic:          0xE9,
			segment_count:  byte(len(segments)),
			spi_mode:       2,    // ESP_IMAGE_SPI_MODE_DIO
			spi_speed_size: 0x1f, // ESP_IMAGE_SPI_SPEED_80M, ESP_IMAGE_FLASH_SIZE_2MB
			entry_addr:     uint32(inf.Entry),
			wp_pin:         0xEE, // disable WP pin
			chip_id:        chip_id,
			hash_appended:  true, // add a SHA256 hash
		})
	case "esp8266":
		// Header format:
		// https://github.com/espressif/esptool/wiki/Firmware-Image-Format
		// Basically a truncated version of the ESP32 header.
		binary.Write(outf, binary.LittleEndian, struct {
			magic          uint8
			segment_count  uint8
			spi_mode       uint8
			spi_speed_size uint8
			entry_addr     uint32
		}{
			magic:          0xE9,
			segment_count:  byte(len(segments)),
			spi_mode:       0,    // irrelevant, replaced by esptool when flashing
			spi_speed_size: 0x20, // spi_speed, spi_size: replaced by esptool when flashing
			entry_addr:     uint32(inf.Entry),
		})
	default:
		return fmt.Errorf("builder: unknown binary format %#v, expected esp32 or esp8266", format)
	}

	// Write all segments to the image.
	// https://github.com/espressif/esptool/wiki/Firmware-Image-Format#segment
	for _, segment := range segments {
		binary.Write(outf, binary.LittleEndian, struct {
			addr   uint32
			length uint32
		}{
			addr:   segment.addr,
			length: uint32(len(segment.data)),
		})
		outf.Write(segment.data)
	}

	// Footer, including checksum.
	// The entire image size must be a multiple of 16, so pad the image to one
	// byte less than that before writing the checksum.
	outf.Write(make([]byte, 15-outf.Len()%16))
	outf.WriteByte(checksum)

	if chip != "esp8266" {
		// SHA256 hash (to protect against image corruption, not for security).
		hash := sha256.Sum256(outf.Bytes())
		outf.Write(hash[:])
	}

	// QEMU (or more precisely, qemu-system-xtensa from Espressif) expects the
	// image to be a certain size.
	if makeImage {
		// Use a default image size of 4MB.
		grow := 4096*1024 - outf.Len()
		if grow > 0 {
			outf.Write(make([]byte, grow))
		}
	}

	// Write the image to the output file.
	return os.WriteFile(outfile, outf.Bytes(), 0666)
}

// createESPAppDescriptor creates the ESP app descriptor structure required by the bootloader
func createESPAppDescriptor(chip string) []byte {
	// ESP app descriptor structure (256 bytes total)
	appDesc := make([]byte, 256)
	
	// Magic word: 0xABCD5432 (little endian)
	appDesc[0] = 0x32
	appDesc[1] = 0x54
	appDesc[2] = 0xCD
	appDesc[3] = 0xAB
	
	// Secure version (4 bytes): 0x00000000
	// Reserved fields (8 bytes): all zeros
	
	// Min efuse block revision (4 bytes): 0x00000000 - compatible with all chips
	appDesc[16] = 0x00
	appDesc[17] = 0x00
	appDesc[18] = 0x00
	appDesc[19] = 0x00
	
	// Max efuse block revision (4 bytes): 0xFFFFFFFF - no maximum limit
	appDesc[20] = 0xFF
	appDesc[21] = 0xFF
	appDesc[22] = 0xFF
	appDesc[23] = 0xFF
	
	// Project name (32 bytes)
	projectName := "TinyGo ESP32-S3"
	copy(appDesc[24:56], []byte(projectName))
	
	// Build time (16 bytes)
	buildTime := "00:00:00"
	copy(appDesc[56:72], []byte(buildTime))
	
	// Build date (16 bytes)
	buildDate := "Jan  1 2024"
	copy(appDesc[72:88], []byte(buildDate))
	
	// IDF version (32 bytes)
	idfVersion := "v5.0.0-tinygo"
	copy(appDesc[88:120], []byte(idfVersion))
	
	// SHA256 hash of ELF file (32 bytes) - leave as zeros for now
	// Reserved fields (88 bytes) - all zeros
	
	return appDesc
}