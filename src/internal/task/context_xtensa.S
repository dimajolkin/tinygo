//go:build tinygo && esp32s3

#include "../../device/esp/xtensa_esp32s3_macros.inc"

// Объявляем внешние функции
.extern debugMark

.macro SPILL_ALL_WINDOWS
#if XCHAL_NUM_AREGS == 64
    and     a12, a12, a12
    rotw    3
    and     a12, a12, a12
    rotw    3
    and     a12, a12, a12
    rotw    3
    and     a12, a12, a12
    rotw    3
    and     a12, a12, a12
    rotw    4
#elif XCHAL_NUM_AREGS == 32
    and     a12, a12, a12
    rotw    3
    and     a12, a12, a12
    rotw    3
    and     a4, a4, a4
    rotw    2
#else
#error Unsupported XCHAL_NUM_AREGS
#endif
.endm

.section .text.tinygo_context_save,"ax",@progbits
.align 4
.global tinygo_context_save
.type   tinygo_context_save,@function

tinygo_context_save:
    mov     a10, a2                  // keep pointer to *oldStack
    addi    sp, sp, -XT_FRAME_SIZE
    addi    a11, sp, XT_FRAME_SIZE   // original SP before frame

    s32i    a0,  sp, XT_STK_PC
    s32i    a0,  sp, XT_STK_A0
    s32i    a11, sp, XT_STK_A1
    s32i    a2,  sp, XT_STK_A2
    s32i    a3,  sp, XT_STK_A3
    s32i    a4,  sp, XT_STK_A4
    s32i    a5,  sp, XT_STK_A5
    s32i    a6,  sp, XT_STK_A6
    s32i    a7,  sp, XT_STK_A7
    s32i    a8,  sp, XT_STK_A8
    s32i    a9,  sp, XT_STK_A9
    s32i    a10, sp, XT_STK_A10
    s32i    a11, sp, XT_STK_A11
    s32i    a12, sp, XT_STK_A12
    s32i    a13, sp, XT_STK_A13
    s32i    a14, sp, XT_STK_A14
    s32i    a15, sp, XT_STK_A15

    rsr     a12, SAR
    s32i    a12, sp, XT_STK_SAR
    rsr     a12, EXCCAUSE
    s32i    a12, sp, XT_STK_EXCCAUSE
    rsr     a12, EXCVADDR
    s32i    a12, sp, XT_STK_EXCVADDR
    rsr     a12, LBEG
    s32i    a12, sp, XT_STK_LBEG
    rsr     a12, LEND
    s32i    a12, sp, XT_STK_LEND
    rsr     a12, LCOUNT
    s32i    a12, sp, XT_STK_LCOUNT

    rsr     a12, PS
    s32i    a12, sp, XT_STK_PS

    movi    a13, PS_INTLEVEL_EXCM_CLR
    and     a14, a12, a13
    movi    a13, ~PS_EXCM_MASK
    and     a14, a14, a13
    movi    a13, PS_WOE_MASK
    or      a14, a14, a13
    movi    a13, PS_INTLEVEL_MASK
    and     a15, a12, a13
    bgeui   a15, XCHAL_EXCM_LEVEL, 1f
    movi    a15, XCHAL_EXCM_LEVEL
1:
    or      a14, a14, a15
    wsr     a14, PS
    rsync
    rsr     a13, EPC1
    SPILL_ALL_WINDOWS
    wsr     a12, PS
    rsync
    wsr     a13, EPC1
    rsync

    movi    a14, 0
    s32i    a14, sp, XT_STK_EXIT
    s32i    a14, sp, XT_STK_TMP0
    s32i    a14, sp, XT_STK_TMP1
    s32i    a14, sp, XT_STK_TMP2

    s32i    sp,  a10, 0
    movi    a2, 0xEEEEFF00
    call0   debugMark
    retw

.section .text.tinygo_context_restore,"ax",@progbits
.align 4
.global tinygo_context_restore
.type   tinygo_context_restore,@function

tinygo_context_restore:
    mov     sp, a2
    movi    a2, 0xEEEE0001
    call0   debugMark

    l32i    a4, sp, XT_STK_SAR
    wsr     a4, SAR
    l32i    a4, sp, XT_STK_LBEG
    wsr     a4, LBEG
    l32i    a4, sp, XT_STK_LEND
    wsr     a4, LEND
    l32i    a4, sp, XT_STK_LCOUNT
    wsr     a4, LCOUNT
    l32i    a4, sp, XT_STK_EXCCAUSE
    wsr     a4, EXCCAUSE
    l32i    a4, sp, XT_STK_EXCVADDR
    wsr     a4, EXCVADDR

    l32i    a4, sp, XT_STK_PS
    movi    a2, 0xEEEE0000
    call0   debugMark
    wsr     a4, PS
    rsync

    l32i    a0, sp, XT_STK_A0
    l32i    a1, sp, XT_STK_A1
    l32i    a2, sp, XT_STK_A2
    l32i    a3, sp, XT_STK_A3
    l32i    a4, sp, XT_STK_A4
    l32i    a5, sp, XT_STK_A5
    l32i    a6, sp, XT_STK_A6
    l32i    a7, sp, XT_STK_A7
    l32i    a8, sp, XT_STK_A8
    l32i    a9, sp, XT_STK_A9
    l32i    a10, sp, XT_STK_A10
    l32i    a11, sp, XT_STK_A11
    l32i    a12, sp, XT_STK_A12
    l32i    a13, sp, XT_STK_A13
    l32i    a14, sp, XT_STK_A14
    l32i    a15, sp, XT_STK_A15

    movi    a2, 0xEEEE1111
    call0   debugMark
    retw
