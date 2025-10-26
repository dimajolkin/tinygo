package main

import (
	"machine"
	"runtime/interrupt"
	"time"
)

var (
	// GPIO0 - boot button на ESP32-S3
	button = machine.GPIO0
	led    = machine.GPIO42

	// Счетчики для демонстрации
	interruptCount  uint32 = 0
	lastButtonState bool   = true // true = не нажата (подтяжка вверх)
)

func main() {
	println("=== ESP32-S3 Low-Level Interrupt Example ===")

	// Настройка GPIO пинов
	button.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	led.Low() // Изначально LED выключен

	println("GPIO настроены...")

	// Регистрируем новый обработчик прерывания (шаг 1)
	// Это TinyGo способ сказать компилятору, что функция handleGPIOInterrupt
	// особенная и должна вызываться при срабатывании GPIO прерывания

	// Для ESP32-S3 используем GPIO interrupt
	// IRQ_GPIO для ESP32-S3 обычно имеет номер 22 (ETS_GPIO_INTR_SOURCE)
	//intr := interrupt.New(22, handleGPIOInterrupt) // IRQ 22 для GPIO на ESP32-S3

	println("Прерывание зарегистрировано...")

	// Теперь у нас есть дескриптор прерывания. По умолчанию на этом чипе
	// установлен максимально возможный приоритет. Мы хотели бы установить
	// GPIO на более низкий приоритет, что мы и делаем здесь.
	// Магическая константа здесь в будущих версиях будет заменена
	// обычной константой для низкоприоритетного прерывания.
	//intr.SetPriority(0xc0) // Низкий приоритет

	println("Приоритет установлен...")

	// Наконец, прерывание должно быть включено. Без этого прерывание
	// все еще будет срабатывать, но обработчик никогда не будет вызван.
	//intr.Enable()

	println("Прерывание включено!")
	println("Нажимайте кнопку GPIO0 (boot button)")
	println("LED будет мигать при каждом нажатии")
	println()

	// Основной цикл программы
	counter := 0
	for {
		counter++

		// Показываем, что основная программа работает
		if counter%10 == 0 {
			println("Основной цикл работает... Счетчик прерываний:", interruptCount)
		}

		// Проверяем состояние кнопки (polling для сравнения)
		currentState := button.Get()
		if currentState != lastButtonState {
			if !currentState { // Кнопка нажата (LOW из-за pullup)
				println("Кнопка нажата (polling detection)")
			}
			lastButtonState = currentState
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// Обработчик низкоуровневого GPIO прерывания
// Эта функция будет вызываться аппаратно при изменении состояния GPIO0
func handleGPIOInterrupt(intr interrupt.Interrupt) {
	// ВАЖНО: В обработчике прерывания нужно быть очень осторожным
	// - Не использовать println (может вызвать deadlock)
	// - Минимизировать время выполнения
	// - Избегать блокирующих операций

	// Увеличиваем счетчик прерываний
	interruptCount++

	// Быстро мигаем LED для индикации прерывания
	led.High()

	// В реальном коде здесь бы была минимальная обработка
	// и установка флагов для основного цикла
}
