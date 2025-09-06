//go:build esp32s3

package machine

import (
	"device/esp"
	"errors"
	"runtime/volatile"
	"unsafe"
)

const deviceName = esp.Device

const peripheralClock = 40_000000 // 80MHz

// CPUFrequency returns the current CPU frequency of the chip.
// Currently it is a fixed frequency but it may allow changing in the future.
func CPUFrequency() uint32 {
	return 160e6 // 80 MHz
}

var (
	ErrInvalidSPIBus = errors.New("machine: invalid SPI bus")
)

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
	// Output function 256 is a special value reserved for use as a regular GPIO
	// pin. Peripherals (SPI etc) can set a custom output function by calling
	// lowercase configure() instead with a signal name.
	p.configure(config, 256)
}

// configure is the same as Configure, but allows for setting a specific input
// or output signal for peripheral use (SPI, I2C, etc).
// Signals are routed through the GPIO matrix. Output signals use FUNCx_OUT_SEL_CFG,
// input signals use FUNCy_IN_SEL_CFG registers.
func (p Pin) configure(config PinConfig, signal uint32) {
	if p == NoPin {
		// NoPin simplifies peripheral configuration - just skip
		return
	}

	var muxConfig uint32

	// Configure IO_MUX register for this pin
	const function = 1          // Function 1 = GPIO mode for ESP32-S3
	muxConfig |= function << 12 // MCU_SEL field (bits 14:12)

	// Enable input path (required even for output pins for reading back state)
	muxConfig |= 1 << 9 // FUN_IE bit (Input Enable)

	// Set drive strength (affects output current capability)
	muxConfig |= 2 << 10 // FUN_DRV field (bits 11:10): 0=5mA, 1=10mA, 2=20mA, 3=40mA

	// Configure pull resistors
	if config.Mode == PinInputPullup {
		muxConfig |= 1 << 7 // FUN_WPU bit (Weak Pull Up)
	} else if config.Mode == PinInputPulldown {
		muxConfig |= 1 << 8 // FUN_WPD bit (Weak Pull Down)
	}

	// Apply IO_MUX configuration to the pin's pad
	p.mux().Set(muxConfig)

	// Configure GPIO matrix routing
	if signal == 256 {
		// Special value 256 = simple GPIO mode, bypass matrix for better performance
		p.outFunc().Set(256)
	} else {
		// Route specific peripheral signal through this pin
		p.outFunc().Set(signal)
	}

	// Set output enable based on pin mode
	switch config.Mode {
	case PinOutput:
		// Enable output driver
		if p < 32 {
			esp.GPIO.ENABLE_W1TS.Set(1 << p)
		} else {
			esp.GPIO.ENABLE1_W1TS.Set(1 << (p - 32))
		}
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

var DefaultUART = UART0

var (
	UART0  = &_UART0
	_UART0 = UART{Bus: esp.UART0, Buffer: NewRingBuffer()}
	UART1  = &_UART1
	_UART1 = UART{Bus: esp.UART1, Buffer: NewRingBuffer()}
	UART2  = &_UART2
	_UART2 = UART{Bus: esp.UART2, Buffer: NewRingBuffer()}
)

type UART struct {
	Bus    *esp.UART_Type
	Buffer *RingBuffer
}

func (uart *UART) Configure(config UARTConfig) {
	if config.BaudRate == 0 {
		config.BaudRate = 115200
	}

	// Set baud rate using peripheral clock divider
	uart.Bus.CLKDIV.Set(peripheralClock / config.BaudRate)

	// Configure default UART pins for ESP32-S3 development boards
	if uart.Bus == esp.UART0 {
		// GPIO43 = TX, GPIO44 = RX (common ESP32-S3 dev board pinout)
		GPIO43.Configure(PinConfig{Mode: PinOutput})
		GPIO43.outFunc().Set(1) // Route UART0 TX signal to GPIO43

		GPIO44.Configure(PinConfig{Mode: PinInput})
		inFunc(1).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(GPIO44)) // Route GPIO44 to UART0 RX
	}
}

func (uart *UART) writeByte(b byte) error {
	// Wait for TX FIFO to have space (max 128 bytes)
	for (uart.Bus.STATUS.Get()>>16)&0xff >= 128 {
		// UART_TXFIFO_CNT field indicates current TX buffer usage
	}
	uart.Bus.FIFO.Set(uint32(b))
	return nil
}

func (uart *UART) flush() {
	// ESP32-S3 UART automatically flushes, no action needed
}

// USB Serial/JTAG Controller for ESP32-S3
type USB_DEVICE struct {
	Bus *esp.USB_DEVICE_Type
}

var (
	_USBCDC = USB_DEVICE{
		Bus: esp.USB_DEVICE,
	}

	USBCDC Serialer = _USBCDC
)

type Serialer interface {
	WriteByte(c byte) error
	Write(data []byte) (n int, err error)
	Configure(config UARTConfig) error
	Buffered() int
	ReadByte() (byte, error)
	DTR() bool
	RTS() bool
}

func (usbdev USB_DEVICE) Configure(config UARTConfig) error {
	// USB Serial/JTAG is initialized by runtime, no additional config needed
	return nil
}

func (usbdev USB_DEVICE) WriteByte(c byte) error {
	// Check if USB host is connected by looking at configuration status
	// If no host is connected, just drop the byte silently
	if !usbdev.isHostConnected() {
		return nil
	}

	// Host is connected - wait for TX FIFO space with reasonable timeout
	timeout := 10000 // Generous timeout for connected host
	for usbdev.Bus.GetEP1_CONF_SERIAL_IN_EP_DATA_FREE() == 0 && timeout > 0 {
		timeout--
	}

	// Even with host connected, don't hang forever
	if timeout == 0 {
		return nil
	}

	// Write byte to USB Serial/JTAG endpoint
	usbdev.Bus.SetEP1_RDWR_BYTE(uint32(c))

	// Trigger transmission
	usbdev.Bus.SetEP1_CONF_WR_DONE(1)

	return nil
}

// isHostConnected checks if USB Serial/JTAG host is actually connected
func (usbdev USB_DEVICE) isHostConnected() bool {
	// Check USB device state - if configured, host is likely connected
	// ESP32-S3 USB Serial/JTAG reports connection status via device state
	return usbdev.Bus.GetEP1_CONF_SERIAL_IN_EP_DATA_FREE() > 0 ||
		usbdev.Bus.GetEP1_CONF_SERIAL_OUT_EP_DATA_AVAIL() == 0
}

func (usbdev USB_DEVICE) Write(data []byte) (n int, err error) {
	for _, c := range data {
		err = usbdev.WriteByte(c)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (usbdev USB_DEVICE) ReadByte() (byte, error) {
	// TODO: Implement USB Serial/JTAG input reading
	return 0, errors.New("ReadByte not implemented")
}

func (usbdev USB_DEVICE) Buffered() int {
	// Return number of bytes available to read
	return int(usbdev.Bus.GetEP1_CONF_SERIAL_OUT_EP_DATA_AVAIL())
}

func (usbdev USB_DEVICE) DTR() bool {
	// Data Terminal Ready - not applicable for USB Serial/JTAG
	return false
}

func (usbdev USB_DEVICE) RTS() bool {
	// Request To Send - not applicable for USB Serial/JTAG
	return false
}

func (usbdev USB_DEVICE) flush() {
	// Force transmission of any buffered data
	usbdev.Bus.SetEP1_CONF_WR_DONE(1)
}
