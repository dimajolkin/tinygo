//go:build esp32s3

package machine

import (
	"device/esp"
	"errors"
)

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
