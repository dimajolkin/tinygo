//go:build esp32s3

package runtime

import (
	"device/esp"
	"machine"
)

// This is the function called on startup after the flash (IROM/DROM) is
// initialized and the stack pointer has been set.
//
//export main
func main() {
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

	// Disable RTC watchdog.
	esp.RTC_CNTL.SetWDTWPROTECT(0x50D83AA1)
	esp.RTC_CNTL.SetWDTCONFIG0_WDT_EN(0)
	esp.RTC_CNTL.SetWDTWPROTECT(0x0) // Re-enable write protect

	// Disable Timer 0 watchdog.
	esp.TIMG1.WDTWPROTECT.Set(0x50D83AA1) // write protect
	esp.TIMG1.WDTCONFIG0.Set(0)           // disable TG0 WDT
	esp.TIMG1.WDTWPROTECT.Set(0x0)        // Re-enable write protect

	esp.TIMG0.WDTWPROTECT.Set(0x50D83AA1) // write protect
	esp.TIMG0.WDTCONFIG0.Set(0)           // disable TG0 WDT
	esp.TIMG0.WDTWPROTECT.Set(0x0)        // Re-enable write protect

	// Disable super watchdog.
	esp.RTC_CNTL.SetSWD_WPROTECT(0x8F1D312A)
	esp.RTC_CNTL.SetSWD_CONF_SWD_DISABLE(1)
	esp.RTC_CNTL.SetSWD_WPROTECT(0x0) // Re-enable write protect

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

	// Initialize UART for println/debug output.
	// This is critical for ESP32S3 to see any output
	machine.InitSerial()

	// DEBUG: Add early runtime debug output
	print("TinyGo ESP32-S3 runtime started\n")

	// Initialize main system timer used for time.Now.
	print("Initializing timer...\n")
	initTimer()

	print("Timer initialized, calling run()...\n")
	// Initialize the heap, call main.main, etc.
	run()

	// Fallback: if main ever returns, hang the CPU.
	print("main.main() returned, exiting...\n")
	exit(0)
}

func abort() {
	// lock up forever
	print("ABORT: TinyGo runtime abort() called - hanging CPU\n")
	for {
		// infinite loop to hang CPU
	}
}

//go:extern _vector_table
var _vector_table [0]uintptr

//go:extern _sbss
var _sbss [0]byte

//go:extern _ebss
var _ebss [0]byte

// ESP App Descriptor structure matching ESP-IDF esp_app_desc_t
// This must be placed in .rodata_desc section for bootloader compatibility
type espAppDesc struct {
	magic_word              uint32     // ESP_APP_DESC_MAGIC_WORD (0xABCD5432)
	secure_version          uint32     // Secure version
	reserv1                 [2]uint32  // reserv1
	version                 [32]byte   // Application version
	project_name            [32]byte   // Project name  
	time                    [16]byte   // Compile time
	date                    [16]byte   // Compile date
	idf_ver                 [32]byte   // Version IDF
	app_elf_sha256          [32]byte   // sha256 of elf file
	min_efuse_blk_rev_full  uint16     // Minimal eFuse block revision supported by image
	max_efuse_blk_rev_full  uint16     // Maximal eFuse block revision supported by image  
	mmu_page_size           uint8      // MMU page size in log base 2 format
	reserv3                 [3]uint8   // reserv3
	reserv2                 [18]uint32 // reserv2
}

// ESP App Descriptor instance - must be in .rodata_desc section
//
//go:section .rodata_desc
var esp_app_desc = espAppDesc{
	magic_word:              0xABCD5432, // ESP_APP_DESC_MAGIC_WORD
	secure_version:          0,
	reserv1:                 [2]uint32{0, 0},
	version:                 [32]byte{'1', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	project_name:            [32]byte{'T', 'i', 'n', 'y', 'G', 'o', ' ', 'A', 'p', 'p', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	time:                    [16]byte{'0', '0', ':', '0', '0', ':', '0', '0', 0, 0, 0, 0, 0, 0, 0, 0},
	date:                    [16]byte{'J', 'a', 'n', ' ', ' ', '1', ' ', '2', '0', '2', '4', 0, 0, 0, 0, 0},
	idf_ver:                 [32]byte{'T', 'i', 'n', 'y', 'G', 'o', '-', 'C', 'o', 'm', 'p', 'a', 't', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	app_elf_sha256:          [32]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	min_efuse_blk_rev_full:  0x0000, // Support all ESP32S3 revisions
	max_efuse_blk_rev_full:  0x0000, // No max limit
	mmu_page_size:           31 - 13, // 8KB page size (1 << 13) -> log2(8192) = 13, so 31-13=18 but ESP uses different calc
	reserv3:                 [3]uint8{0, 0, 0},
	reserv2:                 [18]uint32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
}

// Force the app descriptor to be included by referencing it
func init() {
	// This ensures esp_app_desc is not eliminated by the linker
	// We actually read from it to make it truly used
	if esp_app_desc.magic_word == 0 {
		// This will never execute but forces the linker to keep the symbol
		abort()
	}
}
