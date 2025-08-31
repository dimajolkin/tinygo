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
)

// Configure this pin with the given configuration.
func (p Pin) Configure(config PinConfig) {
	// Output function 256 is a special value reserved for use as a regular GPIO
	// pin. Peripherals (SPI etc) can set a custom output function by calling
	// lowercase configure() instead with a signal name.
	p.configure(config, 256)
}

// configure is the same as Configure, but allows for setting a specific input
// or output signal.
// Signals are always routed through the GPIO matrix for simplicity. Output
// signals are configured in FUNCx_OUT_SEL_CFG which selects a particular signal
// to output on a given pin. Input signals are configured in FUNCy_IN_SEL_CFG,
// which sets the pin to use for a particular input signal.
func (p Pin) configure(config PinConfig, signal uint32) {
	if p == NoPin {
		// This simplifies pin configuration in peripherals such as SPI.
		return
	}

	// TODO: Mux config
}

// outFunc returns the FUNCx_OUT_SEL_CFG register used for configuring the
// output function selection.
func (p Pin) outFunc() *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_OUT_SEL_CFG), uintptr(p)*4))
}

// inFunc returns the FUNCy_IN_SEL_CFG register used for configuring the input
// function selection.
func inFunc(signal uint32) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_IN_SEL_CFG), uintptr(signal)*4))
}

// Set the pin to high or low.
// Warning: only use this on an output pin!
func (p Pin) Set(value bool) {
	if value {
		reg, mask := p.portMaskSet()
		reg.Set(mask)
	} else {
		reg, mask := p.portMaskClear()
		reg.Set(mask)
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

func (p Pin) portMaskSet() (*volatile.Register32, uint32) {
	if p < 32 {
		return &esp.GPIO.OUT_W1TS, 1 << p
	} else {
		return &esp.GPIO.OUT1_W1TS, 1 << (p - 32)
	}
}

func (p Pin) portMaskClear() (*volatile.Register32, uint32) {
	if p < 32 {
		return &esp.GPIO.OUT_W1TC, 1 << p
	} else {
		return &esp.GPIO.OUT1_W1TC, 1 << (p - 32)
	}
}

// Get returns the current value of a GPIO pin when the pin is configured as an
// input or as an output.
func (p Pin) Get() bool {
	if p < 32 {
		return esp.GPIO.IN.Get()&(1<<p) != 0
	} else {
		return esp.GPIO.IN1.Get()&(1<<(p-32)) != 0
	}
}

// TODO: Mux

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

	// Configure UART with basic settings
	uart.Bus.CLKDIV.Set(peripheralClock / config.BaudRate)

	// Configure UART pins for ESP32-S3
	// Most ESP32-S3 dev boards use GPIO43 (TX) and GPIO44 (RX) for UART0
	if uart.Bus == esp.UART0 {
		// Configure TX pin (GPIO43)
		GPIO43.Configure(PinConfig{Mode: PinOutput})
		GPIO43.outFunc().Set(1) // UART0 TX signal

		// Configure RX pin (GPIO44)
		GPIO44.Configure(PinConfig{Mode: PinInput})
		inFunc(1).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(GPIO44)) // UART0 RX signal
	}
}

func (uart *UART) writeByte(b byte) error {
	for (uart.Bus.STATUS.Get()>>16)&0xff >= 128 {
		// Read UART_TXFIFO_CNT from the status register, which indicates how
		// many bytes there are in the transmit buffer. Wait until there are
		// less than 128 bytes in this buffer (the default buffer size).
	}
	uart.Bus.FIFO.Set(uint32(b))
	return nil
}

func (uart *UART) flush() {}

// USB Serial/JTAG Controller for ESP32-S3
// Similar to ESP32-C3 implementation
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
	// Enable USB Serial/JTAG controller according to ESP-IDF documentation

	// 1. Enable USB device clock
	esp.SYSTEM.PERIP_CLK_EN1.SetBits(esp.SYSTEM_PERIP_CLK_EN1_USB_DEVICE_CLK_EN)

	// 2. Release USB device reset
	esp.SYSTEM.PERIP_RST_EN1.ClearBits(esp.SYSTEM_PERIP_RST_EN1_USB_DEVICE_RST)

	// 3. Enable USB pad to use GPIO19/20 for USB Serial/JTAG
	usbdev.Bus.SetCONF0_USB_PAD_ENABLE(1)

	// 4. Enable USB JTAG bridge for serial communication
	usbdev.Bus.SetCONF0_USB_JTAG_BRIDGE_EN(1)

	// 5. Initialize endpoint 1 for serial communication
	// Clear any pending data
	usbdev.Bus.EP1_CONF.ClearBits(0xFF)

	// 6. Enable serial input endpoint
	usbdev.Bus.SetEP1_CONF_SERIAL_IN_EP_DATA_FREE(1)

	return nil
}

func (usbdev USB_DEVICE) WriteByte(c byte) error {
	// Check if USB Serial/JTAG is connected and ready
	// Simple approach: just try to write without complex checks

	// Write byte to USB Serial/JTAG TX FIFO
	usbdev.Bus.SetEP1_RDWR_BYTE(uint32(c))

	// Signal that data is ready to be sent
	usbdev.Bus.SetEP1_CONF_WR_DONE(1)

	// Small delay to allow USB processing
	for i := 0; i < 10; i++ {
		// Simple delay loop
	}

	return nil
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
	// Not implemented for ESP32-S3 yet
	return 0, errors.New("ReadByte not implemented")
}

func (usbdev USB_DEVICE) Buffered() int {
	return int(usbdev.Bus.GetEP1_CONF_SERIAL_OUT_EP_DATA_AVAIL())
}

func (usbdev USB_DEVICE) DTR() bool {
	return false
}

func (usbdev USB_DEVICE) RTS() bool {
	return false
}

func (usbdev USB_DEVICE) flush() {
	// Simplified flush - just trigger write done
	usbdev.Bus.SetEP1_CONF_WR_DONE(1)
}

// TODO: SPI
