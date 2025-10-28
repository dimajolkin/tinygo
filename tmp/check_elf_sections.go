package main

import (
	"debug/elf"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run check_elf_sections.go <elf_file>")
		return
	}

	f, err := elf.Open(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()

	fmt.Println("=== ELF Sections ===")
	fmt.Println()

	for _, section := range f.Sections {
		if section.Name == ".vectors" || 
		   section.Name == ".UserExceptionVector.text" ||
		   section.Name == ".UserExceptionVector.literal" {
			fmt.Printf("Section: %s\n", section.Name)
			fmt.Printf("  Type: %v (0x%x)\n", section.Type, section.Type)
			fmt.Printf("  Flags: %v (0x%x)\n", section.Flags, section.Flags)
			fmt.Printf("  Addr: 0x%08x\n", section.Addr)
			fmt.Printf("  Size: %d bytes\n", section.Size)
			fmt.Printf("  Offset: 0x%x\n", section.Offset)
			
			// Check flags
			hasALLOC := section.Flags&elf.SHF_ALLOC != 0
			hasWRITE := section.Flags&elf.SHF_WRITE != 0
			hasEXECINSTR := section.Flags&elf.SHF_EXECINSTR != 0
			
			fmt.Printf("  Has ALLOC: %v\n", hasALLOC)
			fmt.Printf("  Has WRITE: %v\n", hasWRITE)
			fmt.Printf("  Has EXECINSTR: %v\n", hasEXECINSTR)
			
			// Check if it will be included by TinyGo
			willInclude := section.Type == elf.SHT_PROGBITS && 
			               section.Size > 0 && 
			               section.Flags&elf.SHF_ALLOC != 0
			
			if willInclude {
				fmt.Printf("  ✅ WILL BE INCLUDED in binary (SHT_PROGBITS + ALLOC)\n")
			} else {
				fmt.Printf("  ❌ WILL BE SKIPPED (not SHT_PROGBITS or no ALLOC)\n")
				if section.Type != elf.SHT_PROGBITS {
					fmt.Printf("     Reason: Type is %v, expected SHT_PROGBITS\n", section.Type)
				}
				if section.Flags&elf.SHF_ALLOC == 0 {
					fmt.Printf("     Reason: Missing SHF_ALLOC flag\n")
				}
			}
			fmt.Println()
		}
	}
	
	fmt.Println("=== All sections (first 20) ===")
	for i, section := range f.Sections {
		if i >= 20 {
			break
		}
		progbits := ""
		if section.Type == elf.SHT_PROGBITS {
			progbits = "✓"
		}
		alloc := ""
		if section.Flags&elf.SHF_ALLOC != 0 {
			alloc = "✓"
		}
		fmt.Printf("%2d. %-30s Type=%-15v PROGBITS=%s ALLOC=%s Addr=0x%08x Size=%d\n",
			i, section.Name, section.Type, progbits, alloc, section.Addr, section.Size)
	}
}

