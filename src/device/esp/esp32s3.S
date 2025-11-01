/*
 * ESP32-S3 Startup Code with Interrupt Vector Table Initialization
 * 
 * This file sets up the CPU and initializes the interrupt vector table
 * by setting the VECBASE register to point to _vector_base.
 * 
 * Vector table layout (defined in xtensa_vectors_esp32s3.S):
 *   VECBASE + 0x000: Reset vector (not used on ESP32-S3)
 *   VECBASE + 0x180: User exception vector (Level-1 interrupts)
 *   VECBASE + 0x1C0: Kernel exception vector
 *   VECBASE + 0x200: Double exception vector
 *   VECBASE + 0x240: Level 2-7 interrupt vectors
 * 
 * The vectors are placed in IRAM by the linker script (esp32s3.ld)
 * and do NOT need to be copied at runtime.
 */

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

    // Set VECBASE to point to interrupt vectors in IRAM
    // Note: ROM bootloader already copied vectors from flash to IRAM (0x40374000)
    // We just need to tell the CPU where they are by setting VECBASE register
    call0 runtime.setVecbaseToVectorBase

    // TEST: Initialize _isr_call_count to 99 to verify Go can read it correctly
    movi a2, _isr_call_count
    movi a3, 99
    s32i a3, a2, 0

    // Jump to the runtime start function written in Go.
    call4 main

.section .text.tinygo_scanCurrentStack
.global tinygo_scanCurrentStack
tinygo_scanCurrentStack:
    // TODO: save callee saved registers on the stack
    j tinygo_scanstack
