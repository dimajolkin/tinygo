/*
 * SPDX-FileCopyrightText: 2015-2019 Cadence Design Systems, Inc.
 * SPDX-License-Identifier: MIT
 * SPDX-FileContributor: 2016-2024 Espressif Systems (Shanghai) CO LTD
 *
 * Source (upstream): esp-idf/components/xtensa/xtensa_vectors.S
 * This file is an adapted, minimal stub for TinyGo ESP32-S3 to introduce
 * the `_vector_table` symbol and prepare for full vector table import.
 *
 * Changes from upstream for TinyGo (initial step 1.1):
 * - Reduced to a minimal placeholder defining `_vector_table` only.
 * - All RTOS- and ESP-IDF-specific dependencies are intentionally omitted.
 * - Full vector entries will be added in subsequent steps.
 */

// Minimal kernel/user exception vector entries (stubs) to enable assembly/link
// These sections mirror ESP-IDF names so a future linker script can place them
// at Xtensa-required offsets relative to VECBASE.

    .section .KernelExceptionVector.text, "ax"
    .global _KernelExceptionVector
    .align 4
_KernelExceptionVector:
    wsr.excsave1 a0
    movi a2, 1
    slli a2, a2, 30         // a2 = 0x40000000 (ROM VECBASE)
    addi a2, a2, 0x300
    jx a2

    .section .UserExceptionVector.text, "ax"
    .global _UserExceptionVector
    .type   _UserExceptionVector,@function
    .align 4
_UserExceptionVector:
    wsr.excsave1 a0
    // Temporarily: always bridge to ROM UserException vector to avoid hooks
    movi a2, 1
    slli a2, a2, 30          // 0x40000000
    addi a2, a2, 0x340       // UserException ROM vector
    jx a2

    .section .iram1, "ax"
    .align 4
    .type _tinygo_kernel_exc,@function
_tinygo_kernel_exc:
    // TODO: replace with proper panic/dispatch once handlers are wired
    rfe

    .align 4
    .type _tinygo_user_exc,@function
_tinygo_user_exc:
    // Unused for now (UserException jumps to ROM). Keep symbols only.
    rfi 1

    .global tinygo_irq19_handler
    .weak   tinygo_irq19_handler
    .set    tinygo_irq19_handler, _tinygo_irq19_default

    .align 4
    .type _tinygo_irq19_default,@function
_tinygo_irq19_default:
    rfi 1

    // Level 2..6 interrupt vectors: bridge to ROM
    .section .Level2InterruptVector.text, "ax"
    .global _Level2Vector
    .align 4
_Level2Vector:
    wsr.excsave2 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x180
    jx a2

    .section .Level3InterruptVector.text, "ax"
    .global _Level3Vector
    .align 4
_Level3Vector:
    wsr.excsave3 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x1C0
    jx a2

    .section .Level4InterruptVector.text, "ax"
    .global _Level4Vector
    .align 4
_Level4Vector:
    wsr.excsave4 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x200
    jx a2

    .section .Level5InterruptVector.text, "ax"
    .global _Level5Vector
    .align 4
_Level5Vector:
    wsr.excsave5 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x240
    jx a2

    .section .Level6InterruptVector.text, "ax"
    .global _Level6Vector
    .align 4
_Level6Vector:
    wsr.excsave6 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x280
    jx a2

    .section .NMIExceptionVector.text, "ax"
    .global _NMIExceptionVector
    .align 4
_NMIExceptionVector:
    wsr.excsave7 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x2C0
    jx a2

    .section .DoubleExceptionVector.text, "ax"
    .global _DoubleExceptionVector
    .align 4
_DoubleExceptionVector:
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x3C0
    jx a2

// ----------------------------------------------------------------------------
// Full Window Vectors (overflow/underflow) for Xtensa LX7 (ESP32-S3)
// Adapted from ESP-IDF components/xtensa/xtensa_vectors.S (MIT License)
// ----------------------------------------------------------------------------

    .section .WindowVectors.text, "ax"

    /* Window Overflow Exception for Call4 */
    .org    0x0
    .global _WindowOverflow4
_WindowOverflow4:
    s32e    a0, a5, -16     /* save a0 to call[j+1]'s stack frame */
    s32e    a1, a5, -12     /* save a1 to call[j+1]'s stack frame */
    s32e    a2, a5,  -8     /* save a2 to call[j+1]'s stack frame */
    s32e    a3, a5,  -4     /* save a3 to call[j+1]'s stack frame */
    rfwo                    /* rotates back to call[i] position */

    /* Window Underflow Exception for Call4 */
    .org    0x40
    .global _WindowUnderflow4
_WindowUnderflow4:
    l32e    a0, a5, -16     /* restore a0 from call[i+1]'s stack frame */
    l32e    a1, a5, -12     /* restore a1 from call[i+1]'s stack frame */
    l32e    a2, a5,  -8     /* restore a2 from call[i+1]'s stack frame */
    l32e    a3, a5,  -4     /* restore a3 from call[i+1]'s stack frame */
    rfwu

    /* Window Overflow Exception for Call8 */
    .org    0x80
    .global _WindowOverflow8
_WindowOverflow8:
    s32e    a0, a9, -16     /* save a0 to call[j+1]'s stack frame */
    l32e    a0, a1, -12     /* a0 <- call[j-1]'s sp */
    s32e    a1, a9, -12     /* save a1 to call[j+1]'s stack frame */
    s32e    a2, a9,  -8     /* save a2 to call[j+1]'s stack frame */
    s32e    a3, a9,  -4     /* save a3 to call[j+1]'s stack frame */
    s32e    a4, a0, -32     /* save a4 to call[j]'s stack frame */
    s32e    a5, a0, -28     /* save a5 to call[j]'s stack frame */
    s32e    a6, a0, -24     /* save a6 to call[j]'s stack frame */
    s32e    a7, a0, -20     /* save a7 to call[j]'s stack frame */
    rfwo                    /* rotates back to call[i] position */

    /* Window Underflow Exception for Call8 */
    .org    0xC0
    .global _WindowUnderflow8
_WindowUnderflow8:
    l32e    a0, a9, -16     /* restore a0 from call[i+1]'s stack frame */
    l32e    a1, a9, -12     /* restore a1 from call[i+1]'s stack frame */
    l32e    a2, a9,  -8     /* restore a2 from call[i+1]'s stack frame */
    l32e    a7, a1, -12     /* a7 <- call[i-1]'s sp */
    l32e    a3, a9,  -4     /* restore a3 from call[i+1]'s stack frame */
    l32e    a4, a7, -32     /* restore a4 from call[i]'s stack frame */
    l32e    a5, a7, -28     /* restore a5 from call[i]'s stack frame */
    l32e    a6, a7, -24     /* restore a6 from call[i]'s stack frame */
    l32e    a7, a7, -20     /* restore a7 from call[i]'s stack frame */
    rfwu

    /* Window Overflow Exception for Call12 */
    .org    0x100
    .global _WindowOverflow12
_WindowOverflow12:
    s32e    a0,  a13, -16   /* save a0 to call[j+1]'s stack frame */
    l32e    a0,  a1,  -12   /* a0 <- call[j-1]'s sp */
    s32e    a1,  a13, -12   /* save a1 to call[j+1]'s stack frame */
    s32e    a2,  a13,  -8   /* save a2 to call[j+1]'s stack frame */
    s32e    a3,  a13,  -4   /* save a3 to call[j+1]'s stack frame */
    s32e    a4,  a0,  -48   /* save a4 to end of call[j]'s stack frame */
    s32e    a5,  a0,  -44   /* save a5 */
    s32e    a6,  a0,  -40   /* save a6 */
    s32e    a7,  a0,  -36   /* save a7 */
    s32e    a8,  a0,  -32   /* save a8 */
    s32e    a9,  a0,  -28   /* save a9 */
    s32e    a10, a0,  -24   /* save a10 */
    s32e    a11, a0,  -20   /* save a11 */
    rfwo                    /* rotates back to call[i] position */

    /* Window Underflow Exception for Call12 */
    .org 0x140
    .global _WindowUnderflow12
_WindowUnderflow12:
    l32e    a0,  a13, -16   /* restore a0 from call[i+1]'s stack frame */
    l32e    a1,  a13, -12   /* restore a1 */
    l32e    a2,  a13,  -8   /* restore a2 */
    l32e    a11, a1,  -12   /* a11 <- call[i-1]'s sp */
    l32e    a3,  a13,  -4   /* restore a3 */
    l32e    a4,  a11, -48   /* restore a4 */
    l32e    a5,  a11, -44   /* restore a5 */
    l32e    a6,  a11, -40   /* restore a6 */
    l32e    a7,  a11, -36   /* restore a7 */
    l32e    a8,  a11, -32   /* restore a8 */
    l32e    a9,  a11, -28   /* restore a9 */
    l32e    a10, a11, -24   /* restore a10 */
    l32e    a11, a11, -20   /* restore a11 */
    rfwu


// ----------------------------------------------------------------------------
// Linker anchor to ensure this object file is kept in the final image
// ----------------------------------------------------------------------------
    .section .text.vectors_anchor, "ax"
    .global _tinygo_vectors_present
    .type   _tinygo_vectors_present,@function
_tinygo_vectors_present:
    retw

    // Provide alias for package runtime reference: runtime._tinygo_vectors_present
    .global runtime._tinygo_vectors_present
    .set    runtime._tinygo_vectors_present, _tinygo_vectors_present

// ----------------------------------------------------------------------------
// Alloca exception handler (movsp) - moved here to avoid .org conflicts
// ----------------------------------------------------------------------------
    .section .text.xt_alloca_exc, "ax"
    .align  4
    .global _xt_alloca_exc
    .type   _xt_alloca_exc,@function
_xt_alloca_exc:
    rsr     a0, WINDOWBASE  /* grab WINDOWBASE before rotw changes it */
    rotw    -1              /* WINDOWBASE goes to a4, new a0-a3 are scratch */
    rsr     a2, PS
    #include "xtensa_esp32s3_macros.inc"
    extui   a3, a2, XCHAL_PS_OWB_SHIFT, XCHAL_PS_OWB_BITS
    xor     a3, a3, a4      /* bits changed from old to current windowbase */
    rsr.excsave1 a4         /* restore original a0 (now in a4) */
    slli    a3, a3, XCHAL_PS_OWB_SHIFT
    xor     a2, a2, a3      /* flip changed bits in old window base */
    wsr     a2, PS          /* update PS.OWB to new window base */
    rsync

    bbci.l a4, 31, _WindowUnderflow4
    rotw    -1              /* original a0 goes to a8 */
    bbci.l a8, 30, _WindowUnderflow8
    rotw    -1
    j       _WindowUnderflow12

