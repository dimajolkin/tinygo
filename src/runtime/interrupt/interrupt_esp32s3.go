//go:build esp32s3

// Package interrupt provides interrupt handling for ESP32-S3 (Xtensa LX7)
//
// Based on ESP-IDF interrupt handling:
//   - components/xtensa/xtensa_vectors.S (dispatch logic)
//   - components/esp_hw_support/interrupt.c (interrupt allocation)
//
// Main differences from ESP-IDF:
//   - Uses TinyGo's interrupt.New() instead of esp_intr_alloc()
//   - Simplified PS register manipulation (no FPU/coprocessor state)
//   - Windowed ABI conventions (ESP32-S3)
package interrupt

import (
	"device"
	"errors"
	"math/bits"
	"unsafe"
	_ "unsafe" // for go:linkname
)

// Assembly helper functions from interrupt_helpers_esp32s3.S
// Using direct inline assembly wrappers for reliability (bypasses go:linkname issues)

func read_interrupt() uint32 {
	result := device.AsmFull("rsr.interrupt {}", nil)
	return uint32(uintptr(result))
}

func read_intenable() uint32 {
	result := device.AsmFull("rsr.intenable {}", nil)
	return uint32(uintptr(result))
}

func write_intenable(v uint32) {
	device.AsmFull("wsr.intenable {val}; rsync", map[string]interface{}{"val": uintptr(v)})
}

func write_intset(v uint32) {
	device.AsmFull("wsr.intset {val}; rsync", map[string]interface{}{"val": uintptr(v)})
}

func write_intclear(v uint32) {
	device.AsmFull("wsr.intclear {val}; rsync", map[string]interface{}{"val": uintptr(v)})
}

func get_ps() uint32 {
	result := device.AsmFull("rsr.ps {}", nil)
	return uint32(uintptr(result))
}

func set_ps(v uint32) {
	device.AsmFull("wsr.ps {val}; rsync", map[string]interface{}{"val": uintptr(v)})
}

func rsil_0() uint32 {
	result := device.AsmFull("rsil {}, 0", nil)
	return uint32(uintptr(result))
}

func rsil_1() uint32 {
	result := device.AsmFull("rsil {}, 1", nil)
	return uint32(uintptr(result))
}

func rsil_15() uint32 {
	result := device.AsmFull("rsil {}, 15", nil)
	return uint32(uintptr(result))
}

func rsil_set(level uint32) uint32 {
	// Manual implementation using rsr/wsr (same as __tg_rsil_set)
	oldPS := get_ps()
	newPS := (oldPS &^ 0x0F) | (level & 0x0F)
	set_ps(newPS)
	return oldPS
}

// State holds a full snapshot of the PS register returned by RSIL.
// Only the INTLEVEL field (bits [3:0]) is used on Restore(), the rest of PS
// is preserved as-is by hardware and not modified by software.
// Using uint32 matches the XTensa PS register width.
type State uint32

// Enable enables this interrupt. Right after calling this function, the
// interrupt may be invoked if it was already pending.
func (i Interrupt) Enable() error {
	if i.num < 0 || i.num > 31 {
		return errors.New("interrupt number out of range [0-31]")
	}
	old := rsil_15() // mask IRQs on this CPU during RMW on INTENABLE
	mask := read_intenable()
	mask |= (1 << uint(i.num))
	write_intenable(mask)
	rsil_set(old & 0x0F) // restore previous INTLEVEL
	return nil
}

// SetPriority sets the interrupt priority for this interrupt.
// A lower number means a higher priority.
// Xtensa ESP32-S3 supports interrupt levels 1-7.
func (i Interrupt) SetPriority(priority uint8) error {
	// On Xtensa, priority is controlled by interrupt level (1-7)
	// and by CPU INTLEVEL in PS register.
	// This is a placeholder - actual implementation would need
	// to configure interrupt routing through Interrupt Matrix.
	return nil
}

// SetPSIntLevel sets PS.INTLEVEL to the given level (0..15)
// Level 0 = all interrupts enabled
// Level 15 = all interrupts masked
// WARNING: This is a low-level function. Prefer using Disable()/Restore()
func SetPSIntLevel(level int) {
	lvl := uint32(level & 0x0F)
	cur := get_ps()
	curLvl := cur & 0x0F
	if lvl == curLvl {
		return
	}

	if lvl > curLvl {
		// Raising mask: atomic RSIL when available
		switch lvl {
		case 1:
			rsil_1()
		case 15:
			rsil_15()
		default:
			rsil_set(lvl)
		}
		return
	}

	// Lowering mask (including to 0)
	if lvl == 0 {
		rsil_0()
		return
	}
	rsil_set(lvl)
}

// Disable disables all interrupts (sets PS.INTLEVEL=15) and returns the previous PS snapshot. It
// can be used in a critical section like this:
//
//	state := interrupt.Disable()
//	// critical section
//	interrupt.Restore(state)
//
// Critical sections can be nested. Make sure to call Restore in the same order
// as you called Disable (this happens naturally with the pattern above).
//
// SOURCE: Based on ESP-IDF portmacro.h portSET_INTERRUPT_MASK_FROM_ISR
// ESP-IDF: components/freertos/FreeRTOS-Kernel-SMP/portable/xtensa/include/freertos/portmacro.h
func Disable() (state State) {
	// Atomically set INTLEVEL=15 and get old PS value
	old := rsil_15()
	return State(old)
}

// Restore restores interrupts to what they were before. Give the previous state
// returned by Disable as a parameter. If interrupts were disabled before
// calling Disable, this will not re-enable interrupts, allowing for nested
// critical sections.
//
// This implementation ensures INTLEVEL is restored even if a direct PS write is ignored,
// by falling back to RSIL-based sequences.
//
//go:nosplit
func Restore(state State) {
	// Restore only the INTLEVEL field from the saved PS snapshot
	target := uint32(state) & 0x0F
	curPS := get_ps()
	cur := curPS & 0x0F

	if target == cur {
		return
	}

	if target > cur {
		// Raising mask: prefer RSIL immediates where possible
		switch target {
		case 1:
			rsil_1()
		case 15:
			rsil_15()
		default:
			rsil_set(target)
		}
		return
	}

	// Lowering mask
	if target == 0 {
		rsil_0()
		return
	}

	rsil_set(target)
	if (get_ps() & 0x0F) != target {
		// As a last resort, force 0 then set target
		rsil_0()
		if target != 0 {
			rsil_set(target)
		}
	}
}

// In returns whether the system is currently in an interrupt.
// On Xtensa, we check if PS.EXCM bit is set (exception mode).
func In() bool {
	ps := get_ps()
	// EXCM is bit 4 of PS register
	return (ps & (1 << 4)) != 0
}

// Adding pseudo function calls that is replaced by the compiler with the actual
// functions registered through interrupt.New.
//

//go:linkname callHandlers runtime/interrupt.callHandlers
func callHandlers(num int)

// Debug counter for handleInterrupt calls
var handleInterruptCallCount uint32

// clearCpuInterrupt clears the CPU software/edge interrupt request bit.
// For external level-sensitive sources, the peripheral's own flag must be cleared instead.
func clearCpuInterrupt(intNum uint32) {
	// INTCLEAR is SR #227 (write-only)
	write_intclear(1 << intNum)
}

// pendingCPU returns the current pending CPU interrupts masked by INTENABLE.
func pendingCPU() uint32 {
	ien := read_intenable()
	req := read_interrupt()
	return ien & req
}

// Public helper functions for use in runtime code

// ReadInterrupt returns current pending interrupt mask (INTERRUPT register)
func ReadInterrupt() uint32 {
	return read_interrupt()
}

// ReadIntEnable returns current interrupt enable mask (INTENABLE register)
func ReadIntEnable() uint32 {
	return read_intenable()
}

// WriteIntEnable writes interrupt enable mask (INTENABLE register)
func WriteIntEnable(v uint32) {
	write_intenable(v)
}

// WriteIntSet sets interrupt request bits (INTSET register)
func WriteIntSet(v uint32) {
	write_intset(v)
}

// WriteIntClear clears interrupt request bits (INTCLEAR register)
func WriteIntClear(v uint32) {
	write_intclear(v)
}

// GetPS returns current processor status (PS register)
func GetPS() uint32 {
	return get_ps()
}

// SetPS writes processor status (PS register)
func SetPS(v uint32) {
	set_ps(v)
}

// --- SYSTIMER/INTERRUPT register access checks for ESP32-S3 ---
// Inserted for hardware debug/validation in runtime_esp32s3.go

// handleInterrupt - главный диспетчер прерываний для ESP32-S3 (ESP-IDF style)
// Вызывается напрямую из ассемблерного Level-1 вектора (_xt_lowint1)
//
// NO PARAMETERS: диспетчер сам читает INTERRUPT & INTENABLE и обрабатывает все pending биты
//
// NOTE:
//   - The CPU *request* bit is NOT automatically cleared by hardware when taking the interrupt.
//   - Handler MUST deassert the source: for level-sensitive sources — clear peripheral flag; for edge/software — write INTCLEAR for the CPU line.
//
// Example usage:
//
//	package main
//
//	import (
//		"device/esp"
//		"machine/interrupt"
//	)
//
//	func onTimerInterrupt(interrupt.Interrupt) {
//		// Clear the peripheral interrupt source (level-sensitive)
//		esp.SYSTIMER.INT_CLR.Set(1 << 0)
//		// Perform periodic task
//	}
//
//	func main() {
//		// Initialize SYSTIMER or peripheral
//		// Configure and enable interrupt line 23 (SYSTIMER)
//		intr := interrupt.New(23, onTimerInterrupt)
//		intr.Enable()
//		// Start timer and enable target 0 compare event
//		esp.SYSTIMER.SetCONF_TARGET0_WORK_EN(1)
//		for {
//			device.Asm("waiti 0") // Sleep until next interrupt
//		}
//	}
//
// This demonstrates a level-sensitive interrupt (SYSTIMER). The handler must
// clear its peripheral flag to avoid retriggering. For edge/software interrupts,
// call clearCpuInterrupt(n) instead.
//
//export handleInterrupt
func handleInterrupt() {
	// CRITICAL DEBUG: Signal entry into ISR
	// GPIO 4 HIGH = entered ISR
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << 4) // GPIO_OUT_W1TS_REG: set GPIO4

	// Increment counter
	handleInterruptCallCount++

	// Generic Level-1 dispatcher per ISA: service all currently pending & enabled bits.
	// We recompute the mask each iteration to catch new arrivals during servicing.
	for tries := 0; tries < 8; tries++ { // simple bound to avoid livelock in case of flapping sources
		pend := pendingCPU()
		if pend == 0 {
			break
		}
		// Find lowest set bit using bits.TrailingZeros32
		bit := uint32(bits.TrailingZeros32(pend))
		if bit < 32 {
			// Call registered handler; peripheral handler must clear its own flag
			// For SW/edge sources, handler may call clearCpuInterrupt(bit)
			callHandler(int(bit))
		} else {
			break
		}
	}

	// GPIO 4 LOW = exiting ISR
	*(*uint32)(unsafe.Pointer(uintptr(0x60004014))) = (1 << 4) // GPIO_OUT_W1TC_REG: clear GPIO4
}

//export handleException
func handleException(exccause, excvaddr, epc uint32) {
	// Handle fatal exceptions
	// This should never return
	print("FATAL EXCEPTION!\n")
	print("EXCCAUSE: ")
	printHex32(exccause)
	print("\nEXCVADDR: ")
	printHex32(excvaddr)
	print("\nEPC: ")
	printHex32(epc)
	print("\n")

	// Halt forever
	for {
		device.Asm("waiti 0")
	}
}

func printHex32(val uint32) {
	const hexChars = "0123456789abcdef"
	print("0x")
	for i := 7; i >= 0; i-- {
		nibble := byte((val >> (uint(i) * 4)) & 0xF)
		if nibble < 10 {
			print(string('0' + nibble))
		} else {
			print(string('a' + (nibble - 10)))
		}
	}
}

//go:inline
func callHandler(n int) {
	switch n {
	case 0:
		callHandlers(0)
	case 1:
		callHandlers(1)
	case 2:
		callHandlers(2)
	case 3:
		callHandlers(3)
	case 4:
		callHandlers(4)
	case 5:
		callHandlers(5)
	case 6:
		callHandlers(6)
	case 7:
		callHandlers(7)
	case 8:
		callHandlers(8)
	case 9:
		callHandlers(9)
	case 10:
		callHandlers(10)
	case 11:
		callHandlers(11)
	case 12:
		callHandlers(12)
	case 13:
		callHandlers(13)
	case 14:
		callHandlers(14)
	case 15:
		callHandlers(15)
	case 16:
		callHandlers(16)
	case 17:
		callHandlers(17)
	case 18:
		callHandlers(18)
	case 19:
		callHandlers(19)
	case 20:
		callHandlers(20)
	case 21:
		callHandlers(21)
	case 22:
		callHandlers(22)
	case 23:
		callHandlers(23)
	case 24:
		callHandlers(24)
	case 25:
		callHandlers(25)
	case 26:
		callHandlers(26)
	case 27:
		callHandlers(27)
	case 28:
		callHandlers(28)
	case 29:
		callHandlers(29)
	case 30:
		callHandlers(30)
	case 31:
		callHandlers(31)
	default:
		// unknown line, ignore
	}
}

// GetHandleInterruptCallCount returns the number of times handleInterrupt was called.
// Used for debugging interrupt system.
func GetHandleInterruptCallCount() uint32 {
	return handleInterruptCallCount
}
