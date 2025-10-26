# ESP32-S3 GPIO Interrupt Example

## Описание

Этот пример демонстрирует использование GPIO прерываний на ESP32-S3 с TinyGo.

## Пины

- **Кнопка:** GPIO0 (Boot Button) - стандартная кнопка на всех ESP32-S3 платах
- **LED:** GPIO42 - обычно RGB LED на плате (можно использовать альтернативные пины)

## Использование

```bash
tinygo build -target esp32s3 ./src/examples/esp32s3/interrupt/interrupt.go
# Загрузить на плату и открыть последовательный монитор (115200 baud)
```

## Debug Информация

При запуске программа выводит debug информацию для отладки:

```
DEBUG: SetInterrupt called for GPIO 0 change= 2
DEBUG: Callback registered for GPIO 0
DEBUG: Setting up GPIO interrupt...
DEBUG: Mapping GPIO interrupt to CPU interrupt 19
DEBUG: GPIO_INTERRUPT_PRO_MAP set to 19
DEBUG: interrupt.New created, calling Enable()...
DEBUG: Enabling interrupt 19
DEBUG: Reading current PS register...
DEBUG: Current PS value = <value>
DEBUG: New PS value (INTLEVEL=0) = <value>
DEBUG: Interrupt 19 enabled successfully
DEBUG: SetInterrupt complete for GPIO 0
```

Если вы видите эти выводы, значит инициализация прошла успешно. 

Когда вы нажимаете кнопку, вы должны увидеть:

```
DEBUG: gpioHandleInterrupt called!
DEBUG: GPIO.STATUS = <value>
DEBUG: GPIO.STATUS1 = <value>
DEBUG: GPIO 0 interrupt active, mask= 1
DEBUG: Calling callback for GPIO 0
🔔 INTERRUPT! Button pressed - count: 1
DEBUG: Clearing interrupt flags...
DEBUG: gpioHandleInterrupt complete
```

## Возможные проблемы

### Прерывание не срабатывает
1. Убедитесь, что кнопка подключена к GPIO0 и правильно подключена к земле (GND)
2. Проверьте, что внутренний pull-up резистор включен (используем `machine.PinInputPullup`)
3. Проверьте последовательный монитор (должны быть debug выводы)

### DEBUG выводы не появляются
1. Проверьте скорость последовательного порта (115200 baud)
2. Убедитесь, что используется правильный COM/TTY порт
3. Попробуйте перезагрузить плату

### GPIO.STATUS всегда 0
Это может означать, что:
1. GPIO interrupt не правильно сконфигурирован на аппаратном уровне
2. GPIO_PIN_INT_ENA не установлен
3. GPIO_INTERRUPT_PRO_MAP не правильно отображен

## Альтернативные пины

Вы можете использовать другие пины, изменив константы в `esp32s3_config.go`:

- **Кнопка:** GPIO1, GPIO9 и другие (требуют внешнего pull-up резистора)
- **LED:** GPIO2, GPIO38, GPIO48

## Известные ограничения

- На Xtensa архитектуре управление прерываниями осуществляется через регистры INTLEVEL и INTENABLE
- GPIO interrupt сопоставляется с CPU interrupt level 19
- Максимальная поддержка 49 GPIO пинов (GPIO0-GPIO48)
