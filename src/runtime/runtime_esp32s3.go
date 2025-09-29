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
	"unsafe"
)

// Note: heapStart, heapEnd, and growHeap are defined in baremetal.go
// which is automatically included for ESP32-S3 targets

// Cache control using ESP-IDF registers from device/esp package
// Based on ESP-IDF soc/esp32s3/include/soc/extmem_reg.h
// EXTMEM registers are available through esp.EXTMEM

// Cache control registers - from ESP-IDF soc/esp32s3/include/soc/extmem_reg.h
const (
	EXTMEM_ICACHE_CTRL_REG  = 0x60008000
	EXTMEM_DCACHE_CTRL_REG  = 0x60008044
	EXTMEM_ICACHE_CTRL1_REG = 0x60008004
	EXTMEM_DCACHE_CTRL1_REG = 0x60008048

	// Cache enable bits
	EXTMEM_ICACHE_ENABLE = (1 << 0)
	EXTMEM_DCACHE_ENABLE = (1 << 0)

	// Cache invalidate bits
	EXTMEM_ICACHE_INVALIDATE = (1 << 1)
	EXTMEM_DCACHE_INVALIDATE = (1 << 1)
)

// MMU constants - from ESP-IDF components/soc/esp32s3/include/soc/mmu.h
const (
	MMU_TABLE_BASE    = 0x600C5000 // MMU table base address
	MMU_ENTRY_COUNT   = 512        // Number of MMU entries
	MMU_INVALID_ENTRY = 0x4000     // Invalid entry marker
	MMU_VALID_BIT     = 0x8000     // Valid entry bit
	MMU_PAGE_SIZE     = 64 * 1024  // 64KB MMU page size for ESP32-S3
	DROM_VADDR_START  = 0x3C000000 // DROM virtual address start
	IROM_VADDR_START  = 0x42000000 // IROM virtual address start
	FLASH_PADDR_START = 0x0        // Flash physical address start
)

// initCacheAndMMU initializes the cache and MMU system for ESP32-S3
// Based on ESP-IDF bootloader_utility.c:set_cache_and_start_app() but simplified for TinyGo self-boot
// This function should be called ONLY if ROM bootloader hasn't already done this initialization
func initCacheAndMMU() {
	// For self-booting TinyGo, we need to replicate what ESP-IDF bootloader does:
	// 1. Disable cache
	// 2. Reset MMU table
	// 3. Map DROM and IROM regions
	// 4. Enable cache

	// Step 1: Disable cache (using HAL approach like ESP-IDF)
	// cache_hal_disable(CACHE_LL_LEVEL_EXT_MEM, CACHE_TYPE_ALL);
	disableCache()

	// Step 2: Reset MMU table - equivalent to mmu_hal_unmap_all()
	resetMMUTable()

	// Step 3: Map flash regions
	// Configure DROM mapping (read-only data from flash)
	mapDROM()
	// Configure IROM mapping (instruction cache from flash)
	mapIROM()

	// Step 4: Enable cache
	// cache_hal_enable(CACHE_LL_LEVEL_EXT_MEM, CACHE_TYPE_ALL);
	enableCache()
}

// disableCache disables both instruction and data caches
// Based on ESP-IDF hal/esp32s3/include/hal/cache_ll.h:cache_ll_l1_disable_cache
func disableCache() {
	// Disable ICache
	icacheCtrl := *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_ICACHE_CTRL_REG)))
	icacheCtrl &= ^uint32(EXTMEM_ICACHE_ENABLE)
	*(*uint32)(unsafe.Pointer(uintptr(EXTMEM_ICACHE_CTRL_REG))) = icacheCtrl

	// Disable DCache
	dcacheCtrl := *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_DCACHE_CTRL_REG)))
	dcacheCtrl &= ^uint32(EXTMEM_DCACHE_ENABLE)
	*(*uint32)(unsafe.Pointer(uintptr(EXTMEM_DCACHE_CTRL_REG))) = dcacheCtrl

	// Wait for cache to be disabled
	for {
		icacheCtrl = *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_ICACHE_CTRL_REG)))
		dcacheCtrl = *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_DCACHE_CTRL_REG)))
		if (icacheCtrl&EXTMEM_ICACHE_ENABLE) == 0 && (dcacheCtrl&EXTMEM_DCACHE_ENABLE) == 0 {
			break
		}
	}
}

// enableCache enables both instruction and data caches
// Based on ESP-IDF hal/esp32s3/include/hal/cache_ll.h:cache_ll_l1_enable_cache
func enableCache() {
	// Invalidate both caches first
	*(*uint32)(unsafe.Pointer(uintptr(EXTMEM_ICACHE_CTRL1_REG))) |= EXTMEM_ICACHE_INVALIDATE
	*(*uint32)(unsafe.Pointer(uintptr(EXTMEM_DCACHE_CTRL1_REG))) |= EXTMEM_DCACHE_INVALIDATE

	// Wait for invalidation to complete
	for {
		icacheCtrl1 := *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_ICACHE_CTRL1_REG)))
		dcacheCtrl1 := *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_DCACHE_CTRL1_REG)))
		if (icacheCtrl1&EXTMEM_ICACHE_INVALIDATE) == 0 && (dcacheCtrl1&EXTMEM_DCACHE_INVALIDATE) == 0 {
			break
		}
	}

	// Enable ICache
	*(*uint32)(unsafe.Pointer(uintptr(EXTMEM_ICACHE_CTRL_REG))) |= EXTMEM_ICACHE_ENABLE

	// Enable DCache
	*(*uint32)(unsafe.Pointer(uintptr(EXTMEM_DCACHE_CTRL_REG))) |= EXTMEM_DCACHE_ENABLE
}

// isCacheEnabled checks if both instruction and data caches are enabled
func isCacheEnabled() bool {
	icacheCtrl := *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_ICACHE_CTRL_REG)))
	dcacheCtrl := *(*uint32)(unsafe.Pointer(uintptr(EXTMEM_DCACHE_CTRL_REG)))

	return (icacheCtrl&EXTMEM_ICACHE_ENABLE) != 0 && (dcacheCtrl&EXTMEM_DCACHE_ENABLE) != 0
}

// resetMMUTable resets the MMU translation table
// Based on ESP-IDF components/hal/mmu_hal.c:mmu_hal_unmap_all()
func resetMMUTable() {
	mmuTable := (*[MMU_ENTRY_COUNT]uint32)(unsafe.Pointer(uintptr(MMU_TABLE_BASE)))
	for i := 0; i < MMU_ENTRY_COUNT; i++ {
		mmuTable[i] = MMU_INVALID_ENTRY
	}
}

// mapDROM maps the DROM (Data ROM) region from flash to virtual memory
// Based on ESP-IDF bootloader_utility.c:1065-1083 and hal/esp32s3/mmu_hal.c
func mapDROM() {
	const PAGES_TO_MAP = 32 // Map 2MB (32 * 64KB)
	mapFlashRegion(DROM_VADDR_START, FLASH_PADDR_START, PAGES_TO_MAP)
}

// mapIROM maps the IROM (Instruction ROM) region from flash to virtual memory
// Based on ESP-IDF bootloader_utility.c:1085-1103 and hal/esp32s3/mmu_hal.c
func mapIROM() {
	const PAGES_TO_MAP = 32 // Map 2MB (32 * 64KB)
	mapFlashRegion(IROM_VADDR_START, FLASH_PADDR_START, PAGES_TO_MAP)
}

// mapFlashRegion maps a flash region to virtual memory using the MMU
// Based on ESP-IDF components/hal/esp32s3/mmu_hal.c:mmu_hal_map_region()
func mapFlashRegion(vaddr, paddr uint32, pageCount int) {
	mmuTable := (*[MMU_ENTRY_COUNT]uint32)(unsafe.Pointer(uintptr(MMU_TABLE_BASE)))

	for i := 0; i < pageCount; i++ {
		currentVaddr := vaddr + uint32(i*MMU_PAGE_SIZE)
		currentPaddr := paddr + uint32(i*MMU_PAGE_SIZE)

		var entryIndex uint32

		// Calculate MMU entry index based on virtual address space
		if currentVaddr >= DROM_VADDR_START && currentVaddr < DROM_VADDR_START+32*1024*1024 {
			// DROM space: 0x3C000000-0x3E000000
			entryIndex = (currentVaddr - DROM_VADDR_START) / MMU_PAGE_SIZE
		} else if currentVaddr >= IROM_VADDR_START && currentVaddr < IROM_VADDR_START+32*1024*1024 {
			// IROM space: 0x42000000-0x44000000
			// IROM entries start after DROM entries in the MMU table
			entryIndex = ((currentVaddr - IROM_VADDR_START) / MMU_PAGE_SIZE) + 256
		} else {
			continue // Skip invalid virtual addresses
		}

		if entryIndex >= MMU_ENTRY_COUNT {
			continue // Skip invalid entries
		}

		// Calculate physical page number (ESP32-S3 flash mapping)
		physPageNum := currentPaddr / MMU_PAGE_SIZE

		// Set MMU entry: physical page number with valid bit
		mmuTable[entryIndex] = physPageNum | MMU_VALID_BIT
	}
}

// Debug functions sorted by GPIO number (ascending: 4→5→6→7)
func debugGPIO(n int) {
	*(*uint32)(unsafe.Pointer(uintptr(0x60004024))) |= (1 << n) // GPIO_ENABLE_REG: enable GPIO4 output
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << n)  // GPIO_OUT_W1TS_REG: set GPIO4 high
}

// This is the function called on startup after the flash (IROM/DROM) is
// initialized and the stack pointer has been set.
//
// In this self-booting implementation, we bypass the ESP-IDF bootloader entirely.
// This function acts as both bootloader and application entry point.
//
//export main
func main() {
	// === BOOTLOADER PHASE ===
	// Initialize cache and MMU to enable access to flash memory
	// This replaces the functionality normally provided by ESP-IDF bootloader
	// Based on ESP-IDF bootloader_utility.c:set_cache_and_start_app()

	debugGPIO(5)
	// === APPLICATION PHASE ===
	// This initialization configures the following things:
	// * It disables all watchdog timers. They might be useful at some point in
	//   the future, but will need integration into the scheduler. For now,
	//   they're all disabled.
	// * It sets the CPU frequency to 160MHz, which is the maximum speed allowed
	//   for this CPU. Lower frequencies might be possible in the future, but
	//   running fast and sleeping quickly is often also a good strategy to save
	//   power.
	// TODO: protect certain memory regions, especially the area below the stack
	// to protect against stack overflows. See
	// esp_cpu_configure_region_protection in ESP-IDF.

	// Disable Timer 0 watchdog.
	esp.TIMG0.WDTCONFIG0.Set(0)

	// Disable RTC watchdog.
	esp.RTC_CNTL.WDTWPROTECT.Set(0x50D83AA1)
	esp.RTC_CNTL.WDTCONFIG0.Set(0)

	// Disable super watchdog.
	esp.RTC_CNTL.SWD_WPROTECT.Set(0x8F1D312A)
	esp.RTC_CNTL.SWD_CONF.Set(esp.RTC_CNTL_SWD_CONF_SWD_DISABLE)

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

	// Initialize GPIO and SPI peripherals early (GPIO matrix might be already initialized by ROM)
	initGPIOPeripherals()
	initSPIPeripherals()

	// Initialize UART after USB configuration
	machine.USBCDC.Configure(machine.UARTConfig{BaudRate: 115200})
	machine.InitSerial()

	initTimer()

	for i := 0; i < 10000; i++ {
		print(".")
	}
	print("\n")

	// TEST: Generate 50kHz signal on GPIO36 for debugging
	// testGPIO36_50kHz()

	// Debug: vector base target (don't read VECBASE here to avoid traps)
	println("VEC: _vector_table=", uintptr(unsafe.Pointer(&_vector_table)), " anchor=", uintptr(unsafe.Pointer(&_tinygo_vectors_present)))

	// Re-enable vector table override but be more careful
	enableVecbaseOverride()
	println("VEC: Override re-enabled")

	// Make sure cache is enabled
	if !isCacheEnabled() {
		println("Cache disabled, re-enabling...")
		enableCache()
	} else {
		println("Cache is enabled")
	}

	// Dump vector table for debugging
	dumpVectorTableRaw()
	println("VEC: OV0.MASK=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_0_CORE_0_VECBASE_WORLD_MASK())
	println("VEC: OV1.W0=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_1_CORE_0_VECBASE_OVERRIDE_WORLD0_VALUE())
	println("VEC: OV1.SEL=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_1_CORE_0_VECBASE_OVERRIDE_SEL())
	println("VEC: LOCK=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_LOCK())
	// println("VEC: OV0.MASK=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_0_CORE_0_VECBASE_WORLD_MASK())
	// println("VEC: OV1.W0=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_1_CORE_0_VECBASE_OVERRIDE_WORLD0_VALUE())
	// println("VEC: OV1.SEL=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_1_CORE_0_VECBASE_OVERRIDE_SEL())
	// println("VEC: LOCK=", esp.SENSITIVE.GetCORE_0_VECBASE_OVERRIDE_LOCK())

	// Extra diagnostics: clear GPIO pending only (avoid get_ps/get_interrupt until vectors verified)
	esp.GPIO.SetSTATUS_W1TC(0xFFFFFFFF)
	esp.GPIO.SetSTATUS1_W1TC(0x3FFFFF)

	// Тестируем debugMark из Go-кода
	println("Testing debugMark from Go...")
	debugMark(0x12345678)
	println("debugMark test completed")
	println("debugMark call count so far:", debugMarkCallCount)

	// Hold here to observe stability before entering run()
	run()

	// Fallback: if main ever returns, hang the CPU.
	exit(0)
}

// enableVecbaseOverride sets CORE_0_VECBASE_OVERRIDE_* to point to `_vector_table` in RAM.
// It does not lock or enable for world1. World mask enables world0 only.
func enableVecbaseOverride() {
	// Compute address >> 2 as required by hardware encoding
	vecbaseAddr := (uint32(uintptr(unsafe.Pointer(&_vector_table))) >> 2) & 0x3fffff
	// Program override via SENSITIVE registers directly (SEL=0b01, world0 only)
	esp.SENSITIVE.SetCORE_0_VECBASE_OVERRIDE_0_CORE_0_VECBASE_WORLD_MASK(1)
	esp.SENSITIVE.SetCORE_0_VECBASE_OVERRIDE_1_CORE_0_VECBASE_OVERRIDE_WORLD0_VALUE(vecbaseAddr)
	esp.SENSITIVE.SetCORE_0_VECBASE_OVERRIDE_1_CORE_0_VECBASE_OVERRIDE_SEL(0b01)
}

// dumpVectorTable prints a small hexdump of vector slots at key offsets.
func dumpVectorTable() {
	base := uintptr(unsafe.Pointer(&_vector_table))
	offsets := []uint32{0x0, 0x40, 0x80, 0xC0, 0x100, 0x140, 0x180, 0x1C0, 0x200, 0x240, 0x280, 0x2C0, 0x300, 0x340, 0x3C0}
	names := []string{"WinOV4", "WinUF4", "WinOV8", "WinUF8", "WinOV12", "WinUF12", "Lvl2", "Lvl3", "Lvl4", "Lvl5", "Lvl6", "NMI", "Kernel", "User", "Double"}
	println("VEC DUMP: base=", base)
	for i := 0; i < len(offsets); i++ {
		off := uintptr(offsets[i])
		a0 := *(*uint32)(unsafe.Pointer(base + off + 0))
		a1 := *(*uint32)(unsafe.Pointer(base + off + 4))
		a2 := *(*uint32)(unsafe.Pointer(base + off + 8))
		a3 := *(*uint32)(unsafe.Pointer(base + off + 12))
		println("  ", names[i], "@+", offsets[i], ":", a0, a1, a2, a3)
	}
}

// dumpVectorTableRaw prints fixed slots without using slices/loops to avoid runtime allocations.
func dumpVectorTableRaw() {
	baseIRAM := uintptr(unsafe.Pointer(&_vector_table))
	// helpers (read directly from IRAM)
	printSlot := func(label string, off uintptr) {
		addr := baseIRAM + off
		a0 := *(*uint32)(unsafe.Pointer(addr + 0))
		a1 := *(*uint32)(unsafe.Pointer(addr + 4))
		a2 := *(*uint32)(unsafe.Pointer(addr + 8))
		a3 := *(*uint32)(unsafe.Pointer(addr + 12))
		println("  ", label, "@+ 0x", off, ":", a0, a1, a2, a3)
	}
	println("VEC DUMP: baseIRAM=", baseIRAM)
	printSlot("WinOV4", 0x0)
	printSlot("WinUF4", 0x40)
	printSlot("WinOV8", 0x80)
	printSlot("WinUF8", 0xC0)
	printSlot("WinOV12", 0x100)
	printSlot("WinUF12", 0x140)
	printSlot("Lvl2", 0x180)
	printSlot("Lvl3", 0x1C0)
	printSlot("Lvl4", 0x200)
	printSlot("Lvl5", 0x240)
	printSlot("Lvl6", 0x280)
	printSlot("NMI", 0x2C0)
	printSlot("Kernel", 0x300)
	printSlot("User", 0x340)
	printSlot("Double", 0x3C0)
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

// Глобальный счетчик вызовов debugMark
var debugMarkCallCount uint32

//export debugMark
func debugMark(v uint32) {
	debugMarkCallCount++

	// Используем GPIO для индикации вызова функции
	debugGPIO(4) // Включаем GPIO4 для индикации

	// Попробуем вывести сообщение (может не работать если UART не инициализирован)
	println("SWAPDBG:", v, "call#", debugMarkCallCount)

	// Дополнительная индикация через GPIO в зависимости от значения
	if v == 0xDEADBEEF {
		debugGPIO(6) // GPIO6 для tinygo_startTask
	} else if v == 0x1234 { // SWAP entry marker
		debugGPIO(5) // GPIO5 для tinygo_swapTask entry
	} else if v == 0x5678 { // SWAP completion marker
		debugGPIO(7) // GPIO7 для tinygo_swapTask completion
	} else if v == 0xEEEEFF00 {
		debugGPIO(8) // GPIO8 для context_save
	} else if v >= 0xEEEE0000 && v <= 0xEEEE1111 {
		debugGPIO(9) // GPIO9 для context_restore
	}
}

//export debugDumpUnderflowFrame
func debugDumpUnderflowFrame(sp uintptr) {
	base := sp
	w0 := *(*uint32)(unsafe.Pointer(base - 16))
	w1 := *(*uint32)(unsafe.Pointer(base - 12))
	w2 := *(*uint32)(unsafe.Pointer(base - 8))
	w3 := *(*uint32)(unsafe.Pointer(base - 4))
	println("SWAPUF:", base, w0, w1, w2, w3)
}

//export debugSwapArgs
func debugSwapArgs(newSp uintptr, oldSpPtr uintptr) {
	println("SWAPARGS:", newSp, oldSpPtr)
}

//go:export tinygo_xt_int_enter
func tinygo_xt_int_enter() {
}

//go:export tinygo_xt_int_exit
func tinygo_xt_int_exit() {
}

//go:export tinygo_xt_timer_int
func tinygo_xt_timer_int() {
	// TODO: invoke scheduler tick
}

//go:extern _vector_table
var _vector_table [0]uintptr

//go:extern get_vecbase
func get_vecbase() uint32

//go:extern disableAllInterrupts
func disableAllInterrupts()

//go:extern get_ps
func get_ps() uint32

//go:extern get_interrupt
func get_interrupt() uint32

//go:extern enableVecbaseOverrideAsm
func enableVecbaseOverrideAsm(vecbaseShifted uint32)

//go:extern _tinygo_vectors_present
var _tinygo_vectors_present [0]byte

//go:extern port_xSchedulerRunning
var port_xSchedulerRunning uint32

//go:extern port_interruptNesting
var port_interruptNesting uint32

//go:extern port_switch_flag
var port_switch_flag uint32

//go:extern _xt_tick_divisor
var _xt_tick_divisor uint32
