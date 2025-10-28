//go:build esp32s3

// ESP32-S3 Interrupt Testing and Diagnostic Functions
//
// This file contains all interrupt testing, diagnostics, and debugging functions
// that were originally in runtime_esp32s3.go. These are helper functions for
// development and troubleshooting of interrupt handling.
//
// Functions:
// - testDirectCallToHandleInterrupt() - Direct call test
// - testSoftwareInterrupt() - Software interrupt via wsr.intset
// - testPSRegisterAndWAITI() - PS register and WAITI instruction test
// - testDeepDiagnostics() - Deep diagnostics of interrupt system
// - testGPIOHardwareInterrupt() - GPIO hardware interrupt test
// - prepareInterruptMonitoring() - Prepare SYSTIMER for monitoring
// - monitorBackgroundInterrupts() - Monitor ISR activity
// - validateVectorTableLayout() - Validate vector table placement
// - dumpDiagnosticInfo() - Full diagnostic dump
// - dumpVectorTable() - Debug vector table state
// - dumpVectorTableLayout() - Print vector table memory layout
// - dumpCacheState() - Print cache register state
// - initDebugPin41() - Configure GPIO41 for debug output

package runtime

import (
	"device"
	"device/esp"
	"machine"
	"runtime/volatile"
	"unsafe"
)

// Helper functions for reading/writing special registers
// These mirror the ones in runtime_esp32s3.go but repeated here for clarity

func readPS() uint32 {
	var ps uint32
	device.AsmFull(
		"rsr.ps a2\n"+
			"s32i a2, {ptr}, 0",
		map[string]interface{}{"ptr": uintptr(unsafe.Pointer(&ps))})
	return ps
}

func writePS(val uint32) {
	device.AsmFull("wsr.ps {v}", map[string]interface{}{"v": uintptr(val)})
}

func readINTERRUPT() uint32 {
	var interrupt uint32
	device.AsmFull(
		"rsr.interrupt a2\n"+
			"s32i a2, {ptr}, 0",
		map[string]interface{}{"ptr": uintptr(unsafe.Pointer(&interrupt))})
	return interrupt
}

// Debug pin state/counter
var (
	systimerTickCount uint32
	gpio41State       uint8
	debugPin          machine.Pin
	systimerIRQSeen   uint32
	systimerIRQCount  uint32
)

// testDirectCallToHandleInterrupt - Test 1: Direct call to handleInterrupt (bypasses ASM)
func testDirectCallToHandleInterrupt() {
	println("\n=== Test 1: Direct call to handleInterrupt() ===")
	oldCount := esp.IsrCount
	esp.HandleInterruptDirect() // Will add this function
	newCount := esp.IsrCount
	println("  Before:", oldCount, " After:", newCount)
	if newCount > oldCount {
		println("  SUCCESS! handleInterrupt works!")
	} else {
		println("  FAILED! handleInterrupt doesn't increment counter!")
	}
}

// testSoftwareInterrupt - Test 2: Software interrupt via wsr.intset (tests vector table)
func testSoftwareInterrupt() uint32 {
	println("\n=== Test 2: Software interrupt on CPU_INT 23 ===")

	vectorEntry := esp.VectorEntryCount
	swCount := esp.IsrCount
	beforeGo := esp.IsrCountBeforeGo
	afterGo := esp.IsrCountAfterGo
	println("  Before SW interrupt:")
	println("    VectorEntryCount=", vectorEntry, " (did CPU jump to vector?)")
	println("    IsrCount=", swCount, " (from Go handleInterrupt)")
	println("    IsrCountBeforeGo=", beforeGo, " IsrCountAfterGo=", afterGo)

	device.AsmFull("wsr.intset {v}", map[string]interface{}{"v": uintptr(1 << 23)})
	device.AsmFull("rsync", nil)
	vectorEntry = esp.VectorEntryCount
	swCount = esp.IsrCount
	beforeGo = esp.IsrCountBeforeGo
	afterGo = esp.IsrCountAfterGo

	println("  After SW interrupt:")
	println("    VectorEntryCount=", vectorEntry, " (did CPU jump to vector?)")
	println("    IsrCount=", swCount, " (from Go handleInterrupt)")
	println("    IsrCountBeforeGo=", beforeGo, " IsrCountAfterGo=", afterGo)

	// Detailed diagnostics
	if vectorEntry == 0 {
		println("  FATAL: CPU never jumped to _UserExceptionVector!")
		println("  Check: VECBASE, vector table placement, interrupt routing")
	} else if beforeGo == 0 {
		println("  PROBLEM: Vector entered but crashed before handleInterrupt!")
		println("  Check: PS register setup, EXCM bit, ASM flow")
	} else if afterGo == 0 {
		println("  PROBLEM: handleInterrupt was called but never returned!")
		println("  Check: handleInterrupt crashes, stack overflow")
	} else {
		println("  SUCCESS! Vector table works!")
	}

	return swCount // Return current count for comparison in Test 3
}

// testPSRegisterAndWAITI - Test 3: Check PS.INTLEVEL and try WAITI instruction
func testPSRegisterAndWAITI() {
	println("\n=== Test 3: PS Register & WAITI ===")

	// Check PS register
	psBefore := readPS()
	intlevelBefore := (psBefore >> 0) & 0xF // INTLEVEL is bits 0-3
	excmBefore := (psBefore >> 4) & 0x1     // EXCM is bit 4
	println("  PS register: value=", psBefore)
	println("    INTLEVEL=", intlevelBefore, " (should be 0 for Level-1 interrupts)")
	println("    EXCM=", excmBefore, " (should be 0)")

	if intlevelBefore >= 1 {
		println("  WARNING: INTLEVEL >= 1 will block Level-1 interrupts!")
		println("  Attempting to set INTLEVEL to 0...")
		psNew := psBefore & ^uint32(0xF) // Clear INTLEVEL bits
		writePS(psNew)
		device.AsmFull("rsync", nil)
		psAfter := readPS()
		println("  PS after: value=", psAfter, " INTLEVEL=", (psAfter & 0xF))
	} else {
		println("  OK: INTLEVEL = 0")
	}

	// Try software interrupt with WAITI
	println("\n  Triggering software interrupt and using WAITI...")
	counterBefore := esp.VectorEntryCount

	device.AsmFull("wsr.intset {v}", map[string]interface{}{"v": uintptr(1 << 23)})
	device.AsmFull("rsync", nil)

	// Check INTERRUPT register
	interruptReg := readINTERRUPT()
	println("  INTERRUPT register:", interruptReg)
	println("    Bit 23 set?", (interruptReg&(1<<23)) != 0)

	// Execute WAITI - should immediately trigger interrupt if pending
	println("  Executing WAITI 0 (wait for interrupt)...")
	device.AsmFull("waiti 0", nil)
	println("  Returned from WAITI")

	counterAfter := esp.VectorEntryCount
	println("  VectorEntryCount: before=", counterBefore, " after=", counterAfter)

	if counterAfter > counterBefore {
		println("  SUCCESS: Vector was called during WAITI!")
	} else {
		println("  FAILED: Vector was NOT called during WAITI")
	}
}

// testDeepDiagnostics - Test 5: Deep dive into why interrupts don't fire
func testDeepDiagnostics() {
	println("\n=== Test 5: DEEP DIAGNOSTICS ===")

	// 1. Check VECBASE using inline ASM (safer)
	println("\n1. VECBASE CHECK:")

	var vecbaseVal uint32
	device.AsmFull(
		"rsr.vecbase a2\n"+
			"s32i a2, {ptr}, 0",
		map[string]interface{}{"ptr": uintptr(unsafe.Pointer(&vecbaseVal))})

	println("   VECBASE (via rsr.vecbase) =", vecbaseVal, "(expected 0x40374000)")

	// Check if VECBASE is valid address
	if vecbaseVal == 0 || vecbaseVal == 0xFFFFFFFF {
		println("   ERROR: VECBASE is invalid!")
		println("   VECBASE was NEVER set by call_start_cpu0!")
		return
	}

	// Try to read from _vector_base linker symbol
	linkerVectorBase := uintptr(unsafe.Pointer(&vectorBase))
	println("   _vector_base (linker) =", uint32(linkerVectorBase))

	if uint32(linkerVectorBase) != vecbaseVal {
		println("   ERROR: VECBASE != _vector_base!")
		println("   call_start_cpu0 might not have set VECBASE correctly")
	}

	// Read first 4 instructions at linker vector base (safer than vecbase)
	println("   First 16 bytes at _vector_base:")
	for i := 0; i < 4; i++ {
		addr := linkerVectorBase + uintptr(i*4)
		instr := *(*uint32)(unsafe.Pointer(addr))
		println("     +", i*4, ":", instr)
	}

	// 2. Check all interrupt conditions
	println("\n2. INTERRUPT CONDITIONS:")
	ps := readPS()
	intlevel := ps & 0xF
	excm := (ps >> 4) & 0x1
	intenable := device.AsmFull("rsr.intenable {}", nil)
	interrupt := readINTERRUPT()

	println("   PS        =", ps)
	println("   INTLEVEL  =", intlevel, "(must be 0 for level-1)")
	println("   EXCM      =", excm, "(must be 0)")
	println("   INTENABLE =", uint32(intenable))
	println("   INTERRUPT =", interrupt)

	// Check if interrupt 23 is enabled and pending
	bit23Enabled := (uint32(intenable) & (1 << 23)) != 0
	bit23Pending := (interrupt & (1 << 23)) != 0
	println("   Bit 23 enabled?", bit23Enabled)
	println("   Bit 23 pending?", bit23Pending)

	// 3. Manual interrupt with ALL conditions met
	println("\n3. FORCING INTERRUPT WITH PERFECT CONDITIONS:")

	// Clear INTLEVEL and EXCM
	psClean := ps & ^uint32(0x1F) // Clear INTLEVEL and EXCM
	writePS(psClean)
	device.AsmFull("rsync", nil)

	// Ensure bit 23 is enabled
	device.AsmFull("wsr.intenable {v}", map[string]interface{}{"v": uintptr(1 << 23)})
	device.AsmFull("rsync", nil)

	// Set interrupt
	device.AsmFull("wsr.intset {v}", map[string]interface{}{"v": uintptr(1 << 23)})
	device.AsmFull("rsync", nil)

	// Verify
	psAfter := readPS()
	intEnAfter := device.AsmFull("rsr.intenable {}", nil)
	intRegAfter := readINTERRUPT()

	println("   After forcing:")
	println("     PS.INTLEVEL =", (psAfter & 0xF))
	println("     PS.EXCM     =", ((psAfter >> 4) & 0x1))
	println("     INTENABLE   =", uint32(intEnAfter))
	println("     INTERRUPT   =", intRegAfter)

	counterBefore := esp.VectorEntryCount

	// Try NOP to give CPU a chance
	println("   Executing 100 NOPs...")
	for i := 0; i < 100; i++ {
		device.Asm("nop")
	}

	counterAfter := esp.VectorEntryCount
	println("   VectorEntryCount: before=", counterBefore, " after=", counterAfter)

	if counterAfter > counterBefore {
		println("   SUCCESS!")
	} else {
		println("   FAILED - interrupt STILL doesn't fire!")
		println("   This suggests:")
		println("     - VECBASE might not be used by CPU")
		println("     - ROM code intercepts interrupts?")
		println("     - Hardware configuration issue?")
	}
}

// testGPIOHardwareInterrupt - Test 4: Try real hardware interrupt from GPIO
func testGPIOHardwareInterrupt() {
	println("\n=== Test 4: GPIO Hardware Interrupt ===")
	println("  Configuring GPIO 4 (button) for interrupt...")

	// GPIO 4 already configured for input with pullup in initGPIO()
	// Configure GPIO interrupt: enable, rising edge
	const GPIO_PIN_INT_ENA_REG = 0x60004074
	const GPIO_STATUS_W1TC_REG = 0x60004028
	const GPIO_STATUS_REG = 0x60004020

	// Clear any pending interrupts on GPIO 4
	volatile.StoreUint32((*uint32)(unsafe.Pointer(uintptr(GPIO_STATUS_W1TC_REG))), 1<<4)

	// Enable rising edge interrupt on GPIO 4 (INT_TYPE = 1)
	volatile.StoreUint32((*uint32)(unsafe.Pointer(uintptr(GPIO_PIN_INT_ENA_REG+4*4))), 0x01)

	println("  Press button on GPIO 4 or short to GND and release...")
	println("  Monitoring for 1 second...")

	counterBefore := esp.VectorEntryCount

	// Check status with busy-wait (no time.Sleep in runtime)
	for i := 0; i < 10; i++ {
		// Busy wait ~100ms (240 MHz CPU, ~24M cycles = 100ms)
		for j := 0; j < 24000000; j++ {
			device.Asm("nop")
		}

		status := volatile.LoadUint32((*uint32)(unsafe.Pointer(uintptr(GPIO_STATUS_REG))))
		if status&(1<<4) != 0 {
			println("  GPIO 4 interrupt detected! GPIO_STATUS=", status)
			break
		}

		// Check if vector was called
		if esp.VectorEntryCount > counterBefore {
			println("  Vector called during GPIO monitoring!")
			break
		}
	}

	counterAfter := esp.VectorEntryCount
	isrCount := esp.IsrCount
	println("  Results:")
	println("    VectorEntryCount: before=", counterBefore, " after=", counterAfter)
	println("    IsrCount=", isrCount)

	if counterAfter > counterBefore {
		println("  SUCCESS: GPIO interrupt triggered vector!")
	} else {
		println("  INFO: No GPIO interrupt detected (user may not have pressed button)")
	}
}

// prepareInterruptMonitoring - Prepare SYSTIMER for safe interrupt monitoring
// Rearms target to be far in future to avoid interrupts during println
func prepareInterruptMonitoring() {
	// CRITICAL: Before enabling interrupts, rearm TARGET to be FAR in future
	// This ensures we don't get immediate interrupt during println
	esp.SYSTIMER.SetUNIT1_OP_TIMER_UNIT1_UPDATE(1)
	for esp.SYSTIMER.GetUNIT1_OP_TIMER_UNIT1_VALUE_VALID() == 0 {
	}
	nowSafe := esp.SYSTIMER.UNIT1_VALUE_LO.Get()
	targetSafe := nowSafe + 800000 // +10ms in future
	esp.SYSTIMER.SetTARGET0_LO(targetSafe)
	esp.SYSTIMER.SetCOMP0_LOAD_TIMER_COMP0_LOAD(1)
	esp.SYSTIMER.INT_CLR.Set(1 << 0) // Clear any pending
}

// monitorBackgroundInterrupts - Monitor background SYSTIMER interrupts for a period
// Returns the final interrupt count after monitoring
func monitorBackgroundInterrupts() uint32 {
	println(">>> IRQs enabled! Monitoring for 200 cycles (~20-30ms)...")

	// Monitor ISR count SILENTLY (collect data, print AFTER)
	var samples [11]uint32 // Snapshots at cycles 0, 20, 40, ..., 200
	sampleIdx := 0

	for i := 0; i < 200; i++ {
		// Busy-wait (~100us per cycle on 240MHz)
		for j := 0; j < 100000; j++ {
		}

		// Collect samples every 20 cycles
		if i%20 == 0 || i == 199 {
			if sampleIdx < 11 {
				samples[sampleIdx] = esp.IsrCount
				sampleIdx++
			}
		}
	}

	// Print results
	println(">>> Monitoring complete! Results:")
	for i := 0; i < sampleIdx; i++ {
		cycle := i * 20
		if i == sampleIdx-1 && cycle != 199 {
			cycle = 199
		}
		println("  Cycle", cycle, ": IsrCount=", samples[i])
	}

	finalCount := esp.IsrCount
	println(">>> Final IsrCount=", finalCount)

	return finalCount
}

// validateVectorTableLayout - Validate vector table placement (ESP-IDF compliance check)
func validateVectorTableLayout() {
	vecbase := device.AsmFull("rsr.vecbase {}", nil)
	vectorBaseAddr := uintptr(unsafe.Pointer(&vectorBase))
	vectorsEndAddr := uintptr(unsafe.Pointer(&vectorsEnd))
	textStartAddr := uintptr(unsafe.Pointer(&textStart))

	println("\n=== VECTOR TABLE VALIDATION (ESP-IDF compliance) ===")
	println("VECBASE       =", uint32(uintptr(vecbase)), "(expected 0x40374000)")
	println("_vector_base  =", uint32(vectorBaseAddr))
	println("_vectors_end  =", uint32(vectorsEndAddr))
	println("_text_start   =", uint32(textStartAddr))

	vectorsSize := uint32(vectorsEndAddr - vectorBaseAddr)
	gap := uint32(textStartAddr - vectorsEndAddr)

	println("Vectors size  =", vectorsSize, "bytes (expected 384)")
	println("Gap to .text  =", gap, "bytes")

	// Проверки
	allOk := true

	if uint32(uintptr(vecbase)) != uint32(vectorBaseAddr) {
		println("✗ ERROR: VECBASE != _vector_base")
		allOk = false
	} else {
		println("✓ VECBASE matches _vector_base")
	}

	if vectorsSize > 0x400 {
		println("✗ ERROR: Vectors section exceeds 1KB!")
		allOk = false
	} else if vectorsSize != 0x180 {
		println("⚠ WARNING: Vectors size != 384 bytes (expected)")
	} else {
		println("✓ Vectors size correct (384 bytes)")
	}

	if vectorsEndAddr > textStartAddr {
		println("✗ FATAL: Vectors overlap .text!")
		allOk = false
	} else {
		println("✓ No overlap between .vectors and .text")
	}

	if gap != 0 {
		println("⚠ INFO: Gap of", gap, "bytes between .vectors and .text")
	} else {
		println("✓ .text starts immediately after .vectors")
	}

	if allOk {
		println("✓ Vector table layout OK (ESP-IDF compliant)")
	} else {
		println("✗ Vector table layout has ERRORS!")
	}

	println("=============================================")
}

// dumpDiagnosticInfo - Dump detailed diagnostic information about vector table and interrupt state
func dumpDiagnosticInfo() {
	println("\n=== DIAGNOSTIC DUMP ===")

	// Validate layout first
	validateVectorTableLayout()

	// Dump vector table addresses and contents
	dumpVectorTableLayout()

	// Check SYSTIMER state
	esp.SYSTIMER.SetUNIT1_OP_TIMER_UNIT1_UPDATE(1)
	for esp.SYSTIMER.GetUNIT1_OP_TIMER_UNIT1_VALUE_VALID() == 0 {
	}
	now := esp.SYSTIMER.UNIT1_VALUE_LO.Get()
	target := esp.SYSTIMER.TARGET0_LO.Get()
	intSt := esp.SYSTIMER.INT_ST.Get()
	intEna := esp.SYSTIMER.INT_ENA.Get()
	conf := esp.SYSTIMER.CONF.Get()
	t0conf := esp.SYSTIMER.TARGET0_CONF.Get()

	println("\nSYSTIMER:")
	println("  NOW=", now, " TARGET=", target)
	println("  INT_ST=", intSt, " INT_ENA=", intEna)
	println("  CONF=", conf, " T0CONF=", t0conf)
	println("  Counter crossed target?", now >= target)

	// Check Interrupt Matrix
	mapVal := esp.INTERRUPT_CORE0.GetSYSTIMER_TARGET0_INT_MAP()
	println("\nInterrupt Matrix: SYSTIMER_TARGET0 -> CPU_INT", mapVal)

	// Check CPU interrupt enable
	intEnableReg := device.AsmFull("rsr.intenable {}", nil)
	println("CPU INTENABLE=", uint32(intEnableReg))
	println("  Bit", mapVal, "enabled?", (uint32(intEnableReg)&(1<<mapVal)) != 0)

	println("\n=== END DIAGNOSTIC DUMP ===")
}

// dumpVectorTable prints critical vector table and CPU interrupt state
func dumpVectorTable() {
	// Read VECBASE - base address of the vector table
	vecbase := device.AsmFull("rsr.vecbase {}", nil)

	// Read CPU interrupt control registers (NOTE: rsr.interrupt causes hangs, skip it!)
	ps := device.AsmFull("rsr.ps {}", nil)
	intEna := device.AsmFull("rsr.intenable {}", nil)
	exccause := device.AsmFull("rsr.exccause {}", nil)
	epc1 := device.AsmFull("rsr.epc1 {}", nil)

	println("=== VECTOR TABLE DEBUG ===")
	println("VECBASE=", uint32(uintptr(vecbase)))
	println("PS=", uint32(uintptr(ps)), " (INTLEVEL=", uint32(uintptr(ps)&0x0F), ")")
	println("INTENABLE=", uint32(uintptr(intEna)))
	println("EXCCAUSE=", uint32(uintptr(exccause)))
	println("EPC1=", uint32(uintptr(epc1)))

	// Print interrupt matrix mapping for SYSTIMER
	mapVal := esp.INTERRUPT_CORE0.GetSYSTIMER_TARGET0_INT_MAP()
	println("SYSTIMER_TARGET0 -> CPU_INT", mapVal)

	// Print SYSTIMER status
	st := esp.SYSTIMER.INT_ST.Get()
	ena := esp.SYSTIMER.INT_ENA.Get()
	println("SYSTIMER: INT_ST=", st, " INT_ENA=", ena)
	println("==========================")
}

// dumpVectorTableLayout prints vector table memory layout and first instructions
func dumpVectorTableLayout() {
	// Read VECBASE
	vecbase := device.AsmFull("rsr.vecbase {}", nil)
	vecbaseAddr := uint32(uintptr(vecbase))

	println("=== VECTOR TABLE LAYOUT ===")
	println("VECBASE =", vecbaseAddr, "(expected 0x40374000)")

	// Check linker symbol _vector_base
	linkerVectorBase := uintptr(unsafe.Pointer(&vectorBase))
	println("_vector_base (linker) =", uint32(linkerVectorBase))

	if uint32(linkerVectorBase) != vecbaseAddr {
		println("ERROR: VECBASE != _vector_base!")
		println("  Linker placed vectors at:", uint32(linkerVectorBase))
		println("  But VECBASE points to:", vecbaseAddr)
	} else {
		println("OK: VECBASE matches _vector_base")
	}

	// Check alignment (must be 0x400 = 1024 bytes aligned)
	if (vecbaseAddr & 0x3FF) != 0 {
		println("ERROR: VECBASE not aligned to 1024 bytes!")
	} else {
		println("OK: VECBASE aligned correctly")
	}

	// Vector offsets (each is 0x20 = 32 bytes apart)
	vectors := []struct {
		name   string
		offset uint32
	}{
		{"UserException (Level-1)", 0x000},
		{"DoubleException", 0x020},
		{"KernelException", 0x040},
		{"NMIException", 0x060},
		{"Level2Interrupt", 0x080},
		{"Level3Interrupt", 0x0A0},
		{"Level4Interrupt", 0x0C0},
		{"Level5Interrupt", 0x0E0},
		{"Level6Interrupt", 0x100},
		{"Level7Interrupt", 0x120},
	}

	println("\nVector addresses and first instruction:")
	for _, v := range vectors {
		addr := vecbaseAddr + v.offset
		// Read first 32-bit instruction at vector entry
		firstInstr := *(*uint32)(unsafe.Pointer(uintptr(addr)))
		println("  ", v.name, "@ addr=", addr, " instr=", firstInstr)

		// Check if it looks like valid code (not all zeros/ones)
		if firstInstr == 0 || firstInstr == 0xFFFFFFFF {
			println("    WARNING: Vector looks uninitialized!")
		}

		// Decode first instruction for UserException
		if v.offset == 0 {
			// Compare with linker symbol _UserExceptionVector
			linkerUserVecAddr := uintptr(unsafe.Pointer(&userExceptionVector))
			linkerUserVecInstr := *(*uint32)(unsafe.Pointer(linkerUserVecAddr))
			println("    _UserExceptionVector (linker) @ addr=", uint32(linkerUserVecAddr), " instr=", linkerUserVecInstr)

			if uint32(linkerUserVecAddr) != addr {
				println("    ERROR: Linker placed UserException at different address!")
			}

			if linkerUserVecInstr != firstInstr {
				println("    ERROR: Instruction mismatch!")
				println("      At linker address:", linkerUserVecInstr)
				println("      At VECBASE address:", firstInstr)
			}

			// Expected: wsr.excsave1 a0 (opcode ~0x0090D1??)
			// Or call0 instruction (opcode 0x05 in low bits)
			opcode := firstInstr & 0x0F
			println("    First instruction opcode:", opcode)
			if opcode == 0x05 {
				println("    -> CALL0 instruction (good!)")
			} else if (firstInstr & 0xFF) == 0xD1 {
				println("    -> WSR instruction (good!)")
			} else {
				println("    -> UNKNOWN/BAD instruction!")
			}
		}
	}

	// Print addresses of key ASM functions (from linker symbols)
	println("\nKey function addresses:")
	println("  _UserExceptionVector  expected at 0x", hexString(vecbaseAddr+0x000))
	println("  _xt_user_exc          (should be in .text)")
	println("  _xt_lowint1           (should be in .text)")
	println("  _xt_level1_int_handler_entry (should be in .text)")

	println("==============================")
}

// hexString converts uint32 to hex string (helper for printing)
func hexString(val uint32) string {
	const hexChars = "0123456789ABCDEF"
	result := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		result[i] = hexChars[val&0xF]
		val >>= 4
	}
	return string(result)
}

// dumpCacheState выводит текущее состояние cache регистров для отладки
func dumpCacheState(tag string) {
	// Use constants from memory_init_esp32s3.go
	dcache := volatile.LoadUint32((*uint32)(unsafe.Pointer(uintptr(esp.EXTMEM_DCACHE_CTRL_REG))))
	icache := volatile.LoadUint32((*uint32)(unsafe.Pointer(uintptr(esp.EXTMEM_ICACHE_CTRL_REG))))

	println(">>> [CACHE DEBUG]", tag)
	println("    DCACHE_CTRL =", dcache)
	println("      Enable    =", (dcache&esp.EXTMEM_CACHE_ENABLE_BIT) != 0)
	println("      Invalidate=", (dcache&(1<<1)) != 0)
	println("    ICACHE_CTRL =", icache)
	println("      Enable    =", (icache&esp.EXTMEM_CACHE_ENABLE_BIT) != 0)
	println("      Invalidate=", (icache&(1<<1)) != 0)
}

// initDebugPin41 configures GPIO41 as push-pull output and sets it low.
func initDebugPin41() {
	debugPin = machine.Pin(41)
	debugPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	debugPin.Low()
	gpio41State = 0
}

func dumpSystimerDebug(tag string) {
	mapVal := esp.INTERRUPT_CORE0.GetSYSTIMER_TARGET0_INT_MAP()
	intEna := device.AsmFull("rsr.intenable {}", nil)
	ps := device.AsmFull("rsr.ps {}", nil)
	st := esp.SYSTIMER.INT_ST.Get()
	ena := esp.SYSTIMER.INT_ENA.Get()
	conf := esp.SYSTIMER.CONF.Get()
	t0conf := esp.SYSTIMER.TARGET0_CONF.Get()

	// ВАЖНО: TARGET0 привязан к UNIT1 (строка 337: SetTARGET0_CONF_TARGET0_TIMER_UNIT_SEL(1))
	// Поэтому читаем UNIT1, а не UNIT0!
	esp.SYSTIMER.SetUNIT1_OP_TIMER_UNIT1_UPDATE(1)
	for esp.SYSTIMER.GetUNIT1_OP_TIMER_UNIT1_VALUE_VALID() == 0 {
	}
	now := esp.SYSTIMER.UNIT1_VALUE_LO.Get()
	nowHi := esp.SYSTIMER.UNIT1_VALUE_HI.Get()

	tgt := esp.SYSTIMER.TARGET0_LO.Get()
	tgtHi := esp.SYSTIMER.REAL_TARGET0_HI.Get()
	println("-- SYSTIMER DEBUG (", tag, ") --")
	println("MAP=", mapVal, " INTENABLE=", uint32(intEna), " PS=", uint32(uintptr(ps)&0x0F))
	println("SYSTIMER: INT_ST=", st, " INT_ENA=", ena, " CONF=", conf, " T0CONF=", t0conf)
	println("UNIT1_HI=", nowHi, " UNIT1_LO=", now, " TARGET0_HI=", tgtHi, " TARGET0_LO=", tgt)
}
