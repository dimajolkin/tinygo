//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Xtensa Vector Table for ESP32-S3
 * Sections are kept by linker at fixed IRAM offsets and VECBASE is set to _vector_base.
 * We route Level-1 user exceptions to low-level interrupt handler which calls Go ISR.
 */

.extern _xt_level1_int_handler
.extern _xt_unhandled_exception
.extern _frxt_int_depth

/* Provide storage for _frxt_int_depth if not provided elsewhere */
.section .bss._frxt_int_depth, "aw", @nobits
.align 4
.global _frxt_int_depth
_frxt_int_depth:
    .space 4

/* User Exception Vector (Level-1) -> save a0 and jump to dispatcher */
.section .UserExceptionVector.text, "ax", @progbits
.align 4
.global _UserExceptionVector
_UserExceptionVector:
    wsr.excsave1 a0                         /* save a0 in EXCSAVE_1 (SR=209) */
    call0   _xt_user_exc                    /* jump to user exception handler */
    /* never returns here */
    .space (0x20 - (.-_UserExceptionVector))

/* User exception dispatcher: check if Level-1 interrupt or exception */
.section .text._xt_user_exc, "ax", @progbits
.align 4
.type _xt_user_exc,@function
_xt_user_exc:
    /* Check if this is a Level-1 interrupt (EXCCAUSE == 4) */
    rsr.exccause a0
    bnei    a0, 4, _xt_unhandled_exception  /* if not level1 interrupt, handle as exception */
    /* Fall through to _xt_lowint1 for interrupts */

/* Low-level level-1 interrupt entry: create stack frame and dispatch */
.section .text._xt_lowint1, "ax", @progbits
.align 4
.global _xt_lowint1
_xt_lowint1:
    /* Save minimal context on stack (following ESP-IDF model) */
    mov     a0, sp                          /* a0 = current sp */
    addi    sp, sp, -XT_STK_FRMSZ           /* allocate stack frame (112 bytes) */
    s32i    a0, sp, XT_STK_A1               /* save pre-interrupt SP */
    
    rsr.ps  a0                              /* save PS */
    s32i    a0, sp, XT_STK_PS
    
    rsr.epc1 a0                             /* save interruptee's PC (SR=177) */
    s32i    a0, sp, XT_STK_PC
    
    rsr.excsave1 a0                         /* restore original a0 from EXCSAVE_1 */
    s32i    a0, sp, XT_STK_A0               /* save original a0 */
    
    movi    a0, _xt_user_exit               /* save exit point */
    s32i    a0, sp, XT_STK_EXIT
    
    /* Now jump to the main interrupt handler */
    call0   _xt_level1_int_handler_entry
    /* never returns here */

.global _xt_user_exit
_xt_user_exit:
    /* This is the return point after interrupt handling */
    l32i    a0, sp, XT_STK_PS               /* restore PS */
    wsr.ps  a0
    l32i    a0, sp, XT_STK_PC               /* restore PC */
    wsr.epc1 a0                             /* (SR=177) */
    l32i    a0, sp, XT_STK_A0               /* restore a0 */
    l32i    sp, sp, XT_STK_A1               /* restore SP */
    rsync
    rfe                                      /* return from exception */

/* The rest of vectors can be simple loops/placeholders for now */
.section .DoubleExceptionVector.text, "ax", @progbits
.align 4
_DoubleExceptionVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_DoubleExceptionVector))

.section .KernelExceptionVector.text, "ax", @progbits
.align 4
_KernelExceptionVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_KernelExceptionVector))

.section .NMIExceptionVector.text, "ax", @progbits
.align 4
_NMIExceptionVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_NMIExceptionVector))

.section .Level2InterruptVector.text, "ax", @progbits
.align 4
_Level2InterruptVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_Level2InterruptVector))

.section .Level3InterruptVector.text, "ax", @progbits
.align 4
_Level3InterruptVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_Level3InterruptVector))

.section .Level4InterruptVector.text, "ax", @progbits
.align 4
_Level4InterruptVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_Level4InterruptVector))

.section .Level5InterruptVector.text, "ax", @progbits
.align 4
_Level5InterruptVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_Level5InterruptVector))

.section .Level6InterruptVector.text, "ax", @progbits
.align 4
_Level6InterruptVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_Level6InterruptVector))

.section .Level7InterruptVector.text, "ax", @progbits
.align 4
_Level7InterruptVector:
    j   _xt_unhandled_exception
    .space (0x20 - (.-_Level7InterruptVector))


