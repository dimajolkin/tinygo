package main

import (
	"machine"
	"time"
)

var (
	// Используем встроенную кнопку и LED для ESP32-S3
	button = machine.GPIO0  // Boot button (GPIO0) - стандартная кнопка на ESP32-S3
	led    = machine.GPIO42 // RGB LED (GPIO48) - обычно используется для RGB LED

	// Альтернативные пины для разных плат ESP32-S3:
	// button = machine.GPIO1   // Требует внешнюю подтяжку +3.3V через резистор 10кОм
	// button = machine.GPIO9   // Если GPIO0 не работает
	// led    = machine.GPIO2   // Стандартный LED на некоторых платах
	// led    = machine.GPIO38  // RGB LED на других платах

	// Счетчик нажатий
	pressCount int64 = 0
)

func main() {
	println("Start app!")
	// Настраиваем пины
	button.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	led.High()

	println("Init..")
	// Устанавливаем прерывание на кнопку
	err := button.SetInterrupt(machine.PinFalling, handleButtonPress)
	if err != nil {
		println("Error setting interrupt:", err.Error())
	}

	println("ESP32-S3 Interrupt Example")
	println("Press the boot button (GPIO0) to toggle LED")
	println("Starting main loop...")

	// Основной цикл - мигаем LED медленно
	counter := 0
	for {
		counter++
		println("Loop iteration:", counter, "- LED High")
		led.High()

		println("About to sleep 500ms...")
		time.Sleep(500 * time.Millisecond)
		println("Sleep 1 completed")

		println("Loop iteration:", counter, "- LED Low")
		led.Low()

		println("About to sleep 500ms again...")
		time.Sleep(500 * time.Millisecond)
		println("Sleep 2 completed")

		println("tick - iteration", counter, "completed")

		if !button.Get() {
			println("Button Pressed")
		}
		// Выводим количество нажатий каждые 2 секунды
		if pressCount > 0 {
			println("Button pressed", pressCount, "times")
			break
		}
	}
}

// Обработчик прерывания от кнопки
func handleButtonPress(pin machine.Pin) {
	pressCount++
}
