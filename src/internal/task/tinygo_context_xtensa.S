//go:build scheduler.tasks && esp32s3

#include "xtensa_esp32s3_macros.inc"

.extern _frxt_int_enter
.extern _frxt_int_exit

#define FRAME_SIZE       XT_STK_FRMSZ

// tinygo_context_save(oldSpPtr=a2, frame=a3)
.section .text.tinygo_context_save,"ax",@progbits
.global tinygo_context_save
.type tinygo_context_save,@function
tinygo_context_save:
    entry   sp, 32

    mov.n   a4, a2            // a4 = *oldSpPtr
    mov.n   a5, a3            // a5 = frame pointer

    // Save PS first so we know interrupt level
    rsr.ps  a6
    s32i    a6, a5, XT_STK_PS

    // Save regular registers using helper macro
    mov.n   a2, a5
    call0   _xt_context_save

    // Save return address (PC) and A0/A1 for restore
    rsr.excsave1 a6
    s32i    a6, a5, XT_STK_PC
    s32i    a0, a5, XT_STK_A0
    s32i    a1, a5, XT_STK_A1

    // Save exception status
    rsr.exccause a6
    s32i    a6, a5, XT_STK_EXCCAUSE
    rsr.excvaddr a6
    s32i    a6, a5, XT_STK_EXCVADDR

    // Save SP for Go runtime to resume later
    s32i    a5, a4, 0

    retw.n
.size tinygo_context_save, .-tinygo_context_save

// tinygo_context_restore(newSp=a2, frame=a3)
.section .text.tinygo_context_restore,"ax",@progbits
.global tinygo_context_restore
.type tinygo_context_restore,@function
tinygo_context_restore:
    entry   sp, 32

    mov.n   a4, a2            // new SP
    mov.n   a5, a3            // frame pointer

    // Restore registers
    mov.n   a2, a5
    call0   _xt_context_restore

    // Restore exception state
    l32i    a6, a5, XT_STK_EXCCAUSE
    wsr.exccause a6
    l32i    a6, a5, XT_STK_EXCVADDR
    wsr.excvaddr a6

    // Restore PS before switching stacks
    l32i    a6, a5, XT_STK_PS
    wsr.ps  a6

    // Restore SP and return registers
    mov.n   sp, a4
    l32i    a0, a5, XT_STK_A0
    l32i    a1, a5, XT_STK_A1

    retw.n
.size tinygo_context_restore, .-tinygo_context_restore

