//go:build esp32s3

package machine

import (
	"device/esp"
)

const (
	I2S0_BCK_OUT_IDX  = 22
	I2S0_WS_OUT_IDX   = 24
	I2S0_SD_OUT_IDX   = 25
	I2S0_SD_IN_IDX   = 25
	I2S0_MCLK_OUT_IDX = 23

	I2S1_BCK_OUT_IDX  = 28
	I2S1_WS_OUT_IDX   = 29
	I2S1_SD_OUT_IDX   = 30
	I2S1_SD_IN_IDX   = 31
	I2S1_MCLK_OUT_IDX = 32
)

var (
	I2S0 = &I2S{
		bus: esp.I2S0,
		sig: i2sSignals{
			bckOut:  I2S0_BCK_OUT_IDX,
			wsOut:   I2S0_WS_OUT_IDX,
			doutOut: I2S0_SD_OUT_IDX,
			dinIn:   I2S0_SD_IN_IDX,
			mclkOut: I2S0_MCLK_OUT_IDX,
		},
		id: 0,
	}
	I2S1 = &I2S{
		bus: i2sBusFromBase(0x6002d000),
		sig: i2sSignals{
			bckOut:  I2S1_BCK_OUT_IDX,
			wsOut:   I2S1_WS_OUT_IDX,
			doutOut: I2S1_SD_OUT_IDX,
			dinIn:   I2S1_SD_IN_IDX,
			mclkOut: I2S1_MCLK_OUT_IDX,
		},
		id: 1,
	}
)

func init() {
	enableI2S1ClockFunc = func() {
		esp.SYSTEM.SetPERIP_RST_EN0_I2S1_RST(1)
		esp.SYSTEM.SetPERIP_CLK_EN0_I2S1_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN0_I2S1_RST(0)
	}
}

func i2sGdmaTxDone(ch uint8) bool {
	switch ch {
	case 0:
		return esp.DMA.GetOUT_INT_RAW_CH0_OUT_EOF() != 0
	case 1:
		return esp.DMA.GetOUT_INT_RAW_CH1_OUT_EOF() != 0
	}
	return false
}

func i2sGdmaRxDone(ch uint8) bool {
	switch ch {
	case 0:
		return esp.DMA.GetIN_INT_RAW_CH0_IN_SUC_EOF() != 0
	case 1:
		return esp.DMA.GetIN_INT_RAW_CH1_IN_SUC_EOF() != 0
	}
	return false
}
