//go:build esp32s3

package machine

// ESP32-S3 SPI support based on ESP32-C3 implementation
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

// ESP32-S3 GPIO Matrix signal indices for SPI
const (
	// SPI2 (FSPI) signals - Hardware SPI2
	SPI2_CLK_OUT_IDX = uint32(63)
	SPI2_CLK_IN_IDX  = uint32(63)
	SPI2_Q_OUT_IDX   = uint32(64) // MISO
	SPI2_Q_IN_IDX    = uint32(64)
	SPI2_D_OUT_IDX   = uint32(65) // MOSI
	SPI2_D_IN_IDX    = uint32(65)
	SPI2_CS0_OUT_IDX = uint32(68)

	// SPI3 (HSPI) signals - Hardware SPI3
	SPI3_CLK_OUT_IDX = uint32(74)
	SPI3_CLK_IN_IDX  = uint32(74)
	SPI3_Q_OUT_IDX   = uint32(75) // MISO
	SPI3_Q_IN_IDX    = uint32(75)
	SPI3_D_OUT_IDX   = uint32(76) // MOSI
	SPI3_D_IN_IDX    = uint32(76)
	SPI3_CS0_OUT_IDX = uint32(79)
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
func (spi *SPI) Configure(config SPIConfig) error {
	// Set default frequency if not specified
	if config.Frequency == 0 {
		config.Frequency = 1000000 // Default to 1MHz
	}

	// Configure GPIO pins and matrix routing
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

	// Configure GPIO pins with matrix routing
	if config.SCK != NoPin {
		config.SCK.Configure(PinConfig{Mode: PinOutput})
		config.SCK.outFunc().Set(sckOutIdx)
	}
	if config.SDO != NoPin {
		config.SDO.Configure(PinConfig{Mode: PinOutput})
		config.SDO.outFunc().Set(mosiOutIdx)
	}
	if config.SDI != NoPin {
		config.SDI.Configure(PinConfig{Mode: PinInput})
		inFunc(misoInIdx).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(config.SDI))
	}
	if config.CS != NoPin {
		config.CS.Configure(PinConfig{Mode: PinOutput})
		config.CS.outFunc().Set(csOutIdx)
	}

	// Only busID 2 and 3 are supported (hardware SPI2/SPI3, exposed as SPI0/SPI1)
	switch spi.busID {
	case 2: // Hardware SPI2 (FSPI) - exposed as SPI0 for users
		esp.SYSTEM.SetPERIP_CLK_EN0_SPI2_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI2_RST(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI2_RST(0)

		// Cast to correct type for SPI2
		if bus, ok := spi.Bus.(*esp.SPI2_Type); ok {
			// Initialize SPI master following ESP-IDF HAL spi_ll_master_init exactly
			// Reset timing
			bus.USER1.Set(0) // cs_setup_time = 0, cs_hold_time = 0

			// Use all 64 bytes of the buffer
			bus.SetUSER_USR_MISO_HIGHPART(0)
			bus.SetUSER_USR_MOSI_HIGHPART(0)

			// Disable unneeded ints
			bus.SLAVE.Set(0)
			bus.USER.Set(0)

			// Configure master clock gate
			bus.SetCLK_GATE_MST_CLK_ACTIVE(1)
			bus.SetCLK_GATE_MST_CLK_SEL(1)

			// Configure DMA
			bus.DMA_CONF.Set(0)
			bus.SetDMA_CONF_SLV_TX_SEG_TRANS_CLR_EN(1)
			bus.SetDMA_CONF_SLV_RX_SEG_TRANS_CLR_EN(1)
			// dma_seg_trans_en = 0 (already 0 from DMA_CONF.Set(0))

			// Configure master mode
			bus.SetUSER_USR_MOSI(1)     // Enable MOSI
			bus.SetUSER_USR_MISO(1)     // Enable MISO
			bus.SetUSER_DOUTDIN(1)      // Full-duplex mode
			bus.SetCTRL_WR_BIT_ORDER(0) // MSB first
			bus.SetCTRL_RD_BIT_ORDER(0) // MSB first

			// Configure SPI mode (CPOL/CPHA)
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

			// Set clock divider for frequency
			// ESP32-S3 APB clock is typically 80MHz
			divider := uint32(80000000 / config.Frequency)
			if divider < 1 {
				divider = 1
			}
			if divider > 0x3F {
				divider = 0x3F
			}

			// Configure clock
			bus.CLOCK.Set(0)
			bus.SetCLOCK_CLK_EQU_SYSCLK(0)
			bus.SetCLOCK_CLKDIV_PRE(divider - 1)
			bus.SetCLOCK_CLKCNT_N(divider - 1)
			bus.SetCLOCK_CLKCNT_H((divider / 2) - 1)
			bus.SetCLOCK_CLKCNT_L(divider - 1)
		}

	case 3: // Hardware SPI3 (HSPI) - exposed as SPI1 for users
		esp.SYSTEM.SetPERIP_CLK_EN0_SPI3_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI3_RST(1)
		esp.SYSTEM.SetPERIP_RST_EN0_SPI3_RST(0)

		// Cast to correct type for SPI3 (uses SPI2_Type structure)
		if bus, ok := spi.Bus.(*esp.SPI2_Type); ok {
			// Initialize SPI master following ESP-IDF HAL spi_ll_master_init exactly
			// Reset timing
			bus.USER1.Set(0) // cs_setup_time = 0, cs_hold_time = 0

			// Use all 64 bytes of the buffer
			bus.SetUSER_USR_MISO_HIGHPART(0)
			bus.SetUSER_USR_MOSI_HIGHPART(0)

			// Disable unneeded ints
			bus.SLAVE.Set(0)
			bus.USER.Set(0)

			// Configure master clock gate
			bus.SetCLK_GATE_MST_CLK_ACTIVE(1)
			bus.SetCLK_GATE_MST_CLK_SEL(1)

			// Configure DMA
			bus.DMA_CONF.Set(0)
			bus.SetDMA_CONF_SLV_TX_SEG_TRANS_CLR_EN(1)
			bus.SetDMA_CONF_SLV_RX_SEG_TRANS_CLR_EN(1)
			// dma_seg_trans_en = 0 (already 0 from DMA_CONF.Set(0))

			// Configure master mode
			bus.SetUSER_USR_MOSI(1)     // Enable MOSI
			bus.SetUSER_USR_MISO(1)     // Enable MISO
			bus.SetUSER_DOUTDIN(1)      // Full-duplex mode
			bus.SetCTRL_WR_BIT_ORDER(0) // MSB first
			bus.SetCTRL_RD_BIT_ORDER(0) // MSB first

			// Configure SPI mode (CPOL/CPHA)
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

			// Set clock divider for frequency
			divider := uint32(80000000 / config.Frequency)
			if divider < 1 {
				divider = 1
			}
			if divider > 0x3F {
				divider = 0x3F
			}

			// Configure clock
			bus.CLOCK.Set(0)
			bus.SetCLOCK_CLK_EQU_SYSCLK(0)
			bus.SetCLOCK_CLKDIV_PRE(divider - 1)
			bus.SetCLOCK_CLKCNT_N(divider - 1)
			bus.SetCLOCK_CLKCNT_H((divider / 2) - 1)
			bus.SetCLOCK_CLKCNT_L(divider - 1)
		}

	default:
		return ErrInvalidSPIBus
	}

	return nil
}

// Transfer writes/reads a single byte using the SPI interface.
func (spi *SPI) Transfer(w byte) (byte, error) {
	bus, ok := spi.Bus.(*esp.SPI2_Type)
	if !ok {
		return 0, errors.New("invalid SPI bus type")
	}

	// Set transfer length (8 bits = 7 in register)
	bus.SetMS_DLEN_MS_DATA_BITLEN(7)

	// Write data to buffer
	bus.W0.Set(uint32(w))

	// Start transaction and wait for completion (like ESP32-C3)
	bus.SetCMD_USR(1)
	for bus.GetCMD_USR() != 0 {
		// Wait until CMD_USR becomes 0
	}

	// Read received data
	return byte(bus.GetW0() & 0xFF), nil
}

// Tx handles read/write operation for SPI interface.
func (spi *SPI) Tx(w, r []byte) error {
	bus, ok := spi.Bus.(*esp.SPI2_Type)
	if !ok {
		return errors.New("invalid SPI bus type")
	}

	toTransfer := len(w)
	if len(r) > toTransfer {
		toTransfer = len(r)
	}

	for toTransfer > 0 {
		// Chunk 64 bytes at a time (like ESP32-C3)
		chunkSize := toTransfer
		if chunkSize > 64 {
			chunkSize = 64
		}

		// Fill tx buffer using unsafe.Pointer for fast access (like ESP32-C3)
		transferWords := (*[16]volatile.Register32)(unsafe.Pointer(uintptr(unsafe.Pointer(&bus.W0))))
		if len(w) >= 64 {
			// Optimized path for full 64-byte buffers
			for i := 0; i < 16; i++ {
				word := uint32(w[i*4]) | uint32(w[i*4+1])<<8 | uint32(w[i*4+2])<<16 | uint32(w[i*4+3])<<24
				transferWords[i].Set(word)
			}
		} else {
			// Careful approach for partial buffers
			for i := 0; i < 16; i++ {
				var word uint32
				if i*4+3 < len(w) {
					word |= uint32(w[i*4+3]) << 24
				}
				if i*4+2 < len(w) {
					word |= uint32(w[i*4+2]) << 16
				}
				if i*4+1 < len(w) {
					word |= uint32(w[i*4+1]) << 8
				}
				if i*4+0 < len(w) {
					word |= uint32(w[i*4+0]) << 0
				}
				transferWords[i].Set(word)
			}
		}

		// Do the transfer (like ESP32-C3)
		bus.SetMS_DLEN_MS_DATA_BITLEN(uint32(chunkSize)*8 - 1)

		// Note: ESP32-S3 might not have CMD_UPDATE like ESP32-C3, so we skip it
		// Start transaction
		bus.SetCMD_USR(1)

		// Add timeout to prevent hanging
		timeout := 100000
		for bus.GetCMD_USR() != 0 && timeout > 0 {
			timeout--
		}
		if timeout == 0 {
			return errors.New("SPI timeout in Tx")
		}

		// Read rx buffer
		rxSize := chunkSize
		if rxSize > len(r) {
			rxSize = len(r)
		}
		for i := 0; i < rxSize; i++ {
			r[i] = byte(transferWords[i/4].Get() >> ((i % 4) * 8))
		}

		// Move to next chunk (like ESP32-C3)
		if len(w) < chunkSize {
			w = nil
		} else {
			w = w[chunkSize:]
		}
		if len(r) < chunkSize {
			r = nil
		} else {
			r = r[chunkSize:]
		}
		toTransfer -= chunkSize
	}

	return nil
}
