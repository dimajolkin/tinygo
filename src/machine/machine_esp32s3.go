//go:build esp32s3

package machine

import (
	"device/esp"
	"runtime/volatile"
	"unsafe"
)

const deviceName = esp.Device

const peripheralClock = 40_000000 // 80MHz

// GPIO Matrix output signal value for simple GPIO mode
const GPIO_FUNC_OUT_SEL_SIMPLE_GPIO = 256

// IO MUX function constants for ESP32-S3
const (
	IOMUX_FUNC_GPIO = 1 // GPIO mode (default)
	IOMUX_FUNC_UART = 2 // UART direct connection
	IOMUX_FUNC_I2C  = 3 // I2C direct connection
	IOMUX_FUNC_SPI  = 4 // SPI direct connection
)

// CPUFrequency returns the current CPU frequency of the chip.
// Currently it is a fixed frequency but it may allow changing in the future.
func CPUFrequency() uint32 {
	return 160e6 // 80 MHz
}

const (
	PinOutput PinMode = iota
	PinInput
	PinInputPullup
	PinInputPulldown
)

// Hardware pin numbers
const (
	GPIO0  Pin = 0
	GPIO1  Pin = 1
	GPIO2  Pin = 2
	GPIO3  Pin = 3
	GPIO4  Pin = 4
	GPIO5  Pin = 5
	GPIO6  Pin = 6
	GPIO7  Pin = 7
	GPIO8  Pin = 8
	GPIO9  Pin = 9
	GPIO10 Pin = 10
	GPIO11 Pin = 11
	GPIO12 Pin = 12
	GPIO13 Pin = 13
	GPIO14 Pin = 14
	GPIO15 Pin = 15
	GPIO16 Pin = 16
	GPIO17 Pin = 17
	GPIO18 Pin = 18
	GPIO19 Pin = 19
	GPIO21 Pin = 21
	GPIO22 Pin = 22
	GPIO23 Pin = 23
	GPIO25 Pin = 25
	GPIO26 Pin = 26
	GPIO27 Pin = 27
	GPIO32 Pin = 32
	GPIO33 Pin = 33
	GPIO34 Pin = 34
	GPIO35 Pin = 35
	GPIO36 Pin = 36
	GPIO37 Pin = 37
	GPIO38 Pin = 38
	GPIO39 Pin = 39
	GPIO40 Pin = 40
	GPIO41 Pin = 41
	GPIO42 Pin = 42
	GPIO43 Pin = 43
	GPIO44 Pin = 44
	GPIO45 Pin = 45
	GPIO46 Pin = 46
	GPIO48 Pin = 48
)

// Configure this pin with the given configuration.
func (p Pin) Configure(config PinConfig) {
	// Use GPIO mode and simple GPIO output signal
	p.configure(config, IOMUX_FUNC_GPIO, GPIO_FUNC_OUT_SEL_SIMPLE_GPIO)
}

// configure is the same as Configure, but allows for setting a specific IO MUX function
// and output signal. This enables both GPIO Matrix routing and IO MUX direct connections.
func (p Pin) configure(config PinConfig, iomuxFunc uint32, signal uint32) {
	if p == NoPin {
		// NoPin simplifies peripheral configuration - just skip
		return
	}

	var muxConfig uint32

	// Configure IO_MUX register for this pin
	muxConfig |= iomuxFunc << esp.IO_MUX_GPIO_MCU_SEL_Pos

	// Enable input path (required even for output pins for reading back state)
	muxConfig |= esp.IO_MUX_GPIO_FUN_IE

	// Set drive strength (affects output current capability)
	// For SPI pins, use maximum drive strength (3) for better signal quality
	driveStrength := uint32(2)       // Default: moderate strength (2)
	if iomuxFunc == IOMUX_FUNC_SPI { // SPI function
		driveStrength = 3 // Maximum strength for SPI
	}
	muxConfig |= driveStrength << esp.IO_MUX_GPIO_FUN_DRV_Pos

	// Configure pull resistors
	if config.Mode == PinInputPullup {
		muxConfig |= esp.IO_MUX_GPIO_FUN_WPU // Weak Pull Up
	} else if config.Mode == PinInputPulldown {
		muxConfig |= esp.IO_MUX_GPIO_FUN_WPD // Weak Pull Down
	}

	// Apply IO_MUX configuration to the pin's pad
	p.mux().Set(muxConfig)

	// Set output enable based on pin mode
	switch config.Mode {
	case PinOutput:
		// Enable output driver
		if p < 32 {
			esp.GPIO.ENABLE_W1TS.Set(1 << p)
		} else {
			esp.GPIO.ENABLE1_W1TS.Set(1 << (p - 32))
		}
		// Set the output signal (GPIO Matrix or IO MUX bypass)
		if iomuxFunc == IOMUX_FUNC_GPIO { // GPIO mode - use GPIO Matrix
			p.outFunc().Set(signal)
		}
		// For IO MUX direct connection (iomuxFunc != IOMUX_FUNC_GPIO), we don't set outFunc
		// because the signal goes directly through IO MUX
	case PinInput, PinInputPullup, PinInputPulldown:
		// Disable output driver for input modes
		if p < 32 {
			esp.GPIO.ENABLE_W1TC.Set(1 << p)
		} else {
			esp.GPIO.ENABLE1_W1TC.Set(1 << (p - 32))
		}
	}
}

// outFunc returns the FUNCx_OUT_SEL_CFG register for GPIO matrix output routing.
// Each pin has its own register to select which internal signal to output.
func (p Pin) outFunc() *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_OUT_SEL_CFG), uintptr(p)*4))
}

// inFunc returns the FUNCy_IN_SEL_CFG register for GPIO matrix input routing.
// Each peripheral input signal has its own register to select which pin to read from.
func inFunc(signal uint32) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_IN_SEL_CFG), uintptr(signal)*4))
}

// mux returns the IO_MUX register for pad configuration (drive strength, pull-ups, etc).
// Each GPIO pin has its own IO_MUX register for physical pad properties.
func (p Pin) mux() *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.IO_MUX.GPIO0), uintptr(p)*4))
}

// Set the pin to high or low.
// Warning: only use this on an output pin!
func (p Pin) Set(value bool) {
	// Ensure pin is enabled as output (ESP32-S3 requires explicit output enable)
	if p < 32 {
		esp.GPIO.ENABLE_W1TS.Set(1 << p)
	} else {
		esp.GPIO.ENABLE1_W1TS.Set(1 << (p - 32))
	}

	// Set pin state using write-1-to-set/clear registers for atomic operation
	if value {
		if p < 32 {
			esp.GPIO.OUT_W1TS.Set(1 << p) // Write 1 to set bit
		} else {
			esp.GPIO.OUT1_W1TS.Set(1 << (p - 32))
		}
	} else {
		if p < 32 {
			esp.GPIO.OUT_W1TC.Set(1 << p) // Write 1 to clear bit
		} else {
			esp.GPIO.OUT1_W1TC.Set(1 << (p - 32))
		}
	}
}

// Return the register and mask to enable a given GPIO pin. This can be used to
// implement bit-banged drivers.
//
// Warning: only use this on an output pin!
func (p Pin) PortMaskSet() (*uint32, uint32) {
	reg, mask := p.portMaskSet()
	return &reg.Reg, mask
}

// Return the register and mask to disable a given GPIO pin. This can be used to
// implement bit-banged drivers.
//
// Warning: only use this on an output pin!
func (p Pin) PortMaskClear() (*uint32, uint32) {
	reg, mask := p.portMaskClear()
	return &reg.Reg, mask
}

// portMaskSet returns the register and mask for atomic bit setting.
// ESP32-S3 has separate registers for pins 0-31 and 32+.
func (p Pin) portMaskSet() (*volatile.Register32, uint32) {
	if p < 32 {
		return &esp.GPIO.OUT_W1TS, 1 << p
	} else {
		return &esp.GPIO.OUT1_W1TS, 1 << (p - 32)
	}
}

// portMaskClear returns the register and mask for atomic bit clearing.
// ESP32-S3 has separate registers for pins 0-31 and 32+.
func (p Pin) portMaskClear() (*volatile.Register32, uint32) {
	if p < 32 {
		return &esp.GPIO.OUT_W1TC, 1 << p
	} else {
		return &esp.GPIO.OUT1_W1TC, 1 << (p - 32)
	}
}

// Get returns the current value of a GPIO pin.
// Works for both input and output pins (reads actual pin state).
func (p Pin) Get() bool {
	if p < 32 {
		return esp.GPIO.IN.Get()&(1<<p) != 0
	} else {
		return esp.GPIO.IN1.Get()&(1<<(p-32)) != 0
	}
}
