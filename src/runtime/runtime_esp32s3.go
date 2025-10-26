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

// Debug functions sorted by GPIO number (ascending: 4→5→6→7)
func debugGPIO(n int) {
	*(*uint32)(unsafe.Pointer(uintptr(0x60004024))) |= (1 << n) // GPIO_ENABLE_REG: enable GPIO4 output
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << n)  // GPIO_OUT_W1TS_REG: set GPIO4 high
}

//export main
func main() {
	// Disable Timer Group watchdogs (unlock then disable)
	// TIMG0
	esp.TIMG0.WDTWPROTECT.Set(0x50D83AA1)
	esp.TIMG0.WDTCONFIG0.Set(0)
	// TIMG1
	esp.TIMG1.WDTWPROTECT.Set(0x50D83AA1)
	esp.TIMG1.WDTCONFIG0.Set(0)

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

	// Initialize system tick using SYSTIMER (10ms period)
	initSystimerTick()

	// Configure GPIO41 as debug output (toggled by SYSTIMER ISR)
	initDebugPin41()

	// Force PS.INTLEVEL = 0 to allow IRQs and dump state
	setPSIntLevel(0)
	dumpSystimerDebug("after initSystimerTick")

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

	debugGPIO(7) // Indicate GPIO peripherals initialized
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

	debugGPIO(6) // Indicate SPI peripherals initialized
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

// setPSIntLevel sets PS.INTLEVEL to the given level (0..15)
func setPSIntLevel(level int) {
	oldPs := device.AsmFull("rsr.ps {}", nil)
	ie := device.AsmFull("rsr.intenable {}", nil)
	intr := device.AsmFull("rsr.interrupt {}", nil)
	println("DBG setPSIntLevel: oldPS=", uint32(uintptr(oldPs)&0xFFFF), " level=", level, " INTENABLE=", uint32(uintptr(ie)), " INTERRUPT=", uint32(uintptr(intr)))

	ps := oldPs
	ps &^= 0x0F
	ps |= uintptr(level & 0x0F)
	device.AsmFull("wsr.ps {v}", map[string]interface{}{"v": ps})
	device.AsmFull("rsync", nil)

	newPs := device.AsmFull("rsr.ps {}", nil)
	println("DBG setPSIntLevel: newPS=", uint32(uintptr(newPs)&0xFFFF))
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
	now := esp.SYSTIMER.UNIT0_VALUE_LO.Get()
	tgt := esp.SYSTIMER.TARGET0_LO.Get()
	nowHi := esp.SYSTIMER.UNIT0_VALUE_HI.Get()
	tgtHi := esp.SYSTIMER.REAL_TARGET0_HI.Get()
	println("-- SYSTIMER DEBUG (", tag, ") --")
	println("MAP=", mapVal, " INTENABLE=", uint32(intEna), " PS=", uint32(uintptr(ps)&0x0F))
	println("SYSTIMER: INT_ST=", st, " INT_ENA=", ena, " CONF=", conf, " T0CONF=", t0conf)
	println("UNIT0_HI=", nowHi, " UNIT0_LO=", now, " TARGET0_HI=", tgtHi, " TARGET0_LO=", tgt)
}

// initSystimerTick configures SYSTIMER TARGET0 to generate periodic interrupts every 10ms
// and routes it to a CPU interrupt channel via the Interrupt Matrix.
func initSystimerTick() {
	const systimerClockHz = 80_000_000 // assumed SYSTIMER clock
	const tickPeriodNs = 10_000_000    // 10ms
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

	// Configure periodic mode on UNIT1 (commonly used for alarms) and set period
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

	// Program first shot: latch UNIT0, set TARGET0 = now + period, load, enable INT
	esp.SYSTIMER.SetUNIT0_OP_TIMER_UNIT0_UPDATE(1)
	// read latched now (low then hi)
	nowLo := esp.SYSTIMER.UNIT0_VALUE_LO.Get()
	nowHi := esp.SYSTIMER.UNIT0_VALUE_HI.Get()
	// compute target = now + periodTicks
	tgtLo := nowLo + periodTicks
	tgtHi := nowHi
	if tgtLo < nowLo {
		tgtHi++
	}
	esp.SYSTIMER.SetTARGET0_LO(tgtLo)
	esp.SYSTIMER.SetREAL_TARGET0_HI_TARGET0_HI_RO(tgtHi & 0xFFFFF)
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
