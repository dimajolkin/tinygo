//go:build esp32s3

package interrupt

import (
	"device"
	"errors"
)

// State represents the previous INTLEVEL value (bits [3:0] of PS register on Xtensa).
// We store only INTLEVEL, not the entire PS register, to avoid clobbering other PS bits
// that may have changed between Disable() and Restore() (like CALLINC, WOE, etc.)
type State uintptr

// Disable disables all interrupts and returns the previous interrupt state. It
// can be used in a critical section like this:
//
//	state := interrupt.Disable()
//	// critical section
//	interrupt.Restore(state)
//
// Critical sections can be nested. Make sure to call Restore in the same order
// as you called Disable (this happens naturally with the pattern above).
func Disable() (state State) {
	// Use RSIL instruction: atomically read PS and set INTLEVEL=15
	// This is equivalent to ESP-IDF's XTOS_SET_INTLEVEL(XCHAL_EXCM_LEVEL):
	//   __asm__ __volatile__("rsil %0, 15\n" : "=a" (__tmp) : : "memory");
	// RSIL reads old PS into result register and sets PS.INTLEVEL to immediate value
	ps := device.AsmFull("rsil {}, 15", nil)
	
	// Extract and return only the INTLEVEL field (bits [3:0])
	// This matches ESP-IDF's portSET_INTERRUPT_MASK() behavior:
	//   prev_level = ((prev_level >> SHIFT) & MASK);
	intlevel := (uintptr(ps) & 0x0F)
	return State(intlevel)
}

// Restore restores interrupts to what they were before. Give the previous state
// returned by Disable as a parameter. If interrupts were disabled before
// calling Disable, this will not re-enable interrupts, allowing for nested
// critical sections.
func Restore(state State) {
	print("R1 ")  // DEBUG: Entered Restore
	
	// Read CURRENT PS register (it may have changed since Disable!)
	currentPS := device.AsmFull("rsr.ps {}", nil)
	print("R2 ")  // DEBUG: Read current PS
	
	// Modify only the INTLEVEL field (bits [3:0]), preserve all other bits
	// This matches ESP-IDF's portCLEAR_INTERRUPT_MASK() behavior
	newPS := (uintptr(currentPS) &^ 0x0F) | (uintptr(state) & 0x0F)
	print("R3 ")  // DEBUG: Calculated new PS
	
	// Write back the modified PS register
	device.AsmFull("wsr.ps {v}", map[string]interface{}{
		"v": newPS,
	})
	print("R4 ")  // DEBUG: Wrote PS
	
	device.AsmFull("rsync", nil)
	print("R5\n")  // DEBUG: Done rsync
}

// In returns whether the system is currently in an interrupt.
// On Xtensa, we check if PS.EXCM bit is set (exception mode).
func In() bool {
	ps := device.AsmFull("rsr.ps {}", nil)
	// EXCM is bit 4 of PS register
	return (uintptr(ps) & (1 << 4)) != 0
}

// Adding pseudo function calls that is replaced by the compiler with the actual
// functions registered through interrupt.New.
//
//go:linkname callHandlers runtime/interrupt.callHandlers
func callHandlers(num int)

// Debug counter for handleInterrupt calls
var handleInterruptCallCount uint32

// handleInterrupt - главный диспетчер прерываний для ESP32-S3
// Вызывается из ассемблерного кода _xt_level1_int_handler_entry
//
//export handleInterrupt
func handleInterrupt() {
	// Increment debug counter (ISR-safe)
	handleInterruptCallCount++

	// Read INTERRUPT register to see which CPU interrupt line triggered
	interruptReg := device.AsmFull("rsr.interrupt {}", nil)
	interruptMask := uint32(uintptr(interruptReg))

	// Find which interrupt line is active
	// ESP32-S3 has 32 interrupt lines (0-31)
	for i := uint32(0); i < 32; i++ {
		if interruptMask&(1<<i) != 0 {
			// Clear this CPU interrupt (write-1-to-clear via INTCLEAR register)
			device.AsmFull("wsr.intclear {v}", map[string]interface{}{
				"v": uintptr(1 << i),
			})
			device.AsmFull("rsync", nil)

			// Call registered handler for this interrupt line
			callHandler(int(i))
			break // Handle only one interrupt at a time
		}
	}
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
	// ESP32-S3 supports 32 CPU interrupt lines
	// We need to dispatch to the appropriate handler
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
	}
}

// Enable enables this interrupt. Right after calling this function, the
// interrupt may be invoked if it was already pending.
func (i Interrupt) Enable() error {
	if i.num < 0 || i.num > 31 {
		return errors.New("interrupt number out of range [0-31]")
	}

	// Set INTENABLE bit for this interrupt line
	mask := device.AsmFull("rsr.intenable {}", nil)
	mask |= (1 << uint(i.num))
	device.AsmFull("wsr.intenable {v}", map[string]interface{}{
		"v": mask,
	})
	device.AsmFull("rsync", nil)

	return nil
}

// Disable disables this interrupt.
func (i Interrupt) Disable() error {
	if i.num < 0 || i.num > 31 {
		return errors.New("interrupt number out of range [0-31]")
	}

	// Clear INTENABLE bit for this interrupt line
	mask := device.AsmFull("rsr.intenable {}", nil)
	mask &^= (1 << uint(i.num))
	device.AsmFull("wsr.intenable {v}", map[string]interface{}{
		"v": mask,
	})
	device.AsmFull("rsync", nil)

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

// GetHandleInterruptCallCount returns the number of times handleInterrupt was called.
// Used for debugging interrupt system.
func GetHandleInterruptCallCount() uint32 {
	return handleInterruptCallCount
}
