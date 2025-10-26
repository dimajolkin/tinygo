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

	println("DEBUG: Enabling interrupt", i.num)

	// Disable interrupts temporarily to avoid race conditions
	mask := Disable()
	defer Restore(mask)

	// Read current PS register value
	println("DEBUG: Reading current PS register...")
	psValue := uintptr(device.AsmFull("rsr.ps {}", nil))
	println("DEBUG: Current PS value =", psValue)

	// Clear INTLEVEL bits [3:0] to 0 to allow all interrupts
	// PS_INTLEVEL_MASK is 0x0000000F
	const PS_INTLEVEL_MASK = 0x0F
	psValue = psValue & ^uintptr(PS_INTLEVEL_MASK)

	println("DEBUG: New PS value (INTLEVEL=0) =", psValue)

	// Write modified PS register back
	device.AsmFull("wsr {ps}, PS", map[string]interface{}{
		"ps": psValue,
	})
	device.AsmFull("rsync", nil)

	println("DEBUG: Interrupt", i.num, "enabled successfully")
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
