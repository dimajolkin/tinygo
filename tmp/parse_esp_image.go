package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run parse_esp_image.go <bin_file>")
		return
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()

	fmt.Println("=== ESP32-S3 Image Parser ===")
	fmt.Println()

	// Read header
	header := make([]byte, 24)
	if _, err := f.Read(header); err != nil {
		fmt.Println("Error reading header:", err)
		return
	}

	magic := header[0]
	segmentCount := header[1]
	spiMode := header[2]
	spiSpeedSize := header[3]
	entryAddr := binary.LittleEndian.Uint32(header[4:8])
	wpPin := header[8]
	chipID := binary.LittleEndian.Uint16(header[12:14])
	minChipRev := header[14]

	fmt.Println("Header:")
	fmt.Printf("  Magic: 0x%02x (expected 0xE9)\n", magic)
	fmt.Printf("  Segment count: %d\n", segmentCount)
	fmt.Printf("  SPI mode: 0x%02x\n", spiMode)
	fmt.Printf("  SPI speed+size: 0x%02x\n", spiSpeedSize)
	fmt.Printf("  Entry address: 0x%08x\n", entryAddr)
	fmt.Printf("  WP pin: 0x%02x\n", wpPin)
	fmt.Printf("  Chip ID: 0x%04x (expected 0x0009 for ESP32-S3)\n", chipID)
	fmt.Printf("  Min chip rev: %d\n", minChipRev)

	if magic != 0xE9 {
		fmt.Println("ERROR: Invalid magic!")
		return
	}

	fmt.Println()
	fmt.Println("Segments:")

	for i := 0; i < int(segmentCount); i++ {
		// Read segment header (8 bytes)
		segHdr := make([]byte, 8)
		if _, err := f.Read(segHdr); err != nil {
			fmt.Printf("Error reading segment %d header: %v\n", i, err)
			return
		}

		segAddr := binary.LittleEndian.Uint32(segHdr[0:4])
		segSize := binary.LittleEndian.Uint32(segHdr[4:8])

		fmt.Printf("  Segment %d:\n", i+1)
		fmt.Printf("    Address: 0x%08x\n", segAddr)
		fmt.Printf("    Size: %d bytes (%.2f KB)\n", segSize, float64(segSize)/1024)

		// Identify segment type by address
		segType := "unknown"
		if segAddr >= 0x3fc88000 && segAddr < 0x3fd00000 {
			segType = "DRAM (RAM)"
		} else if segAddr >= 0x40370000 && segAddr < 0x403e0000 {
			segType = "IRAM"
			if segAddr == 0x40374000 {
				segType = "IRAM (.vectors expected here!)"
			}
		} else if segAddr >= 0x42000000 && segAddr < 0x44000000 {
			segType = "Flash"
		}
		fmt.Printf("    Type: %s\n", segType)

		// Read first 16 bytes of segment data
		segData := make([]byte, 16)
		n, _ := f.Read(segData)
		fmt.Printf("    First %d bytes: ", n)
		for j := 0; j < n; j++ {
			fmt.Printf("%02x ", segData[j])
		}
		fmt.Println()

		// Check if it looks like vector code
		if segAddr == 0x40374000 && n >= 4 {
			if segData[0] == 0x00 && segData[1] == 0xd1 && segData[2] == 0x13 {
				fmt.Println("    ✅ LOOKS LIKE VECTOR CODE (WSR a0, EXCSAVE_1)!")
			} else {
				fmt.Println("    ❌ DOES NOT LOOK LIKE VECTOR CODE!")
				asUint32 := binary.LittleEndian.Uint32(segData[0:4])
				if asUint32 == 0x40374000 {
					fmt.Println("       Contains address 0x40374000 instead of code!")
				}
			}
		}

		// Skip rest of segment data
		remaining := int64(segSize) - 16
		if remaining > 0 {
			// Align to 4 bytes
			toSkip := remaining
			if remaining%4 != 0 {
				toSkip = remaining + (4 - remaining%4)
			}
			f.Seek(toSkip, io.SeekCurrent)
		}
		fmt.Println()
	}

	// Read footer (checksum)
	checksum := make([]byte, 1)
	if _, err := f.Read(checksum); err == nil {
		fmt.Printf("Footer checksum: 0x%02x\n", checksum[0])
	}
}

