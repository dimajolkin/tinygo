//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Xtensa Exception and Interrupt Handlers for ESP32-S3
 * 
 * Этот файл содержит обработчики исключений и прерываний
 * для ESP32-S3 с поддержкой вложенных прерываний.
 */

// Глобальные переменные состояния прерываний
// (объявлены в Go коде)
.extern _frxt_int_depth
.extern _frxt_int_handlers
.extern handleInterrupt
.extern handleException

/*
 * _frxt_int_enter - Вход в обработчик прерывания
 * 
 * Назначение:
 *   - Увеличить глубину вложенности прерываний
 *   - Сохранить текущий уровень прерывания
 *   - Разрешить более высокоприоритетные прерывания
 * 
 * Входные параметры:
 *   a2 = номер уровня текущего прерывания
 *   
 * Выходные параметры:
 *   нет
 */
.section .text._frxt_int_enter,"ax",@progbits
.align 4
.global _frxt_int_enter
.type _frxt_int_enter,@function

_frxt_int_enter:
    // a2 = уровень прерывания (1-7)
    
    // Получить адрес счетчика глубины
    movi    a3, _frxt_int_depth
    l32i    a4, a3, 0           // a4 = текущая глубина
    
    // Увеличить глубину вложенности
    addi    a4, a4, 1
    s32i    a4, a3, 0           // Сохранить новую глубину
    
    // Разрешить более высокие уровни прерываний
    // PS регистр содержит INTLEVEL в битах [3:0]
    // Установить INTLEVEL на текущий уровень минус 1
    rsr.ps  a3
    movi    a4, PS_INTLEVEL_MASK // Маска для очистки уровня
    and     a3, a3, a4          // Очистить текущий уровень
    
    // Вычесть 1 из уровня для разрешения более высоких
    addi    a2, a2, -1
    andi    a2, a2, 0xF         // Обезопасить (0-7 уровни)
    
    // Объединить новый уровень с PS
    or      a3, a3, a2
    wsr.ps  a3
    rsync
    
    ret

/*
 * _frxt_int_exit - Выход из обработчика прерывания
 * 
 * Назначение:
 *   - Уменьшить глубину вложенности прерываний
 *   - Восстановить предыдущий уровень прерывания
 *   - Проверить необходимость переключения задач
 * 
 * Входные параметры:
 *   нет
 *   
 * Выходные параметры:
 *   нет
 */
.section .text._frxt_int_exit,"ax",@progbits
.align 4
.global _frxt_int_exit
.type _frxt_int_exit,@function

_frxt_int_exit:
    // Получить адрес счетчика глубины
    movi    a2, _frxt_int_depth
    l32i    a3, a2, 0           // a3 = текущая глубина
    
    // Уменьшить глубину вложенности
    addi    a3, a3, -1
    s32i    a3, a2, 0           // Сохранить новую глубину
    
    // Восстановить уровень прерывания из PS
    // В Call0 ABI мы восстанавливаем предыдущий уровень
    rsr.ps  a2
    movi    a3, PS_INTLEVEL_MASK
    and     a2, a2, a3
    
    // Восстановить предыдущий уровень (для вложенности)
    // Установить уровень на максимальный (прерывания заблокированы)
    movi    a3, 15              // INTLEVEL = 15 = без прерываний
    or      a2, a2, a3
    wsr.ps  a2
    rsync
    
    ret

/*
 * _xt_level1_int_handler - Основной обработчик прерывания уровня 1
 * 
 * Назначение:
 *   Основной обработчик для прерываний уровня 1 (и выше)
 *   
 * Входные параметры:
 *   A0 = адрес возврата
 *   PS = program status (с сохраненным INTLEVEL)
 *   EPC1 = адрес инструкции, которая была прервана
 *   
 * Действия:
 *   1. Сохранить минимальный контекст
 *   2. Определить источник прерывания
 *   3. Вызвать _frxt_int_enter()
 *   4. Вызвать обработчик (Go код)
 *   5. Вызвать _frxt_int_exit()
 *   6. Восстановить контекст и вернуться
 */
.section .text._xt_level1_int_handler,"ax",@progbits
.align 4
.global _xt_level1_int_handler
.type _xt_level1_int_handler,@function

_xt_level1_int_handler:
    // Сохранить контекст текущего выполнения
    // a1 (sp) = текущий указатель стека
    
    // Сохранить на стек минимальный контекст
    addi    sp, sp, -32         // 8 слов (32 байта) для служебной информации
    
    s32i    a0, sp, 0           // a0 = return address
    s32i    a1, sp, 4           // a1 = stack pointer (исходное значение)
    s32i    a2, sp, 8           // a2 = используется для работы
    s32i    a3, sp, 12          // a3 = используется для работы
    s32i    a4, sp, 16          // a4 = используется для работы
    s32i    a5, sp, 20          // a5 = используется для работы
    s32i    a6, sp, 24          // a6 = используется для работы
    s32i    a7, sp, 28          // a7 = используется для работы
    
    // Вызвать Go функцию обработчика
    movi    a2, 1               // a2 = уровень прерывания (1)
    call    _frxt_int_enter     // Вход в обработчик
    
    // Вызвать основной обработчик (Go код)
    call    handleInterrupt
    
    call    _frxt_int_exit      // Выход из обработчика
    
    // Восстановить минимальный контекст
    l32i    a0, sp, 0
    l32i    a1, sp, 4
    l32i    a2, sp, 8
    l32i    a3, sp, 12
    l32i    a4, sp, 16
    l32i    a5, sp, 20
    l32i    a6, sp, 24
    l32i    a7, sp, 28
    
    addi    sp, sp, 32          // Освободить стек
    
    // Возвращаемся из прерывания
    rfe                         // Return From Exception
    nop
    nop

/*
 * _xt_unhandled_exception - Обработчик необработанного исключения
 * 
 * Назначение:
 *   Обработка необработанных исключений (фатальные ошибки)
 *   
 * Входные параметры:
 *   EXCCAUSE = код исключения
 *   EPC1 = адрес инструкции, вызвавшей исключение
 *   EXCVADDR = адрес, вызвавший исключение (для ошибок доступа)
 *   
 * Действия:
 *   1. Сохранить контекст
 *   2. Вызвать Go обработчик исключения
 *   3. Зацикл (halt)
 */
.section .text._xt_unhandled_exception,"ax",@progbits
.align 4
.global _xt_unhandled_exception
.type _xt_unhandled_exception,@function

_xt_unhandled_exception:
    // Сохранить полный контекст для отладки
    call    _xt_context_save    // a2 = &контекст
    
    // Сохранить указатель на контекст
    mov     a3, a2
    
    // Получить информацию об исключении
    rsr.exccause a2             // a2 = код исключения
    rsr.excvaddr a4             // a4 = адрес исключения
    rsr.epc1 a5                 // a5 = PC исключения
    
    // Вызвать Go функцию обработчика
    // handleException(exccause uint32, excvaddr uint32, epc uint32)
    call    handleException
    
    // Если вернулись, зациклиться (halt)
.exception_halt_loop:
    j       .exception_halt_loop // Бесконечный цикл

/*
 * _xt_check_interrupt_level - Получить текущий уровень прерывания
 * 
 * Возвращает:
 *   a2 = текущий уровень прерывания (0-7)
 */
.section .text._xt_check_interrupt_level,"ax",@progbits
.align 4
.global _xt_check_interrupt_level
.type _xt_check_interrupt_level,@function

_xt_check_interrupt_level:
    rsr.ps  a2
    andi    a2, a2, PS_INTLEVEL_MASK  // Извлечь INTLEVEL [3:0]
    ret

/*
 * _xt_check_interrupt_status - Получить текущий статус прерываний
 * 
 * Возвращает:
 *   a2 = битовая маска активных прерываний
 */
.section .text._xt_check_interrupt_status,"ax",@progbits
.align 4
.global _xt_check_interrupt_status
.type _xt_check_interrupt_status,@function

_xt_check_interrupt_status:
    rsr.interrupt a2            // a2 = регистр INTERRUPT
    ret

/*
 * _xt_set_interrupt_mask - Установить маску прерываний
 * 
 * Входные параметры:
 *   a2 = новая маска прерываний (INTENABLE)
 */
.section .text._xt_set_interrupt_mask,"ax",@progbits
.align 4
.global _xt_set_interrupt_mask
.type _xt_set_interrupt_mask,@function

_xt_set_interrupt_mask:
    wsr.intenable a2
    rsync
    ret

/*
 * _xt_get_interrupt_mask - Получить маску прерываний
 * 
 * Возвращает:
 *   a2 = текущая маска прерываний (INTENABLE)
 */
.section .text._xt_get_interrupt_mask,"ax",@progbits
.align 4
.global _xt_get_interrupt_mask
.type _xt_get_interrupt_mask,@function

_xt_get_interrupt_mask:
    rsr.intenable a2
    ret
