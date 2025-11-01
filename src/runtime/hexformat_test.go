//go:build esp32s3

package runtime

import (
	"testing"
)

func TestHexFormat(t *testing.T) {
	tests := []struct {
		input    uint32
		expected string
	}{
		{0x00000000, "0x00000000"},
		{0x00000001, "0x00000001"},
		{0x0000000F, "0x0000000f"},
		{0x000000FF, "0x000000ff"},
		{0x00000FFF, "0x00000fff"},
		{0x0000FFFF, "0x0000ffff"},
		{0x000FFFFF, "0x000fffff"},
		{0x00FFFFFF, "0x00ffffff"},
		{0x0FFFFFFF, "0x0fffffff"},
		{0xFFFFFFFF, "0xffffffff"},
		{0x12345678, "0x12345678"},
		{0xDEADBEEF, "0xdeadbeef"},
		{0x40374000, "0x40374000"}, // _vector_base
		{0x3FC8C000, "0x3fc8c000"}, // DRAM region
	}

	for _, tt := range tests {
		result := hexFormat(tt.input)
		if result != tt.expected {
			t.Errorf("hexFormat(0x%08X) = %q, want %q", tt.input, result, tt.expected)
			// Дополнительная отладка
			t.Logf("  Length: got %d, want %d", len(result), len(tt.expected))
			for i := 0; i < len(result) && i < len(tt.expected); i++ {
				if result[i] != tt.expected[i] {
					t.Logf("  Byte[%d]: got '%c' (0x%02X), want '%c' (0x%02X)", 
						i, result[i], result[i], tt.expected[i], tt.expected[i])
				}
			}
		}
	}
}

func TestHexFormatStringBuilding(t *testing.T) {
	// Тест базовой сборки строки
	const hexChars = "0123456789abcdef"
	var buf [10]byte
	buf[0] = '0'
	buf[1] = 'x'
	buf[2] = '4'
	buf[3] = '0'
	buf[4] = '3'
	buf[5] = '7'
	buf[6] = '4'
	buf[7] = '0'
	buf[8] = '0'
	buf[9] = '0'
	
	result := string(buf[:])
	expected := "0x40374000"
	
	if result != expected {
		t.Errorf("Manual string building failed: got %q, want %q", result, expected)
		t.Logf("  Length: got %d, want %d", len(result), len(expected))
	} else {
		t.Logf("✓ Manual string building works: %q", result)
	}
}

func TestHexFormatBitOperations(t *testing.T) {
	// Тест bit shift операций
	val := uint32(0x40374000)
	
	tests := []struct {
		shift    uint
		mask     uint32
		expected uint32
		char     byte
	}{
		{28, 0xF, 0x4, '4'},
		{24, 0xF, 0x0, '0'},
		{20, 0xF, 0x3, '3'},
		{16, 0xF, 0x7, '7'},
		{12, 0xF, 0x4, '4'},
		{8, 0xF, 0x0, '0'},
		{4, 0xF, 0x0, '0'},
		{0, 0xF, 0x0, '0'},
	}
	
	const hexChars = "0123456789abcdef"
	
	for i, tt := range tests {
		nibble := (val >> tt.shift) & tt.mask
		char := hexChars[nibble]
		
		if nibble != tt.expected {
			t.Errorf("Bit[%d]: (0x%08X >> %d) & 0x%X = 0x%X, want 0x%X", 
				i, val, tt.shift, tt.mask, nibble, tt.expected)
		}
		
		if char != tt.char {
			t.Errorf("Char[%d]: hexChars[0x%X] = '%c', want '%c'", 
				i, nibble, char, tt.char)
		}
	}
}

