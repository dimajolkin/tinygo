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
 * _frxt_int_enter - Interrupt entry hook (ESP-IDF compatible)
 * Based on ESP-IDF: components/freertos/FreeRTOS-Kernel-SMP/portable/xtensa/portasm.S
 *
 * Purpose:
 *   - Save complete context (a12-a13 first, then call _xt_context_save for rest)
 *   - Increment interrupt nesting depth counter
 *   - Switch to interrupt stack if nesting == 1
 *
 * Calling convention:
 *   - Uses Call0 ABI
 *   - Called from _xt_lowint1 after minimal context (a0, a1, PS, PC, EXIT) saved
 *   - Returns to caller who will then set up PS for C and call handlers
 */
.section .text._frxt_int_enter,"ax",@progbits
.align 4
.global _frxt_int_enter
.type _frxt_int_enter,@function

_frxt_int_enter:
    /* Save a12-a13 in the stack frame as required by _xt_context_save */
    s32i    a12, sp, XT_STK_A12
    s32i    a13, sp, XT_STK_A13

    /* Save return address in a safe place (free a0) */
    mov     a12, a0

    /* Save the rest of the interrupted context (preserves A12-13) */
    call0   _xt_context_save

    /* 
     * Manage interrupt nesting depth.
     * NOTE: For now, we don't switch to interrupt stack (simplified version).
     * Full ESP-IDF also:
     *   - Checks port_xSchedulerRunning
     *   - Switches to interrupt stack on first nesting level
     *   - Saves current TCB's SP
     */
    
    movi    a2, _frxt_int_depth
    l32i    a3, a2, 0                   /* a3 = current nesting depth */
    addi    a3, a3, 1                   /* increment nesting count */
    s32i    a3, a2, 0                   /* save nesting count */

    /* Restore return address and return */
    mov     a0, a12
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
    /* Called from vector via call0 */
    
    /* Prepare Call0 parameters (a2, a3, a4) for Go function */
    rsr     a2, EXCCAUSE        // a2 = exccause (first param)
    rsr     a3, EXCVADDR        // a3 = excvaddr (second param)
    rsr     a4, 177             // a4 = epc (EPC_1 = 177, third param)
    
    /* Call Go handler using call0 (simple, no window management) */
    call0   handleException
    
    /* If we ever return (should never happen), halt CPU */
1:
    waiti   0
    j       1b
