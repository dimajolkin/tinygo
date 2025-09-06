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

// ROM cache function addresses - from ESP-IDF components/esp_rom/esp32s3/ld/esp32s3.rom.ld
const (
	ROM_Cache_Disable_ICache = 0x4000186c // uint32_t Cache_Disable_ICache(void)
	ROM_Cache_Disable_DCache = 0x40001884 // uint32_t Cache_Disable_DCache(void)
	ROM_Cache_Enable_ICache  = 0x40001878 // void Cache_Enable_ICache(uint32_t autoload)
	ROM_Cache_Enable_DCache  = 0x40001890 // void Cache_Enable_DCache(uint32_t autoload)
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
// Based on ESP-IDF components/bootloader_support/src/bootloader_utility.c:set_cache_and_start_app()
func initCacheAndMMU() {
	// Disable both caches before reconfiguration
	// Implementation based on ESP-IDF hal/esp32s3/include/hal/cache_ll.h:cache_ll_l1_disable_icache/dcache
	cacheDisableICache := *(*func() uint32)(unsafe.Pointer(uintptr(ROM_Cache_Disable_ICache)))
	cacheDisableDCache := *(*func() uint32)(unsafe.Pointer(uintptr(ROM_Cache_Disable_DCache)))

	icacheAutoload := cacheDisableICache()
	dcacheAutoload := cacheDisableDCache()

	// Reset MMU table - equivalent to mmu_hal_unmap_all()
	// Based on ESP-IDF components/hal/esp32s3/include/hal/mmu_ll.h
	resetMMUTable()

	// Configure DROM mapping (read-only data from flash)
	// Map flash address 0x0 to virtual address 0x3C000000 (DROM_LOW)
	// Based on ESP-IDF bootloader_utility.c:1065-1083
	mapDROM()

	// Configure IROM mapping (instruction cache from flash)
	// Map flash address 0x0 to virtual address 0x42000000 (IROM_LOW)
	// Based on ESP-IDF bootloader_utility.c:1085-1103
	mapIROM()

	// Re-enable caches with autoload settings
	// Implementation based on ESP-IDF hal/esp32s3/include/hal/cache_ll.h:cache_ll_l1_enable_icache/dcache
	cacheEnableICache := *(*func(uint32))(unsafe.Pointer(uintptr(ROM_Cache_Enable_ICache)))
	cacheEnableDCache := *(*func(uint32))(unsafe.Pointer(uintptr(ROM_Cache_Enable_DCache)))

	cacheEnableICache(icacheAutoload)
	cacheEnableDCache(dcacheAutoload)
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
		// Calculate MMU entry index from virtual address
		entryIndex := (vaddr + uint32(i*MMU_PAGE_SIZE) - DROM_VADDR_START) / MMU_PAGE_SIZE
		if entryIndex >= MMU_ENTRY_COUNT {
			continue // Skip invalid entries
		}

		// Calculate physical page number (ESP32-S3 flash mapping)
		physPageNum := (paddr + uint32(i*MMU_PAGE_SIZE)) / MMU_PAGE_SIZE

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
	// TODO: ROM bootloader may already initialize cache/MMU, test without this first
	// initCacheAndMMU()

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

	// Initialize UART after USB configuration
	machine.USBCDC.Configure(machine.UARTConfig{BaudRate: 115200})
	machine.InitSerial()

	initTimer()

	for i := 0; i < 10000; i++ {
		print(".")
	}
	print("\n")

	// Now use standard run() which will call initHeap() again but it should be safe
	run()

	// Fallback: if main ever returns, hang the CPU.
	exit(0)
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
