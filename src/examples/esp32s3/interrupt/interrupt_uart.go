package main

import (
	"machine"
	"time"
)

// Пример: UART Interrupt Test для ESP32-S3
// Отправляет приветственное сообщение через UART
// и ожидает входящих данных

func main() {
	println("=== ESP32-S3 UART Interrupt Test ===")

	// Инициализация UART0
	// GPIO43 = TX, GPIO44 = RX
	uart := machine.UART0
	uart.Configure(machine.UARTConfig{
		BaudRate: 115200,
		TX:       machine.GPIO43,
		RX:       machine.GPIO44,
	})

	println("UART configured at 115200 baud")

	// Приветственное сообщение
	msg := "=== UART Ready ===\nType something and press Enter:\n"
	uart.Write([]byte(msg))

	// Буфер для приема
	buffer := make([]byte, 256)
	bufIdx := 0

	// Основной цикл
	counter := 0
	for {
		counter++

		// Проверяем наличие данных в UART
		if uart.Buffered() > 0 {
			b, err := uart.ReadByte()
			if err == nil {
				// Эхо-возврат байта
				uart.WriteByte(b)

				// Сохраняем в буфер
				if b == '\r' || b == '\n' {
					// Полная строка
					if bufIdx > 0 {
						println("Received:", string(buffer[:bufIdx]))
						uart.Write([]byte("OK\n"))
						bufIdx = 0
					}
				} else {
					buffer[bufIdx] = b
					bufIdx++
					if bufIdx >= len(buffer) {
						bufIdx = 0
					}
				}
			}
		}

		// Периодический вывод для проверки что программа живая
		if counter%100 == 0 {
			uart.Write([]byte(".\n"))
		}

		time.Sleep(10 * time.Millisecond)
	}
}
