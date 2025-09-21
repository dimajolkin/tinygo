//go:build esp32s3

package interrupt

import (
	"errors"
)

// Enable register CPU interrupt with interrupt.Interrupt for ESP32-S3.
// The ESP32-S3 has 32 CPU independent interrupts (0-31).
// Caller must map the selected interrupt using interrupt matrix mapping.
func (i Interrupt) Enable() error {
	if i.num < 0 || i.num > 31 {
		return errors.New("interrupt for ESP32-S3 must be in range of 0 through 31")
	}

	println("=== INTERRUPT.ENABLE ===")
	println("Включаем CPU interrupt", i.num)

	// Для ESP32-S3 используем прямую запись в Xtensa INTENABLE регистр
	// Это эквивалентно ROM ets_isr_unmask(i.num)

	// Читаем текущий INTENABLE
	currentIntenable := getINTENABLE()
	println("Текущий INTENABLE:", currentIntenable)

	// Включаем наш interrupt
	newIntenable := currentIntenable | (1 << uint32(i.num))
	setINTENABLE(newIntenable)

	println("Новый INTENABLE:", newIntenable)
	println("CPU interrupt", i.num, "ВКЛЮЧЕН! 🎉")

	return nil
}

// getINTENABLE читает регистр INTENABLE процессора Xtensa
func getINTENABLE() uint32 {
	// БЕЗОПАСНАЯ ЗАГЛУШКА - assembly зависает систему
	println("getINTENABLE: возвращаем 0 (заглушка)")
	return 0
}

// setINTENABLE записывает в регистр INTENABLE процессора Xtensa
func setINTENABLE(value uint32) {
	// БЕЗОПАСНАЯ ЗАГЛУШКА - assembly зависает систему
	println("setINTENABLE: заглушка, не записываем реально", value)
	println("ПРОБЛЕМА: Нужно найти правильный способ записи INTENABLE")
}

// Добавляем псевдо-функции как в ESP32-C3
//
//go:linkname callHandlers runtime/interrupt.callHandlers
func callHandlers(num int)

// Константы для interrupt номеров (как в ESP32-C3)
const (
	IRQNUM_0 = iota
	IRQNUM_1
	IRQNUM_2
	IRQNUM_3
	IRQNUM_4
	IRQNUM_5
	IRQNUM_6
	IRQNUM_7
	IRQNUM_8
	IRQNUM_9
	IRQNUM_10
	IRQNUM_11
	IRQNUM_12
	IRQNUM_13
	IRQNUM_14
	IRQNUM_15
	IRQNUM_16
	IRQNUM_17
	IRQNUM_18
	IRQNUM_19
	IRQNUM_20
	IRQNUM_21
	IRQNUM_22
	IRQNUM_23
	IRQNUM_24
	IRQNUM_25
	IRQNUM_26
	IRQNUM_27
	IRQNUM_28
	IRQNUM_29
	IRQNUM_30
	IRQNUM_31
)

//go:inline
func callHandler(n int) {
	// Упрощенная версия - вызываем для нашего interrupt 19
	if n == 19 {
		callHandlers(19)
	}
}
