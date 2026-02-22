//go:build esp32s3 || esp32c3

package machine

import (
	"device/esp"
	"errors"
	"runtime/volatile"
	"unsafe"
)

const (
	i2sPLL160M = 2
	i2sPLLFreq = 160e6 // I2S default clock is PLL_F160M (fixed 160 MHz), not APB 80 MHz (soc_periph_i2s_clk_src_t)
)

type I2S struct {
	bus *esp.I2S_Type
	sig i2sSignals
	id  uint8
}

type i2sSignals struct {
	bckOut, wsOut, doutOut uint32
	dinIn, mclkOut         uint32
}

type I2SMode uint8
type I2SStandard uint8
type I2SClockSource uint8
type I2SDataFormat uint8

const (
	I2SModeSource I2SMode = iota
	I2SModeReceiver
	I2SModePDM
	I2SModeSourceReceiver
)

const (
	I2StandardPhilips I2SStandard = iota
	I2SStandardMSB
	I2SStandardLSB
)

const (
	I2SClockSourceInternal I2SClockSource = iota
	I2SClockSourceExternal
)

const (
	I2SDataFormatDefault I2SDataFormat = 0
	I2SDataFormat8bit                  = 8
	I2SDataFormat16bit                 = 16
	I2SDataFormat24bit                 = 24
	I2SDataFormat32bit                 = 32
)

var ErrInvalidSampleFrequency = errors.New("i2s: invalid sample frequency")

type I2SConfig struct {
	SCK             Pin
	WS              Pin
	SDO             Pin
	SDI             Pin
	Mode            I2SMode
	Standard        I2SStandard
	ClockSource     I2SClockSource
	DataFormat      I2SDataFormat
	AudioFrequency  uint32
	MainClockOutput bool
	Stereo          bool
}

func (i2s *I2S) Configure(config I2SConfig) error {
	if config.AudioFrequency == 0 {
		config.AudioFrequency = 16000
	}
	if config.DataFormat == I2SDataFormatDefault {
		config.DataFormat = I2SDataFormat16bit
	}

	if i2s.id == 0 {
		esp.SYSTEM.SetPERIP_RST_EN0_I2S0_RST(1)
		esp.SYSTEM.SetPERIP_CLK_EN0_I2S0_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN0_I2S0_RST(0)
	} else {
		enableI2S1Clock()
	}

	bus := i2s.bus

	bus.TX_CLKM_CONF.Set(0)
	bus.RX_CLKM_CONF.Set(0)
	bus.SetTX_CLKM_CONF_CLK_EN(1)
	bus.SetTX_CLKM_CONF_TX_CLK_SEL(i2sPLL160M)
	bus.SetRX_CLKM_CONF_RX_CLK_ACTIVE(1)
	bus.SetRX_CLKM_CONF_RX_CLK_SEL(i2sPLL160M)

	mclkDiv, bckDiv := i2sClockDivs(i2sPLLFreq, config.AudioFrequency, config.DataFormat)
	bus.SetTX_CLKM_CONF_TX_CLKM_DIV_NUM(mclkDiv)
	bus.TX_CLKM_DIV_CONF.Set(0)
	bus.SetRX_CLKM_CONF_RX_CLKM_DIV_NUM(mclkDiv)
	bus.RX_CLKM_DIV_CONF.Set(0)

	bus.SetTX_CLKM_CONF_TX_CLK_ACTIVE(1)
	bus.SetRX_CLKM_CONF_RX_CLK_ACTIVE(1)
	bus.SetRX_CLKM_CONF_MCLK_SEL(0)

	slotBits := uint32(16)
	switch config.DataFormat {
	case I2SDataFormat8bit:
		slotBits = 8
	case I2SDataFormat16bit:
		slotBits = 16
	case I2SDataFormat24bit:
		slotBits = 24
	case I2SDataFormat32bit:
		slotBits = 32
	}
	// Standard I2S on ESP32-S3/C3 is implemented as TDM with 2 slots (stereo). Per ESP-IDF HAL:
	// tx_tdm_ws_width = width-1, tx_half_sample_bits = slot_bit_width-1; wrong half_sample_bits prevents WS from toggling.
	bitsMod := slotBits - 1
	halfSample := slotBits - 1

	wsWidth := slotBits - 1
	bus.SetTX_CONF1_TX_TDM_WS_WIDTH(wsWidth)
	bus.SetRX_CONF1_RX_TDM_WS_WIDTH(wsWidth)
	bus.SetTX_CONF1_TX_BCK_DIV_NUM(bckDiv - 1)
	bus.SetTX_CONF1_TX_BITS_MOD(bitsMod)
	bus.SetTX_CONF1_TX_TDM_CHAN_BITS(bitsMod)
	bus.SetTX_CONF1_TX_MSB_SHIFT(1)
	bus.SetTX_CONF1_TX_HALF_SAMPLE_BITS(halfSample)
	bus.SetRX_CONF1_RX_BCK_DIV_NUM(bckDiv - 1)
	bus.SetRX_CONF1_RX_BITS_MOD(bitsMod)
	bus.SetRX_CONF1_RX_TDM_CHAN_BITS(bitsMod)
	bus.SetRX_CONF1_RX_MSB_SHIFT(1)
	bus.SetRX_CONF1_RX_HALF_SAMPLE_BITS(halfSample)

	bus.SetTX_CONF_TX_SLAVE_MOD(0)
	bus.SetTX_CONF_TX_PCM_BYPASS(1)
	bus.SetTX_CONF_TX_PDM_EN(0)
	bus.SetTX_CONF_TX_STOP_EN(0)
	bus.SetTX_CONF_TX_CHAN_MOD(0)
	bus.SetTX_CONF_TX_LEFT_ALIGN(1)
	bus.SetTX_CONF_TX_TDM_EN(1)
	if !config.Stereo {
		bus.SetTX_CONF_TX_MONO(1)
		bus.SetTX_CONF_TX_CHAN_EQUAL(1)
		bus.SetTX_CONF_TX_MONO_FST_VLD(0)
	} else {
		bus.SetTX_CONF_TX_MONO(0)
	}
	bus.SetTX_TDM_CTRL_TX_TDM_TOT_CHAN_NUM(1)
	bus.SetTX_TDM_CTRL_TX_TDM_CHAN0_EN(1)
	bus.SetTX_TDM_CTRL_TX_TDM_CHAN1_EN(1)

	bus.SetRX_CONF_RX_SLAVE_MOD(0)
	bus.SetRX_CONF_RX_PCM_BYPASS(1)
	bus.SetRX_CONF_RX_PDM_EN(0)
	bus.SetRX_CONF_RX_LEFT_ALIGN(1)
	bus.SetRX_CONF_RX_TDM_EN(1)
	bus.SetRX_TDM_CTRL_RX_TDM_TOT_CHAN_NUM(1)
	bus.SetRX_TDM_CTRL_RX_TDM_PDM_CHAN0_EN(1)
	bus.SetRX_TDM_CTRL_RX_TDM_PDM_CHAN1_EN(1)
	bus.SetTX_CONF_TX_WS_IDLE_POL(0)
	bus.SetRX_CONF_RX_WS_IDLE_POL(0)

	if config.SCK != NoPin {
		config.SCK.configure(PinConfig{Mode: PinOutput}, i2s.sig.bckOut)
	}
	if config.WS != NoPin {
		config.WS.configure(PinConfig{Mode: PinOutput}, i2s.sig.wsOut)
	}
	if config.SDO != NoPin {
		config.SDO.configure(PinConfig{Mode: PinOutput}, i2s.sig.doutOut)
	}
	if config.SDI != NoPin {
		config.SDI.Configure(PinConfig{Mode: PinInput})
		inFunc(i2s.sig.dinIn).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | (uint32(config.SDI) << esp.GPIO_FUNC_IN_SEL_CFG_IN_SEL_Pos))
	}

	bus.SetTX_CONF_TX_RESET(1)
	bus.SetTX_CONF_TX_RESET(0)
	bus.SetTX_CONF_TX_FIFO_RESET(1)
	bus.SetTX_CONF_TX_FIFO_RESET(0)
	bus.SetRX_CONF_RX_RESET(1)
	bus.SetRX_CONF_RX_RESET(0)
	bus.SetRX_CONF_RX_FIFO_RESET(1)
	bus.SetRX_CONF_RX_FIFO_RESET(0)

	bus.SetTX_CONF_TX_UPDATE(1)
	for bus.GetTX_CONF_TX_UPDATE() != 0 {
	}
	bus.SetRX_CONF_RX_UPDATE(1)
	for bus.GetRX_CONF_RX_UPDATE() != 0 {
	}

	txStart := config.Mode == I2SModeSource || config.Mode == I2SModeSourceReceiver
	rxStart := config.Mode == I2SModeReceiver || config.Mode == I2SModeSourceReceiver
	if config.Mode == I2SModeSource {
		rxStart = true
	}
	if txStart {
		bus.SetTX_CONF_TX_UPDATE(1)
		for bus.GetTX_CONF_TX_UPDATE() != 0 {
		}
		bus.SetTX_CONF_TX_START(1)
	}
	if rxStart {
		bus.SetRX_CONF_RX_UPDATE(1)
		for bus.GetRX_CONF_RX_UPDATE() != 0 {
		}
		bus.SetRX_CONF_RX_START(1)
	}
	return nil
}

func i2sClockDivs(pllFreq, sampleRate uint32, format I2SDataFormat) (mclkDiv, bckDiv uint32) {
	slotBits := uint32(16)
	switch format {
	case I2SDataFormat8bit:
		slotBits = 8
	case I2SDataFormat16bit:
		slotBits = 16
	case I2SDataFormat24bit:
		slotBits = 24
	case I2SDataFormat32bit:
		slotBits = 32
	}
	bckFreq := sampleRate * 2 * slotBits
	if bckFreq == 0 {
		return 2, 2
	}
	mclkDiv = pllFreq / (2 * bckFreq)
	if mclkDiv < 2 {
		mclkDiv = 2
	}
	if mclkDiv > 255 {
		mclkDiv = 255
	}
	actualMclk := pllFreq / mclkDiv
	bckDiv = actualMclk / bckFreq
	if bckDiv < 2 {
		bckDiv = 2
	}
	return mclkDiv, bckDiv
}

func (i2s *I2S) SetSampleFrequency(freq uint32) error {
	if freq == 0 {
		return ErrInvalidSampleFrequency
	}
	var format I2SDataFormat = I2SDataFormat16bit
	mclkDiv, bckDiv := i2sClockDivs(i2sPLLFreq, freq, format)
	bus := i2s.bus
	bus.SetTX_CLKM_CONF_TX_CLKM_DIV_NUM(mclkDiv)
	bus.SetTX_CONF1_TX_BCK_DIV_NUM(bckDiv - 1)
	bus.SetRX_CLKM_CONF_RX_CLKM_DIV_NUM(mclkDiv)
	bus.SetRX_CONF1_RX_BCK_DIV_NUM(bckDiv - 1)
	bus.SetTX_CONF_TX_UPDATE(1)
	for bus.GetTX_CONF_TX_UPDATE() != 0 {
	}
	bus.SetRX_CONF_RX_UPDATE(1)
	for bus.GetRX_CONF_RX_UPDATE() != 0 {
	}
	return nil
}

func (i2s *I2S) Enable(enabled bool) {
	if enabled {
		i2s.bus.SetTX_CONF_TX_START(1)
		i2s.bus.SetRX_CONF_RX_START(1)
	} else {
		i2s.bus.SetTX_CONF_TX_START(0)
		i2s.bus.SetRX_CONF_RX_START(0)
	}
}

const (
	i2sGDMAPeriI2S0 = 3
	i2sGDMAPeriI2S1 = 4
)

var i2sTxDesc i2sDmaDesc

// i2sDmaDesc — формат как lldesc_t: word0 = size|length|eof|owner, word1 = buf, word2 = next.
type i2sDmaDesc struct {
	word0 volatile.Register32
	word1 volatile.Register32
	word2 volatile.Register32
}

func (d *i2sDmaDesc) set(buf uintptr, size uint32, eof bool) {
	if size > 4092 {
		size = 4092
	}
	size = size & ^uint32(3)
	if size == 0 {
		size = 4
	}
	w0 := size & 0xfff
	w0 |= (size & 0xfff) << 12
	if eof {
		w0 |= 1 << 30
	}
	w0 |= 1 << 31
	d.word0.Set(w0)
	d.word1.Set(uint32(buf))
	d.word2.Set(0)
}

func (i2s *I2S) gdmaInit() {
	esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(1)
	esp.SYSTEM.SetPERIP_CLK_EN1_DMA_CLK_EN(1)
	esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(0)
	esp.DMA.SetMISC_CONF_CLK_EN(1)
}

func (i2s *I2S) gdmaPeriSel() uint32 {
	if i2s.id == 0 {
		return i2sGDMAPeriI2S0
	}
	return i2sGDMAPeriI2S1
}

func (i2s *I2S) WriteMono(p []uint16) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	size := uint32(len(p) * 2)
	if size > 4092 {
		size = 4092
	}
	n = int(size) / 2
	return n, i2sWriteTx(i2s, unsafe.Pointer(unsafe.SliceData(p)), size)
}

func (i2s *I2S) WriteStereo(p []uint32) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	size := uint32(len(p) * 4)
	if size > 4092 {
		size = 4092
	}
	n = int(size) / 4
	return n, i2sWriteTx(i2s, unsafe.Pointer(unsafe.SliceData(p)), size)
}

func i2sWriteTx(i2s *I2S, buf unsafe.Pointer, size uint32) error {
	i2sTxDesc.set(uintptr(buf), size, true)
	descAddr := uint32(uintptr(unsafe.Pointer(&i2sTxDesc))) & 0xFFFFF

	i2s.gdmaInit()
	ch := i2s.id
	esp.DMA.SetOUT_PERI_SEL_CH0_PERI_OUT_SEL(i2s.gdmaPeriSel())
	if ch == 1 {
		esp.DMA.SetOUT_PERI_SEL_CH1_PERI_OUT_SEL(i2s.gdmaPeriSel())
	}
	switch ch {
	case 0:
		esp.DMA.SetOUT_CONF0_CH0_OUT_RST(1)
		esp.DMA.SetOUT_CONF0_CH0_OUT_RST(0)
		esp.DMA.SetOUT_CONF0_CH0_OUTDSCR_BURST_EN(1)
		esp.DMA.SetOUT_CONF0_CH0_OUT_DATA_BURST_EN(1)
	case 1:
		esp.DMA.SetOUT_CONF0_CH1_OUT_RST(1)
		esp.DMA.SetOUT_CONF0_CH1_OUT_RST(0)
		esp.DMA.SetOUT_CONF0_CH1_OUTDSCR_BURST_EN(1)
		esp.DMA.SetOUT_CONF0_CH1_OUT_DATA_BURST_EN(1)
	}
	i2sGdmaTxClearDone(ch)

	switch ch {
	case 0:
		esp.DMA.SetOUT_LINK_CH0_OUTLINK_ADDR(descAddr)
		esp.DMA.SetOUT_LINK_CH0_OUTLINK_START(1)
	case 1:
		esp.DMA.SetOUT_LINK_CH1_OUTLINK_ADDR(descAddr)
		esp.DMA.SetOUT_LINK_CH1_OUTLINK_START(1)
	}
	i2s.bus.SetTX_CONF_TX_UPDATE(1)
	for i2s.bus.GetTX_CONF_TX_UPDATE() != 0 {
	}
	i2s.bus.SetTX_CONF_TX_START(1)

	for i := 0; i < 1000000; i++ {
		if i2sGdmaTxDone(ch) || i2s.bus.GetINT_RAW_TX_DONE_INT_RAW() != 0 {
			break
		}
	}
	switch ch {
	case 0:
		esp.DMA.SetOUT_LINK_CH0_OUTLINK_STOP(1)
	case 1:
		esp.DMA.SetOUT_LINK_CH1_OUTLINK_STOP(1)
	}
	i2sGdmaTxClearDone(ch)
	return nil
}

func (i2s *I2S) ReadMono(p []uint16) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	size := uint32(len(p) * 2)
	if size > 4092 {
		size = 4092
	}
	n = int(size) / 2
	return n, i2sReadRx(i2s, unsafe.Pointer(unsafe.SliceData(p)), size)
}

func (i2s *I2S) ReadStereo(p []uint32) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	size := uint32(len(p) * 4)
	if size > 4092 {
		size = 4092
	}
	n = int(size) / 4
	return n, i2sReadRx(i2s, unsafe.Pointer(unsafe.SliceData(p)), size)
}

func i2sReadRx(i2s *I2S, buf unsafe.Pointer, size uint32) error {
	size = (size + 3) & ^uint32(3)
	numWords := size / 4
	i2s.bus.SetRXEOF_NUM_RX_EOF_NUM(numWords)

	var desc i2sDmaDesc
	desc.set(uintptr(buf), size, true)
	descAddr := uint32(uintptr(unsafe.Pointer(&desc))) & 0xFFFFF

	i2s.gdmaInit()
	ch := i2s.id
	esp.DMA.SetIN_PERI_SEL_CH0_PERI_IN_SEL(i2s.gdmaPeriSel())
	if ch == 1 {
		esp.DMA.SetIN_PERI_SEL_CH1_PERI_IN_SEL(i2s.gdmaPeriSel())
	}
	i2sGdmaRxClearDone(ch)
	switch ch {
	case 0:
		esp.DMA.SetIN_LINK_CH0_INLINK_ADDR(descAddr)
		esp.DMA.SetIN_LINK_CH0_INLINK_START(1)
	case 1:
		esp.DMA.SetIN_LINK_CH1_INLINK_ADDR(descAddr)
		esp.DMA.SetIN_LINK_CH1_INLINK_START(1)
	}

	for i := 0; i < 1000000; i++ {
		if i2sGdmaRxDone(ch) || i2s.bus.GetINT_RAW_RX_DONE_INT_RAW() != 0 {
			break
		}
	}
	switch ch {
	case 0:
		esp.DMA.SetIN_LINK_CH0_INLINK_STOP(1)
	case 1:
		esp.DMA.SetIN_LINK_CH1_INLINK_STOP(1)
	}
	i2sGdmaRxClearDone(ch)
	return nil
}

func i2sBusFromBase(base uintptr) *esp.I2S_Type {
	return (*esp.I2S_Type)(unsafe.Pointer(base))
}
