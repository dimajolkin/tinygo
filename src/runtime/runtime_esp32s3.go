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
	"unsafe"
)

var systimerIRQCount = 0

// External symbols from linker script (esp32s3.ld) and assembly
// These are defined in the linker script and assembly files
//
//go:extern _vector_base
var vectorBaseSymbol [0]byte

//go:extern _vectors_end
var vectorsEndSymbol [0]byte

//go:extern _vectors_flash_start
var vectorsFlashStartSymbol [0]uint32

//go:extern _text_start
var textStartSymbol [0]byte

//go:extern _UserExceptionVector
var userExceptionVectorSymbol [0]byte

//go:extern _isr_call_count
var isrCallCount [1]uint32

//go:extern _exception_handler_called
var exceptionHandlerCalled [1]uint32

//go:extern _user_exc_called
var userExcCalled [1]uint32

//go:extern _to_unhandled_exc_called
var toUnhandledExcCalled [1]uint32

//go:extern _recover_test_flag
var recoverTestFlag uint32

// Helper functions to get addresses from linker symbols
// These are needed because direct access to symbols doesn't work reliably in TinyGo
func getVectorBase() uintptr {
	return uintptr(unsafe.Pointer(&vectorBaseSymbol))
}

func getVectorsEnd() uintptr {
	return uintptr(unsafe.Pointer(&vectorsEndSymbol))
}

func getTextStart() uintptr {
	return uintptr(unsafe.Pointer(&textStartSymbol))
}

func getUserExceptionVector() uintptr {
	return uintptr(unsafe.Pointer(&userExceptionVectorSymbol))
}

// VECBASE management functions
// These ensure VECBASE is correctly set early in startup

//go:linkname setVecbaseToVectorBase runtime.setVecbaseToVectorBase
func setVecbaseToVectorBase()

// readVecbase reads the current VECBASE register value
func readVecbase() uintptr {
	return device.AsmFull("rsr.vecbase {}", nil)
}

// ensureVecbase ensures VECBASE points to _vector_base
// MUST be called early in main() before any exceptions/interrupts
func ensureVecbase() {
	vectorBase := getVectorBase()
	vecbase := readVecbase()

	if vecbase == 0 || vecbase != vectorBase {
		// VECBASE is wrong or zero - fix it!
		// DEBUG: Signal that we're trying to set VECBASE
		debugGPIO(6) // GPIO6 = trying to set VECBASE

		setVecbaseToVectorBase()

		// CRITICAL: Synchronize after VECBASE change (isync + memw)
		device.Asm("isync")
		device.Asm("memw")

		// Small delay to ensure VECBASE is committed
		for i := 0; i < 10; i++ {
			device.Asm("nop")
		}

		// Verify it was set correctly
		vecbase2 := readVecbase()
		if vecbase2 != vectorBase {

			// Try reading multiple times - maybe cache issue?
			for retry := 0; retry < 5; retry++ {
				device.Asm("isync")
				vecbase3 := readVecbase()
				if vecbase3 == vectorBase {

					return
				}
				// Delay between retries
				for i := 0; i < 100; i++ {
					device.Asm("nop")
				}
			}

			// Failed after retries - halt
			for {
				device.Asm("waiti 0")
			}
		}
	}
}

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

	// CRITICAL: Disable watchdogs FIRST after clearbss() when Go structures are ready
	// esp.TIMG0/TIMG1/RTC_CNTL are global pointers that need .bss to be initialized
	// Must disable before any delays to prevent watchdog reset
	disableWatchdogs()

	// CRITICAL: Set VECBASE right after watchdogs disabled but before any other initialization
	// This ensures exception vectors are ready if any early exceptions occur
	// ensureVecbase()

	// Initialize memory subsystems (MMU, cache buses, autoload)
	// This complements the basic cache init done in esp32s3.S
	// Reference: ESP-IDF bootloader_esp32s3.c and cache_hal_init()
	//esp.InitMemorySubsystems()

	// Initialize GPIO and SPI peripherals early (GPIO matrix might be already initialized by ROM)
	initGPIOPeripherals()
	initSPIPeripherals()

	// Initialize UART after USB configuration
	machine.USBCDC.Configure(machine.UARTConfig{BaudRate: 115200})
	machine.InitSerial()

	// Validate vector table layout (ESP-IDF compliance)
	initTimer()
	for i := 0; i < 10000; i++ {
		print(".")
	}
	print("\n")

	checkVectorsInMemory()

	// Initialize SYSTIMER for system tick
	//initSystimerTick()  // DISABLED: testing exceptions without interrupts

	testUnhandledException()

	// Check if SYSTIMER interrupts are working
	//checkSystemTimer()  // DISABLED: will test after exception handling works

	// Exception test (DANGEROUS - will halt system!)
	// Uncomment to verify exception handling:
	//
	// testUnhandledException()    // Tests _xt_unhandled_exception handler

	// Call the standard runtime
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

// printhex32 prints a 32-bit value in the same 0xXXXXXXXX style as printptr.
// We simply cast to uintptr and reuse printptr because ESP32-S3 is 32-bit.
func printhex32(v uint32) {
	printptr(uintptr(v))
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

// checkVectorsInMemory inspects the vector table in memory and validates correct placement
// of exception vectors. Offsets are now derived dynamically from linked symbols (not hardcoded).
func checkVectorsInMemory() {
	println("\n=== VECTOR TABLE MEMORY CHECK ===")

	// Known-good BSS anchors (sanity that symbols resolve)
	sbssAddr := uintptr(unsafe.Pointer(&_sbss))
	ebssAddr := uintptr(unsafe.Pointer(&_ebss))
	textStartAddr := uintptr(unsafe.Pointer(&textStartSymbol))
	vectorsEndAddr := uintptr(unsafe.Pointer(&vectorsEndSymbol))

	println("_sbss address (decimal):", uint32(sbssAddr))
	println("_ebss address (decimal):", uint32(ebssAddr))
	println("_text_start address (decimal):", uint32(textStartAddr))
	println("_vectors_end address (decimal):", uint32(vectorsEndAddr))

	// Vector base from linker vs CPU register
	vectorBaseAddr := getVectorBase()
	vecbase := readVecbase()
	println("_vector_base address (decimal):", uint32(vectorBaseAddr))
	println("VECBASE register (decimal):", uint32(uintptr(vecbase)))

	print("_sbss hex: ")
	printptr(sbssAddr)
	println()
	print("_ebss hex: ")
	printptr(ebssAddr)
	println()
	print("_vector_base hex: ")
	printptr(vectorBaseAddr)
	println()
	print("VECBASE hex: ")
	printptr(uintptr(vecbase))
	println()

	if vectorBaseAddr == uintptr(vecbase) {
		println("\u2713 VECBASE correctly points to _vector_base")
	} else {
		println("\u2717 ERROR: VECBASE mismatch!")
	}

	// Check that .text doesn't overlap with .vectors
	if textStartAddr >= vectorsEndAddr {
		println("\u2713 .text starts after .vectors (no overlap)")
		print("  Gap: ")
		printhex32(uint32(textStartAddr - vectorsEndAddr))
		println(" bytes")
	} else {
		println("\u2717 ERROR: .text OVERLAPS with .vectors!")
		println("  _vectors_end:", uint32(vectorsEndAddr))
		println("  _text_start:", uint32(textStartAddr))
	}

	// Fetch symbol address for `_UserExceptionVector` early
	uevSym := getUserExceptionVector()

	// Helper to read a 32-bit word; returns (val, ok)
	read32 := func(addr uintptr) (uint32, bool) {
		if addr == 0 {
			return 0, false
		}
		return *(*uint32)(unsafe.Pointer(addr)), true
	}

	println("\n=== VECTOR TABLE CONTENTS (BY LEVEL) ===\n")

	base := vectorBaseAddr

	// Canonical ESP-IDF/LX7 defaults (for reference only)
	const (
		defOffKernel = uintptr(0x040)
		defOffNMI    = uintptr(0x060)
		defOffL2     = uintptr(0x080)
		defOffL3     = uintptr(0x0A0)
		defOffL4     = uintptr(0x0C0)
		defOffL5     = uintptr(0x0E0)
		defOffL6     = uintptr(0x100)
		defOffL7     = uintptr(0x120)
		defOffL1     = uintptr(0x180) // UserException / Level-1 (typical on ESP32-S3)
		defOffDouble = uintptr(0x1C0)
	)

	// Actual Level‑1 offset from linked assembly symbol (do NOT hardcode)
	offL1 := uintptr(0)
	if base != 0 && uevSym >= base {
		offL1 = uevSym - base
	}

	// Sanity for Level‑1: must be 0x20‑aligned and within first 0x200 bytes
	if (offL1&0x1F) != 0 || offL1 >= 0x200 {
		println("✗ WARNING: _UserExceptionVector offset looks odd:", uint32(offL1))
		println("  Expect 0x180 on ESP32‑S3; will still dump using symbol address")
	}

	// 1) Level‑1 / UserException: dump using computed offset from symbol
	print("[1] UserExceptionVector (Level-1) at offset ")
	printhex32(uint32(offL1))
	println(":")
	u0, _ := read32(base + offL1)
	u1, _ := read32(base + offL1 + 4)
	print("  +")
	printhex32(uint32(offL1 + 0x00))
	print(": ")
	printptr(uintptr(u0))
	println()
	print("  +")
	printhex32(uint32(offL1 + 0x04))
	print(": ")
	printptr(uintptr(u1))
	println()
	if offL1 != defOffL1 {
		print("  (note) canonical ESP‑IDF offset for L1 is ")
		printhex32(uint32(defOffL1))
		println("; using actual symbol offset above")
	}

	// 2) The rest of the vectors (single word dump is enough: call0 stub)
	type vec struct {
		name string
		off  uintptr
	}
	others := []vec{
		{"KernelExceptionVector", defOffKernel},
		{"NMIExceptionVector", defOffNMI},
		{"Level2InterruptVector", defOffL2},
		{"Level3InterruptVector", defOffL3},
		{"Level4InterruptVector", defOffL4},
		{"Level5InterruptVector", defOffL5},
		{"Level6InterruptVector", defOffL6},
		{"Level7InterruptVector", defOffL7},
		{"DoubleExceptionVector", defOffDouble},
	}
	for i, v := range others {
		print("[", i+2, "] ", v.name, " at offset ")
		printhex32(uint32(v.off))
		println(":")
		w0, _ := read32(base + v.off)
		print("  +")
		printhex32(uint32(v.off))
		print(": ")
		printptr(uintptr(w0))
		// Many of these are small call0 stubs in ROM/IRAM, opcode low byte 0xC5
		if (w0 & 0xFF) == 0xC5 {
			print(" (call0 -> _xt_unhandled_exception)")
		}
		println()
	}

	// --- SUMMARY / VALIDATION ---
	println("\n=== SUMMARY ===")
	// Count non-zero across the range [0x040..0x1C0] step 4 + L1 block first two words
	totalNonZero := 0
	for off := uintptr(0x040); off <= 0x1C0; off += 4 {
		w, _ := read32(base + off)
		if w != 0 && w != 0xFFFFFFFF {
			totalNonZero++
		}
	}
	// include L1 +0/+4 (already inside loop if 0x180..0x184) but safe to keep
	println("Total non-zero words:", totalNonZero, "/", (0x1C0-0x040)/4+1)

	// Validate Level-1 actually present at computed offset
	if u0 == 0 {
		println("\u2717 WARNING: Level-1 vector at computed offset is zero (unexpected)")
	} else {
		println("\u2713 Level-1 vector present at computed offset")
	}

	print("UserExceptionVector symbol addr: ")
	println(uint32(uevSym))
	if uevSym != base+offL1 {
		println("\u2717 WARNING: _UserExceptionVector != VECBASE+computed_off (unexpected)")
	}

	// Show first two words at the symbol (should match the +offL1 dump)
	uS0, _ := read32(uevSym + 0)
	uS1, _ := read32(uevSym + 4)
	print("UserException tramp[0..1]: ")
	printptr(uintptr(uS0))
	print(" ")
	printptr(uintptr(uS1))
	println()
	// Explicit cross-check: words at VECBASE+offL1 vs symbol address
	if u0 != uS0 || u1 != uS1 {
		println("✗ MISMATCH: words at VECBASE+offL1 differ from symbol address contents")
		println("  Hint: verify that IRAM region is readable via data bus and that cache/MMU are on")
	} else {
		println("✓ VECBASE+offL1 matches symbol contents")
	}

	// Quick range check: vector code must live in IRAM 0x4030_0000..0x407F_FFFF
	if uevSym < 0x40300000 || uevSym >= 0x40800000 {
		println("\u2717 WARNING: _UserExceptionVector outside IRAM range")
	}

	// Alignment / base consistency checks
	if (vectorBaseAddr & 0x1FF) != 0 {
		println("\u2717 WARNING: _vector_base is not 0x200-aligned")
	}
	if vectorBaseAddr != uintptr(vecbase) {
		println("\u2717 WARNING: VECBASE != _vector_base (unexpected)")
	}
	print("Computed L1 offset: ")
	printhex32(uint32(offL1))
	print("  | Canonical (ESP‑IDF): ")
	printhex32(uint32(defOffL1))
	println()

	println("\n=== END MEMORY CHECK ===\n")
}

//go:extern _vector_table
var _vector_table [0]uintptr

//go:extern _sbss
var _sbss [0]byte

//go:extern _ebss
var _ebss [0]byte

// initSystimerTick configures SYSTIMER TARGET0 to generate periodic interrupts every 1ms
// for the TinyGo scheduler, following ESP-IDF's FreeRTOS implementation.
//
// Architecture (ESP32-S3):
//   - SYSTIMER has 2 counter units (UNIT0, UNIT1) - 64-bit counters at 16MHz
//   - SYSTIMER has 3 alarms (TARGET0, TARGET1, TARGET2) - comparators
//   - Each alarm can connect to any counter unit
//   - Alarms support ONESHOT or PERIODIC mode
//
// This function implements the same sequence as:
//
//	esp-idf/components/freertos/port_systick.c: esp_setup_sys_time()
//
// ESP-IDF uses SYSTIMER_COUNTER_OS_TICK (UNIT0) with SYSTIMER_ALARM_OS_TICK_CORE0 (TARGET0)
// in PERIODIC mode for the OS tick (1000 Hz = 1ms period).
//
// Key differences from ESP-IDF:
//   - ESP-IDF uses FreeRTOS scheduler, we use TinyGo's goroutine scheduler
//   - ESP-IDF allocates interrupt handler dynamically, we use static registration
//   - ESP-IDF enables core stalling, we don't (single-threaded on CPU0)
//
// References:
//   - esp-idf/components/freertos/port_systick.c
//   - esp-idf/components/hal/systimer_hal.c
//   - esp-idf/components/hal/esp32s3/include/hal/systimer_ll.h
//   - esp-idf/components/soc/esp32s3/include/soc/systimer_struct.h
//
// testInterruption - изолированный тест векторной таблицы через программное прерывание
// Global counter for testInterruption handler (cannot use closure in interrupt handlers)
var swHandled int

// Handler for software interrupt line 1 (used in testInterruption)
func swInterruptHandler(_ interrupt.Interrupt) {
	const swLine = 1
	// Clear the CPU request bit (edge/software source) using assembly helper
	interrupt.WriteIntClear(1 << swLine)
	swHandled++
}

// initSystimerTick - упрощённая инициализация SYSTIMER для системного тика
// Основано на ESP-IDF esp_setup_sys_time()
func initSystimerTick() {
	const systimerClockHz = 16_000_000 // 16MHz
	const tickPeriodMs = 1             // 1ms
	const cpuInterruptLine = 1         // CPU interrupt line (Level 1)

	periodTicks := uint32((systimerClockHz * tickPeriodMs) / 1000) // 16000 ticks
	println("SYSTIMER: Initializing, period", periodTicks, "ticks (1ms)")

	// Step 1: Enable SYSTIMER peripheral clock and reset
	esp.SYSTEM.SetPERIP_CLK_EN0_SYSTIMER_CLK_EN(1)
	esp.SYSTEM.SetPERIP_RST_EN0_SYSTIMER_RST(1) // Assert reset
	esp.SYSTEM.SetPERIP_RST_EN0_SYSTIMER_RST(0) // Release reset

	// Step 2: Map SYSTIMER TARGET0 → CPU interrupt line 1
	esp.INTERRUPT_CORE0.SetSYSTIMER_TARGET0_INT_MAP(cpuInterruptLine)

	// Step 3: Enable SYSTIMER clocks
	esp.SYSTIMER.SetCONF_SYSTIMER_CLK_FO(1)
	esp.SYSTIMER.CONF.SetBits(esp.SYSTIMER_CONF_CLK_EN)

	// Step 4: Reset UNIT0 counter to 0
	esp.SYSTIMER.SetUNIT0_LOAD_HI_TIMER_UNIT0_LOAD_HI(0)
	esp.SYSTIMER.SetUNIT0_LOAD_LO(0)
	esp.SYSTIMER.SetUNIT0_LOAD_TIMER_UNIT0_LOAD(1) // Apply

	// Step 5: Enable UNIT0 counter
	esp.SYSTIMER.SetCONF_TIMER_UNIT0_WORK_EN(1)

	// Step 6: Configure TARGET0 (ONESHOT mode initially)
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_TIMER_UNIT_SEL(0) // Connect to UNIT0
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD_MODE(0)    // ONESHOT

	// Step 7: Enable counter stall by CPU0 (debugging)
	esp.SYSTIMER.CONF.SetBits(1 << 28)

	// Step 8: Register interrupt handler
	intr := interrupt.New(cpuInterruptLine, systimerHandleInterrupt)
	intr.Enable()

	// Step 9: Configure periodic alarm
	esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(0) // Disable

	// Set period
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD(periodTicks)

	// Set initial target
	currentCounter := uint64(esp.SYSTIMER.UNIT0_VALUE_LO.Get()) | (uint64(esp.SYSTIMER.UNIT0_VALUE_HI.Get()) << 32)
	initialTarget := currentCounter + uint64(periodTicks)
	esp.SYSTIMER.TARGET0_LO.Set(uint32(initialTarget & 0xFFFFFFFF))
	esp.SYSTIMER.TARGET0_HI.Set(uint32(initialTarget >> 32))

	// Apply
	esp.SYSTIMER.SetCOMP0_LOAD_TIMER_COMP0_LOAD(1)

	// Clear pending
	esp.SYSTIMER.INT_CLR.Set(1 << 0)

	// Enable alarm
	esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(1)

	// Switch to PERIODIC mode
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD_MODE(1)

	// Enable interrupt
	esp.SYSTIMER.INT_ENA.SetBits(1 << 0)

	// Step 10: Enable INTENABLE bit (keep INTLEVEL=15 during init!)
	oldMask := interrupt.Disable()
	ien := interrupt.ReadIntEnable()
	ien |= (1 << cpuInterruptLine)
	interrupt.WriteIntEnable(ien)
	interrupt.Restore(oldMask)

	// Step 11: Lower INTLEVEL to 0 to enable interrupts
	interrupt.SetPSIntLevel(0)

	println("SYSTIMER: Initialized and ACTIVE, period", periodTicks, "ticks (1ms)")
}

// systimerHandleInterrupt handles SYSTIMER TARGET0 interrupt (1ms tick).
func systimerHandleInterrupt(intr interrupt.Interrupt) {
	// Clear interrupt status FIRST
	esp.SYSTIMER.INT_CLR.Set(1 << 0)

	// Increment counter
	systimerIRQCount++
}

// checkSystemTimer verifies that SYSTIMER interrupts are working correctly.
func checkSystemTimer() {
	println("\n=== SYSTIMER INTERRUPT TEST ===")

	startCount := systimerIRQCount
	println("Initial IRQ count:", startCount)

	// Wait for interrupts (should fire every 1ms)
	println("Waiting 100ms for interrupts...")
	for i := 0; i < 100; i++ {
		sleepTicks(nanosecondsToTicks(1_000_000)) // 1ms
	}

	endCount := systimerIRQCount
	interruptsReceived := endCount - startCount

	println("Final IRQ count:", endCount)
	println("Interrupts received:", interruptsReceived)

	if interruptsReceived > 0 {
		println("✓ SYSTIMER interrupts are WORKING!")
		println("  Rate:", interruptsReceived, "interrupts per 100ms")

		// Calculate accuracy (should be ~100 for 100ms with 1ms period)
		expectedCount := 100
		accuracy := (interruptsReceived * 100) / expectedCount
		println("  Accuracy:", accuracy, "%")

		if accuracy < 90 || accuracy > 110 {
			println("  ⚠️  Warning: Interrupt rate outside expected range (90-110%)")
		}
	} else {
		println("✗ ERROR: No SYSTIMER interrupts received!")
		println("  Checking system state...")

		// Debug info - CPU state
		psNow := interrupt.GetPS()
		ienNow := interrupt.ReadIntEnable()
		intPending := interrupt.ReadInterrupt()

		println("\n  CPU State:")
		println("    PS (INTLEVEL):", psNow&0x0F, "(should be 0)")
		println("    INTENABLE:", ienNow, "(bit 1 should be set)")
		println("    INTERRUPT:", intPending, "(shows pending interrupts)")

		// Debug info - SYSTIMER state
		println("\n  SYSTIMER State:")
		println("    INT_ST:", esp.SYSTIMER.INT_ST.Get(), "(interrupt status)")
		println("    INT_ENA:", esp.SYSTIMER.INT_ENA.Get(), "(bit 0 should be set)")
		println("    INT_RAW:", esp.SYSTIMER.INT_RAW.Get(), "(raw interrupt)")

		confReg := esp.SYSTIMER.CONF.Get()
		println("    CONF:", confReg)
		println("      UNIT0_WORK_EN:", (confReg>>30)&1, "(should be 1)")
		println("      TARGET0_WORK_EN:", (confReg>>24)&1, "(should be 1)")

		target0Conf := esp.SYSTIMER.TARGET0_CONF.Get()
		println("    TARGET0_CONF:", target0Conf)
		println("      PERIOD_MODE:", (target0Conf>>30)&1, "(should be 1)")
		println("      PERIOD:", target0Conf&0x3FFFFFF)

		// Debug info - Interrupt Matrix
		println("\n  Interrupt Matrix:")
		actualMap := esp.INTERRUPT_CORE0.GetSYSTIMER_TARGET0_INT_MAP()
		println("    SYSTIMER_TARGET0 → CPU line:", actualMap, "(should be 1)")

		// Debug info - Counter value
		println("\n  Counter Status:")
		esp.SYSTIMER.SetUNIT0_OP_TIMER_UNIT0_UPDATE(1)
		for esp.SYSTIMER.GetUNIT0_OP_TIMER_UNIT0_VALUE_VALID() == 0 {
		}
		counterLo := esp.SYSTIMER.UNIT0_VALUE_LO.Get()
		counterHi := esp.SYSTIMER.UNIT0_VALUE_HI.Get()
		println("    UNIT0_VALUE_HI:", counterHi)
		println("    UNIT0_VALUE_LO:", counterLo)

		targetLo := esp.SYSTIMER.TARGET0_LO.Get()
		targetHi := esp.SYSTIMER.TARGET0_HI.Get()
		println("    TARGET0_HI:", targetHi)
		println("    TARGET0_LO:", targetLo)

		if counterLo > targetLo {
			println("    ⚠️  Counter already passed target!")
		}
	}

	println("=== END TEST ===\n")
}

// testUnhandledException triggers a real CPU exception to verify the full exception path:
// CPU → Vector Table → _xt_unhandled_exception → handleException
// This will cause a fatal exception and halt the system!
func testUnhandledException() {
	println("\n=== TESTING EXCEPTION HANDLING (FULL PATH) ===")
	println("⚠️  This will trigger a REAL CPU EXCEPTION!")
	println("Expected flow:")
	println("  1. CPU detects exception")
	println("  2. CPU jumps to vector table")
	println("  3. Vector handler calls _xt_unhandled_exception")
	println("  4. _xt_unhandled_exception calls handleException")
	println()
	println("Expected output:")
	println("  FATAL EXCEPTION!")
	println("  EXCCAUSE: 0x00000000 (Illegal Instruction)")
	println("  EXCVADDR: 0x00000000")
	println("  EPC: 0x4037XXXX or 0x4200XXXX (address of illegal instruction)")
	println("  <system halts>")
	println()

	println("Triggering Illegal Instruction exception...")
	println()

	// Check VECBASE before triggering exception
	vecbase := readVecbase()
	println("DEBUG: VECBASE =", uint32(uintptr(vecbase)))

	// Check PS (should have INTLEVEL=0 for exceptions to work)
	ps := device.AsmFull("rsr.ps {}", nil)
	println("DEBUG: PS =", uint32(uintptr(ps)))
	println("DEBUG: PS.INTLEVEL =", uint32(uintptr(ps))&0x0F)
	println("DEBUG: PS.WOE =", (uint32(uintptr(ps))>>18)&1)

	// Check if vector exists at computed offset from VECBASE
	userExcVec := (*uint32)(unsafe.Pointer(uintptr(vecbase) + 0x180))
	print("DEBUG: UserExceptionVector[VECBASE+0x180] = ")
	printptr(uintptr(*userExcVec))
	println(" (decimal:", *userExcVec, ")")

	// Check ALL debug counters BEFORE triggering exception
	excCountBefore := exceptionHandlerCalled[0]
	userExcBefore := userExcCalled[0]
	toUnhandledBefore := toUnhandledExcCalled[0]

	println("DEBUG: Exception handler call count BEFORE:", excCountBefore)
	println("DEBUG: User exception dispatcher count BEFORE:", userExcBefore)
	println("DEBUG: To unhandled exception count BEFORE:", toUnhandledBefore)

	// CRITICAL: Re-check vector contents RIGHT before triggering exception!
	println()
	println("=== FINAL CHECK: Vector contents before exception ===")
	userExcVec2 := (*uint32)(unsafe.Pointer(uintptr(vecbase) + 0x180))
	userExcVec3 := (*uint32)(unsafe.Pointer(uintptr(vecbase) + 0x184))
	print("  [VECBASE+0x180]: ")
	printptr(uintptr(*userExcVec2))
	println()
	print("  [VECBASE+0x184]: ")
	printptr(uintptr(*userExcVec3))
	println()
	if *userExcVec2 == 0 || *userExcVec2 == 0xFFFFFFFF {
		println("  ✗ WARNING: Vector looks invalid (zero or all 1s)!")
	}

	println()
	println("All checks passed, triggering exception NOW...")
	println()

	// IMPORTANT: Xtensa LX7 does NOT generate exception on divide-by-zero!
	// We also CAN'T use Go nil pointer dereference - TinyGo runtime catches it with panic!
	// Instead, we use inline assembly to trigger ILLEGAL INSTRUCTION exception.
	// This will trigger EXCCAUSE=0 (IllegalInstructionCause)
	// CPU will automatically:
	// 1. Save PC to EPC1
	// 2. Set EXCCAUSE=0
	// 3. Jump to UserExceptionVector (VECBASE+0x180)
	// 4. Vector handler calls _xt_unhandled_exception
	// 5. _xt_unhandled_exception calls handleException

	println("Executing: SYSCALL instruction (system call exception)")
	println("(next instruction will cause exception)")

	// CRITICAL: Check PS register RIGHT before syscall
	ps2 := device.AsmFull("rsr.ps {}", nil)
	psVal := uint32(uintptr(ps2))

	println()
	println("=== PS REGISTER CHECK (right before syscall) ===")
	print("PS = ")
	printptr(uintptr(psVal))
	println()
	println("PS.INTLEVEL =", psVal&0x0F)
	println("PS.EXCM =", (psVal>>4)&1, " <- MUST BE 0 for exceptions to work!")
	println("PS.UM =", (psVal>>5)&1)
	println("PS.WOE =", (psVal>>18)&1)

	// CRITICAL: Verify vector code is executable
	// Check that we can actually READ the instruction at vector address
	println()
	println("=== VECTOR EXECUTABILITY CHECK ===")
	vectorAddr := uintptr(vecbase) + 0x180
	vectorInstr := (*uint32)(unsafe.Pointer(vectorAddr))
	print("Instruction at VECBASE+0x180: ")
	printptr(uintptr(*vectorInstr))
	println()

	// Check if instruction looks valid (not all zeros or all ones)
	if *vectorInstr == 0 {
		println("✗ ERROR: Vector instruction is ZERO!")
	} else if *vectorInstr == 0xFFFFFFFF {
		println("✗ ERROR: Vector instruction is all ONES!")
	} else {
		println("✓ Vector instruction looks valid")
	}

	// Verify VECBASE alignment (must be 0x200-aligned)
	if vecbase&0x1FF != 0 {
		println("✗ ERROR: VECBASE is not 0x200-aligned!")
		print("  VECBASE alignment: ")
		printhex32(uint32(vecbase & 0x1FF))
		println()
	} else {
		println("✓ VECBASE is properly aligned (0x200)")
	}

	// CRITICAL: Re-check VECBASE RIGHT before exception
	println()
	println("=== VECBASE FINAL CHECK (right before exception) ===")
	vecbaseFinal := readVecbase()
	vectorBaseFinal := getVectorBase()
	print("VECBASE register: ")
	printptr(uintptr(vecbaseFinal))
	println()
	print("_vector_base symbol: ")
	printptr(vectorBaseFinal)
	println()
	if vecbaseFinal != vectorBaseFinal {
		println("✗ CRITICAL: VECBASE mismatch right before exception!")
		println("  Attempting to fix...")
		setVecbaseToVectorBase()
		device.Asm("isync")
		device.Asm("memw")
		vecbaseAfterFix := readVecbase()
		if vecbaseAfterFix == vectorBaseFinal {
			println("✓ VECBASE fixed!")
		} else {
			println("✗ FAILED to fix VECBASE!")
		}
	} else {
		println("✓ VECBASE is correct")
	}

	// Check vector contents one more time
	userExcVecFinal := (*uint32)(unsafe.Pointer(uintptr(vecbaseFinal) + 0x180))
	print("UserExceptionVector[VECBASE+0x180] = ")
	printptr(uintptr(*userExcVecFinal))
	println()
	if *userExcVecFinal == 0 || *userExcVecFinal == 0xFFFFFFFF {
		println("✗ CRITICAL: Vector is corrupted!")
	}

	if (psVal>>4)&1 == 1 {
		println("✗ ERROR: PS.EXCM=1 blocks ALL exceptions!")
		println("Attempting to clear EXCM bit...")
		// Try to clear EXCM
		device.Asm(
			"rsr.ps a2\n" +
				"movi a3, 0xFFFFFFEF\n" + // Mask to clear bit 4 (EXCM)
				"and a2, a2, a3\n" +
				"wsr.ps a2\n" +
				"rsync\n",
		)
		// Re-check
		ps3 := device.AsmFull("rsr.ps {}", nil)
		println("PS after clearing EXCM:", uint32(uintptr(ps3)))
	}

	println()
	println("Executing SOFTWARE INTERRUPT (Level-1) NOW...")
	println("  This will trigger Level-1 INTERRUPT (not exception)")
	println("  Goes through UserExceptionVector → _xt_user_exc → _xt_lowint1")

	// TEST: Call DebugGPIO4Set to verify the function works from Go
	println("Testing DebugGPIO4Set() function...")
	//interrupt.DebugGPIO4Set()
	println("✓ DebugGPIO4Set() called - GPIO4 should be HIGH")

	// IMPORTANT: Set _recover_test_flag so exception handler will do RFE instead of halting
	println("Setting _recover_test_flag to enable RFE after exception...")
	recoverTestFlag = 1

	// Small delay to see GPIO4
	for i := 0; i < 10000; i++ {
		device.Asm("nop")
	}

	// Small delay to ensure GPIO is visible
	for i := 0; i < 1000; i++ {
		device.Asm("nop")
	}

	// CRITICAL: Check PS.INTLEVEL before setting interrupt
	psBeforeInt := device.AsmFull("rsr.ps {}", nil)
	psIntLevel := uint32(uintptr(psBeforeInt)) & 0x0F
	println("PS.INTLEVEL before interrupt:", psIntLevel)
	if psIntLevel > 0 {
		println("✗ ERROR: PS.INTLEVEL > 0 blocks Level-1 interrupts!")
		println("  Attempting to clear INTLEVEL...")
		device.Asm(
			"rsr.ps a2\n" +
				"movi a3, 0xFFFFFFF0\n" + // Clear INTLEVEL bits
				"and a2, a2, a3\n" +
				"wsr.ps a2\n" +
				"rsync\n",
		)
	}

	// Execute SOFTWARE INTERRUPT via INTSET
	// This WILL trigger Level-1 interrupt (EXCCAUSE=4)
	// Goes through UserExceptionVector (Level-1) → _xt_user_exc → _xt_lowint1
	// This bypasses all exception blocking mechanisms!

	// CRITICAL: On ESP32-S3, only SOFTWARE interrupts (INT7 and INT29) can be triggered via INTSET!
	// According to ESP-IDF: XCHAL_INTSETTABLE_MASK = XCHAL_INTTYPE_MASK_SOFTWARE
	// INT7 and INT29 are the only SOFTWARE interrupts on ESP32-S3.
	// Using interrupt 7 (bit 7) instead of 0 or 1.
	// XCHAL_INT7_LEVEL = 1 (Level-1 interrupt)

	// Check state BEFORE setup
	intenableBefore := device.AsmFull("rsr.intenable {}", nil)
	interruptBefore := device.AsmFull("rsr.interrupt {}", nil)
	println("INTENABLE BEFORE setup:", uint32(uintptr(intenableBefore)))
	println("INTERRUPT BEFORE setup:", uint32(uintptr(interruptBefore)))

	// Re-check VECBASE one more time
	vecbaseFinal = readVecbase()
	println("VECBASE FINAL CHECK before interrupt:", vecbaseFinal)
	if vecbaseFinal != 0x40374000 {
		println("✗ ERROR: VECBASE is wrong! Expected 0x40374000, got:", vecbaseFinal)
		return
	}

	device.Asm(
		// Step 0: Clear ALL pending interrupts first (including bit 7)
		"rsr.interrupt a2\n" + // Read all pending interrupts
			"wsr.intclear a2\n" + // Clear ALL pending interrupts
			"rsync\n" +
			// Now specifically clear bit 7 again to be sure
			"movi a2, 128\n" + // Bit 7 = CPU interrupt 7 (SOFTWARE type)
			"wsr.intclear a2\n" +
			"rsync\n" +
			// Step 1: Enable interrupt line 7 in INTENABLE
			"rsr.intenable a2\n" +
			"movi a3, 128\n" + // Bit 7 = CPU interrupt 7
			"or a2, a2, a3\n" + // Enable bit 7
			"wsr.intenable a2\n" +
			"rsync\n" +
			// Step 2: Trigger interrupt by setting INTSET (only works for SOFTWARE interrupts!)
			"movi a2, 128\n" + // Bit 7 = CPU interrupt 7 (SOFTWARE)
			"wsr.intset a2\n" + // Set interrupt pending
			"rsync\n" +
			"nop\n" + // Allow interrupt to be taken
			"nop\n" +
			"nop\n",
	)

	// Verify interrupt is pending and enabled (checking bit 7 now)
	interruptPending := device.AsmFull("rsr.interrupt {}", nil)
	interruptVal := uint32(uintptr(interruptPending))
	println("INTERRUPT (pending) after INTSET:", interruptVal)
	if interruptVal&(1<<7) == 0 {
		println("✗ ERROR: Interrupt 7 is NOT pending!")
	} else {
		println("✓ Interrupt 7 (SOFTWARE) is pending")
	}

	intenableCheck := device.AsmFull("rsr.intenable {}", nil)
	intenableVal := uint32(uintptr(intenableCheck))
	println("INTENABLE after setup:", intenableVal)
	if intenableVal&(1<<7) == 0 {
		println("✗ ERROR: Interrupt 7 is NOT enabled in INTENABLE!")
	} else {
		println("✓ Interrupt 7 is enabled in INTENABLE")
	}

	psAfterInt := device.AsmFull("rsr.ps {}", nil)
	psIntLevelAfter := uint32(uintptr(psAfterInt)) & 0x0F
	println("PS.INTLEVEL after setup:", psIntLevelAfter)
	if psIntLevelAfter > 0 {
		println("✗ ERROR: PS.INTLEVEL still blocks interrupts!")
	}

	// Long delay to allow interrupt to be taken
	println("Waiting for interrupt to be taken...")
	for i := 0; i < 100000; i++ {
		device.Asm("nop")
	}

	// Check state AFTER delay
	interruptAfter := device.AsmFull("rsr.interrupt {}", nil)
	interruptValAfter := uint32(uintptr(interruptAfter))
	println("INTERRUPT (pending) AFTER delay:", interruptValAfter)
	if interruptValAfter&(1<<7) != 0 {
		println("⚠ WARNING: Interrupt 7 is STILL pending after delay!")
		println("  This means interrupt was NOT taken by CPU")
	} else {
		println("✓ Interrupt 7 was cleared (may have been processed)")
	}

	// Check if we're still at the same code location (GPIO5 toggle means we didn't enter ISR)
	// Should NEVER reach here - if we do, toggle GPIO5
	println("✗ ERROR: Software interrupt did not trigger!")
	println("✗ ERROR: We should never reach here!")
	println("  Check: Are GPIO6 or GPIO7 lighting up? (vectors)")
	println("  Check: Is interrupt 7 still pending?", interruptValAfter&(1<<7) != 0)
}
