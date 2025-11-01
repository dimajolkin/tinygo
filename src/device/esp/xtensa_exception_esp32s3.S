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
    
    // TEMPORARY: DO NOT change INTLEVEL to avoid nested interrupts during debugging
    // Keep INTLEVEL high (set by hardware when entering exception)
    // This prevents re-entrant interrupts
    // TODO: Re-enable this once basic interrupts work
    // rsr     a3, PS
    // movi    a4, 0xFFFFFFF0       // очистить младшие 4 бита INTLEVEL
    // and     a3, a3, a4
    // addi    a2, a2, -1
    // movi    a4, 0x7
    // and     a2, a2, a4           // ограничить до 0..7
    // or      a3, a3, a2
    // wsr     a3, PS
    // rsync
    
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
    
    // Не трогаем PS здесь: уровень был установлен во входе
    
    ret

/*
 * _xt_level1_int_handler_entry - Основной обработчик прерывания уровня 1
 * 
 * Назначение:
 *   Основной обработчик для прерываний уровня 1 (и выше)
 *   Вызывается из _xt_lowint1, где уже сохранены A0, A1(SP), PS, PC
 *   
 * Входные параметры:
 *   sp = указатель на стек-фрейм с сохраненным минимальным контекстом
 *   [sp+XT_STK_A0] = оригинальный A0
 *   [sp+XT_STK_A1] = оригинальный SP
 *   [sp+XT_STK_PS] = оригинальный PS
 *   [sp+XT_STK_PC] = оригинальный PC (из EPC_1)
 *   
 * Действия:
 *   1. Сохранить остальные регистры (A2-A15, SAR, EXCCAUSE, etc)
 *   2. Вызвать _frxt_int_enter()
 *   3. Вызвать обработчик (Go код)
 *   4. Вызвать _frxt_int_exit()
 *   5. Восстановить все регистры и вернуться через _xt_user_exit
 */
.section .text._xt_level1_int_handler_entry,"ax",@progbits
.align 4
.global _xt_level1_int_handler_entry
.type _xt_level1_int_handler_entry,@function

_xt_level1_int_handler_entry:
    /* TEMPORARY: Absolutely minimal handler for testing ISR entry/exit
     * _xt_lowint1 already saved A0, A1, PS, PC on stack
     * We just restore them and return directly */
    
    /* Restore PS */
    l32i    a0, sp, XT_STK_PS
    wsr.ps  a0
    
    /* Restore PC to EPC1 */
    l32i    a0, sp, XT_STK_PC
    wsr.epc1 a0
    
    /* Restore A0 */
    l32i    a0, sp, XT_STK_A0
    
    /* Restore SP */
    l32i    sp, sp, XT_STK_A1
    
    /* Sync and return */
    rsync
    rfe

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
    /* Save minimal context on stack */
    addi    sp, sp, -XT_STK_FRMSZ           /* allocate stack frame */
    
    /* Save all registers */
    s32i    a0, sp, XT_STK_A0
    s32i    a1, sp, XT_STK_A1
    s32i    a2, sp, XT_STK_A2
    s32i    a3, sp, XT_STK_A3
    s32i    a4, sp, XT_STK_A4
    s32i    a5, sp, XT_STK_A5
    s32i    a6, sp, XT_STK_A6
    s32i    a7, sp, XT_STK_A7
    s32i    a8, sp, XT_STK_A8
    s32i    a9, sp, XT_STK_A9
    s32i    a10, sp, XT_STK_A10
    s32i    a11, sp, XT_STK_A11
    s32i    a12, sp, XT_STK_A12
    s32i    a13, sp, XT_STK_A13
    s32i    a14, sp, XT_STK_A14
    s32i    a15, sp, XT_STK_A15
    
    /* Save exception info */
    rsr     a2, EXCCAUSE            /* a2 = exception cause */
    s32i    a2, sp, XT_STK_EXCCAUSE
    
    rsr     a3, EXCVADDR            /* a3 = exception address */
    s32i    a3, sp, XT_STK_EXCVADDR
    
    rsr.epc1 a4                     /* a4 = exception PC (SR=177) */
    s32i    a4, sp, XT_STK_PC
    
    rsr     a5, PS                  /* a5 = PS */
    s32i    a5, sp, XT_STK_PS
    
    /* Call Go exception handler
       handleException(exccause uint32, excvaddr uint32, epc uint32) */
    call0   handleException
    
    /* If returned, halt */
.exception_halt_loop:
    j       .exception_halt_loop    /* infinite loop */

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
    rsr     a2, PS
    movi    a3, 0x0F                  // PS_INTLEVEL_MASK
    and     a2, a2, a3                // Извлечь INTLEVEL [3:0]
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
    rsr     a2, INTERRUPT       // a2 = регистр INTERRUPT
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
    wsr     a2, INTENABLE
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
    rsr     a2, INTENABLE
    ret
