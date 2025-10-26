package main

import (
	"machine"
	"time"
)

// Пример: Timer Interrupt Test для ESP32-S3
// Демонстрирует работу таймерных прерываний
// LED мигает с периодом 1 секунда

var (
	led        = machine.GPIO42
	tickCount  uint32
	lastSecond uint32
)

func main() {
	println("=== ESP32-S3 Timer Interrupt Test ===")

	// Инициализация LED (GPIO42)
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	led.Low()

	println("LED configured on GPIO42")

	// Основной цикл
	counter := 0
	for {
		counter++

		// Получаем текущее время в миллисекундах
		now := uint32(time.Since(time.Time{}).Milliseconds())

		// Каждые 1000 мс (1 сек)
		if now-lastSecond >= 1000 {
			lastSecond = now
			tickCount++

			// Переключаем LED
			if tickCount%2 == 0 {
				led.High()
				println("LED ON - Tick:", tickCount)
			} else {
				led.Low()
				println("LED OFF - Tick:", tickCount)
			}
		}

		// Периодический вывод состояния
		if counter%1000 == 0 {
			println("Main loop running... Ticks:", tickCount)
		}

		time.Sleep(1 * time.Millisecond)
	}
}
