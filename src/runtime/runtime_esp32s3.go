//go:build esp32s3

// ESP32-S3 runtime implementation - Self-Booting TinyGo Application
//
// This implementation creates a self-booting application that bypasses the standard ESP-IDF bootloader.
// Instead of relying on the ESP-IDF bootloader to load the application at 0x10000, this runtime
// is designed to be loaded directly at 0x0 as a bootloader replacement.
//
// Memory layout (416KB DRAM total at 0x3FC88000-0x3FCF0000):
// - Stack: 8KB (grows from top of DRAM)
// - .data/.bss: ~8KB (global variables, placed after stack)
// - Heap: 64KB (GC managed, limited to avoid excessive metadata overhead)
// - Free: ~336KB (available for dynamic allocation/other uses)
//
// Hardware notes:
// - ROM functions (memset/memcpy) disabled due to alignment requirements
// - Uses compiler-generated memory functions instead
// - USB Serial/JTAG provides debug output without external UART
// - Cache and MMU initialization based on ESP-IDF bootloader_utility.c
// - ROM cache functions from esp32s3/rom/cache.h

package runtime

import (
	"device"
	"device/esp"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)

// External ASM function from vecbase_esp32s3.S (getVecbase)
// Note: VECBASE is already set in call_start_cpu0 (esp32s3.S), we just read it here
func getVecbase() uintptr

// Helper functions for reading/writing special registers
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

// External symbols from linker script and ASM (esp32s3.ld, xtensa_vectors_esp32s3.S)
//
//go:extern _vector_base
var vectorBase [0]byte

//go:extern _vectors_end
var vectorsEnd [0]byte

//go:extern _text_start
var textStart [0]byte

//go:extern _UserExceptionVector
var userExceptionVector [0]byte

// Debug functions sorted by GPIO number (ascending: 4→5→6→7)
func debugGPIO(n int) {
	*(*uint32)(unsafe.Pointer(uintptr(0x60004024))) |= (1 << n) // GPIO_ENABLE_REG: enable GPIO4 output
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << n)  // GPIO_OUT_W1TS_REG: set GPIO4 high
}

// disableWatchdogs отключает все аппаратные watchdog таймеры для предотвращения
// автоматического сброса системы во время разработки и отладки.
//
// Watchdog (сторожевой таймер) - это аппаратный механизм защиты от зависаний.
// Если программа не "кормит" (не сбрасывает) watchdog в течение заданного времени,
// то процессор автоматически перезагружается. Это защищает от бесконечных циклов
// и других критических сбоев в production коде.
//
// ESP32-S3 имеет несколько типов watchdog'ов:
//
// 1. TIMG0/TIMG1 MWDT (Main Watchdog Timer)
//   - Watchdog таймеры в группах таймеров 0 и 1
//   - Используются для защиты основного кода приложения
//   - Timeout по умолчанию: ~2 секунды
//
// 2. RTC WDT (RTC Watchdog Timer)
//   - Работает от RTC часов (низкочастотных)
//   - Активен даже в режимах глубокого сна
//   - Используется для защиты загрузки и инициализации
//   - Timeout по умолчанию: ~9 секунд
//
// 3. Super Watchdog (SWD)
//   - Дополнительный уровень защиты
//   - Работает независимо от основных watchdog'ов
//   - Может быть отключен только с правильным ключом разблокировки
//
// ВАЖНО: В production коде watchdog'и должны быть ВКЛЮЧЕНЫ и регулярно
// сбрасываться (feed) для обеспечения надёжности системы!
//
// Для отключения watchdog'а нужна последовательность:
// 1. Записать магический ключ в регистр WDTWPROTECT (write protect)
// 2. Записать 0 в регистр WDTCONFIG0 для отключения
// 3. (Опционально) Заблокировать регистр обратно
//
// Reference: ESP32-S3 Technical Reference Manual, Chapter "Watchdog Timers"
func disableWatchdogs() {
	// Disable Timer Group 0 Main Watchdog Timer (TIMG0 MWDT)
	// Этот watchdog защищает код на CPU0
	esp.TIMG0.WDTWPROTECT.Set(0x50D83AA1) // Unlock: Magic key для разблокировки
	esp.TIMG0.WDTCONFIG0.Set(0)           // Disable: Отключить все стадии watchdog

	// Disable Timer Group 1 Main Watchdog Timer (TIMG1 MWDT)
	// Этот watchdog может использоваться для дополнительной защиты
	esp.TIMG1.WDTWPROTECT.Set(0x50D83AA1) // Unlock: Тот же ключ разблокировки
	esp.TIMG1.WDTCONFIG0.Set(0)           // Disable: Отключить все стадии watchdog

	// Disable RTC Watchdog Timer (RTC WDT)
	// Этот watchdog работает от RTC часов и активен при загрузке
	// ROM bootloader ESP32-S3 включает его автоматически для защиты загрузки
	esp.RTC_CNTL.WDTWPROTECT.Set(0x50D83AA1) // Unlock: Разблокировать RTC WDT
	esp.RTC_CNTL.WDTCONFIG0.Set(0)           // Disable: Полностью отключить

	// CRITICAL: Super Watchdog (SWD) - Enable AUTO_FEED (like ESP-IDF does)
	// Super watchdog CANNOT be fully disabled, must use auto-feed instead!
	// Reference: ESP-IDF bootloader_super_wdt_auto_feed() in bootloader_esp32s3.c
	esp.RTC_CNTL.SWD_WPROTECT.Set(0x8F1D312A)                             // Unlock: Специальный ключ для SWD
	esp.RTC_CNTL.SWD_CONF.SetBits(esp.RTC_CNTL_SWD_CONF_SWD_AUTO_FEED_EN) // Enable auto-feed (NOT disable!)
	esp.RTC_CNTL.SWD_WPROTECT.Set(0)                                      // Lock back
}

//export main
func main() {
	// IMPORTANT: Do NOTHING before clearbss() that requires initialized Go variables!
	// The .bss section (zero-initialized globals) is not ready yet.

	// Change CPU frequency from 20MHz to 80MHz, by switching from the XTAL to
	// the PLL clock source (see table "CPU Clock Frequency" in the reference
	// manual).
	esp.SYSTEM.SYSCLK_CONF.Set(1 << esp.SYSTEM_SYSCLK_CONF_SOC_CLK_SEL_Pos)

	// Change CPU frequency from 80MHz to 160MHz by setting SYSTEM_CPUPERIOD_SEL
	// to 1 (see table "CPU Clock Frequency" in the reference manual).
	// Note: we might not want to set SYSTEM_CPU_WAIT_MODE_FORCE_ON to save
	// power. It is set here to keep the default on reset.
	esp.SYSTEM.CPU_PER_CONF.Set(esp.SYSTEM_CPU_PER_CONF_CPU_WAIT_MODE_FORCE_ON | esp.SYSTEM_CPU_PER_CONF_PLL_FREQ_SEL | 1<<esp.SYSTEM_CPU_PER_CONF_CPUPERIOD_SEL_Pos)

	clearbss()

	// CRITICAL: Disable watchdogs AFTER clearbss() when Go structures are ready
	// esp.TIMG0/TIMG1/RTC_CNTL are global pointers that need .bss to be initialized
	disableWatchdogs()

	// Initialize memory subsystems (MMU, cache buses, autoload)
	// This complements the basic cache init done in esp32s3.S
	// Reference: ESP-IDF bootloader_esp32s3.c and cache_hal_init()
	//esp.InitMemorySubsystems()

	// Initialize GPIO and SPI peripherals early (GPIO matrix might be already initialized by ROM)
	initGPIOPeripherals()
	initSPIPeripherals()

	// Initialize USB Serial/JTAG BEFORE using it
	//initUSBSerial()

	// Initialize USB Serial/JTAG registers (critical!)
	// Reference: ESP-IDF components/hal/esp32s3/include/hal/usb_serial_jtag_ll.h
	//initUSBSerialRegisters()

	// Initialize UART after USB configuration
	machine.USBCDC.Configure(machine.UARTConfig{BaudRate: 115200})
	machine.InitSerial()

	// Validate vector table layout (ESP-IDF compliance)
	initTimer()
	//initSystimerTick()

	for i := 0; i < 10000; i++ {
		print(".")
	}
	//print("\n")

	testPrintSubsystem()

	abort()
	// Configure GPIO41 as debug output (toggled by SYSTIMER ISR)
	//initDebugPin41()

	// Prepare SYSTIMER and enable interrupts
	prepareInterruptMonitoring()
	setPSIntLevel(0) // Enable interrupts!

	// CRITICAL: Disable interrupts before printing results!
	// println is NOT reentrant and crashes if interrupted
	old := interrupt.Disable()

	// Monitor background SYSTIMER interrupts
	finalCount := monitorBackgroundInterrupts()

	if finalCount == 0 {
		println("FATAL: No ISR fired!")

		// Run all diagnostic tests in order
		//testDirectCallToHandleInterrupt()
		//testSoftwareInterrupt()
		//testPSRegisterAndWAITI()
		testDeepDiagnostics()
		testGPIOHardwareInterrupt()

		// Dump all diagnostic information
		dumpDiagnosticInfo()

		println("\nHalting...")
		for {
		}
	}

	println("SUCCESS! ISR is working! GPIO41 should be blinking.")
	println(">>> Re-enabling interrupts for background loop...")

	interrupt.Restore(old) // Re-enable interrupts

	// Background: reflect ISR tick counter to GPIO41 without touching ISR
	var last uint32
	for {
		if esp.IsrCount != last {
			last = esp.IsrCount
			if gpio41State == 0 {
				debugPin.High()
				gpio41State = 1
			} else {
				debugPin.Low()
				gpio41State = 0
			}
		}
	}

	// Now use standard run() which will call initHeap() again but it should be safe
	run()

	// Fallback: if main ever returns, hang the CPU.
	exit(0)
}

func putchar(c byte) {
	machine.Serial.WriteByte(c)
}

func getchar() byte {
	for machine.Serial.Buffered() == 0 {
		Gosched()
	}
	v, _ := machine.Serial.ReadByte()
	return v
}

func buffered() int {
	return machine.Serial.Buffered()
}

// Initialize .bss: zero-initialized global variables.
// The .data section has already been loaded by the ROM bootloader.
func clearbss() {
	ptr := unsafe.Pointer(&_sbss)
	for ptr != unsafe.Pointer(&_ebss) {
		*(*uint32)(ptr) = 0
		ptr = unsafe.Add(ptr, 4)
	}
}

// initTimer configures TIMG0 as a free‑running counter for monotonic time (ticks/sleep).
// It does not generate interrupts and does not touch the Interrupt Matrix.
func initTimer() {
	// Configure timer 0 in timer group 0, for timekeeping.
	//   EN:       Enable the timer.
	//   INCREASE: Count up every tick (as opposed to counting down).
	//   DIVIDER:  16-bit prescaler, set to 2 for dividing the APB clock by two (80MHz / 2 = 40MHz).

	// First disable the timer
	esp.TIMG0.T0CONFIG.Set(0)

	// Set the timer counter value to 0.
	esp.TIMG0.T0LOADLO.Set(0)
	esp.TIMG0.T0LOADHI.Set(0)
	esp.TIMG0.T0LOAD.Set(0) // Trigger reload

	// Configure timer using ESP32-S3 specific methods:
	esp.TIMG0.SetT0CONFIG_DIVIDER(2)    // Set prescaler to 2 (80MHz / 2 = 40MHz)
	esp.TIMG0.SetT0CONFIG_INCREASE(1)   // Count up
	esp.TIMG0.SetT0CONFIG_AUTORELOAD(0) // No auto-reload
	esp.TIMG0.SetT0CONFIG_EN(1)         // Enable timer
}

func ticks() timeUnit {
	// First, update the LO and HI register pair by writing any value to the register.
	esp.TIMG0.T0UPDATE.Set(0)
	// Then read the two 32-bit parts of the timer.
	lo := esp.TIMG0.T0LO.Get()
	hi := esp.TIMG0.T0HI.Get()
	result := timeUnit(uint64(lo) | uint64(hi)<<32)
	return result
}

func nanosecondsToTicks(ns int64) timeUnit {
	// 25 = 1e9 / (80MHz / 2)
	return timeUnit(ns / 25)
}

func ticksToNanoseconds(ticks timeUnit) int64 {
	// See nanosecondsToTicks.
	return int64(ticks) * 25
}

// sleepTicks busy-waits until the given number of ticks have passed.
func sleepTicks(d timeUnit) {
	sleepUntil := ticks() + d
	for ticks() < sleepUntil {
		// TODO: suspend the CPU to not burn power here unnecessarily.
	}
}

func exit(code int) {
	abort()
}

// initGPIOPeripherals initializes GPIO and IO_MUX peripherals exactly like ESP-IDF
// Based on ESP-IDF gpio_hal_init and bootloader GPIO initialization
func initGPIOPeripherals() {
	// Enable GPIO peripheral clock - needed for GPIO matrix routing
	esp.GPIO.SetCLOCK_GATE_CLK_EN(1)

	// Also enable GPIO sigma delta clock if needed
	esp.GPIO_SD.SetSIGMADELTA_CG_CLK_EN(1)
	esp.GPIO_SD.SetSIGMADELTA_MISC_FUNCTION_CLK_EN(1)
}

// initUSBSerialRegisters ensures USB Serial/JTAG is properly configured
// ROM bootloader already initializes USB, so we just ensure critical bits are set
func initUSBSerialRegisters() {
	// NOTE: ROM bootloader already:
	// - Enabled USB_DEVICE clock (PERIP_CLK_EN1)
	// - Released USB from reset
	// - Basic endpoint configuration

	// We only ensure these critical registers are set (idempotent operations):

	// 1. Ensure USB peripheral clock is enabled (ROM already did this, but safe to set again)
	esp.SYSTEM.SetPERIP_CLK_EN1_USB_DEVICE_CLK_EN(1)

	// 2. ⚠️ DO NOT RESET USB! ROM already initialized it and host is connected
	// Resetting would break the connection!
	// esp.SYSTEM.SetPERIP_RST_EN1_USB_DEVICE_RST(1) // ❌ DON'T DO THIS

	// 3. Ensure USB internal clock is enabled
	// Reference: ESP-IDF usb_serial_jtag_ll.h - usb_serial_jtag_ll_clk_enable()
	esp.USB_DEVICE.SetMISC_CONF_CLK_EN(1)

	// 4. Ensure USB memory is powered up (not in power-down mode)
	// Reference: ESP-IDF usb_serial_jtag_ll.h - usb_serial_jtag_ll_phy_enable()
	esp.USB_DEVICE.SetMEM_CONF_USB_MEM_PD(0) // 0 = Power ON, 1 = Power DOWN

	// Small delay for register writes to take effect
	for i := 0; i < 100; i++ {
		device.Asm("nop")
	}
}

// initUSBSerial initializes USB Serial/JTAG peripheral
// Based on ESP-IDF USB Serial/JTAG driver initialization
func initUSBSerial() {
	// Enable USB Serial/JTAG peripheral clock
	// Reference: ESP-IDF components/hal/esp32s3/include/hal/usb_serial_jtag_ll.h
	esp.SYSTEM.SetPERIP_CLK_EN1_USB_DEVICE_CLK_EN(1)

	// Release USB Serial/JTAG from reset
	esp.SYSTEM.SetPERIP_RST_EN1_USB_DEVICE_RST(1) // Assert reset
	esp.SYSTEM.SetPERIP_RST_EN1_USB_DEVICE_RST(0) // Release reset

	// Small delay for peripheral stabilization
	for i := 0; i < 1000; i++ {
		device.Asm("nop")
	}
}

// initSPIPeripherals initializes SPI2 and SPI3 peripherals exactly like ESP-IDF
// Based on ESP-IDF spi_ll_enable_bus_clock and spi_ll_reset_register
func initSPIPeripherals() {
	// SPI2 (FSPI) - exactly like ESP-IDF spi_ll_enable_bus_clock
	esp.SYSTEM.SetPERIP_CLK_EN0_SPI2_CLK_EN(1) // Enable clock
	esp.SYSTEM.SetPERIP_RST_EN0_SPI2_RST(1)    // Assert reset
	esp.SYSTEM.SetPERIP_RST_EN0_SPI2_RST(0)    // Release reset

	// SPI3 (HSPI) - exactly like ESP-IDF spi_ll_enable_bus_clock
	esp.SYSTEM.SetPERIP_CLK_EN0_SPI3_CLK_EN(1) // Enable clock
	esp.SYSTEM.SetPERIP_RST_EN0_SPI3_RST(1)    // Assert reset
	esp.SYSTEM.SetPERIP_RST_EN0_SPI3_RST(0)    // Release reset

	// SPI01 base clock - needed for SPI2/SPI3 operation
	esp.SYSTEM.SetPERIP_CLK_EN0_SPI01_CLK_EN(1) // Base SPI clock

	// Initialize SPI2 master mode like ESP-IDF spi_ll_master_init
	initSPI2Master()
}

// initSPI2Master initializes SPI2 in master mode exactly like ESP-IDF spi_ll_master_init
func initSPI2Master() {
	bus := esp.SPI2

	// Reset timing - exact ESP-IDF sequence
	bus.USER1.Set(0) // cs_setup_time = 0, cs_hold_time = 0

	// Use all 64 bytes of the buffer
	bus.SetUSER_USR_MISO_HIGHPART(0)
	bus.SetUSER_USR_MOSI_HIGHPART(0)

	// Disable unneeded interrupts
	bus.SLAVE.Set(0)
	bus.USER.Set(0)

	// Configure master clock gate - critical for clock generation!
	bus.SetCLK_GATE_MST_CLK_ACTIVE(1) // Enable master clock
	bus.SetCLK_GATE_MST_CLK_SEL(1)    // Select master clock

	// Configure DMA
	bus.DMA_CONF.Set(0)
	bus.SetDMA_CONF_SLV_TX_SEG_TRANS_CLR_EN(1)
	bus.SetDMA_CONF_SLV_RX_SEG_TRANS_CLR_EN(1)
	bus.SetDMA_CONF_DMA_SLV_SEG_TRANS_EN(0)
}

func abort() {
	// lock up forever
	for {
		device.Asm("waiti 0")
	}
}

//go:extern _vector_table
var _vector_table [0]uintptr

//go:extern _sbss
var _sbss [0]byte

//go:extern _ebss
var _ebss [0]byte

// safePrint1 prints 1 arg with interrupts disabled
func safePrint1(a interface{}) {
	old := interrupt.Disable()
	println(a)
	interrupt.Restore(old)
}

// safePrint2 prints 2 args with interrupts disabled
func safePrint2(a, b interface{}) {
	old := interrupt.Disable()
	println(a, b)
	interrupt.Restore(old)
}

// safePrint3 prints 3 args with interrupts disabled
func safePrint3(a, b, c interface{}) {
	old := interrupt.Disable()
	println(a, b, c)
	interrupt.Restore(old)
}

// safePrint4 prints 4 args with interrupts disabled
func safePrint4(a, b, c, d interface{}) {
	old := interrupt.Disable()
	println(a, b, c, d)
	interrupt.Restore(old)
}

// setPSIntLevel sets PS.INTLEVEL to the given level (0..15)
// WARNING: Setting level=0 enables ALL interrupts! Must be called when ready.
func setPSIntLevel(level int) {
	// Read current PS
	oldPs := device.AsmFull("rsr.ps {}", nil)

	// Modify PS.INTLEVEL field (bits [3:0])
	ps := oldPs
	ps &^= 0x0F                 // Clear INTLEVEL bits
	ps |= uintptr(level & 0x0F) // Set new INTLEVEL

	// Write new PS and sync
	device.AsmFull("wsr.ps {v}", map[string]interface{}{"v": ps})
	device.AsmFull("rsync", nil)
}

// dumpSystimerDebug prints key SYSTIMER/CPU interrupt state
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

// initSystimerTick configures SYSTIMER TARGET0 to generate periodic interrupts every 10ms
// and routes it to a CPU interrupt channel via the Interrupt Matrix.
func initSystimerTick() {
	const systimerClockHz = 80_000_000 // assumed SYSTIMER clock
	const tickPeriodNs = 1_000_000     // 1ms (для быстрого тестирования)
	const cpuInterruptForSystimer = 23 // CPU-level interrupt line (avoid pending 20)
	const debugSkipISRRegistration = false

	// Compute period in timer ticks: ticks = Freq * period
	periodTicks := uint32((systimerClockHz * tickPeriodNs) / 1_000_000_000)
	if periodTicks == 0 {
		periodTicks = 1
	}
	println("SYST step1 periodTicks=", periodTicks)

	// Temporarily block interrupts during configuration (safe way)
	old := interrupt.Disable()
	println("SYST step2 mask IRQs (Disable)")

	// Map SYSTIMER TARGET0 to selected CPU interrupt channel on core0
	esp.INTERRUPT_CORE0.SetSYSTIMER_TARGET0_INT_MAP(cpuInterruptForSystimer)
	println("SYST step3 map cpuInt=", cpuInterruptForSystimer)

	// Ensure SYSTIMER clocks enabled
	esp.SYSTIMER.SetCONF_SYSTIMER_CLK_FO(1)
	esp.SYSTIMER.CONF.Set(esp.SYSTIMER.CONF.Get() | esp.SYSTIMER_CONF_CLK_EN)
	esp.SYSTIMER.SetCONF_TIMER_UNIT0_WORK_EN(1)
	esp.SYSTIMER.SetCONF_TIMER_UNIT1_WORK_EN(1)
	println("SYST step4 clocks on")

	// Configure periodic mode on UNIT1 for TARGET0 and set period
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_TIMER_UNIT_SEL(1)
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD_MODE(1)
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD(periodTicks)
	// Load comparator configuration (no arm yet)
	esp.SYSTIMER.SetCOMP0_LOAD_TIMER_COMP0_LOAD(1)
	println("SYST step5 cfg period + comp load")

	println("SYST step8 before isr registration")
	if !debugSkipISRRegistration {
		println("SYST step8.1 calling interrupt.New")
		_ = interrupt.New(cpuInterruptForSystimer, systimerHandleInterrupt)
		println("SYST step8.2 after interrupt.New (Enable deferred)")
	} else {
		println("SYST step8 skipped isr registration (debug)")
	}

	// Program first shot: latch UNIT1, wait valid, set TARGET0 = now + period, load, enable INT
	esp.SYSTIMER.SetUNIT1_OP_TIMER_UNIT1_UPDATE(1)
	for esp.SYSTIMER.GetUNIT1_OP_TIMER_UNIT1_VALUE_VALID() == 0 {
	}
	// read latched UNIT1 now (low then hi)
	nowLo := esp.SYSTIMER.UNIT1_VALUE_LO.Get()
	nowHi := esp.SYSTIMER.UNIT1_VALUE_HI.Get()
	// compute target = now + periodTicks
	tgtLo := nowLo + periodTicks
	tgtHi := nowHi
	if tgtLo < nowLo {
		tgtHi++
	}
	esp.SYSTIMER.SetTARGET0_HI_TIMER_TARGET0_HI(tgtHi & 0xFFFFF)
	esp.SYSTIMER.SetTARGET0_LO(tgtLo)
	esp.SYSTIMER.SetCOMP0_LOAD_TIMER_COMP0_LOAD(1)
	// Clear any pending status and enable SYSTIMER interrupt bit
	esp.SYSTIMER.INT_CLR.Set(1 << 0)
	esp.SYSTIMER.INT_ENA.SetBits(1 << 0)
	println("SYST step9 armed first shot: nowHI=", nowHi, " nowLO=", nowLo, " tgtHI=", tgtHi, " tgtLO=", tgtLo)

	// Arm periodic alarm
	esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(1)
	println("SYST after WORK_EN: T0CONF=", esp.SYSTIMER.TARGET0_CONF.Get())
	println("SYST step10 work en")

	// Unmask interrupts (log VECBASE/masks/status before)
	vec := device.AsmFull("rsr.vecbase {}", nil)
	ps := device.AsmFull("rsr.ps {}", nil)
	ien := device.AsmFull("rsr.intenable {}", nil)
	ist := device.AsmFull("rsr.interrupt {}", nil)
	println("SYST pre-unmask: VECBASE=", uint32(uintptr(vec)),
		" MAP=", esp.INTERRUPT_CORE0.GetSYSTIMER_TARGET0_INT_MAP(),
		" PS=", uint32(uintptr(ps)&0xFFFF),
		" INTENABLE=", uint32(uintptr(ien)),
		" INTERRUPT=", uint32(uintptr(ist)))
	// Mask CPU interrupts to 0 before restore to avoid immediate IRQ burst
	device.AsmFull("wsr.intenable {v}", map[string]interface{}{"v": uintptr(0)})
	device.AsmFull("rsync", nil)
	println("SYST step11 about to Restore (INTENABLE=0)")
	interrupt.Restore(old)
	println("SYST step11 restored, PS=", uint32(uintptr(device.AsmFull("rsr.ps {}", nil))&0xFFFF))

	// Now enable CPU interrupt line for SYSTIMER
	if !debugSkipISRRegistration {
		println("SYST step12 enabling IRQ line")
		// INTLEVEL=15 (mask all) while enabling line and clearing pending
		psTmp := device.AsmFull("rsr.ps {}", nil)
		psTmp &^= 0x0F
		psTmp |= 15
		device.AsmFull("wsr.ps {v}", map[string]interface{}{"v": psTmp})
		device.AsmFull("rsync", nil)

		// Clear peripheral pending if any
		if (esp.SYSTIMER.INT_ST.Get() & 1) != 0 {
			esp.SYSTIMER.INT_CLR.Set(1 << 0)
			println("SYST step12 cleared INT_ST")
		}

		// Set INTENABLE bit for selected CPU interrupt line
		ien := device.AsmFull("rsr.intenable {}", nil)
		ien |= (1 << cpuInterruptForSystimer)
		device.AsmFull("wsr.intenable {v}", map[string]interface{}{"v": ien})
		device.AsmFull("rsync", nil)
		ien2 := device.AsmFull("rsr.intenable {}", nil)
		intr2 := device.AsmFull("rsr.interrupt {}", nil)
		println("SYST step12 INTENABLE=", uint32(uintptr(ien2)), " INTERRUPT=", uint32(uintptr(intr2)))

		// Rearm comparator relative to current UNIT1 time (avoid past target)
		esp.SYSTIMER.SetUNIT1_OP_TIMER_UNIT1_UPDATE(1)
		for esp.SYSTIMER.GetUNIT1_OP_TIMER_UNIT1_VALUE_VALID() == 0 {
		}
		rNowLo := esp.SYSTIMER.UNIT1_VALUE_LO.Get()
		rNowHi := esp.SYSTIMER.UNIT1_VALUE_HI.Get()
		rTgtLo := rNowLo + periodTicks
		rTgtHi := rNowHi
		if rTgtLo < rNowLo {
			rTgtHi++
		}
		esp.SYSTIMER.SetTARGET0_HI_TIMER_TARGET0_HI(rTgtHi & 0xFFFFF)
		esp.SYSTIMER.SetTARGET0_LO(rTgtLo)
		esp.SYSTIMER.SetCOMP0_LOAD_TIMER_COMP0_LOAD(1)
		println("SYST step12 rearm: nowHI=", rNowHi, " nowLO=", rNowLo, " tgtHI=", rTgtHi, " tgtLO=", rTgtLo)

		// Drop to INTLEVEL=1 first, then 0 (avoid immediate burst)
		ps1 := device.AsmFull("rsr.ps {}", nil)
		ps1 &^= 0x0F
		ps1 |= 1
		device.AsmFull("wsr.ps {v}", map[string]interface{}{"v": ps1})
		device.AsmFull("rsync", nil)
		setPSIntLevel(0)
	}

	// Minimal log
	println("SYSTIMER tick armed: periodTicks=", periodTicks)
}

// systimerHandleInterrupt handles SYSTIMER TARGET0 interrupt (10ms tick).
func systimerHandleInterrupt(intr interrupt.Interrupt) {
	// Clear interrupt status only and bump counter. Avoid any non-ISR-safe calls.
	esp.SYSTIMER.INT_CLR.Set(1 << 0)
	systimerIRQCount++
}

// systimer debug pin state/counter (toggled every 10 ticks => 100ms)
var (
	systimerTickCount uint32
	gpio41State       uint8
	debugPin          machine.Pin
	systimerIRQSeen   uint32
	systimerIRQCount  uint32
)

// initDebugPin41 configures GPIO41 as push-pull output and sets it low.
func initDebugPin41() {
	debugPin = machine.Pin(41)
	debugPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	debugPin.Low()
	gpio41State = 0
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
