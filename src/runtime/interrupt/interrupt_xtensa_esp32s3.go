//go:build esp32s3

package interrupt

import (
	"device"
	"errors"
	"unsafe"
)

// Enable register CPU interrupt with interrupt.Interrupt for ESP32-S3.
// Uses REGISTER-ONLY approach like ESP32-C3 (no ROM functions).
func (i Interrupt) Enable() error {
	println("=== INTERRUPT.ENABLE (РЕГИСТРЫ) ===")
	println("Включаем CPU interrupt", i.num, "через РЕГИСТРЫ")

	if i.num < 0 || i.num > 31 {
		return errors.New("interrupt for ESP32-S3 must be in range of 0 through 31")
	}

	// ЭТАП 1: Отключаем прерывания на время настройки
	println("Отключаем прерывания...")
	oldPS := disableInterrupts()
	defer enableInterrupts(oldPS)

	// ЭТАП 2: Включаем CPU interrupt через Xtensa INTENABLE
	println("Включаем CPU interrupt", i.num, "в INTENABLE...")
	err := enableCPUInterrupt(i.num)
	if err != nil {
		return err
	}

	println("CPU interrupt", i.num, "ВКЛЮЧЕН через регистры! ✅")
	return nil
}

// disableInterrupts отключает прерывания и возвращает старое значение PS
func disableInterrupts() uint32 {
	// Используем rsil как в interrupt_xtensa.go
	return uint32(device.AsmFull("rsil {}, 15", nil))
}

// enableInterrupts восстанавливает прерывания
func enableInterrupts(oldPS uint32) {
	// Используем wsr PS как в interrupt_xtensa.go
	device.AsmFull("wsr {state}, PS", map[string]interface{}{
		"state": oldPS,
	})
}

// enableCPUInterrupt включает CPU interrupt в Xtensa INTENABLE
func enableCPUInterrupt(interruptNum int) error {
	println("enableCPUInterrupt: включаем interrupt", interruptNum)

	// Читаем текущий INTENABLE - возвращаем значение напрямую
	currentINTENABLE := uint32(device.AsmFull("rsr {}, INTENABLE", nil))
	println("Текущий INTENABLE:", currentINTENABLE)

	// Включаем наш бит
	mask := uint32(1 << interruptNum)
	newINTENABLE := currentINTENABLE | mask

	// Записываем новое значение
	device.AsmFull("wsr {intenable}, INTENABLE", map[string]interface{}{
		"intenable": newINTENABLE,
	})

	// Проверяем результат
	resultINTENABLE := uint32(device.AsmFull("rsr {}, INTENABLE", nil))
	println("Новый INTENABLE:", resultINTENABLE)

	if (resultINTENABLE & mask) != 0 {
		println("CPU interrupt", interruptNum, "успешно включен!")
		return nil
	} else {
		println("ОШИБКА: CPU interrupt", interruptNum, "не включился!")
		return errors.New("failed to enable CPU interrupt")
	}
}

// getINTENABLE читает регистр INTENABLE процессора Xtensa
func getINTENABLE() uint32 {
	// БЕЗОПАСНАЯ ЗАГЛУШКА - TinyGo assembly зависает
	println("getINTENABLE: заглушка - TinyGo не поддерживает rsr.intenable")
	return 0
}

// setINTENABLE записывает в регистр INTENABLE процессора Xtensa
func setINTENABLE(value uint32) {
	// Используем ROM функцию ets_isr_unmask вместо прямого assembly
	println("setINTENABLE: используем ROM ets_isr_unmask вместо assembly")

	// Вызываем ROM функцию ets_isr_unmask(interrupt_num)
	callROMUnmask(19) // Включаем CPU interrupt 19

	println("setINTENABLE: ROM ets_isr_unmask(19) вызван! 🎉")
}

// callROMUnmask вызывает ROM функцию ets_isr_unmask
func callROMUnmask(interruptNum int) {
	// Адрес ROM функции ets_isr_unmask = 0x40001b90
	const ROM_ETS_ISR_UNMASK_ADDR = 0x40001b90

	println("callROMUnmask: РЕАЛЬНЫЙ вызов ROM ets_isr_unmask(", interruptNum, ")")

	// Используем ROM функцию из runtime
	callROMEtsIsrUnmask(interruptNum)

	println("callROMUnmask: ROM ets_isr_unmask выполнен! 🚀")
}

// callROMEtsIsrUnmask - обертка для ROM функции из runtime
func callROMEtsIsrUnmask(interruptNum int) {
	println("callROMEtsIsrUnmask: используем нашу xt_ints_on реализацию!")

	// Вызываем нашу assembly реализацию xt_ints_on
	// Передаем маску бита для interrupt 19: (1 << 19) = 524288
	mask := uint32(1 << interruptNum)
	result := xtIntsOn(mask)

	println("xt_ints_on(", mask, ") вернул:", result)
	println("CPU interrupt", interruptNum, "включен через assembly! 🎉")
}

// xtIntsOn - заглушка, реальная реализация в runtime
func xtIntsOn(mask uint32) uint32 {
	println("xtIntsOn: вызываем runtime.xtIntsOn!")
	return runtime_xtIntsOn(mask)
}

//go:linkname runtime_xtIntsOn runtime.xtIntsOn
func runtime_xtIntsOn(mask uint32) uint32

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
		// ИНДИКАТОР: входим в callHandlers
		*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << 8) // GPIO8 ON
		
		callHandlers(19)
		
		// Если дошли сюда - callHandlers завершился успешно
		// (без дополнительного индикатора, GPIO7 покажет это)
	}
}

//export handleInterrupt
func handleInterrupt() {
	// ESP32-S3 XTENSA INTERRUPT DISPATCHER - С ДИАГНОСТИКОЙ!
	
	// ИНДИКАТОР 1: handleInterrupt вызван
	// GPIO5 = HIGH означает "handleInterrupt вызван"
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << 5) // GPIO5 ON
	
	// ВРЕМЕННАЯ ЗАГЛУШКА: предполагаем что это наше прерывание 19
	interruptNumber := 19
	
	// ИНДИКАТОР 2: перед callHandler
	// GPIO6 = HIGH означает "перед callHandler"
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << 6) // GPIO6 ON
	
	// Вызываем зарегистрированные обработчики TinyGo
	callHandler(interruptNumber)
	
	// ИНДИКАТОР 3: после callHandler
	// GPIO7 = HIGH означает "после callHandler"
	*(*uint32)(unsafe.Pointer(uintptr(0x60004008))) = (1 << 7) // GPIO7 ON
	
	// handleInterrupt завершен
}
