//go:build esp32s3

package machine

import (
	"device/esp"
)

// soc/esp32s3/include/soc/gpio_sig_map.h (I2S0O_*, I2S0I_*, I2S1O_*, I2S1I_*).
const (
	I2S0_BCK_OUT_IDX  = 22 // I2S0O_BCK_OUT_IDX
	I2S0_WS_OUT_IDX   = 24 // I2S0O_WS_OUT_IDX
	I2S0_SD_OUT_IDX   = 25 // I2S0O_SD_OUT_IDX
	I2S0_SD_IN_IDX    = 25 // I2S0I_SD_IN_IDX
	I2S0_MCLK_OUT_IDX = 23 // I2S0_MCLK_OUT_IDX

	I2S1_BCK_OUT_IDX  = 28 // I2S1O_BCK_OUT_IDX
	I2S1_WS_OUT_IDX   = 29 // I2S1O_WS_OUT_IDX
	I2S1_SD_OUT_IDX   = 30 // I2S1O_SD_OUT_IDX
	I2S1_SD_IN_IDX    = 30 // I2S1I_SD_IN_IDX (не 31)
	I2S1_MCLK_OUT_IDX = 21 // I2S1_MCLK_OUT_IDX
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

func i2sGdmaTxClearDone(ch uint8) {
	switch ch {
	case 0:
		esp.DMA.SetOUT_INT_CLR_CH0_OUT_EOF(1)
	case 1:
		esp.DMA.SetOUT_INT_CLR_CH1_OUT_EOF(1)
	}
}

func i2sGdmaRxClearDone(ch uint8) {
	switch ch {
	case 0:
		esp.DMA.SetIN_INT_CLR_CH0_IN_SUC_EOF(1)
	case 1:
		esp.DMA.SetIN_INT_CLR_CH1_IN_SUC_EOF(1)
	}
}
