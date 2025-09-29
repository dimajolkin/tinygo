/*
 * Local Xtensa constants for ESP32-S3 (LX7) used by TinyGo assembly.
 * Keep this file minimal to avoid pulling full ESP-IDF headers.
 */

#define XCHAL_NUM_AREGS    64

    .set XCHAL_PS_OWB_SHIFT, 8        /* PS.OWB bitfield shift */
    .set XCHAL_PS_OWB_BITS,  4        /* PS.OWB bitfield width */
    .set PS_INTLEVEL_MASK,   0x0000000F
    .set PS_EXCM_MASK,       0x00000010
    .set PS_WOE_MASK,        0x00040000
    .set PS_UM_MASK,         0x00000020
    .set PS_CALLINC_MASK,    0x00030000
    .set PS_CALLINC_SHIFT,   16
    .set PS_CALLINC_1,       0x00010000
    .set PS_INTLEVEL_EXCM_CLR, 0xFFFFFFE0
    .set XCHAL_EXCM_LEVEL,   3

    .set XT_STK_EXIT,        0
    .set XT_STK_PC,          4
    .set XT_STK_PS,          8
    .set XT_STK_A0,          12
    .set XT_STK_A1,          16
    .set XT_STK_A2,          20
    .set XT_STK_A3,          24
    .set XT_STK_A4,          28
    .set XT_STK_A5,          32
    .set XT_STK_A6,          36
    .set XT_STK_A7,          40
    .set XT_STK_A8,          44
    .set XT_STK_A9,          48
    .set XT_STK_A10,         52
    .set XT_STK_A11,         56
    .set XT_STK_A12,         60
    .set XT_STK_A13,         64
    .set XT_STK_A14,         68
    .set XT_STK_A15,         72
    .set XT_STK_SAR,         76
    .set XT_STK_EXCCAUSE,    80
    .set XT_STK_EXCVADDR,    84
    .set XT_STK_LBEG,        88
    .set XT_STK_LEND,        92
    .set XT_STK_LCOUNT,      96
    .set XT_STK_TMP0,        100
    .set XT_STK_TMP1,        104
    .set XT_STK_TMP2,        108

    .set XT_FRAME_SIZE,      112


