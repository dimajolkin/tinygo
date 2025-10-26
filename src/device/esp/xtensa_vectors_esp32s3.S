//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Xtensa Vector Table for ESP32-S3
 * Sections are kept by linker at fixed IRAM offsets and VECBASE is set to _vector_base.
 * We route Level-1 user exceptions to low-level interrupt handler which calls Go ISR.
 */

.extern _xt_level1_int_handler
.extern _frxt_int_depth

/* Provide storage for _frxt_int_depth if not provided elsewhere */
.section .bss._frxt_int_depth, "aw", @nobits
.align 4
.global _frxt_int_depth
_frxt_int_depth:
    .space 4

/* User Exception Vector (Level-1) -> jump to low-level handler */
.section .UserExceptionVector.text, "ax", @progbits
.align 4
.global _UserExceptionVector
_UserExceptionVector:
    j   _xt_lowint1
    nop
    nop
    nop
    nop
    nop
    nop
    nop

/* Low-level level-1 interrupt entry: prepare minimal state and branch */
.section .text._xt_lowint1, "ax", @progbits
.align 4
.global _xt_lowint1
_xt_lowint1:
    // Directly tail-call the common level1 handler
    j   _xt_level1_int_handler
    nop

/* The rest of vectors can be simple loops/placeholders for now */
.section .DoubleExceptionVector.text, "ax", @progbits
.align 4
_DoubleExceptionVector:
    j   _DoubleExceptionVector

.section .KernelExceptionVector.text, "ax", @progbits
.align 4
_KernelExceptionVector:
    j   _KernelExceptionVector

.section .NMIExceptionVector.text, "ax", @progbits
.align 4
_NMIExceptionVector:
    j   _NMIExceptionVector

.section .Level2InterruptVector.text, "ax", @progbits
.align 4
_Level2InterruptVector:
    j   _Level2InterruptVector

.section .Level3InterruptVector.text, "ax", @progbits
.align 4
_Level3InterruptVector:
    j   _Level3InterruptVector

.section .Level4InterruptVector.text, "ax", @progbits
.align 4
_Level4InterruptVector:
    j   _Level4InterruptVector

.section .Level5InterruptVector.text, "ax", @progbits
.align 4
_Level5InterruptVector:
    j   _Level5InterruptVector

.section .Level6InterruptVector.text, "ax", @progbits
.align 4
_Level6InterruptVector:
    j   _Level6InterruptVector

.section .Level7InterruptVector.text, "ax", @progbits
.align 4
_Level7InterruptVector:
    j   _Level7InterruptVector


