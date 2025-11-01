package main

import (
	"time"
)

// hexFormat форматирует uint32 как 0xXXXXXXXX  
func hexFormat(val uint32) string {
	const hexChars = "0123456789abcdef"
	var buf [10]byte
	buf[0] = '0'
	buf[1] = 'x'
	// Fill from left to right (high nibble first)
	buf[2] = hexChars[(val>>28)&0xF]
	buf[3] = hexChars[(val>>24)&0xF]
	buf[4] = hexChars[(val>>20)&0xF]
	buf[5] = hexChars[(val>>16)&0xF]
	buf[6] = hexChars[(val>>12)&0xF]
	buf[7] = hexChars[(val>>8)&0xF]
	buf[8] = hexChars[(val>>4)&0xF]
	buf[9] = hexChars[val&0xF]
	return string(buf[:])
}

func assertEqual(name string, got, want string) {
	if got == want {
		println("✓", name, "PASS:", got)
	} else {
		println("✗", name, "FAIL:")
		println("  Got: ", got)
		println("  Want:", want)
		println("  Got length: ", len(got))
		println("  Want length:", len(want))
		// Вывести байты
		println("  Got bytes:")
		for i := 0; i < len(got); i++ {
			println("    [", i, "] =", uint8(got[i]), "('", string(got[i]), "')")
		}
		println("  Want bytes:")
		for i := 0; i < len(want); i++ {
			println("    [", i, "] =", uint8(want[i]), "('", string(want[i]), "')")
		}
	}
}

func main() {
	time.Sleep(2 * time.Second) // Дождаться подключения монитора
	
	println("\n=== HEXFORMAT UNIT TESTS ===\n")
	
	// Test 1: Zero
	assertEqual("Test 0x00000000", hexFormat(0x00000000), "0x00000000")
	
	// Test 2: Small values
	assertEqual("Test 0x00000001", hexFormat(0x00000001), "0x00000001")
	assertEqual("Test 0x0000000F", hexFormat(0x0000000F), "0x0000000f")
	assertEqual("Test 0x000000FF", hexFormat(0x000000FF), "0x000000ff")
	
	// Test 3: Medium values
	assertEqual("Test 0x0000FFFF", hexFormat(0x0000FFFF), "0x0000ffff")
	assertEqual("Test 0x12345678", hexFormat(0x12345678), "0x12345678")
	
	// Test 4: Large values
	assertEqual("Test 0xFFFFFFFF", hexFormat(0xFFFFFFFF), "0xffffffff")
	assertEqual("Test 0xDEADBEEF", hexFormat(0xDEADBEEF), "0xdeadbeef")
	
	// Test 5: Real addresses
	assertEqual("Test 0x40374000", hexFormat(0x40374000), "0x40374000")
	assertEqual("Test 0x3FC8C000", hexFormat(0x3FC8C000), "0x3fc8c000")
	
	// Test 6: Edge cases
	assertEqual("Test 0x00000010", hexFormat(0x00000010), "0x00000010")
	assertEqual("Test 0x00000100", hexFormat(0x00000100), "0x00000100")
	assertEqual("Test 0x00001000", hexFormat(0x00001000), "0x00001000")
	assertEqual("Test 0x00010000", hexFormat(0x00010000), "0x00010000")
	assertEqual("Test 0x00100000", hexFormat(0x00100000), "0x00100000")
	assertEqual("Test 0x01000000", hexFormat(0x01000000), "0x01000000")
	assertEqual("Test 0x10000000", hexFormat(0x10000000), "0x10000000")
	
	println("\n=== END TESTS ===\n")
	
	// Держать программу запущенной
	for {
		time.Sleep(10 * time.Second)
	}
}

