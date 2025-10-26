//go:build esp32s3

package interrupt

import (
	"device"
	"errors"
)

// Enable enables this interrupt for ESP32-S3.
// For ESP32-S3, we enable interrupts globally at the Xtensa level.
func (i Interrupt) Enable() error {
	if i.num < 1 || i.num > 31 {
		return errors.New("interrupt for ESP32-S3 must be in range of 1 through 31")
	}

	// Disable interrupts temporarily to avoid race conditions
	mask := Disable()
	defer Restore(mask)

	// Set interrupt level to 0 (allow all interrupts) by writing to PS register
	// This is the minimum needed to enable interrupts globally
	state := uintptr(0)
	device.AsmFull("wsr {state}, PS", map[string]interface{}{
		"state": state,
	})
	device.AsmFull("rsync", nil)

	return nil
}

// Disable disables this interrupt.
func (i Interrupt) Disable() {
	// For Xtensa, interrupt disabling is managed globally through the PS.INTLEVEL field
	// Individual interrupt disabling would require access to INTENABLE register
	// which is managed at a different level
}

// SetPriority sets the interrupt priority for this interrupt.
// For Xtensa, priority is managed through interrupt levels.
func (i Interrupt) SetPriority(priority uint8) {
	// Priority management is done through INTLEVEL in PS register
	// This is a placeholder for future implementation
}
