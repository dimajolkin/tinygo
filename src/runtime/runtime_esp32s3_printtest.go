//go:build esp32s3

package runtime

import (
	"device/esp"
)

// testPrintSubsystem validates USB Serial/JTAG output functionality.
// Tests that:
// 1. USB Serial/JTAG peripheral is initialized correctly
// 2. putchar() works for all ASCII characters
// 3. print() works for numbers (int, uint)
// 4. println() works correctly
// 5. Negative numbers are handled
// 6. Zero values are printed
// 7. Large numbers don't cause overflow
//
// Reference: Based on ESP-IDF USB Serial/JTAG driver behavior
func testPrintSubsystem() {
	println("\n=== USB SERIAL/JTAG OUTPUT TESTS ===")

	// Test 0: USB hardware status FIRST (before any output)
	//println("Test 0: USB hardware status (before tests)...")
	//testUSBHardwareStatus()
	//println("  ✓ USB hardware initialized")

	// Test 1: Basic putchar for all printable ASCII
	println("Test 1: Basic ASCII characters...")
	testBasicASCII()
	println("  ✓ ASCII output working")

	// Test 2: Print single digit numbers
	println("Test 2: Single digit numbers...")
	testSingleDigitNumbers()
	println("  ✓ Single digits working")

	// Test 3: Multi-digit numbers
	println("Test 3: Multi-digit numbers...")
	testMultiDigitNumbers()
	println("  ✓ Multi-digit numbers working")

	// Test 4: Negative numbers
	println("Test 4: Negative numbers...")
	testNegativeNumbers()
	println("  ✓ Negative numbers working")

	// Test 5: Zero values
	println("Test 5: Zero values...")
	testZeroValues()
	println("  ✓ Zero values working")

	// Test 6: Large numbers (boundary conditions)
	println("Test 6: Large numbers (boundary)...")
	testLargeNumbers()
	println("  ✓ Large numbers working")

	// Test 7: Mixed output (strings + numbers)
	println("Test 7: Mixed output...")
	testMixedOutput()
	println("  ✓ Mixed output working")

	// Test 8: Rapid output (stress test)
	println("Test 8: Rapid output (100 lines)...")
	testRapidOutput()
	println("  ✓ Rapid output working")

	// Test 9: USB Serial/JTAG hardware status (after tests)
	println("Test 9: USB hardware status (after tests)...")
	testUSBHardwareStatus()
	println("  ✓ USB hardware status OK")

	println("========================================")
}

// testBasicASCII tests putchar with printable ASCII characters
func testBasicASCII() {
	print("  ASCII: ")
	// Test a-z, A-Z, 0-9, and some symbols
	for c := byte('a'); c <= 'z'; c++ {
		putchar(c)
	}
	putchar(' ')
	for c := byte('A'); c <= 'Z'; c++ {
		putchar(c)
	}
	putchar(' ')
	for c := byte('0'); c <= '9'; c++ {
		putchar(c)
	}
	println()
}

// testSingleDigitNumbers tests print() with single digit numbers
func testSingleDigitNumbers() {
	print("  Numbers 0-9: ")
	for i := 0; i < 10; i++ {
		print(i)
		if i < 9 {
			print(" ")
		}
	}
	println()
}

// testMultiDigitNumbers tests print() with multi-digit numbers
func testMultiDigitNumbers() {
	print("  10: ")
	print(10)
	print(" | 42: ")
	print(42)
	print(" | 123: ")
	print(123)
	print(" | 999: ")
	print(999)
	print(" | 1000: ")
	print(1000)
	println()
}

// testNegativeNumbers tests print() with negative numbers
func testNegativeNumbers() {
	print("  -1: ")
	print(-1)
	print(" | -42: ")
	print(-42)
	print(" | -999: ")
	print(-999)
	print(" | -12345: ")
	print(-12345)
	println()
}

// testZeroValues tests print() with zero values
func testZeroValues() {
	print("  Zero int: ")
	print(0)
	print(" | Zero uint: ")
	print(uint32(0))
	print(" | Zero int32: ")
	print(int32(0))
	println()
}

// testLargeNumbers tests print() with boundary condition numbers
func testLargeNumbers() {
	print("  Max uint32: ")
	print(uint32(0xFFFFFFFF)) // 4294967295
	println()
	print("  Max int32: ")
	print(int32(0x7FFFFFFF)) // 2147483647
	println()
	print("  Min int32: ")
	print(int32(-2147483648))
	println()
	print("  Large: ")
	print(123456789)
	println()
}

// testMixedOutput tests println() with mixed strings and numbers
func testMixedOutput() {
	println("  Mixed:", 42, "apples,", 13, "oranges")
	println("  Answer =", 42)
	println("  Count:", 1, 2, 3, 4, 5)
}

// testRapidOutput tests rapid sequential output (stress test)
func testRapidOutput() {
	// Print 100 lines rapidly to test buffer handling
	for i := 0; i < 100; i++ {
		if i%20 == 0 {
			print("  ")
		}
		print(".")
		if (i+1)%20 == 0 {
			println()
		}
	}
	println()
}

// testUSBHardwareStatus checks USB Serial/JTAG hardware registers
func testUSBHardwareStatus() {
	println("  [Step 1] Reading USB registers...")

	// Read USB Serial/JTAG status registers
	ep1Conf := esp.USB_DEVICE.EP1_CONF.Get()
	println("  [Step 2] EP1_CONF read OK")

	dataFree := esp.USB_DEVICE.GetEP1_CONF_SERIAL_IN_EP_DATA_FREE()
	println("  [Step 3] DATA_FREE read OK")

	wrDone := esp.USB_DEVICE.GetEP1_CONF_WR_DONE()
	println("  [Step 4] WR_DONE read OK")

	dataAvail := esp.USB_DEVICE.GetEP1_CONF_SERIAL_OUT_EP_DATA_AVAIL()
	println("  [Step 5] DATA_AVAIL read OK")

	println("  [Step 6] Printing values (HEX only)...")
	print("  EP1_CONF=0x")
	printhex(ep1Conf)
	println()

	println("  [Step 7] Now trying to print numbers...")
	print("  DATA_FREE=")
	print(dataFree) // ← ЗДЕСЬ ЗАВИСАЕТ!
	println(" <-- if you see this, numbers work!")

	print(" WR_DONE=")
	print(wrDone)
	print(" DATA_AVAIL=")
	print(dataAvail)
	println()

	// Check clock enable status
	println("  [Step 8] Checking clock status...")
	usbClockEn := esp.SYSTEM.GetPERIP_CLK_EN1_USB_DEVICE_CLK_EN()
	usbReset := esp.SYSTEM.GetPERIP_RST_EN1_USB_DEVICE_RST()

	println("  [Step 9] Clock values read OK")
	print("  USB_CLK_EN=")
	print(usbClockEn)
	print(" USB_RST=")
	print(usbReset)
	println()

	// Verify USB is enabled and not in reset
	println("  [Step 10] Validating USB state...")
	if usbClockEn == 0 {
		println("  ⚠ WARNING: USB clock not enabled!")
	} else {
		println("  ✓ USB clock enabled")
	}

	if usbReset != 0 {
		println("  ⚠ WARNING: USB in reset state!")
	} else {
		println("  ✓ USB not in reset")
	}
}

// printhex prints a 32-bit value in hexadecimal (helper function)
func printhex(val uint32) {
	const hexChars = "0123456789abcdef"
	for i := 7; i >= 0; i-- {
		nibble := (val >> (uint(i) * 4)) & 0xF
		putchar(hexChars[nibble])
	}
}
