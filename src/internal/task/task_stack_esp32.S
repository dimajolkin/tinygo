//go:build tinygo && esp32 && !esp32s3

.section .text.tinygo_startTask,"ax",@progbits
.global  tinygo_startTask
.type    tinygo_startTask, %function
tinygo_startTask:
    // mark: entered tinygo_startTask
    movi a2, 0x53544152  // 'STAR'
    call0  debugMark
    // Now that we're on the new task stack, restore original PS
    // to re-enable interrupts at the previous level.
    rsr.ps  a4
    movi    a5, 0xFFFFFFF0
    and     a4, a4, a5        // clear INTLEVEL bits [3:0]
    wsr.ps  a4
    rsync
    // Small assembly stub for starting a goroutine. This already runs on the
    // new stack, control reaches this function after returning from the initial
    // tinygo_swapTask below (the retw.n instruction).
    //
    // The stack was set up in such a way that it looks as if this function was
    // paused using tinygo_swapTask by setting up the parent register window and
    // return pointer as a call4 instruction - except such a call never took
    // place. Instead, the stack pointer is switched to the new stack after all
    // live-but-invisible registers have been flushed to the stack. This means
    // that all registers as present in tinygo_swapTask are moved four up (a2 in
    // tinygo_swapTask is a6 in this function). We don't use any of those
    // registers however. Instead, the retw.n instruction will load them through
    // an underflow exception from the stack which means we get a0-a3 as defined
    // in task_stack_esp32.go.

    // Branch to the "goroutine start" function. The first (and only) parameter
    // is stored in a2, but has to be moved to a6 to make it appear as a2 in the
    // goroutine start function (due to changing the register window by four
    // with callx4).
    mov.n a6, a2
    callx4 a3

    // After return, exit this goroutine. This call never returns.
    call4  tinygo_task_exit

.section .text.tinygo_swapTask,"ax",@progbits
.global tinygo_swapTask
.type tinygo_swapTask, %function
tinygo_swapTask:
    // This function gets the following parameters:
    // a2 = newStack uintptr
    // a3 = oldStack *uintptr

    // Reserve 32 bytes on the stack. It really needs to be 32 bytes, with 16
    // extra at the bottom to adhere to the ABI.
    entry sp, 32

    // mark: entered swapTask
    movi a2, 0x53574150  // 'SWAP'
    call0  debugMark

    // Raise INTLEVEL to 15 while flushing register windows to avoid any
    // interrupt entering with transient SP/register-window state.
    rsr.ps  a4
    movi    a5, 0xFFFFFFF0
    and     a5, a4, a5        // clear INTLEVEL bits [3:0]
    movi    a6, 0xF
    or      a5, a5, a6        // set INTLEVEL=15
    wsr.ps  a5
    rsync

    // Flush all unsaved registers to the stack.
    // This trick has been borrowed from the Zephyr project:
    // https://github.com/zephyrproject-rtos/zephyr/blob/d79b003758/arch/xtensa/include/xtensa-asm2-s.h#L17
    and a12, a12, a12
    rotw 3
    and a12, a12, a12
    rotw 3
    and a12, a12, a12
    rotw 3
    and a12, a12, a12
    rotw 3
    and a12, a12, a12
    rotw 4

    // Keep INTLEVEL=15 across SP switch and retw to avoid races on LX7.
    // Original PS will be restored (INTLEVEL cleared) in tinygo_startTask.

    // At this point, the following is true:
    //     WindowStart == 1 << WindowBase
    // Therefore, we don't need to do this manually.
    // It also means that the stack pointer can now be safely modified.

    // Save the current stack pointer in oldStack.
    s32i.n  sp, a3, 0

    // Switch to the new stack pointer (newStack).
    mov.n   sp, a2

    // Ensure underflow vector (_WindowUnderflow4) can restore a0..a3 from
    // [a5-16..-4]. Our archInit sets:
    //   r.a0 at (sp-16), r.a1 at (sp-12), r.a2 at (sp-8), r.a3 at (sp-4)
    // but r.locals[0] (at sp+0) holds the disguised return address for a0.
    // Copy locals[0] (sp+0) into the a0 slot (sp-16) expected by vector.
    l32i.n  a4, sp, 0        // a4 = *(sp+0) locals[0]
    addi    a5, sp, -16
    s32i    a4, a5, 0        // store into (sp-16)

    // Return into the new stack. This instruction will trigger a window
    // underflow, reloading the saved registers from the stack.
    // mark: about to retw
    movi a2, 0x52545700  // 'RTW\0'
    call0  debugMark
    retw.n
