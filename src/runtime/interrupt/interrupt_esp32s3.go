//go:build esp32s3

package interrupt

import (
	"device"
	"errors"
)

// readINTENABLE returns current CPU interrupt enable mask.
func readINTENABLE() uint32 {
	v := device.AsmFull("rsr.intenable {}", nil)
	return uint32(v)
}

// writeINTENABLE writes CPU interrupt enable mask and rsyncs.
func writeINTENABLE(mask uint32) {
	device.AsmFull("wsr {v}, INTENABLE", map[string]interface{}{"v": uintptr(mask)})
	device.AsmFull("rsync", nil)
}

// Enable enables this interrupt for ESP32-S3.
// For ESP32-S3, we enable interrupts globally at the Xtensa level.
func (i Interrupt) Enable() error {
	if i.num < 1 || i.num > 31 {
		return errors.New("interrupt for ESP32-S3 must be in range of 1 through 31")
	}

	// Disable interrupts temporarily to avoid race conditions
	mask := Disable()
	defer Restore(mask)

	// Read current PS register value
	psValue := uintptr(device.AsmFull("rsr.ps {}", nil))

	// Clear INTLEVEL bits [3:0] to 0 to allow all interrupts
	// PS_INTLEVEL_MASK is 0x0000000F
	const PS_INTLEVEL_MASK = 0x0F
	psValue = psValue & ^uintptr(PS_INTLEVEL_MASK)

	// Write modified PS register back
	device.AsmFull("wsr {ps}, PS", map[string]interface{}{
		"ps": psValue,
	})
	device.AsmFull("rsync", nil)

	// Enable this CPU interrupt line in INTENABLE
	m := readINTENABLE()
	m |= (1 << uint(i.num))
	writeINTENABLE(m)

	return nil
}

// Disable disables this interrupt.
func (i Interrupt) Disable() {
	if i.num < 1 || i.num > 31 {
		return
	}
	// Clear this CPU interrupt line in INTENABLE
	m := readINTENABLE()
	m &^= (1 << uint(i.num))
	writeINTENABLE(m)
}

// SetPriority sets the interrupt priority for this interrupt.
// For Xtensa, priority is managed through interrupt levels.
func (i Interrupt) SetPriority(priority uint8) {
	// Priority management is done through INTLEVEL in PS register
	// This is a placeholder for future implementation
}
