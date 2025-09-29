/*
 * Temporary TinyGo ESP32-S3 vector table.
 */

#include "xtensa_esp32s3_macros.inc"

    .section .WindowVectors.text, "ax"

    .org    0x0
    .global _WindowOverflow4
_WindowOverflow4:
    s32e    a0, a5, -16
    s32e    a1, a5, -12
    s32e    a2, a5,  -8
    s32e    a3, a5,  -4
    rfwo

    .org    0x40
    .global _WindowUnderflow4
_WindowUnderflow4:
    l32e    a0, a5, -16
    l32e    a1, a5, -12
    l32e    a2, a5,  -8
    l32e    a3, a5,  -4
    rfwu

    .org    0x80
    .global _WindowOverflow8
_WindowOverflow8:
    s32e    a0, a9, -16
    l32e    a0, a1, -12
    s32e    a1, a9, -12
    s32e    a2, a9,  -8
    s32e    a3, a9,  -4
    s32e    a4, a0, -32
    s32e    a5, a0, -28
    s32e    a6, a0, -24
    s32e    a7, a0, -20
    rfwo

    .org    0xC0
    .global _WindowUnderflow8
_WindowUnderflow8:
    l32e    a0, a9, -16
    l32e    a1, a9, -12
    l32e    a2, a9,  -8
    l32e    a7, a1, -12
    l32e    a3, a9,  -4
    l32e    a4, a7, -32
    l32e    a5, a7, -28
    l32e    a6, a7, -24
    l32e    a7, a7, -20
    rfwu

    .org    0x100
    .global _WindowOverflow12
_WindowOverflow12:
    s32e    a0,  a13, -16
    l32e    a0,  a1,  -12
    s32e    a1,  a13, -12
    s32e    a2,  a13,  -8
    s32e    a3,  a13,  -4
    s32e    a4,  a0,  -48
    s32e    a5,  a0,  -44
    s32e    a6,  a0,  -40
    s32e    a7,  a0,  -36
    s32e    a8,  a0,  -32
    s32e    a9,  a0,  -28
    s32e    a10, a0,  -24
    s32e    a11, a0,  -20
    rfwo

    .org    0x140
    .global _WindowUnderflow12
_WindowUnderflow12:
    l32e    a0,  a13, -16
    l32e    a1,  a13, -12
    l32e    a2,  a13,  -8
    l32e    a11, a1,  -12
    l32e    a3,  a13,  -4
    l32e    a4,  a11, -48
    l32e    a5,  a11, -44
    l32e    a6,  a11, -40
    l32e    a7,  a11, -36
    l32e    a8,  a11, -32
    l32e    a9,  a11, -28
    l32e    a10, a11, -24
    l32e    a11, a11, -20
    rfwu

    .section .Level2InterruptVector.text, "ax"
    .align 4
    .global _Level2Vector
_Level2Vector:
    wsr.excsave2 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x180
    jx a2

    .section .Level3InterruptVector.text, "ax"
    .align 4
    .global _Level3Vector
_Level3Vector:
    wsr.excsave3 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x1C0
    jx a2

    .section .Level4InterruptVector.text, "ax"
    .align 4
    .global _Level4Vector
_Level4Vector:
    wsr.excsave4 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x200
    jx a2

    .section .Level5InterruptVector.text, "ax"
    .align 4
    .global _Level5Vector
_Level5Vector:
    wsr.excsave5 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x240
    jx a2

    .section .Level6InterruptVector.text, "ax"
    .align 4
    .global _Level6Vector
_Level6Vector:
    wsr.excsave6 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x280
    jx a2

    .section .NMIExceptionVector.text, "ax"
    .align 4
    .global _NMIExceptionVector
_NMIExceptionVector:
    wsr.excsave7 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x2C0
    jx a2

    .section .KernelExceptionVector.text, "ax"
    .align 4
    .global _KernelExceptionVector
_KernelExceptionVector:
    wsr.excsave1 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x300
    jx a2

    .section .UserExceptionVector.text, "ax"
    .align 4
    .global _UserExceptionVector
_UserExceptionVector:
    wsr.excsave1 a0
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x340
    jx a2

    .section .DoubleExceptionVector.text, "ax"
    .align 4
    .global _DoubleExceptionVector
_DoubleExceptionVector:
    movi a2, 1
    slli a2, a2, 30
    addi a2, a2, 0x3C0
    jx a2

    .section .text.vectors_anchor, "ax"
    .global _tinygo_vectors_present
    .type   _tinygo_vectors_present,@function
_tinygo_vectors_present:
    retw

    .global runtime._tinygo_vectors_present
    .set    runtime._tinygo_vectors_present, _tinygo_vectors_present
