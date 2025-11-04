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

/* Special Register Numbers (from Xtensa ISA) */
#define EPC_1           177
#define EXCSAVE_1       209
#define PS              230
#define EXCCAUSE        232
#define EXCVADDR        238
#define SAR             3
#define LBEG            0
#define LEND            1
#define LCOUNT          2

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


/* Debug counter for exception handler */
.section .bss._exception_handler_called, "aw", @nobits
.align 4
.global _exception_handler_called
_exception_handler_called:
    .space 4

/* Exported globals for last exception info and test flag */
.section .bss._last_exccause, "aw", @nobits
.global _last_exccause
_last_exccause: .space 4

.section .bss._last_excvaddr, "aw", @nobits
.global _last_excvaddr
_last_excvaddr: .space 4

.section .bss._last_epc, "aw", @nobits
.global _last_epc
_last_epc: .space 4

.section .bss._recover_test_flag, "aw", @nobits
.global _recover_test_flag
_recover_test_flag: .space 4

.section .text._xt_unhandled_exception,"ax",@progbits
.align 4
.global _xt_unhandled_exception
.type _xt_unhandled_exception,@function

_xt_unhandled_exception:
    /* Debug GPIO toggle at entry (GPIO2, bit 2 of GPIO_OUT_REG 0x60004000) */
    movi    a2, 0x60004000
    l32i    a3, a2, 0
    movi    a4, (1<<2)
    xor     a3, a3, a4
    s32i    a3, a2, 0

    /* DEBUG: Increment counter to prove we got here */
    .literal_position
    l32r    a5, .Lexc_counter_addr
    l32i    a6, a5, 0
    addi    a6, a6, 1
    s32i    a6, a5, 0

    /* Save exception info to global variables */
    rsr     a7, EXCCAUSE
    movi    a8, _last_exccause
    s32i    a7, a8, 0
    rsr     a7, EXCVADDR
    movi    a8, _last_excvaddr
    s32i    a7, a8, 0
    rsr     a7, EPC_1
    movi    a8, _last_epc
    s32i    a7, a8, 0

    /* Create minimal exception frame on stack (ESP-IDF style) */
    mov     a5, sp                      // Save original SP
    addi    sp, sp, -32                 // Allocate 32 bytes (8 words) for minimal frame
    s32i    a5, sp, 0                   // Save original SP

    rsr     a6, PS
    s32i    a6, sp, 4                   // Save PS
    rsr     a6, EPC_1
    s32i    a6, sp, 8                   // Save PC (EPC_1)
    rsr     a6, EXCSAVE_1
    s32i    a6, sp, 12                  // Save a0
    rsr     a6, EXCCAUSE
    s32i    a6, sp, 16                  // Save EXCCAUSE
    rsr     a6, EXCVADDR
    s32i    a6, sp, 20                  // Save EXCVADDR

    /* Setup PS for C code (Call0 ABI: INTLEVEL + UM, no WOE) */
    movi    a6, 0x00060020              // PS_INTLEVEL(6) | PS_UM
    wsr     a6, PS
    rsync

    /* Prepare Call0 parameters (a2, a3, a4) for Go function */
    rsr     a2, EXCCAUSE                // a2 = exccause (first param)
    rsr     a3, EXCVADDR                // a3 = excvaddr (second param)
    rsr     a4, EPC_1                   // a4 = epc (third param)

    /* Call Go handler using call0 */
    call0   handleException

    /* Restore SP after handleException returns */
    l32i    sp, sp, 0                   // Restore SP

    /* Check _recover_test_flag: if set, try to recover by advancing EPC1 and rfe */
    movi    a8, _recover_test_flag
    l32i    a7, a8, 0
    beqz    a7, 1f
    rsr     a2, EPC_1
    addi    a2, a2, 2
    wsr     a2, EPC_1
    rfe
1:
    waiti   0
    j       1b

/* Literal pool for _xt_unhandled_exception */
.align 4
.Lexc_counter_addr:
    .word _exception_handler_called
