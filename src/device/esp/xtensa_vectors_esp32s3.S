//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Xtensa Vector Table for ESP32-S3
 * Sections are kept by linker at fixed IRAM offsets and VECBASE is set to _vector_base.
 * We route Level-1 user exceptions to low-level interrupt handler which calls Go ISR.
 */

/* Special Register definitions (from ESP-IDF xtensa/specreg.h) */
#define EPC_1           177
#define EXCSAVE_1       209
#define PS              230
#define EXCCAUSE        232

.extern _xt_level1_int_handler
.extern _xt_unhandled_exception
.extern _frxt_int_depth

/* Provide storage for interrupt counters */
.section .bss._frxt_int_depth, "aw", @nobits
.align 4
.global _frxt_int_depth
_frxt_int_depth:
    .space 4

.section .bss.VectorEntryCount, "aw", @nobits
.align 4
.global VectorEntryCount
VectorEntryCount:
    .space 4

.section .bss.IsrCountBeforeGo, "aw", @nobits
.align 4
.global IsrCountBeforeGo
IsrCountBeforeGo:
    .space 4

.section .bss.IsrCountAfterGo, "aw", @nobits
.align 4
.global IsrCountAfterGo
IsrCountAfterGo:
    .space 4

/* User Exception Vector (Level-1) -> save a0 and jump to dispatcher */
.begin literal_prefix .UserExceptionVector
.section .UserExceptionVector.text, "ax"
.global _UserExceptionVector
.type _UserExceptionVector,@function
.align 4
_UserExceptionVector:
    wsr.excsave1 a0                         /* preserve a0 (dot notation like in esp32s3.S) */
    call0   _xt_user_exc                    /* user exception handler */
    /* never returns here - call0 is used as a jump */
.end literal_prefix

/* User exception dispatcher: check if Level-1 interrupt or exception */
.section .text._xt_user_exc, "ax", @progbits
.align 4
.type _xt_user_exc,@function
_xt_user_exc:
    /* DEBUG: Increment VectorEntryCount to track vector entry */
    wsr.excsave1 a2                         /* save a2 temporarily */
    movi    a0, VectorEntryCount
    l32i    a2, a0, 0
    addi    a2, a2, 1
    s32i    a2, a0, 0
    rsr.excsave1 a2                         /* restore a2 */
    rsr.excsave1 a0                         /* restore a0 (was saved by vector) */
    
    /* Check if this is a Level-1 interrupt (EXCCAUSE == 4) */
    wsr.excsave1 a0                         /* save a0 again for exccause read */
    rsr.exccause a0                         /* read EXCCAUSE (dot notation) */
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
    
    rsr.ps  a0                              /* save PS (dot notation) */
    s32i    a0, sp, XT_STK_PS
    
    rsr.epc1 a0                             /* save interruptee's PC from EPC_1 (dot notation) */
    s32i    a0, sp, XT_STK_PC
    
    rsr.excsave1 a0                         /* restore original a0 from EXCSAVE_1 (dot notation) */
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
    wsr.ps  a0                              /* restore PS (dot notation) */
    l32i    a0, sp, XT_STK_PC               /* restore PC */
    wsr.epc1 a0                             /* restore to EPC_1 (dot notation) */
    l32i    a0, sp, XT_STK_A0               /* restore a0 */
    l32i    sp, sp, XT_STK_A1               /* restore SP */
    rsync
    rfe                                      /* return from exception */

/* The rest of vectors - simple placeholders */
.begin literal_prefix .DoubleExceptionVector
.section .DoubleExceptionVector.text, "ax"
.align 4
_DoubleExceptionVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .KernelExceptionVector
.section .KernelExceptionVector.text, "ax"
.align 4
_KernelExceptionVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .NMIExceptionVector
.section .NMIExceptionVector.text, "ax"
.align 4
_NMIExceptionVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .Level2InterruptVector
.section .Level2InterruptVector.text, "ax"
.align 4
_Level2InterruptVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .Level3InterruptVector
.section .Level3InterruptVector.text, "ax"
.align 4
_Level3InterruptVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .Level4InterruptVector
.section .Level4InterruptVector.text, "ax"
.align 4
_Level4InterruptVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .Level5InterruptVector
.section .Level5InterruptVector.text, "ax"
.align 4
_Level5InterruptVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .Level6InterruptVector
.section .Level6InterruptVector.text, "ax"
.align 4
_Level6InterruptVector:
    j   _xt_unhandled_exception
.end literal_prefix

.begin literal_prefix .Level7InterruptVector
.section .Level7InterruptVector.text, "ax"
.align 4
_Level7InterruptVector:
    j   _xt_unhandled_exception
.end literal_prefix


