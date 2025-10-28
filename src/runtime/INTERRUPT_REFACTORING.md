# ESP32-S3 Runtime Interrupt Code Refactoring

## Overview

В целях повышения читаемости и организации кода, функции тестирования и диагностики прерываний были перенесены из основного файла `runtime_esp32s3.go` в отдельный файл `runtime_esp32s3_interrupt.go`.

## File Organization

### `runtime_esp32s3.go` (838 строк)
Основной файл runtime для ESP32-S3, содержит:
- **Инициализация системы**: `main()`, `clearbss()`, `disableWatchdogs()`
- **Управление памятью**: `initGPIOPeripherals()`, `initSPIPeripherals()`, `initTimer()`
- **Планировщик и стек**: `ticks()`, `sleepTicks()`, функции для работы со стеком
- **Утилиты**: `exit()`, `abort()`, `putchar()`, `getchar()`
- **Вспомогательные регистровые операции**: `readPS()`, `writePS()`, `readINTERRUPT()` (дублированы для независимости)

### `runtime_esp32s3_interrupt.go` (651 строка)
Файл с функциями тестирования и диагностики прерываний для разработки, содержит:

#### Тестовые функции
- `testDirectCallToHandleInterrupt()` - Test 1: прямой вызов handleInterrupt
- `testSoftwareInterrupt()` - Test 2: программное прерывание через wsr.intset
- `testPSRegisterAndWAITI()` - Test 3: проверка PS регистра и WAITI инструкции
- `testDeepDiagnostics()` - Test 5: глубокая диагностика системы прерываний
- `testGPIOHardwareInterrupt()` - Test 4: аппаратное прерывание от GPIO

#### Функции мониторинга
- `prepareInterruptMonitoring()` - подготовка SYSTIMER для безопасного мониторинга
- `monitorBackgroundInterrupts()` - мониторинг активности ISR во времени

#### Функции валидации и отладки
- `validateVectorTableLayout()` - проверка соответствия ESP-IDF стандартам
- `dumpDiagnosticInfo()` - полный дамп диагностической информации
- `dumpVectorTable()` - отладка таблицы векторов
- `dumpVectorTableLayout()` - детальный вывод памяти таблицы векторов
- `dumpCacheState()` - состояние кэш регистров

#### Вспомогательные функции
- `hexString()` - преобразование uint32 в hex строку
- `initDebugPin41()` - инициализация GPIO41 для отладки

#### Переменные отладки
- `systimerTickCount`, `gpio41State`, `debugPin`, `systimerIRQSeen`, `systimerIRQCount`

## Benefits

1. **Разделение ответственности**: основной runtime код отделён от диагностических функций
2. **Лучшая читаемость**: каждый файл имеет чёткий назначение
3. **Удобство разработки**: все тестовые функции в одном месте для быстрого доступа
4. **Минимизация основного файла**: runtime_esp32s3.go остаётся компактным и сосредоточенным
5. **Независимость**: каждый файл содержит необходимые вспомогательные функции (нет жёсткой зависимости)

## Usage

### Основной runtime (production)
```go
import "runtime"
// использует runtime_esp32s3.go
```

### Диагностика и тестирование (development)
```go
// Функции из runtime_esp32s3_interrupt.go доступны в том же пакете
testDeepDiagnostics()
dumpDiagnosticInfo()
```

## Import Dependencies

Оба файла используют:
```go
import (
	"device"
	"device/esp"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)
```

## Build Status

✅ Оба файла компилируются корректно
✅ Функции дублированы по необходимости для независимости
✅ Совместимы с ESP32-S3 целевой платформой (`//go:build esp32s3`)

## Future Improvements

1. Перенести диагностические функции в отдельный пакет `runtime/debug` (если потребуется)
2. Добавить флаги сборки для исключения диагностического кода из production bildsов
3. Расширить набор тестов для других сценариев прерываний
4. Добавить профилирование производительности ISR обработчиков

## Related Files

- `src/device/esp/esp32s3.S` - assembler точки входа прерываний
- `src/device/esp/interrupt_esp32s3.go` - Go интеграция прерываний
- `.cursor/rtos.md` - полная документация архитектуры RTOS
