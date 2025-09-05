//go:build esp32s3

// ESP32-S3 runtime implementation
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

package runtime

import (
	"device"
	"device/esp"
	"machine"
	"unsafe"
)

// Note: heapStart, heapEnd, and growHeap are defined in baremetal.go
// which is automatically included for ESP32-S3 targets

// Debug functions sorted by GPIO number (ascending: 4→5→6→7)
func debugGPIO(n int) {
	*(*uint32)(unsafe.Pointer(uintptr(0x60004024))) |= (1 << n) // GPIO_ENABLE_REG: enable GPIO4 output
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << n)  // GPIO_OUT_W1TS_REG: set GPIO4 high
}

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

	debugGPIO(5)

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
