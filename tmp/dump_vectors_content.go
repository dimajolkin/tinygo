package main

import (
	"debug/elf"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run dump_vectors_content.go <elf_file>")
		return
	}

	f, err := elf.Open(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()

	// Find .vectors section
	vectors := f.Section(".vectors")
	if vectors == nil {
		fmt.Println("ERROR: .vectors section not found!")
		return
	}

	data, err := vectors.Data()
	if err != nil {
		fmt.Println("ERROR reading section data:", err)
		return
	}

	fmt.Printf("=== .vectors Section Content ===\n")
	fmt.Printf("Address: 0x%08x\n", vectors.Addr)
	fmt.Printf("Size: %d bytes\n", vectors.Size)
	fmt.Println()

	fmt.Println("First 128 bytes (hex dump):")
	for i := 0; i < len(data) && i < 128; i += 16 {
		fmt.Printf("%08x: ", int(vectors.Addr)+i)
		
		// Hex bytes
		for j := 0; j < 16 && i+j < len(data); j++ {
			if (i+j) < len(data) {
				fmt.Printf("%02x ", data[i+j])
			} else {
				fmt.Printf("   ")
			}
			if j == 7 {
				fmt.Printf(" ")
			}
		}
		
		// ASCII
		fmt.Printf(" |")
		for j := 0; j < 16 && i+j < len(data); j++ {
			if (i+j) < len(data) {
				b := data[i+j]
				if b >= 32 && b < 127 {
					fmt.Printf("%c", b)
				} else {
					fmt.Printf(".")
				}
			}
		}
		fmt.Printf("|\n")
	}
	
	fmt.Println()
	fmt.Println("=== Analysis ===")
	
	// Check first instruction
	if len(data) >= 4 {
		instr1 := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
		fmt.Printf("First 4 bytes (as uint32): 0x%08x (%d decimal)\n", instr1, instr1)
		
		// Check if it looks like an address (0x40374000)
		if instr1 == 0x40374000 {
			fmt.Println("  ❌ ERROR: This is the ADDRESS 0x40374000, not code!")
			fmt.Println("  The vector contains DATA instead of instructions!")
		} else if instr1 == 0x40374124 {
			fmt.Println("  ❌ ERROR: This is an ADDRESS (0x40374124), not code!")
		} else {
			// Try to decode Xtensa instruction
			fmt.Println("  Attempting to decode as Xtensa instruction...")
			
			// WSR a0, EXCSAVE_1 should be: 0x0013d000
			// call0 should start with 0x05 or 0x45
			opcode := data[0]
			if opcode == 0x00 && data[1] == 0xd0 && data[2] == 0x13 {
				fmt.Println("  ✅ Looks like WSR a0, EXCSAVE_1 instruction!")
			} else if opcode == 0x05 || opcode == 0x45 {
				fmt.Println("  ✅ Looks like CALL0 instruction!")
			} else {
				fmt.Printf("  ⚠️  Unknown opcode: 0x%02x\n", opcode)
			}
		}
	}
	
	// Check offsets
	fmt.Println()
	fmt.Println("Expected vector offsets:")
	fmt.Println("  +0x000: UserException (Level-1)")
	fmt.Println("  +0x020: DoubleException")
	fmt.Println("  +0x040: KernelException")
	fmt.Println("  +0x060: NMIException")
	fmt.Println("  +0x080: Level2Interrupt")
	fmt.Println("  +0x0A0: Level3Interrupt")
	fmt.Println("  +0x0C0: Level4Interrupt")
	fmt.Println("  +0x0E0: Level5Interrupt")
	fmt.Println("  +0x100: Level6Interrupt")
	fmt.Println("  +0x120: Level7Interrupt")
}

