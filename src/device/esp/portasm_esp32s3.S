//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"
#include "xtensa_rtos_esp32s3.h"

    .extern port_xSchedulerRunning
    .extern port_interruptNesting
    .extern port_switch_flag
    .extern _xt_tick_divisor
    .extern port_yield_from_isr

    .text

    .global tinygo_startTask
    .type   tinygo_startTask,@function
// tinygo_startTask(a2=arg, a3=fn)
tinygo_startTask:
    rsr.ps  a4
    movi    a5, 0xFFFFFFF0
    and     a4, a4, a5
    wsr.ps  a4
    rsync
    mov.n   a6, a2
    callx4  a3
    call4   tinygo_task_exit
    .size   tinygo_startTask, .-tinygo_startTask

    .global tinygo_swapTask
    .type   tinygo_swapTask,@function
// tinygo_swapTask(a2=newStack, a3=*oldStack)
tinygo_swapTask:
    entry   sp, 32
    rsr.ps  a4
    movi    a5, 0xFFFFFFF0
    and     a5, a4, a5
    movi    a6, 0xF
    or      a5, a5, a6
    wsr.ps  a5
    rsync
    SPILL_ALL_WINDOWS
    s32i.n  sp, a3, 0
    mov.n   sp, a2
    l32i.n  a4, sp, 0
    addi    a5, sp, -16
    s32i    a4, a5, 0
    retw.n
    .size   tinygo_swapTask, .-tinygo_swapTask

// ----------------------------------------------------------------------------
// Interrupt enter/exit hooks
// ----------------------------------------------------------------------------

    .global _frxt_int_enter
    .type   _frxt_int_enter,@function
_frxt_int_enter:
    entry   sp, XT_STK_FRMSZ

    s32i    a0, sp, XT_STK_A0
    s32i    a1, sp, XT_STK_A1
    s32i    a2, sp, XT_STK_A2
    s32i    a3, sp, XT_STK_A3
    s32i    a4, sp, XT_STK_A4
    s32i    a5, sp, XT_STK_A5
    s32i    a6, sp, XT_STK_A6
    s32i    a7, sp, XT_STK_A7
    s32i    a8, sp, XT_STK_A8
    s32i    a9, sp, XT_STK_A9
    s32i    a10, sp, XT_STK_A10
    s32i    a11, sp, XT_STK_A11
    s32i    a12, sp, XT_STK_A12
    s32i    a13, sp, XT_STK_A13
    s32i    a14, sp, XT_STK_A14
    s32i    a15, sp, XT_STK_A15

    rsr.ps  a2
    s32i    a2, sp, XT_STK_PS
    rsr.excsave1 a2
    s32i    a2, sp, XT_STK_TMP0
    rsr.sar a2
    s32i    a2, sp, XT_STK_SAR
    rsr.lbeg a2
    s32i    a2, sp, XT_STK_LBEG
    rsr.lend a2
    s32i    a2, sp, XT_STK_LEND
    rsr.lcount a2
    s32i    a2, sp, XT_STK_LCOUNT

    addi    a2, sp, XT_STK_FRMSZ
    s32i    a2, sp, XT_STK_A1

    movi    a2, port_interruptNesting
    l32i    a3, a2, 0
    addi    a3, a3, 1
    s32i    a3, a2, 0

    call0   tinygo_xt_int_enter

    movi    a2, port_interruptNesting
    l32i    a3, a2, 0
    addi    a3, a3, -1
    s32i    a3, a2, 0

    l32i    a2, sp, XT_STK_LCOUNT
    wsr.lcount a2
    l32i    a2, sp, XT_STK_LEND
    wsr.lend a2
    l32i    a2, sp, XT_STK_LBEG
    wsr.lbeg a2
    l32i    a2, sp, XT_STK_SAR
    wsr.sar a2

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

    l32i    a2, sp, XT_STK_PS
    wsr.ps  a2

    retw.n
    .size   _frxt_int_enter, .-_frxt_int_enter

    .global _frxt_int_exit
    .type   _frxt_int_exit,@function
_frxt_int_exit:
    entry   sp, 32

    movi    a2, port_switch_flag
    l32i    a3, a2, 0
    beqz    a3, 1f
    movi    a3, 0
    s32i    a3, a2, 0
    call0   tinygo_xt_int_exit
1:
    retw.n
    .size   _frxt_int_exit, .-_frxt_int_exit

// ----------------------------------------------------------------------------
// Yield helpers
// ----------------------------------------------------------------------------

    .global vPortYield
    .type   vPortYield,@function
vPortYield:
    entry   sp, 32
    movi    a2, port_switch_flag
    movi    a3, 1
    s32i    a3, a2, 0
    retw.n
    .size   vPortYield, .-vPortYield

    .global vPortYieldFromISR
    .type   vPortYieldFromISR,@function
vPortYieldFromISR:
    entry   sp, 32
    movi    a2, port_switch_flag
    movi    a3, 1
    s32i    a3, a2, 0
    call0   port_yield_from_isr
    retw.n
    .size   vPortYieldFromISR, .-vPortYieldFromISR

    .global _frxt_timer_int
    .type   _frxt_timer_int,@function
_frxt_timer_int:
    entry   sp, 32
    movi    a2, 1
    slli    a2, a2, XT_TIMER_INTNUM
    wsr.intclear a2

    call0   tinygo_xt_timer_int
    retw.n
    .size   _frxt_timer_int, .-_frxt_timer_int

    .global _frxt_tick_timer_init
    .type   _frxt_tick_timer_init,@function
_frxt_tick_timer_init:
    entry   sp, 32

    movi    a2, 1
    slli    a2, a2, XT_TIMER_INTNUM
    rsr.intenable a3
    or      a3, a3, a2
    wsr.intenable a3

    movi    a2, _xt_tick_divisor
    l32i    a2, a2, 0
    wsr.ccount a2

    retw.n
    .size   _frxt_tick_timer_init, .-_frxt_tick_timer_init

// ----------------------------------------------------------------------------
// Глобальные данные, используемые портом
// ----------------------------------------------------------------------------

    .section .bss.portasm, "aw", @nobits
    .align 4
    .global port_xSchedulerRunning
port_xSchedulerRunning:
    .word   0

    .global port_interruptNesting
port_interruptNesting:
    .word   0

    .global port_switch_flag
port_switch_flag:
    .word   0

    .global _xt_tick_divisor
_xt_tick_divisor:
    .word   0

    .section .text.port_yield_from_isr, "ax"
    .align 4
    .global port_yield_from_isr
    .type   port_yield_from_isr,@function
port_yield_from_isr:
    retw.n
    .size   port_yield_from_isr, .-port_yield_from_isr
