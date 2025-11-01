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

// External symbols from linker script (esp32s3.ld)
// These are defined in the linker script and point to specific memory addresses
//
//go:extern _vector_base
var vectorBaseSymbol [0]byte

//go:extern _vectors_end
var vectorsEndSymbol [0]byte

//go:extern _text_start
var textStartSymbol [0]byte

//go:extern _UserExceptionVector
var userExceptionVectorSymbol [0]byte

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

// Compatibility aliases for test code
var (
	vectorBase          = getVectorBase
	vectorsEnd          = getVectorsEnd
	textStart           = getTextStart
	userExceptionVector = getUserExceptionVector
)

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

	// Initialize UART after USB configuration
	machine.USBCDC.Configure(machine.UARTConfig{BaudRate: 115200})
	machine.InitSerial()

	// Validate vector table layout (ESP-IDF compliance)
	initTimer()
	for i := 0; i < 10000; i++ {
		print(".")
	}
	print("\n")

	// ДИАГНОСТИКА: проверить что векторы действительно в IRAM
	checkVectorsInMemory()

	// Initialize SYSTIMER for system tick
	initSystimerTick()

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

// checkVectorsInMemory проверяет, что векторы действительно скопированы в IRAM
// и показывает первые 64 байта векторной таблицы
func checkVectorsInMemory() {
	println("\n=== VECTOR TABLE MEMORY CHECK ===")

	// ТЕСТ: Сравнить с _sbss и _ebss - они ТОЧНО работают!
	sbssAddr := uintptr(unsafe.Pointer(&_sbss))
	ebssAddr := uintptr(unsafe.Pointer(&_ebss))
	println("_sbss address (decimal):", uint32(sbssAddr))
	println("_ebss address (decimal):", uint32(ebssAddr))

	// Получить адрес _vector_base (через функцию-обертку)
	vectorBaseAddr := getVectorBase()
	println("_vector_base address (decimal):", uint32(vectorBaseAddr))

	// Получить текущий VECBASE из регистра
	vecbase := device.AsmFull("rsr.vecbase {}", nil)
	println("VECBASE register (decimal):", uint32(uintptr(vecbase)))

	// Выводим в hex формате используя printptr()
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

	// Проверить что адреса совпадают
	if vectorBaseAddr == uintptr(vecbase) {
		println("✓ VECBASE correctly points to _vector_base")
	} else {
		println("✗ ERROR: VECBASE mismatch!")
	}

	// Прочитать ключевые векторы (показываем только non-zero для краткости)
	println("\nVector table contents (non-zero entries only):")
	ptr := (*[32]uint32)(unsafe.Pointer(vectorBaseAddr))
	nonZeroCount := 0
	for i := 0; i < 32; i++ {
		val := ptr[i]
		if val != 0 && val != 0xFFFFFFFF {
			offset := i * 4
			println("  Offset", offset, "bytes: value =", val)
			nonZeroCount++
		}
	}
	println("Total non-zero words:", nonZeroCount, "/ 32")

	// Декодировать какие векторы присутствуют (по смещениям)
	println("\nVector presence analysis:")
	if ptr[0] != 0 {
		println("  ✓ UserExceptionVector at +0x00")
	}
	if ptr[8] != 0 { // 0x20 / 4 = 8
		println("  ✓ DoubleExceptionVector at +0x20")
	}
	if ptr[16] != 0 { // 0x40 / 4 = 16
		println("  ✓ KernelExceptionVector at +0x40")
	}
	if ptr[24] != 0 { // 0x60 / 4 = 24
		println("  ✓ NMIExceptionVector at +0x60")
	}

	// Проверить что UserExceptionVector имеет правильную инструкцию
	// Первая инструкция должна быть примерно: wsr a0, EXCSAVE_1 (0x00Dxxx)
	firstInstr := ptr[0]
	if (firstInstr & 0x00FF00) == 0x00D100 {
		println("✓ First instruction looks like 'wsr a0, EXCSAVE_1'")
	} else {
		println("✗ First instruction doesn't match expected pattern")
		println("  Expected: 0x00D1xxxx (wsr a0, EXCSAVE_1)")
		print("  Got:      ")
		printptr(uintptr(firstInstr))
		println()
	}

	println("=== END MEMORY CHECK ===\n")
}

//go:extern _vector_table
var _vector_table [0]uintptr

//go:extern _sbss
var _sbss [0]byte

//go:extern _ebss
var _ebss [0]byte

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

// initSystimerTick configures SYSTIMER TARGET0 to generate periodic interrupts every 10ms
// and routes it to a CPU interrupt channel via the Interrupt Matrix.
func initSystimerTick() {
	const systimerClockHz = 80_000_000 // assumed SYSTIMER clock
	const tickPeriodNs = 1_000_000     // 1ms
	const cpuInterruptForSystimer = 23 // CPU-level interrupt line

	// Compute period in timer ticks: ticks = Freq * period
	periodTicks := uint32((systimerClockHz * tickPeriodNs) / 1_000_000_000)
	if periodTicks == 0 {
		periodTicks = 1
	}

	// Temporarily block interrupts during configuration
	old := interrupt.Disable()

	// Map SYSTIMER TARGET0 to selected CPU interrupt channel on core0
	esp.INTERRUPT_CORE0.SetSYSTIMER_TARGET0_INT_MAP(cpuInterruptForSystimer)

	// Ensure SYSTIMER clocks enabled
	esp.SYSTIMER.SetCONF_SYSTIMER_CLK_FO(1)
	esp.SYSTIMER.CONF.Set(esp.SYSTIMER.CONF.Get() | esp.SYSTIMER_CONF_CLK_EN)
	esp.SYSTIMER.SetCONF_TIMER_UNIT0_WORK_EN(1)
	esp.SYSTIMER.SetCONF_TIMER_UNIT1_WORK_EN(1)

	// Configure periodic mode on UNIT1 for TARGET0 and set period
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_TIMER_UNIT_SEL(1)
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD_MODE(1)
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD(periodTicks)
	esp.SYSTIMER.SetCOMP0_LOAD_TIMER_COMP0_LOAD(1)

	// Register interrupt handler
	println("SYST: Registering handler...")
	_ = interrupt.New(cpuInterruptForSystimer, systimerHandleInterrupt)
	println("SYST: Handler registered")

	// Program first shot: latch UNIT1, wait valid, set TARGET0 = now + period
	esp.SYSTIMER.SetUNIT1_OP_TIMER_UNIT1_UPDATE(1)
	for esp.SYSTIMER.GetUNIT1_OP_TIMER_UNIT1_VALUE_VALID() == 0 {
	}
	nowLo := esp.SYSTIMER.UNIT1_VALUE_LO.Get()
	nowHi := esp.SYSTIMER.UNIT1_VALUE_HI.Get()
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
	println("SYST: Peripheral INT enabled")

	// Arm periodic alarm
	esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(1)
	println("SYST: WORK_EN set")

	// Mask CPU interrupts temporarily
	device.AsmFull("wsr.intenable {v}", map[string]interface{}{"v": uintptr(0)})
	device.AsmFull("rsync", nil)
	interrupt.Restore(old)
	println("SYST: Restored old state")

	// Enable CPU interrupt line for SYSTIMER with INTLEVEL=15 temporarily
	psTmp := device.AsmFull("rsr.ps {}", nil)
	psTmp &^= 0x0F
	psTmp |= 15
	device.AsmFull("wsr.ps {v}", map[string]interface{}{"v": psTmp})
	device.AsmFull("rsync", nil)
	println("SYST: INTLEVEL=15")

	// Clear peripheral pending if any
	if (esp.SYSTIMER.INT_ST.Get() & 1) != 0 {
		esp.SYSTIMER.INT_CLR.Set(1 << 0)
		println("SYST: Cleared pending INT_ST")
	}

	// Set INTENABLE bit for selected CPU interrupt line
	ien := device.AsmFull("rsr.intenable {}", nil)
	ien |= (1 << cpuInterruptForSystimer)
	device.AsmFull("wsr.intenable {v}", map[string]interface{}{"v": ien})
	device.AsmFull("rsync", nil)
	println("SYST: INTENABLE bit 23 set")

	// Rearm comparator relative to current time
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
	println("SYST: Rearmed comparator")

	// Drop to INTLEVEL=0 (enable interrupts)
	println("SYST: About to enable interrupts (INTLEVEL=0)...")
	setPSIntLevel(0)
	println("SYST: Interrupts enabled!")

	// Wait 10ms for at least one interrupt to fire
	println("SYST: Waiting for first interrupt...")
	startCount := systimerIRQCount

	// TEST: Temporarily disable interrupts to see if loop completes
	println("SYST: Test - disabling IRQs temporarily...")
	setPSIntLevel(15)

	// Simple busy wait
	println("SYST: Starting busy wait...")
	for i := 0; i < 1000000; i++ {
		device.Asm("nop")
	}
	println("SYST: Busy wait completed!")

	// Re-enable interrupts
	println("SYST: Re-enabling IRQs...")
	setPSIntLevel(0)

	// Now wait for interrupt
	println("SYST: Waiting with IRQs enabled...")
	for i := 0; i < 1000000; i++ {
		device.Asm("nop")
		if systimerIRQCount > startCount {
			println("SYST: Got interrupt in loop!")
			break
		}
	}

	// Check if interrupt fired
	println("SYST: After wait, count=", systimerIRQCount)
	if systimerIRQCount > startCount {
		println("✓ SYSTIMER interrupts working! Count:", systimerIRQCount)
	} else {
		println("✗ ERROR: No SYSTIMER interrupts received!")
		println("  INT_ST:", esp.SYSTIMER.INT_ST.Get(), "INT_ENA:", esp.SYSTIMER.INT_ENA.Get())
		ien := device.AsmFull("rsr.intenable {}", nil)
		ist := device.AsmFull("rsr.interrupt {}", nil)
		println("  INTENABLE:", uint32(uintptr(ien)), "INTERRUPT:", uint32(uintptr(ist)))
	}
}

// systimerHandleInterrupt handles SYSTIMER TARGET0 interrupt (1ms tick).
func systimerHandleInterrupt(intr interrupt.Interrupt) {
	// CRITICAL: Clear interrupt status FIRST to avoid re-triggering
	esp.SYSTIMER.INT_CLR.Set(1 << 0)

	// Increment counter (simple, ISR-safe, no print/println!)
	systimerIRQCount++
}
