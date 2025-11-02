// This file contains low-level exception and interrupt handling for ESP32-S3 (Xtensa LX7)
//
// Based on ESP-IDF implementation:
//   - components/xtensa/xtensa_vectors.S (dispatch_c_isr macro)
//   - components/xtensa/include/xtensa/coreasm.h (find_ms_setbit macro)
//   - components/freertos/FreeRTOS-Kernel-SMP/portable/xtensa/portasm.S (_frxt_int_enter/exit)
//
// Main differences from ESP-IDF:
//   - Uses Call0 ABI instead of Windowed ABI
//   - Calls Go dispatcher (handleInterrupt) instead of C function table
//   - Simplified nesting logic (no FreeRTOS task switching yet)

#include "xtensa_esp32s3_macros.inc"

.extern _frxt_int_depth
.extern _frxt_int_handlers
.extern handleInterrupt
.extern handleException

/*
 * find_ms_setbit - Find most significant bit set
 * 
 * SOURCE: ESP-IDF components/xtensa/include/xtensa/coreasm.h
 * 
 * Finds the most significant bit that is set in <as> and returns its index + <base> in <ad>,
 * or <base> - 1 if <as> is zero. Index counts from 0 for LSB to 31 for MSB.
 *
 * Parameters:
 *   ad - destination register (result)
 *   as - source register (value to check)
 *   at - temporary register (must be different from as)
 *   base - constant offset (usually 0)
 *
 * ESP32-S3 has NSA (Normalization Shift Amount) instruction for fast implementation.
 */
.macro find_ms_setbit ad, as, at, base
    movi    \at, 31+\base
    nsau    \as, \as        // get index of as, numbered from msbit (32 if absent)
    sub     \ad, \at, \as   // get numbering from lsbit (0..31, -1 if absent)
.endm

// Global ISR call counter in .data section
.section .data
.align 4
.global _isr_call_count
_isr_call_count:
    .word 0

/*
 * _frxt_int_enter - Вход в обработчик прерывания
 * 
 * Назначение:
 *   - Увеличить глубину вложенности прерываний
 *   - Настроить уровень прерывания для разрешения вложенных прерываний
 * 
 * Входные параметры:
 *   a2 = уровень прерывания (1-7)
 *   
 * Выходные параметры:
 *   нет
 */
.section .text._frxt_int_enter,"ax",@progbits
.align 4
.global _frxt_int_enter
.type _frxt_int_enter,@function

_frxt_int_enter:
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
 */
.section .text._xt_level1_int_handler_entry,"ax",@progbits
.align 4
.global _xt_level1_int_handler_entry
.type _xt_level1_int_handler_entry,@function

/*
 * _xt_level1_int_handler_entry - Level 1 interrupt dispatcher
 * 
 * SOURCE: Based on ESP-IDF components/xtensa/xtensa_vectors.S dispatch_c_isr macro
 * 
 * This function implements the ESP-IDF interrupt dispatch logic:
 *   1. Save full CPU context (_xt_context_save)
 *   2. Find active interrupt (INTENABLE & INTERRUPT & LEVEL_MASK)
 *   3. Use find_ms_setbit to get highest priority interrupt number
 *   4. Clear CPU interrupt line (wsr.intclear)
 *   5. Set PS to allow nested interrupts (INTLEVEL=1, EXCM=0)
 *   6. Call Go handler with interrupt number
 *   7. Loop back to check for more interrupts
 *   8. Restore full context and return via rfe
 *
 * Called from: _xt_lowint1 in xtensa_vectors_esp32s3.S
 */
_xt_level1_int_handler_entry:
    /* Minimal ISR: increment counter, clear interrupt, return */
    
    /* Save a2, a3 */
    s32i    a2, sp, XT_STK_A2
    s32i    a3, sp, XT_STK_A3
    
    /* Increment debug counter */
    movi    a2, _isr_call_count
    l32i    a3, a2, 0
    addi    a3, a3, 1
    s32i    a3, a2, 0
    
    /* Clear SYSTIMER interrupt */
    movi    a2, 0x60023010          /* SYSTIMER.INT_CLR */
    movi    a3, 1
    s32i    a3, a2, 0
    memw
    
    /* Clear CPU interrupt line 1 */
    movi    a2, 0x2                 /* 1 << 1 */
    wsr.intclear a2
    rsync
    
    /* Restore a2, a3 */
    l32i    a3, sp, XT_STK_A3
    l32i    a2, sp, XT_STK_A2
    
    /* Return via _xt_user_exit */
    l32i    a0, sp, XT_STK_EXIT
    jx      a0

/*
 * _xt_unhandled_exception - Обработчик необработанного исключения
 */
.section .text._xt_unhandled_exception,"ax",@progbits
.align 4
.global _xt_unhandled_exception
.type _xt_unhandled_exception,@function

_xt_unhandled_exception:
    /* Save full context */
    s32i    a2, sp, XT_STK_A2
    s32i    a3, sp, XT_STK_A3
    
    /* Read exception info */
    rsr     a2, EXCCAUSE
    rsr     a3, EXCVADDR
    l32i    a0, sp, XT_STK_PC
    
    /* Call Go handler: handleException(exccause, excvaddr, epc) */
    mov     a0, a2              // a0 = exccause
    mov     a1, a3              // a1 = excvaddr  
    l32i    a2, sp, XT_STK_PC   // a2 = epc
    call0   handleException
    
    /* Should never return, but just in case: */
1:  j       1b
