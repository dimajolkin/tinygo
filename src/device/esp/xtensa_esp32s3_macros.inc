/*
 * Local Xtensa constants for ESP32-S3 (LX7) used by TinyGo assembly.
 * Keep this file minimal to avoid pulling full ESP-IDF headers.
 */

#define XCHAL_NUM_AREGS    64
#define __XTENSA_CALL0_ABI__ 1
#define XCHAL_HAVE_WINDOWED  1
#define XCHAL_HAVE_THREADPTR 1
#define XCHAL_HAVE_LOOPS     1

#define XCHAL_PS_OWB_SHIFT 8
#define XCHAL_PS_OWB_BITS  4
#define PS_INTLEVEL_MASK   0x0000000F
#define PS_EXCM_MASK       0x00000010
#define PS_WOE_MASK        0x00040000
#define PS_WOE             PS_WOE_MASK
#define PS_UM_MASK         0x00000020
#define PS_UM              0x00000020
#define PS_CALLINC_MASK    0x00030000
#define PS_CALLINC_SHIFT   16
#define PS_CALLINC_1       0x00010000
#define PS_INTLEVEL_EXCM_CLR 0xFFFFFFE0
#define XCHAL_EXCM_LEVEL   3

#define XCHAL_EXTRA_SA_SIZE 0
#define XCHAL_EXTRA_SA_ALIGN 16
#define XCHAL_TOTAL_SA_ALIGN 16
#define XCHAL_CP_NUM       0
#define XCHAL_CP_MAX       0

#define XT_USE_SWPRI       0
#define XT_USE_OVLY        0
#define XT_RTOS_TIMER_INT  _frxt_timer_int
#define XT_USE_THREAD_SAFE_CLIB 0

#define XCHAL_INTLEVEL1_MASK 0x000637FF
#define XCHAL_INTLEVEL2_MASK 0x00380000
#define XCHAL_INTLEVEL3_MASK 0x28C08800
#define XCHAL_INTLEVEL4_MASK 0x53000000
#define XCHAL_INTLEVEL5_MASK 0x84010000
#define XCHAL_INTLEVEL6_MASK 0x00000000
#define XCHAL_INTLEVEL7_MASK 0x00004000

#define XCHAL_TIMER0_INTERRUPT 6
#define XCHAL_TIMER1_INTERRUPT 15
#define XCHAL_TIMER2_INTERRUPT 16
#define XCHAL_TIMER3_INTERRUPT 23
#define XT_TIMER_INTNUM      XCHAL_TIMER1_INTERRUPT
#define XT_TIMER_INTEN       (1 << XT_TIMER_INTNUM)

#define XT_STK_EXIT        0
#define XT_STK_PC          4
#define XT_STK_PS          8
#define XT_STK_A0          12
#define XT_STK_A1          16
#define XT_STK_A2          20
#define XT_STK_A3          24
#define XT_STK_A4          28
#define XT_STK_A5          32
#define XT_STK_A6          36
#define XT_STK_A7          40
#define XT_STK_A8          44
#define XT_STK_A9          48
#define XT_STK_A10         52
#define XT_STK_A11         56
#define XT_STK_A12         60
#define XT_STK_A13         64
#define XT_STK_A14         68
#define XT_STK_A15         72
#define XT_STK_SAR         76
#define XT_STK_EXCCAUSE    80
#define XT_STK_EXCVADDR    84
#define XT_STK_LBEG        88
#define XT_STK_LEND        92
#define XT_STK_LCOUNT      96
#define XT_STK_TMP0        100
#define XT_STK_TMP1        104
#define XT_STK_TMP2        108

#define XT_SOL_EXIT        0
#define XT_SOL_PC          4
#define XT_SOL_PS          8
#define XT_SOL_THREADPTR   12
#define XT_SOL_A12         16
#define XT_SOL_A13         20
#define XT_SOL_A14         24
#define XT_SOL_A15         28
#define XT_SOL_FRMSZ       32

#define XT_STK_FRMSZ      144
#define XT_FRAME_SIZE      144

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


