//go:build esp32c3 && !m5stamp_c3

package machine

import (
	"device/esp"
)

const (
	I2S0_BCK_OUT_IDX  = 13
	I2S0_WS_OUT_IDX   = 14
	I2S0_SD_OUT_IDX   = 15
	I2S0_SD_IN_IDX    = 15
	I2S0_MCLK_OUT_IDX = 12
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
)

func enableI2S1Clock() {}

func i2sGdmaTxDone(ch uint8) bool {
	return esp.DMA.GetINT_RAW_CH0_OUT_EOF() != 0
}

func i2sGdmaRxDone(ch uint8) bool {
	return esp.DMA.GetINT_RAW_CH0_IN_SUC_EOF() != 0
}

func i2sGdmaTxClearDone(ch uint8) {
	esp.DMA.SetINT_CLR_CH0_OUT_EOF(1)
}

func i2sGdmaRxClearDone(ch uint8) {
	esp.DMA.SetINT_CLR_CH0_IN_SUC_EOF(1)
}
