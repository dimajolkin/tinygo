#include "xtensa_esp32s3_macros.inc"
#include "xtensa_rtos_esp32s3.h"

    .text

    .global _xt_context_save
    .type   _xt_context_save,@function
_xt_context_save:
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

    rsr     a3,  SAR
    s32i    a3,  sp, XT_STK_SAR

    rsr     a3,  LBEG
    s32i    a3,  sp, XT_STK_LBEG
    rsr     a3,  LEND
    s32i    a3,  sp, XT_STK_LEND
    rsr     a3,  LCOUNT
    s32i    a3,  sp, XT_STK_LCOUNT

    ret

    .global _xt_context_restore
    .type   _xt_context_restore,@function
_xt_context_restore:
    l32i    a3,  sp, XT_STK_LBEG
    wsr     a3,  LBEG
    l32i    a3,  sp, XT_STK_LEND
    wsr     a3,  LEND
    l32i    a3,  sp, XT_STK_LCOUNT
    wsr     a3,  LCOUNT

    l32i    a2,  sp, XT_STK_A2
    l32i    a3,  sp, XT_STK_A3
    l32i    a4,  sp, XT_STK_A4
    l32i    a5,  sp, XT_STK_A5
    l32i    a6,  sp, XT_STK_A6
    l32i    a7,  sp, XT_STK_A7
    l32i    a8,  sp, XT_STK_A8
    l32i    a9,  sp, XT_STK_A9
    l32i    a10, sp, XT_STK_A10
    l32i    a11, sp, XT_STK_A11
    l32i    a12, sp, XT_STK_A12
    l32i    a13, sp, XT_STK_A13
    l32i    a14, sp, XT_STK_A14
    l32i    a15, sp, XT_STK_A15

    l32i    a3,  sp, XT_STK_SAR
    wsr     a3,  SAR

    ret
