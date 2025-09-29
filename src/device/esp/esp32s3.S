
// The following definitions were copied from:
// esp-idf/components/xtensa/include/xtensa/corebits.h
#define PS_WOE_MASK                0x00040000
#define PS_OWB_MASK                0x00000F01
#define PS_CALLINC_MASK            0x00030000
#define PS_WOE                     PS_WOE_MASK

// Only calling it call_start_cpu0 for consistency with ESP-IDF.
.section .literal.call_start_cpu0
1:
    .long _stack_top

.section .text.call_start_cpu0
.global call_start_cpu0
call_start_cpu0:

    // We need to set the stack pointer to a different value. This is somewhat
    // complicated in the Xtensa architecture. The code below is a modified
    // version of the following code:
    // https://github.com/espressif/esp-idf/blob/c77c4ccf/components/xtensa/include/xt_instr_macros.h#L47

    // Disable WOE.
    rsr.ps a2
    movi a3, ~(PS_WOE_MASK)
    and a2, a2, a3
    wsr.ps a2
    rsync

    // Set WINDOWSTART to 1 << WINDOWBASE.
    rsr.windowbase  a2
    ssl  a2
    movi a2, 1
    sll  a2, a2
    wsr.windowstart a2
    rsync

    // Load new stack pointer.
    l32r sp, 1b

    // Re-enable WOE.
    rsr.ps a2
    movi a3, PS_WOE
    or a2, a2, a3
    wsr.ps a2
    rsync

    // Enable the FPU (coprocessor 0 so the lowest bit).
    movi a2, 1
    wsr.cpenable a2
    rsync


    // Jump to the runtime start function written in Go.
    call4 main

.section .text.tinygo_scanCurrentStack
.global tinygo_scanCurrentStack
tinygo_scanCurrentStack:
    // TODO: save callee saved registers on the stack
    j tinygo_scanstack

// ----------------------------------------------------------------------------
// Helper for examples: set INTENABLE bits (OR with existing value)
// Signature in Go: //go:extern asmSetIntenable
//                  func asmSetIntenable(mask uint32)
// Arg a2 = mask (Xtensa call0 ABI)
// ----------------------------------------------------------------------------

    .section .text.asmSetIntenable
    .global asmSetIntenable
    .type asmSetIntenable,@function
asmSetIntenable:
    rsr.intenable a3
    or      a3, a3, a2
    wsr.intenable a3
    rsync
    retw

    // Provide alias for package main reference: main.asmSetIntenable
    .global main.asmSetIntenable
    .set    main.asmSetIntenable, asmSetIntenable

// ----------------------------------------------------------------------------
// Debug helpers to read PS and INTERRUPT quickly from Go
// ----------------------------------------------------------------------------

    .section .text.get_ps
    .global get_ps
    .type get_ps,@function
get_ps:
    rsr.ps a2
    retw
    .global runtime.get_ps
    .set    runtime.get_ps, get_ps

    .section .text.get_interrupt
    .global get_interrupt
    .type get_interrupt,@function
get_interrupt:
    rsr.interrupt a2
    retw
    .global runtime.get_interrupt
    .set    runtime.get_interrupt, get_interrupt

// ----------------------------------------------------------------------------
// Debug helper: read VECBASE special register (returns in a2)
// Signature in Go: //go:extern get_vecbase
//                  func get_vecbase() uint32
// ----------------------------------------------------------------------------

    .section .text.get_vecbase
    .global get_vecbase
    .type get_vecbase,@function
get_vecbase:
    rsr.vecbase a2
    retw

    // Provide alias for package runtime reference: runtime.get_vecbase
    .global runtime.get_vecbase
    .set    runtime.get_vecbase, get_vecbase

// ----------------------------------------------------------------------------
// Disable all maskable interrupts by setting PS.INTLEVEL to 15
// Signature in Go: //go:extern disableAllInterrupts
//                  func disableAllInterrupts()
// ----------------------------------------------------------------------------

    .section .text.disableAllInterrupts
    .global disableAllInterrupts
    .type disableAllInterrupts,@function
disableAllInterrupts:
    rsr.ps  a2
    movi    a3, 0xFFFFFFF0
    and     a2, a2, a3        // clear INTLEVEL bits [3:0]
    movi    a3, 0xF
    or      a2, a2, a3        // set INTLEVEL=15
    wsr.ps  a2
    rsync
    retw

    // Provide alias for package runtime reference: runtime.disableAllInterrupts
    .global runtime.disableAllInterrupts
    .set    runtime.disableAllInterrupts, disableAllInterrupts

// ----------------------------------------------------------------------------
// Atomic VECBASE override programming under raised INTLEVEL
// Signature in Go: //go:extern enableVecbaseOverrideAsm
//                  func enableVecbaseOverrideAsm(vecbaseShifted uint32)
// vecbaseShifted = (_vector_table >> 2) & 0x3FFFFF
// ----------------------------------------------------------------------------

    .section .text.enableVecbaseOverrideAsm
    .global enableVecbaseOverrideAsm
    .type enableVecbaseOverrideAsm,@function
enableVecbaseOverrideAsm:
    // a2 = vecbaseShifted
    // Save PS and raise INTLEVEL to 15
    rsr.ps  a3
    movi    a4, 0xFFFFFFF0
    and     a5, a3, a4
    movi    a4, 0xF
    or      a5, a5, a4
    wsr.ps  a5
    rsync

    // SENSITIVE base
    movi    a6, 0x600C1000

    // CORE_0_VECBASE_OVERRIDE_0 (offset 0x1BC): set WORLD_MASK bit 0 (write 1)
    movi    a7, 0x1BC
    add     a7, a6, a7
    movi    a8, 1
    s32i    a8, a7, 0

    // CORE_0_VECBASE_OVERRIDE_1 (0x1C0): write WORLD0_VALUE and SEL=0b01 directly
    movi    a7, 0x1C0
    add     a7, a6, a7
    // a8 = (a2 & 0x3FFFFF) | (1<<22)
    movi    a9, 0x3FFFFF
    and     a8, a2, a9
    movi    a10, 1
    slli    a10, a10, 22
    or      a8, a8, a10
    s32i    a8, a7, 0

    // CORE_0_VECBASE_OVERRIDE_LOCK (0x1B8): set bit 0
    movi    a7, 0x1B8
    add     a7, a6, a7
    movi    a8, 1
    s32i    a8, a7, 0

    // Restore PS
    wsr.ps  a3
    rsync
    retw

    // Provide alias for package runtime reference: runtime.enableVecbaseOverrideAsm
    .global runtime.enableVecbaseOverrideAsm
    .set    runtime.enableVecbaseOverrideAsm, enableVecbaseOverrideAsm

    .section .text.tinygo_irq19_handler
    .global tinygo_irq19_handler
    .type tinygo_irq19_handler,@function

tinygo_irq19_handler:
    // Clear GPIO interrupt status for GPIO0-31 and GPIO32-53 just in case
    // GPIO base: use device registers via absolute addresses (from SVD)
    // GPIO.STATUS_W1TC @ esp.GPIO base + 0x10 (offset differs, using absolute)
    // For ESP32-S3: GPIO STATUS_W1TC at 0x6000401C, STATUS1_W1TC at 0x60004044
    movi    a2, 0x6000401C
    movi    a3, 0xFFFFFFFF
    s32i    a3, a2, 0

    movi    a2, 0x60004044
    movi    a3, 0x003FFFFF
    s32i    a3, a2, 0

    rfi 1
