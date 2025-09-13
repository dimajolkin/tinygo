//go:build esp32s3

package machine

// ESP32-S3 SPI support based on ESP-IDF HAL
// Simple but correct implementation following spi_ll.h
// SPI0 = hardware SPI2 (FSPI), SPI1 = hardware SPI3 (HSPI)
// https://docs.espressif.com/projects/esp-idf/en/latest/esp32s3/api-reference/peripherals/spi_master.html

import (
	"device/esp"
	"errors"
	"runtime/volatile"
	"unsafe"
)

const (
	SPI_MODE0 = uint8(0)
	SPI_MODE1 = uint8(1)
	SPI_MODE2 = uint8(2)
	SPI_MODE3 = uint8(3)
)

// ESP32-S3 GPIO Matrix signal indices for SPI - CORRECTED from ESP-IDF gpio_sig_map.h
const (
	// SPI2 (FSPI) signals - Hardware SPI2 - CORRECT VALUES from ESP-IDF
	SPI2_CLK_OUT_IDX = uint32(101) // FSPICLK_OUT_IDX
	SPI2_CLK_IN_IDX  = uint32(101) // FSPICLK_IN_IDX
	SPI2_Q_OUT_IDX   = uint32(102) // FSPIQ_OUT_IDX (MISO)
	SPI2_Q_IN_IDX    = uint32(102) // FSPIQ_IN_IDX
	SPI2_D_OUT_IDX   = uint32(103) // FSPID_OUT_IDX (MOSI)
	SPI2_D_IN_IDX    = uint32(103) // FSPID_IN_IDX
	SPI2_CS0_OUT_IDX = uint32(110) // FSPICS0_OUT_IDX

	// SPI3 (HSPI) signals - Hardware SPI3 - CORRECTED from ESP-IDF gpio_sig_map.h
	// Source: /Users/dimajolkin/esp/esp-idf/components/soc/esp32s3/include/soc/gpio_sig_map.h
	SPI3_CLK_OUT_IDX = uint32(66) // Line 136: SPI3_CLK_OUT_IDX
	SPI3_CLK_IN_IDX  = uint32(66) // Line 135: SPI3_CLK_IN_IDX
	SPI3_Q_OUT_IDX   = uint32(67) // Line 138: SPI3_Q_OUT_IDX (MISO)
	SPI3_Q_IN_IDX    = uint32(67) // Line 137: SPI3_Q_IN_IDX
	SPI3_D_OUT_IDX   = uint32(68) // Line 140: SPI3_D_OUT_IDX (MOSI)
	SPI3_D_IN_IDX    = uint32(68) // Line 139: SPI3_D_IN_IDX
	SPI3_CS0_OUT_IDX = uint32(71) // Line 146: SPI3_CS0_OUT_IDX
)

var (
	ErrInvalidSPIBus = errors.New("machine: SPI bus is invalid")
)

// Serial Peripheral Interface on the ESP32-S3.
type SPI struct {
	Bus   interface{}
	busID uint8
}

var (
	SPI0 = &SPI{Bus: esp.SPI2, busID: 2} // Primary SPI (FSPI)
	SPI1 = &SPI{Bus: esp.SPI3, busID: 3} // Secondary SPI (HSPI)
)

// SPIConfig is used to store config info for SPI.
type SPIConfig struct {
	Frequency uint32
	SCK       Pin   // Serial Clock
	SDO       Pin   // Serial Data Out (MOSI)
	SDI       Pin   // Serial Data In  (MISO)
	CS        Pin   // Chip Select (optional)
	LSBFirst  bool  // MSB is default
	Mode      uint8 // SPI_MODE0 is default
}

// Configure and make the SPI peripheral ready to use.
// Implementation following ESP-IDF HAL with GPIO Matrix routing
func (spi *SPI) Configure(config SPIConfig) error {

	// Set default frequency if not specified
	if config.Frequency == 0 {
		config.Frequency = 1000000 // Default to 1MHz
	}

	// Get GPIO Matrix signal indices for this SPI bus
	var sckOutIdx, mosiOutIdx, misoInIdx, csOutIdx uint32
	switch spi.busID {
	case 2: // SPI2 (FSPI)
		sckOutIdx = SPI2_CLK_OUT_IDX
		mosiOutIdx = SPI2_D_OUT_IDX
		misoInIdx = SPI2_Q_IN_IDX
		csOutIdx = SPI2_CS0_OUT_IDX
	case 3: // SPI3 (HSPI)
		sckOutIdx = SPI3_CLK_OUT_IDX
		mosiOutIdx = SPI3_D_OUT_IDX
		misoInIdx = SPI3_Q_IN_IDX
		csOutIdx = SPI3_CS0_OUT_IDX
	default:
		return ErrInvalidSPIBus
	}

	// Configure GPIO pins using GPIO Matrix routing
	// Note: We use GPIO Matrix instead of IO MUX for flexibility

	// Configure GPIO pins using GPIO Matrix routing
	configureSPIGPIOMatrix(config, sckOutIdx, mosiOutIdx, misoInIdx, csOutIdx)

	// Enable peripheral clock and reset
	// Without bootloader, we need to be more explicit about clock initialization
	switch spi.busID {
	case 2: // Hardware SPI2 (FSPI)
		esp.SYSTEM.SetPERIP_CLK_EN0_SPI2_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI2_RST(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI2_RST(0)
	case 3: // Hardware SPI3 (HSPI)
		esp.SYSTEM.SetPERIP_CLK_EN0_SPI3_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI3_RST(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI3_RST(0)
	}

	// Get bus handle - both SPI2 and SPI3 use SPI2_Type
	bus, ok := spi.Bus.(*esp.SPI2_Type)
	if !ok {
		return ErrInvalidSPIBus
	}

	// Reset timing: cs_setup_time = 0, cs_hold_time = 0
	bus.USER1.Set(0)

	// Use all 64 bytes of the buffer
	bus.SetUSER_USR_MISO_HIGHPART(0)
	bus.SetUSER_USR_MOSI_HIGHPART(0)

	// Disable unneeded interrupts and clear all USER bits first
	bus.SLAVE.Set(0)
	bus.USER.Set(0)

	// Clear other important registers like ESP32-C3
	bus.MISC.Set(0)
	bus.CTRL.Set(0)
	bus.CLOCK.Set(0)

	// Clear data buffers like ESP32-C3
	bus.W0.Set(0)
	bus.W1.Set(0)
	bus.W2.Set(0)
	bus.W3.Set(0)

	// Configure master clock gate - CRITICAL: need CLK_EN bit!
	bus.SetCLK_GATE_CLK_EN(1)         // Enable basic SPI clock (bit 0)
	bus.SetCLK_GATE_MST_CLK_ACTIVE(1) // Enable master clock (bit 1)
	bus.SetCLK_GATE_MST_CLK_SEL(1)    // Select master clock (bit 2)

	// Configure DMA following ESP-IDF HAL
	// Reset DMA configuration
	bus.DMA_CONF.Set(0)
	// Set DMA segment transaction clear enable bits
	bus.SetDMA_CONF_SLV_TX_SEG_TRANS_CLR_EN(1)
	bus.SetDMA_CONF_SLV_RX_SEG_TRANS_CLR_EN(1)
	// dma_seg_trans_en = 0 (already 0 from DMA_CONF.Set(0))

	// Configure master mode
	bus.SetUSER_USR_MOSI(1)     // Enable MOSI
	bus.SetUSER_USR_MISO(1)     // Enable MISO
	bus.SetUSER_DOUTDIN(1)      // Full-duplex mode
	bus.SetCTRL_WR_BIT_ORDER(0) // MSB first
	bus.SetCTRL_RD_BIT_ORDER(0) // MSB first

	// CRITICAL: Enable clock output (from working test)
	bus.SetMISC_CK_DIS(0) // Enable CLK output - THIS IS KEY!

	// Configure SPI mode (CPOL/CPHA) following ESP-IDF HAL
	switch config.Mode {
	case SPI_MODE0:
		// CPOL=0, CPHA=0 (default)
	case SPI_MODE1:
		bus.SetUSER_CK_OUT_EDGE(1) // CPHA=1
	case SPI_MODE2:
		bus.SetMISC_CK_IDLE_EDGE(1) // CPOL=1
		bus.SetUSER_CK_OUT_EDGE(1)  // CPHA=1
	case SPI_MODE3:
		bus.SetMISC_CK_IDLE_EDGE(1) // CPOL=1
	}

	// Calculate clock divider for frequency
	// ESP32-S3 APB clock is typically 80MHz
	apbClock := uint32(80000000)

	// Try to get actual CPU frequency for better APB clock estimation
	if cpuFreq := CPUFrequency(); cpuFreq > 0 {
		if cpuFreq <= 80000000 {
			apbClock = cpuFreq // APB = CPU for frequencies <= 80MHz
		} else {
			apbClock = cpuFreq / 4 // APB = CPU/4 for higher frequencies
		}
	}

	// Calculate divider, ensuring it's within valid range
	divider := apbClock / config.Frequency
	if divider < 1 {
		divider = 1
	}
	if divider > 0x3F {
		divider = 0x3F // Maximum divider value
	}

	// Configure clock (after clearing CLOCK register above)
	bus.SetCLOCK_CLK_EQU_SYSCLK(0)
	bus.SetCLOCK_CLKDIV_PRE(divider - 1)
	bus.SetCLOCK_CLKCNT_N(divider - 1)
	bus.SetCLOCK_CLKCNT_H((divider / 2) - 1)
	bus.SetCLOCK_CLKCNT_L(divider - 1)

	return nil
}

func formatHex(val uint32) string {
	hex := "0x"
	digits := "0123456789ABCDEF"

	for i := 7; i >= 0; i-- {
		hex += string(digits[(val>>(i*4))&0xF])
	}

	return hex
}

// Transfer writes/reads a single byte using the SPI interface.
// Implementation following ESP-IDF HAL spi_ll_user_start with proper USER register setup
func (spi *SPI) Transfer(w byte) (byte, error) {
	// Both SPI2 and SPI3 use SPI2_Type
	bus, ok := spi.Bus.(*esp.SPI2_Type)
	if !ok {
		return 0, errors.New("invalid SPI bus type")
	}

	// Set transfer length (8 bits = 7 in register)
	bus.SetMS_DLEN_MS_DATA_BITLEN(7)

	// Clear any pending interrupt flags BEFORE starting transaction
	bus.SetDMA_INT_CLR_TRANS_DONE_INT_CLR(1)

	// Write data to buffer (use W0 register)
	bus.W0.Set(uint32(w))

	// CRITICAL: Apply configuration before transmission (like ESP-IDF spi_ll_apply_config)
	bus.SetCMD_UPDATE(1)
	for bus.GetCMD_UPDATE() != 0 {
		// Wait for config to be applied
	}

	// Start transaction following ESP-IDF HAL spi_ll_user_start
	bus.SetCMD_USR(1)

	// Wait for completion using CMD_USR flag (like ESP32-C3 approach)
	// Hardware clears CMD_USR when transaction is complete
	timeout := 100000
	for bus.GetCMD_USR() != 0 && timeout > 0 {
		timeout--
		// Wait for CMD_USR to be cleared by hardware
	}

	if timeout == 0 {
		return 0, errors.New("SPI transfer timeout")
	}

	// Read received data from W0 register
	result := byte(bus.W0.Get() & 0xFF)
	return result, nil
}

// Tx handles read/write operation for SPI interface.
// Simple implementation using ESP-IDF HAL approach - byte by byte for now
func (spi *SPI) Tx(w, r []byte) error {
	// For simplicity, process byte by byte using Transfer
	// This is not efficient but correct and simple
	maxLen := len(w)
	if len(r) > maxLen {
		maxLen = len(r)
	}

	for i := 0; i < maxLen; i++ {
		var writeByte byte = 0
		if i < len(w) {
			writeByte = w[i]
		}

		readByte, err := spi.Transfer(writeByte)
		if err != nil {
			return err
		}

		if i < len(r) {
			r[i] = readByte
		}
	}

	return nil
}

// configureSPIGPIOMatrix configures SPI pins using GPIO Matrix routing
// This provides more flexibility than IO MUX and is required for proper signal routing
func configureSPIGPIOMatrix(config SPIConfig, sckOutIdx, mosiOutIdx, misoInIdx, csOutIdx uint32) {
	// Configure SCK (Clock) pin
	if config.SCK != NoPin {
		configurePinForSPI(config.SCK, sckOutIdx, PinOutput)
	}

	// Configure SDO (MOSI) pin
	if config.SDO != NoPin {
		configurePinForSPI(config.SDO, mosiOutIdx, PinOutput)
	}

	// Configure SDI (MISO) pin
	if config.SDI != NoPin {
		configurePinForSPI(config.SDI, misoInIdx, PinInput)
		// Configure input routing for MISO
		inFunc(misoInIdx).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(config.SDI))
	}

	// Configure CS (Chip Select) pin
	if config.CS != NoPin {
		configurePinForSPI(config.CS, csOutIdx, PinOutput)
	}
}

// configurePinForSPI configures a single pin for SPI using direct GPIO matrix setup
// This ensures proper signal routing that works reliably
func configurePinForSPI(pin Pin, signal uint32, mode PinMode) {
	if pin == NoPin {
		return
	}

	pinNum := uint32(pin)

	// Enable GPIO output/input
	if mode == PinOutput {
		esp.GPIO.ENABLE_W1TS.Set(1 << pinNum)
	}

	// Configure IO MUX for GPIO function (not dedicated peripheral function)
	// This allows GPIO Matrix to control the pin
	// Use the same address calculation as in working test
	// Working test used 0x60009048 for GPIO12, so: 0x60009048 - 12*4 = 0x60009018
	iomuxAddr := uintptr(0x60009018 + pinNum*4) // Base address that gives 0x60009048 for GPIO12
	iomux := (*volatile.Register32)(unsafe.Pointer(iomuxAddr))

	// Configure: function=2 (GPIO), input_enable=1, drive_strength=3, pull_up=1
	muxConfig := (iomux.Get() & ^uint32(0x7000)) | (2 << 12) | (1 << 8) | (3 << 10)
	if mode == PinOutput {
		muxConfig |= 1 << 7 // Enable pull-up for output pins
	}
	iomux.Set(muxConfig)

	// Route signal through GPIO Matrix
	outFuncAddr := unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_OUT_SEL_CFG), uintptr(pinNum)*4)
	outFunc := (*volatile.Register32)(outFuncAddr)
	outFunc.Set(signal)
}
