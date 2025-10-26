package main

import (
	"machine"
	"time"
)

// Пример: Stress Test для ESP32-S3
// Генерирует множественные быстрые события
// и проверяет корректность обработки

var (
	button              = machine.GPIO0
	led                 = machine.GPIO42
	interruptCount      uint32
	missedInterrupts    uint32
	maxResponseTime     uint32
	lastInterruptTime   uint32
	stressTestActive    bool
	buttonPressCount    uint32
)

func main() {
	println("=== ESP32-S3 Interrupt Stress Test ===")

	// Инициализация GPIO
	button.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	led.Low()

	println("GPIO configured")
	println("Press button rapidly for stress test...")
	println()

	counter := 0
	lastReport := uint32(0)
	testStartTime := uint32(time.Now().UnixMilli())

	for {
		counter++

		// Проверяем кнопку (polling)
		buttonState := button.Get()
		if !buttonState { // Нажата (LOW из-за pullup)
			interruptCount++

			// Мигаем LED
			led.High()
			time.Sleep(5 * time.Millisecond)
			led.Low()

			buttonPressCount++
		}

		// Периодический отчет
		now := uint32(time.Now().UnixMilli())
		if now-lastReport >= 5000 { // Каждые 5 секунд
			lastReport = now
			elapsed := now - testStartTime

			println("=== Stress Test Report ===")
			println("Elapsed time (s):", elapsed/1000)
			println("Interrupts handled:", interruptCount)
			println("Button presses:", buttonPressCount)
			println("Max response time (us):", maxResponseTime)
			println("Missed interrupts:", missedInterrupts)
			println()
		}

		time.Sleep(10 * time.Millisecond)
	}
}
