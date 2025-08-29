
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
    // Debug: Light up GPIO4 immediately to signal entry point reached
    // ESP32-S3 GPIO base address is different from ESP32
    movi a2, 0x60004000     // GPIO base address for ESP32-S3
    movi a3, 0x10           // GPIO4 bit mask (1 << 4)
    s32i a3, a2, 0x24       // GPIO_ENABLE_REG offset 0x24: enable GPIO4 output
    s32i a3, a2, 0x8        // GPIO_OUT_W1TS_REG offset 0x8: set GPIO4 high
    
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
