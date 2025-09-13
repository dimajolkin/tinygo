//go:build esp32s3

package machine

import (
	"device/esp"
	"runtime/volatile"
	"unsafe"
)

// ESP32-S3 IO MUX experimental functionality
// This file contains IO MUX direct connection code for potential future use
// Currently not used in production - GPIO Matrix approach is used instead

// configureSPIIOIMUX настраивает SPI пины через IO MUX для прямого подключения
// Экспериментальная функция - может давать лучшее качество сигнала чем GPIO Matrix
func configureSPIIOIMUX(config SPIConfig) {
	println("DEBUG: Configuring IO MUX for direct SPI hardware connection")

	// Определяем функцию IO MUX в зависимости от набора пинов
	var function uint32
	if config.SCK == 36 && config.SDO == 35 {
		function = 4 // Try function 4 instead of 2 - maybe ESP32-S3 uses same function for both pin sets
		println("DEBUG: Using Octal SPI pin set (function 4 - testing)")
	} else if config.SCK == 12 && config.SDO == 11 {
		function = 4 // SPI2_FUNC_NUM - Standard SPI pins
		println("DEBUG: Using standard SPI pin set (function 4)")
	} else {
		println("ERROR: Unsupported IO MUX pin combination")
		return
	}

	// Настраиваем IO MUX регистры напрямую
	// IO_MUX base: 0x60009000
	if config.SCK != NoPin {
		println("DEBUG: Setting up SCK pin", uint8(config.SCK), "with IO MUX function", function)
		configureIOIMUXPin(config.SCK, function, true) // output
	}
	if config.SDO != NoPin {
		println("DEBUG: Setting up SDO pin", uint8(config.SDO), "with IO MUX function", function)
		configureIOIMUXPin(config.SDO, function, true) // output
	}
	if config.SDI != NoPin {
		println("DEBUG: Setting up SDI pin", uint8(config.SDI), "with IO MUX function", function)
		configureIOIMUXPin(config.SDI, function, false) // input
	}
	if config.CS != NoPin {
		println("DEBUG: Setting up CS pin", uint8(config.CS), "with IO MUX function", function)
		configureIOIMUXPin(config.CS, function, true) // output
	}
}

// configureIOIMUXPin настраивает один пин через IO MUX для прямого подключения
// Экспериментальная функция - альтернатива GPIO Matrix
func configureIOIMUXPin(pin Pin, function uint32, isOutput bool) {
	// IO_MUX регистр для каждого пина: IO_MUX_GPIOn_REG
	// Базовый адрес: 0x60009000 + pin_offset
	// Смещения для пинов можно найти в soc/io_mux_reg.h

	var iomuxReg *volatile.Register32
	switch pin {
	case 10:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x60009040))) // IO_MUX_GPIO10_REG
	case 11:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x60009044))) // IO_MUX_GPIO11_REG
	case 12:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x60009048))) // IO_MUX_GPIO12_REG
	case 13:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x6000904C))) // IO_MUX_GPIO13_REG
	case 34:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x6000908C))) // IO_MUX_GPIO34_REG
	case 35:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x60009090))) // IO_MUX_GPIO35_REG
	case 36:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x60009094))) // IO_MUX_GPIO36_REG
	case 37:
		iomuxReg = (*volatile.Register32)(unsafe.Pointer(uintptr(0x60009098))) // IO_MUX_GPIO37_REG
	default:
		println("ERROR: Pin", uint8(pin), "not supported for IO MUX")
		return
	}

	// Читаем текущее значение
	current := iomuxReg.Get()

	// Настраиваем IO MUX регистр
	// Биты 12-14: MCU_SEL (function select)
	// Бит 8: FUN_PU (pull-up enable)
	// Биты 10-11: FUN_DRV (drive strength)
	// Бит 9: FUN_PD (pull-down enable) - должен быть 0
	newValue := current & ^uint32(0x7000) // Очищаем MCU_SEL (биты 12-14)
	newValue &= ^uint32(0x200)            // Очищаем FUN_PD (бит 9)
	newValue |= function << 12            // Устанавливаем функцию
	newValue |= 1 << 8                    // Включаем pull-up (FUN_PU)
	newValue |= 3 << 10                   // Максимальная сила тока (FUN_DRV = 3)

	println("DEBUG: GPIO", uint8(pin), "setting function", function, "drive=3, pullup=1")
	iomuxReg.Set(newValue)

	// Проверяем что записалось
	verify := iomuxReg.Get()
	// Debug removed
	if verify != newValue {
		println("WARNING: GPIO", uint8(pin), "IO MUX verification failed!")
	}

	// Включаем GPIO как выход/вход
	if isOutput {
		if pin < 32 {
			esp.GPIO.ENABLE_W1TS.Set(1 << pin)
		} else {
			esp.GPIO.ENABLE1_W1TS.Set(1 << (pin - 32))
		}
		println("DEBUG: GPIO", uint8(pin), "enabled as output")
	} else {
		// Для входа просто убеждаемся, что он не в режиме выхода
		if pin < 32 {
			esp.GPIO.ENABLE_W1TC.Set(1 << pin)
		} else {
			esp.GPIO.ENABLE1_W1TC.Set(1 << (pin - 32))
		}
		println("DEBUG: GPIO", uint8(pin), "configured as input")
	}
}

// configurePinForSPIIOIMUX - альтернативная версия configurePinForSPI с прямым IO MUX
// Экспериментальная функция для тестирования качества сигнала
func configurePinForSPIIOIMUX(pin Pin, signal uint32, mode PinMode) {
	if pin == NoPin {
		return
	}

	pinNum := uint32(pin)

	// Enable GPIO output/input
	if mode == PinOutput {
		esp.GPIO.ENABLE_W1TS.Set(1 << pinNum)
	}

	// Try IO MUX direct connection for standard SPI pins (like Arduino)
	// This should give better signal quality than GPIO Matrix
	var useDirectIOIMUX bool
	var iomuxFunction uint32

	if pin == 12 { // SCK
		useDirectIOIMUX = true
		iomuxFunction = 4 // SPI2_CLK function
	} else if pin == 11 { // MOSI
		useDirectIOIMUX = true
		iomuxFunction = 4 // SPI2_D function
	} else if pin == 13 { // MISO
		useDirectIOIMUX = true
		iomuxFunction = 4 // SPI2_Q function
	} else if pin == 10 { // CS
		useDirectIOIMUX = true
		iomuxFunction = 4 // SPI2_CS0 function
	}

	// Configure IO MUX register
	iomuxAddr := uintptr(0x60009018 + pinNum*4)
	iomux := (*volatile.Register32)(unsafe.Pointer(iomuxAddr))

	if useDirectIOIMUX {
		// Direct IO MUX connection (like Arduino) - stronger signal
		muxConfig := (iomux.Get() & ^uint32(0x7000)) | (iomuxFunction << 12) | (1 << 8) | (3 << 10)
		if mode == PinInput {
			muxConfig |= 1 << 7 // Enable pull-up for input pins only
		}
		iomux.Set(muxConfig)
		// Don't configure GPIO Matrix for direct IO MUX pins
		return
	} else {
		// Fallback to GPIO Matrix for non-standard pins
		muxConfig := (iomux.Get() & ^uint32(0x7000)) | (2 << 12) | (1 << 8) | (3 << 10)
		if mode == PinOutput {
			muxConfig |= 1 << 7 // Enable pull-up for output pins
		}
		iomux.Set(muxConfig)
	}

	// Route signal through GPIO Matrix (only for non-direct IO MUX pins)
	if !useDirectIOIMUX {
		outFuncAddr := unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_OUT_SEL_CFG), uintptr(pinNum)*4)
		outFunc := (*volatile.Register32)(outFuncAddr)
		outFunc.Set(signal)
	}
}

// Примечания по использованию IO MUX:
//
// Преимущества IO MUX:
// - Прямое аппаратное подключение SPI периферии к пинам
// - Более сильные сигналы (лучше drive capability)
// - Меньше задержки (нет промежуточной GPIO Matrix)
// - Такой же подход как в Arduino/ESP-IDF
//
// Недостатки IO MUX:
// - Работает только для стандартных SPI пинов
// - Менее гибкий чем GPIO Matrix
// - Сложнее в настройке и отладке
//
// Использование:
// Чтобы использовать IO MUX вместо GPIO Matrix, замените вызов
// configurePinForSPI() на configurePinForSPIIOIMUX() в основном SPI драйвере
