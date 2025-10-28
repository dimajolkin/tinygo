package main

import (
	"debug/elf"
	"fmt"
	"os"
	"sort"
)

type segment struct {
	addr uint32
	size uint64
	name string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run analyze_esp_image.go <elf_file>")
		return
	}

	f, err := elf.Open(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()

	fmt.Println("=== ELF Sections that SHOULD be in ESP32-S3 image ===")
	fmt.Println()
	fmt.Println("Criteria (from builder/esp.go):")
	fmt.Println("  - Type == SHT_PROGBITS")
	fmt.Println("  - Size > 0")
	fmt.Println("  - Flags & SHF_ALLOC != 0")
	fmt.Println()

	var segments []segment
	for _, section := range f.Sections {
		// Same logic as builder/esp.go:44-47
		if section.Type != elf.SHT_PROGBITS || section.Size == 0 || section.Flags&elf.SHF_ALLOC == 0 {
			continue
		}

		segments = append(segments, segment{
			addr: uint32(section.Addr),
			size: section.Size,
			name: section.Name,
		})
	}

	// Sort by address (same as builder/esp.go:66)
	sort.Slice(segments, func(i, j int) bool { return segments[i].addr < segments[j].addr })

	fmt.Printf("Total segments to include: %d\n", len(segments))
	fmt.Println()

	totalSize := uint64(0)
	for i, seg := range segments {
		fmt.Printf("%2d. 0x%08x - 0x%08x (%6d bytes) %s\n",
			i+1, seg.addr, seg.addr+uint32(seg.size), seg.size, seg.name)
		totalSize += seg.size
	}

	fmt.Println()
	fmt.Printf("Total size of all segments: %d bytes (%.2f KB)\n", totalSize, float64(totalSize)/1024)

	fmt.Println()
	fmt.Println("=== Checking critical sections ===")
	
	found := map[string]bool{}
	for _, seg := range segments {
		if seg.name == ".vectors" {
			fmt.Printf("✅ .vectors FOUND at 0x%08x (%d bytes)\n", seg.addr, seg.size)
			found[".vectors"] = true
		}
		if seg.name == ".text" {
			fmt.Printf("✅ .text FOUND at 0x%08x (%d bytes)\n", seg.addr, seg.size)
			found[".text"] = true
		}
		if seg.name == ".rodata" {
			fmt.Printf("✅ .rodata FOUND at 0x%08x (%d bytes)\n", seg.addr, seg.size)
			found[".rodata"] = true
		}
	}

	if !found[".vectors"] {
		fmt.Println("❌ .vectors NOT FOUND in segments!")
	}

	fmt.Println()
	fmt.Println("=== ESP32-S3 Image Structure ===")
	fmt.Println("Header: 24 bytes (magic=0xE9, entry point, etc.)")
	fmt.Println("For each segment:")
	fmt.Println("  Segment header: 8 bytes (addr + size)")
	fmt.Println("  Segment data: <size> bytes (4-byte aligned)")
	fmt.Println("Footer: 1 byte (checksum)")
}

