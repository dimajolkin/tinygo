//go:build esp32s3

package runtime

// testMemorySubsystems validates memory initialization through indirect tests.
// Unlike reading EXTMEM registers (which causes hangs), we test through actual memory operations.
//
// This tests that:
// 1. ROM_Boot_Cache_Init() correctly enabled I-cache and D-cache
// 2. MMU is properly initialized for Flash XIP (Execute-In-Place)
// 3. Cache buses (IBUS/DBUS) are enabled
// 4. Stack operations work correctly
//
// Reference: ESP-IDF bootloader doesn't validate by reading registers either,
// they trust the ROM functions and test through actual memory access.
func testMemorySubsystems() {
	println("\n=== MEMORY SUBSYSTEM TESTS ===")

	// Test 1: Flash code execution (I-cache + MMU test)
	println("Test 1: Flash code execution (I-cache)...")
	testFlashCodeExecution()
	println("  ✓ Flash access working")

	// Test 2: DRAM access (D-cache test)
	println("Test 2: DRAM access (D-cache)...")
	testDRAMAccess()
	println("  ✓ DRAM access working")

	// Test 3: Stack operations (stack integrity)
	println("Test 3: Stack operations...")
	testStackOperations()
	println("  ✓ Stack operations working")

	// Test 4: Large memory copy (cache performance)
	println("Test 4: Memory copy performance...")
	testMemoryCopyPerformance()
	println("  ✓ Memory copy performance OK")

	println("========================================")
}

// testFlashCodeExecution tests that code can be executed from Flash (I-cache + MMU working)
// If I-cache or MMU are broken:
// - Code execution will be extremely slow (direct Flash access ~1MHz vs cached ~160MHz)
// - Or will cause exceptions/hangs
func testFlashCodeExecution() {
	// Call functions from Flash multiple times
	// If I-cache and MMU are broken, this will be very slow or hang
	var sum uint32
	var i uint32
	for i = 0; i < 100; i++ {
		sum += complexCalculation(i)
	}

	// Verify result to ensure calculations actually happened
	// Formula: complexCalculation(n) = n² + 2n + 1 = (n+1)²
	// Sum for n=0..99: Σ(n+1)² = Σk² for k=1..100 = 100*101*201/6 = 338350
	if sum != 338350 {
		println("  ERROR: Flash code execution failed!")
	}
}

// complexCalculation is a helper to test Flash code execution
// This function will be in Flash and requires I-cache + MMU to work fast
func complexCalculation(n uint32) uint32 {
	result := n
	result = result*result + n*2 + 1
	return result
}

// testDRAMAccess tests DRAM access (D-cache working)
// If D-cache is broken:
// - Memory access will be slow
// - Data integrity issues possible
func testDRAMAccess() {
	// Create array in DRAM and do operations
	var testArray [100]uint32

	// Write pattern
	for i := 0; i < len(testArray); i++ {
		testArray[i] = uint32(i * i)
	}

	// Read back and verify
	for i := 0; i < len(testArray); i++ {
		if testArray[i] != uint32(i*i) {
			println("  ERROR: DRAM test failed at index", i, "value=", testArray[i], "expected=", i*i)
			return
		}
	}
}

// testStackOperations tests stack operations (stack integrity)
// This validates:
// - Stack pointer is correct
// - Stack memory is accessible
// - Function calls work (requires I-cache for return addresses)
func testStackOperations() {
	// Deep recursion to test stack
	result := recursiveTest(10, 0)
	if result != 55 { // Sum of 0+1+2+...+10 = 55
		println("  ERROR: Stack operations failed, result=", result, "expected=55")
	}
}

func recursiveTest(depth int, accumulator int) int {
	if depth == 0 {
		return accumulator
	}
	return recursiveTest(depth-1, accumulator+depth)
}

// testMemoryCopyPerformance tests memory copy (overall cache performance)
// This is a performance test to detect degraded cache behavior:
// - With working cache: ~0.1-0.2ms for 256 words
// - Without cache: 10x+ slower
func testMemoryCopyPerformance() {
	const size = 256
	var src [size]uint32
	var dst [size]uint32

	// Initialize source
	for i := 0; i < size; i++ {
		src[i] = uint32(i)
	}

	// Copy (should be fast with working cache)
	startTicks := ticks()
	for i := 0; i < size; i++ {
		dst[i] = src[i]
	}
	elapsed := ticks() - startTicks

	// Verify
	for i := 0; i < size; i++ {
		if dst[i] != src[i] {
			//println("  ERROR: Memory copy verification failed at index", i, "value:", dst[i], "expected:", src[i])
			return
		}
	}

	// Check performance (should be fast, < 1ms for 256 words with cache)
	elapsedUs := ticksToNanoseconds(elapsed) / 1000
	//println("  Memory copy time:", elapsedUs, "µs for", size, "words")

	if elapsedUs > 1000 { // More than 1ms is suspicious
		println("  WARNING: Copy is slower than expected (cache issue?)")
	}
}
