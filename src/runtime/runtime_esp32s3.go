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

//go:extern _text_start
var textStartSymbol [0]byte

//go:extern _UserExceptionVector
var userExceptionVectorSymbol [0]byte

//go:extern _isr_call_count
var isrCallCount [1]uint32

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
func initSystimerTick() {
	// SYSTIMER clock frequency for ESP32-S3
	// ESP-IDF: systimer_ll_get_counter_clock_src() returns 16MHz
	// Formula: 40MHz XTAL / 2.5 divider = 16MHz
	// Source: components/esp_hw_support/port/esp32s3/systimer.c:16
	const systimerClockHz = 16_000_000 // 16MHz (NOT 80MHz!)
	const tickPeriodNs = 1_000_000     // 1ms (same as CONFIG_FREERTOS_HZ=1000)
	const cpuInterruptForSystimer = 23 // CPU-level interrupt line (arbitrary choice)

	// Compute period in timer ticks: ticks = Freq * period
	periodTicks := uint32((systimerClockHz * tickPeriodNs) / 1_000_000_000)
	if periodTicks == 0 {
		periodTicks = 1
	}
	println("SYST: SYSTIMER frequency: 16MHz, period:", tickPeriodNs, "ns =", periodTicks, "ticks")

	// Temporarily block interrupts during configuration
	old := interrupt.Disable()
	println("SYST: After Disable(), old INTLEVEL=", uint32(old))

	// Verify interrupts are actually disabled
	psAfterDisable := device.AsmFull("rsr.ps {}", nil)
	intlevelNow := uint32(uintptr(psAfterDisable)) & 0x0F
	println("SYST: Current INTLEVEL (should be 15):", intlevelNow)

	// === CRITICAL: Enable SYSTIMER peripheral clock and reset ===
	// ESP-IDF: systimer_ll_enable_bus_clock(true) + systimer_ll_reset_register()
	// File: components/hal/esp32s3/include/hal/systimer_ll.h:39-50
	// Without this, SYSTIMER peripheral is DEAD (no interrupts, no counter updates)!
	println("SYST: Enabling SYSTIMER bus clock...")
	esp.SYSTEM.SetPERIP_CLK_EN0_SYSTIMER_CLK_EN(1)
	println("SYST: Resetting SYSTIMER peripheral...")
	esp.SYSTEM.SetPERIP_RST_EN0_SYSTIMER_RST(1) // Assert reset
	esp.SYSTEM.SetPERIP_RST_EN0_SYSTIMER_RST(0) // Release reset
	println("SYST: SYSTIMER peripheral ready!")

	// Map SYSTIMER TARGET0 to selected CPU interrupt channel on core0
	println("SYST: Mapping SYSTIMER_TARGET0 to CPU interrupt", cpuInterruptForSystimer)
	esp.INTERRUPT_CORE0.SetSYSTIMER_TARGET0_INT_MAP(cpuInterruptForSystimer)

	// Verify the mapping was written
	actualMapping := esp.INTERRUPT_CORE0.GetSYSTIMER_TARGET0_INT_MAP()
	println("SYST: Verified mapping:", actualMapping, "(expected:", cpuInterruptForSystimer, ")")

	// === ESP-IDF-style SYSTIMER initialization ===
	// Based on: esp-idf/components/freertos/port_systick.c: esp_setup_sys_time()
	// Reference: esp-idf/components/hal/systimer_hal.c
	//            esp-idf/components/hal/esp32s3/include/hal/systimer_ll.h

	// 1. Enable clocks
	// ESP-IDF: systimer_ll_enable_bus_clock(true)
	// File: components/freertos/port_systick.c:66
	esp.SYSTIMER.SetCONF_SYSTIMER_CLK_FO(1)
	esp.SYSTIMER.CONF.Set(esp.SYSTIMER.CONF.Get() | esp.SYSTIMER_CONF_CLK_EN)
	println("SYST: Clocks enabled")

	// 2. Reset UNIT0 counter to 0 (CRITICAL for periodic mode!)
	// ESP-IDF: systimer_ll_set_counter_value(dev, SYSTIMER_COUNTER_OS_TICK, 0);
	//          systimer_ll_apply_counter_value(dev, SYSTIMER_COUNTER_OS_TICK);
	// File: components/freertos/port_systick.c:73-74
	// Reason: Periodic mode triggers when (counter % period == 0), so counter must start at 0!
	println("SYST: Resetting UNIT0 counter to 0...")
	esp.SYSTIMER.SetUNIT0_LOAD_HI_TIMER_UNIT0_LOAD_HI(0)
	esp.SYSTIMER.SetUNIT0_LOAD_LO(0)
	esp.SYSTIMER.SetUNIT0_LOAD_TIMER_UNIT0_LOAD(1) // Apply
	println("SYST: UNIT0 reset to 0")

	// 3. Enable counter
	// ESP-IDF: systimer_hal_enable_counter(&systimer_hal, SYSTIMER_COUNTER_OS_TICK);
	// File: components/freertos/port_systick.c:88
	// Implementation: systimer_ll_enable_counter() sets bit 30 (for UNIT0) in CONF register
	esp.SYSTIMER.SetCONF_TIMER_UNIT0_WORK_EN(1)
	println("SYST: UNIT0 counter enabled")

	// 4. Configure TARGET0 - CRITICAL: Two-step initialization like ESP-IDF!
	// ESP-IDF sequence (port_systick.c:93-103):
	//   Step 1: systimer_hal_select_alarm_mode(..., SYSTIMER_ALARM_MODE_ONESHOT)  [line 94]
	//   Step 2: systimer_hal_set_alarm_period(...)                                [line 102]
	//   Step 3: systimer_hal_select_alarm_mode(..., SYSTIMER_ALARM_MODE_PERIOD)   [line 103]
	// WHY: Hardware requires ONESHOT mode init before switching to PERIOD mode!

	// Step 1: Connect to UNIT0 and set ONESHOT mode first (CRITICAL!)
	// ESP-IDF: systimer_hal_connect_alarm_counter(&systimer_hal, alarm_id, SYSTIMER_COUNTER_OS_TICK);
	//          systimer_hal_select_alarm_mode(&systimer_hal, alarm_id, SYSTIMER_ALARM_MODE_ONESHOT);
	println("SYST: Step 1: Configuring TARGET0 as ONESHOT initially...")
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_TIMER_UNIT_SEL(0) // Connect to UNIT0
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD_MODE(0)    // ONESHOT mode (period_mode = 0)
	println("SYST: TARGET0 -> UNIT0, ONESHOT mode (temporary)")

	// Step 2: Set period value
	// ESP-IDF: systimer_hal_set_alarm_period(&systimer_hal, alarm_id, 1000000UL / CONFIG_FREERTOS_HZ);
	println("SYST: Step 2: Setting period...")
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD(periodTicks) // Set period (16000 ticks = 1ms @ 16MHz)
	println("SYST: Period set to", periodTicks, "ticks")

	// Step 3: Switch to PERIODIC mode
	// ESP-IDF: systimer_hal_select_alarm_mode(&systimer_hal, alarm_id, SYSTIMER_ALARM_MODE_PERIOD);
	println("SYST: Step 3: Switching to PERIODIC mode...")
	esp.SYSTIMER.SetTARGET0_CONF_TARGET0_PERIOD_MODE(1) // PERIODIC mode (period_mode = 1)
	println("SYST: TARGET0 now in PERIODIC mode!")

	// Configure counter stall behavior (ESP-IDF does this!)
	// ESP-IDF: systimer_hal_counter_can_stall_by_cpu(&systimer_hal, SYSTIMER_COUNTER_OS_TICK, cpuid, true);
	// File: components/freertos/port_systick.c:85
	// Formula: bit = (28 - counter_id * 2) - cpu_id = (28 - 0*2) - 0 = 28
	// This allows CPU0 to stall UNIT0 counter (useful for debugging)
	println("SYST: Configuring counter stall for CPU0...")
	confStall := esp.SYSTIMER.CONF.Get()
	confStall |= (1 << 28) // Enable stall for UNIT0 by CPU0
	esp.SYSTIMER.CONF.Set(confStall)
	println("SYST: Counter can stall by CPU0")

	// 3. Register interrupt handler
	println("SYST: Registering handler...")
	_ = interrupt.New(cpuInterruptForSystimer, systimerHandleInterrupt)
	println("SYST: Handler registered")

	// 5. Configure periodic alarm (ESP-IDF sequence)
	// ESP-IDF: systimer_hal_set_alarm_period() function:
	//   - systimer_ll_enable_alarm(dev, alarm_id, false);
	//   - systimer_ll_set_alarm_period(dev, alarm_id, period);
	//   - systimer_ll_apply_alarm_value(dev, alarm_id);
	//   - systimer_ll_enable_alarm(dev, alarm_id, true);
	// File: components/hal/systimer_hal.c:119-124
	// Note: For PERIODIC mode, DO NOT set TARGET_LO/HI, only PERIOD!
	println("SYST: Configuring periodic alarm (ESP-IDF style)...")

	// Disable alarm first
	// ESP-IDF: systimer_ll_enable_alarm(dev, alarm_id, false)
	// Implementation: dev->conf.val &= ~(1 << (24 - alarm_id))
	esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(0)
	println("SYST: Alarm disabled for configuration")

	// Apply period value
	// ESP-IDF: systimer_ll_apply_alarm_value(dev, alarm_id)
	// Implementation: dev->comp_load[alarm_id].val = 0x01
	println("SYST: Applying period via COMP_LOAD...")
	esp.SYSTIMER.SetCOMP0_LOAD_TIMER_COMP0_LOAD(1)

	// Clear interrupt flags
	println("SYST: Clearing INT...")
	esp.SYSTIMER.INT_CLR.Set(1 << 0)

	// Enable alarm FIRST (like ESP-IDF!)
	// ESP-IDF: systimer_ll_enable_alarm(dev, alarm_id, true)
	// Implementation: dev->conf.val |= 1 << (24 - alarm_id)
	// File: components/hal/systimer_hal.c:119 (inside systimer_hal_set_alarm_period)
	println("SYST: Enabling alarm (WORK_EN=1)...")
	esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(1)

	// THEN enable interrupt (like ESP-IDF!)
	// ESP-IDF: systimer_hal_enable_alarm_int(&systimer_hal, alarm_id)
	// File: components/freertos/port_systick.c:87
	println("SYST: Enabling interrupt (INT_ENA)...")
	esp.SYSTIMER.INT_ENA.SetBits(1 << 0)

	// Verify it was set
	confAfter := esp.SYSTIMER.CONF.Get()
	workEnBit := (confAfter >> 24) & 1
	println("SYST: After enable: CONF=", confAfter, "WORK_EN bit=", workEnBit)
	if workEnBit != 1 {
		println("✗ ERROR: WORK_EN not set! Hardware issue?")
	}
	println("SYST: Periodic alarm armed (ESP-IDF style)!")

	// Verify interrupts are still disabled before Restore
	psBeforeRestore := device.AsmFull("rsr.ps {}", nil)
	intlevelBefore := uint32(uintptr(psBeforeRestore)) & 0x0F
	println("SYST: Before Restore(), INTLEVEL=", intlevelBefore, "(should be 15)")
	println("SYST: Will restore to INTLEVEL=", uint32(old))

	// About to restore interrupts
	println("SYST: About to call interrupt.Restore()...")

	// Restore interrupts (PS register) - NOW interrupts can fire
	interrupt.Restore(old)

	println("SYST: Returned from interrupt.Restore()!")

	// Verify interrupts are restored
	psAfterRestore := device.AsmFull("rsr.ps {}", nil)
	intlevelAfter := uint32(uintptr(psAfterRestore)) & 0x0F
	println("SYST: After Restore(), INTLEVEL=", intlevelAfter, "(should be", uint32(old), ")")
	println("SYST: PS.INTLEVEL restored, now enabling CPU interrupt line...")

	// NOW enable CPU interrupt line for SYSTIMER through INTENABLE
	println("SYST: Reading INTENABLE...")
	ien := device.AsmFull("rsr.intenable {}", nil)
	println("SYST: Current INTENABLE:", uint32(uintptr(ien)))

	println("SYST: Setting bit 23...")
	ien |= (1 << cpuInterruptForSystimer)
	println("SYST: New INTENABLE:", uint32(uintptr(ien)))

	// CRITICAL: Clear any pending SYSTIMER interrupt RIGHT before enabling INTENABLE
	// Otherwise we get infinite interrupt loop!
	println("SYST: Clearing SYSTIMER INT_ST before enabling INTENABLE...")
	esp.SYSTIMER.INT_CLR.Set(1 << 0)

	// Also clear CPU interrupt if pending
	device.AsmFull("wsr.intclear {v}", map[string]interface{}{
		"v": uintptr(1 << cpuInterruptForSystimer),
	})
	device.AsmFull("rsync", nil)
	println("SYST: Cleared, now writing INTENABLE...")

	device.AsmFull("wsr.intenable {v}", map[string]interface{}{"v": ien})
	println("SYST: Wrote INTENABLE, doing rsync...")

	device.AsmFull("rsync", nil)
	println("SYST: INTENABLE bit 23 set - interrupts now ACTIVE!")

	// PERIODIC mode: alarm auto-reloads, just wait for interrupts
	println("SYST: Waiting for interrupts...")
	startCount := systimerIRQCount
	startHandlerCount := interrupt.GetHandleInterruptCallCount()

	// Read ASM counter before test (global variable)
	println("SYST: ASM ISR counter before test:", isrCallCount[0])

	// Verify SYSTIMER state before test
	confVal := esp.SYSTIMER.CONF.Get()
	target0Conf := esp.SYSTIMER.TARGET0_CONF.Get()
	println("SYST: Before test: CONF=", confVal, "TARGET0_CONF=", target0Conf)
	println("SYST: WORK_EN bit:", (confVal>>24)&1, "(should be 1)")
	println("SYST: PERIOD_MODE bit:", (target0Conf>>30)&1, "(should be 1)")
	println("SYST: PERIOD value:", target0Conf&0x3FFFFFF)

	println("SYST: IRQs enabled, starting wait loop...")

	// Wait a bit
	for i := 0; i < 100000; i++ {
		if i%10000 == 0 {
			// Read ASM counter
			asmCount := isrCallCount[0]

			// Read INTERRUPT register to see if bit 23 is active
			intReg := device.AsmFull("rsr.interrupt {}", nil)
			intBit23 := (uint32(uintptr(intReg)) >> 23) & 1

			// Read SYSTIMER INT_ST
			intST := esp.SYSTIMER.INT_ST.Get()

			println("SYST: iter", i, "ASM:", asmCount, "INT[23]:", intBit23, "INT_ST:", intST)
		}
		device.Asm("nop")
	}

	println("SYST: Wait loop completed, stopping alarm...")

	// Stop alarm
	interrupt.SetPSIntLevel(15)
	println("SYST: IRQs disabled")

	esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(0)
	println("SYST: Alarm disabled")

	esp.SYSTIMER.INT_CLR.Set(1 << 0)
	println("SYST: INT cleared")

	interrupt.SetPSIntLevel(0)
	println("SYST: IRQs restored")

	println("SYST: Test completed!")
	println("SYST: ASM ISR counter:", isrCallCount[0])

	// Check results
	endHandlerCount := interrupt.GetHandleInterruptCallCount()
	println("SYST: Results:")
	println("  ASM ISR calls:", isrCallCount[0])
	println("  handleInterrupt calls:", endHandlerCount-startHandlerCount)
	println("  systimerHandleInterrupt calls:", systimerIRQCount-startCount)

	if systimerIRQCount > startCount {
		println("✓ SYSTIMER interrupts working! Count:", systimerIRQCount)
	} else {
		println("✗ ERROR: No SYSTIMER interrupts received!")
		println("  INT_ST:", esp.SYSTIMER.INT_ST.Get(), "INT_ENA:", esp.SYSTIMER.INT_ENA.Get())
		ien := device.AsmFull("rsr.intenable {}", nil)
		ist := device.AsmFull("rsr.interrupt {}", nil)
		println("  INTENABLE:", uint32(uintptr(ien)), "INTERRUPT:", uint32(uintptr(ist)))

		// Detailed SYSTIMER diagnostics
		println("\nDETAILED SYSTIMER STATE:")
		confVal := esp.SYSTIMER.CONF.Get()
		println("  CONF:", confVal)
		println("    UNIT0_WORK_EN (bit 30):", (confVal>>30)&1)
		println("    UNIT1_WORK_EN (bit 29):", (confVal>>29)&1)
		println("    TARGET0_WORK_EN (bit 24):", (confVal>>24)&1)

		target0Conf := esp.SYSTIMER.TARGET0_CONF.Get()
		println("  TARGET0_CONF:", target0Conf)
		println("    PERIOD_MODE (bit 30):", (target0Conf>>30)&1)
		println("    UNIT_SEL (bit 31):", (target0Conf>>31)&1)
		println("    PERIOD:", target0Conf&0x3FFFFFF)

		println("  TARGET0_LO:", esp.SYSTIMER.TARGET0_LO.Get())
		println("  TARGET0_HI:", esp.SYSTIMER.TARGET0_HI.Get())

		// Read current UNIT0 timer value (TARGET0 is connected to UNIT0!)
		esp.SYSTIMER.SetUNIT0_OP_TIMER_UNIT0_UPDATE(1)
		for esp.SYSTIMER.GetUNIT0_OP_TIMER_UNIT0_VALUE_VALID() == 0 {
		}
		currentLo := esp.SYSTIMER.UNIT0_VALUE_LO.Get()
		currentHi := esp.SYSTIMER.UNIT0_VALUE_HI.Get()
		println("  UNIT0_VALUE_LO:", currentLo)
		println("  UNIT0_VALUE_HI:", currentHi)
		println("  UNIT0_OP:", esp.SYSTIMER.UNIT0_OP.Get())

		// Check Interrupt Matrix
		actualMap := esp.INTERRUPT_CORE0.GetSYSTIMER_TARGET0_INT_MAP()
		println("  Interrupt Matrix mapping:", actualMap)

		// Check if target was reached
		if currentLo > esp.SYSTIMER.TARGET0_LO.Get() {
			println("  ⚠️  Counter PASSED target but INT_ST=0!")
		}

		// Check INT_RAW - shows interrupt before masking
		intRaw := esp.SYSTIMER.INT_RAW.Get()
		println("  INT_RAW:", intRaw, "(shows interrupt status before INT_ENA mask)")
		if intRaw != 0 {
			println("  ⚠️  INT_RAW is set but INT_ST=0! Problem with INT_ENA or interrupt routing!")
		}
	}
}

// systimerHandleInterrupt handles SYSTIMER TARGET0 interrupt (1ms tick).
func systimerHandleInterrupt(intr interrupt.Interrupt) {
	// CRITICAL: Clear interrupt status FIRST to avoid re-triggering
	esp.SYSTIMER.INT_CLR.Set(1 << 0)

	// Increment counter (simple, ISR-safe, no print/println!)
	systimerIRQCount++
}
