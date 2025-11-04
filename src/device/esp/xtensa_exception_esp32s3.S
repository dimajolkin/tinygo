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
//   - All handlers (exception/interrupt) use Call0 ABI calling convention
//   - CPU context save/restore simplified (no windowed register spill)
//   - Verified for direct VECBASE dispatch (no FreeRTOS vector trampolines)
//   - Minimal L1 ISR path for SYSTIMER only (no full dispatcher loop yet)

#include "xtensa_esp32s3_macros.inc"

.extern _frxt_int_depth
.extern _frxt_int_handlers
.extern handleInterrupt
.extern handleException

.extern _vector_base
.extern UserExceptionVector
.extern KernelExceptionVector
.extern NMIExceptionVector
.extern Level2InterruptVector
.extern Level3InterruptVector
.extern Level4InterruptVector
.extern Level5InterruptVector
.extern Level6InterruptVector
.extern Level7InterruptVector
.extern DoubleExceptionVector

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

// Note: _isr_call_count is defined in xtensa_vectors_esp32s3.S (.bss section)

.section .text._frxt_int_enter,"ax",@progbits
.align 4
.global _frxt_int_enter
.type _frxt_int_enter,@function

/*
 * _frxt_int_enter - Interrupt entry hook
 *
 * Purpose:
 *   - Increment interrupt nesting depth counter
 *   - Do NOT change PS.INTLEVEL here (hardware already raised it on entry)
 *   - Placeholder for future FreeRTOS-like preemption logic
 *
 * Calling convention:
 *   - Uses Call0 ABI
 *   - a2 may carry the current interrupt level (1..7), but we ignore it for now
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

.section .text._frxt_int_exit,"ax",@progbits
.align 4
.global _frxt_int_exit
.type _frxt_int_exit,@function

/*
 * _frxt_int_exit - Interrupt exit hook
 *
 * Purpose:
 *   - Decrement interrupt nesting depth counter
 *   - Do not restore PS here (return path will RFE/jump via vector epilogue)
 *   - No task switching yet (no FreeRTOS port hooks)
 *
 * Calling convention:
 *   - Uses Call0 ABI
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
 * _xt_level1_int_handler_entry - Minimal ISR for Level-1 interrupts
 *
 * Current implementation:
 *   - Minimal path to verify CPU-level interrupt delivery and clearing
 *   - Increments a debug counter (_isr_call_count)
 *   - Clears SYSTIMER interrupt (peripheral) and CPU int line 1 (INTCLEAR)
 *   - Returns via vector epilogue (_xt_user_exit)
 *
 * Not yet implemented:
 *   - Full dispatcher loop over INTENABLE & INTERRUPT
 *   - Nested interrupt unmasking (explicit PS.INTLEVEL lowering)
 *   - FreeRTOS task switching hooks
 *
 * ABI:
 *   - Call0 ABI, no window rotations; saves only a2/a3 scratch used here
 */
.section .text._xt_level1_int_handler_entry,"ax",@progbits
.align 4
.global _xt_level1_int_handler_entry
.type _xt_level1_int_handler_entry,@function

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

.section .text._xt_unhandled_exception,"ax",@progbits
.align 4
.global _xt_unhandled_exception
.type _xt_unhandled_exception,@function

/*
 * _xt_unhandled_exception - Default handler for unhandled exceptions
 *
 * Notes:
 *   - Uses Call0 ABI
 *   - Passes (EXCCAUSE, EXCVADDR, EPC) in (a2, a3, a4) to Go
 *   - Calls handleException (Go)
 *   - If handleException returns, the CPU is halted deterministically (waiti loop)
 */
.section .text._xt_unhandled_exception,"ax",@progbits
.align 4
.global _xt_unhandled_exception
.type _xt_unhandled_exception,@function

_xt_unhandled_exception:
    /* Exception frame already set up by vector handler */

    /* Prepare Call0 ABI args (a2..a4) */
    rsr     a2, EXCCAUSE        // a2 = exccause
    rsr     a3, EXCVADDR        // a3 = excvaddr
    l32i    a4, sp, XT_STK_PC   // a4 = epc

    /* Call Go handler using call0 (Call0 ABI; no window rotation) */
    call0   handleException

    /* If we ever return, park the CPU deterministically */
1:
    waiti   0                   // low‑power idle; safe in EXCM context
    j       1b
