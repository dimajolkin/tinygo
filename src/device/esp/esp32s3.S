
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
2:
    .long _vector_base

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

    /******************************************************************
     * CRITICAL: Initialize cache/MMU and copy vectors
     * 
     * TinyGo is self-booting without ESP-IDF bootloader.
     * We must initialize cache before accessing Flash.
     * 
     * Minimal cache init (based on ESP-IDF bootloader):
     * 1. Disable caches
     * 2. Invalidate caches  
     * 3. Enable caches
     ******************************************************************/
    
    // ESP32-S3 cache control registers
    #define EXTMEM_DCACHE_CTRL_REG    0x3FF43000
    #define EXTMEM_ICACHE_CTRL_REG    0x3FF43008
    #define EXTMEM_CACHE_ENABLE       (1 << 0)
    #define EXTMEM_CACHE_INVALIDATE   (1 << 1)
    
    // Step 1: Disable caches
    movi a2, EXTMEM_DCACHE_CTRL_REG
    l32i a3, a2, 0
    movi a4, ~EXTMEM_CACHE_ENABLE
    and a3, a3, a4
    s32i a3, a2, 0
    
    movi a2, EXTMEM_ICACHE_CTRL_REG
    l32i a3, a2, 0
    and a3, a3, a4
    s32i a3, a2, 0
    
    // Step 2: Invalidate caches
    movi a2, EXTMEM_DCACHE_CTRL_REG
    l32i a3, a2, 0
    movi a4, EXTMEM_CACHE_INVALIDATE
    or a3, a3, a4
    s32i a3, a2, 0
    
    movi a2, EXTMEM_ICACHE_CTRL_REG
    l32i a3, a2, 0
    or a3, a3, a4
    s32i a3, a2, 0
    
    // Wait for invalidation complete
.Lwait_dcache_inv:
    movi a2, EXTMEM_DCACHE_CTRL_REG
    l32i a3, a2, 0
    movi a4, EXTMEM_CACHE_INVALIDATE
    and a3, a3, a4
    bnez a3, .Lwait_dcache_inv
    
.Lwait_icache_inv:
    movi a2, EXTMEM_ICACHE_CTRL_REG
    l32i a3, a2, 0
    movi a4, EXTMEM_CACHE_INVALIDATE
    and a3, a3, a4
    bnez a3, .Lwait_icache_inv
    
    // Step 3: Enable caches
    movi a2, EXTMEM_DCACHE_CTRL_REG
    l32i a3, a2, 0
    movi a4, EXTMEM_CACHE_ENABLE
    or a3, a3, a4
    s32i a3, a2, 0
    
    movi a2, EXTMEM_ICACHE_CTRL_REG
    l32i a3, a2, 0
    or a3, a3, a4
    s32i a3, a2, 0
    
    // NOTE: Additional memory initialization (MMU, cache bus, autoload)
    // is done in Go code (initMemorySubsystems) after basic cache is ready.
    // This allows better error handling and logging.
    
    // Now safe to access Flash - check if vectors already in place
    l32r a0, 2b              // _vector_base address
    l32i a1, a0, 0           // First word at _vector_base
    movi a2, 0x13d100        // Expected: WSR a0, EXCSAVE_1 (lower 3 bytes)
    movi a3, 0xFFFFFF        // Mask for lower 24 bits
    and a1, a1, a3           // Keep lower 3 bytes
    beq a1, a2, .Lvectors_ok // Already copied, skip
    
    // Copy vectors from Flash to IRAM
    movi a0, _vectors_flash_start
    movi a1, _vector_base
    movi a2, _vectors_flash_end

.Lcopy_vectors:
    l32i a3, a0, 0          // Load from Flash
    s32i a3, a1, 0          // Store to IRAM
    addi a0, a0, 4
    addi a1, a1, 4
    bltu a0, a2, .Lcopy_vectors

.Lvectors_ok:
    // Set VECBASE to vector table now in IRAM
    l32r a2, 2b
    wsr.vecbase a2
    rsync

    // Jump to the runtime start function written in Go.
    call4 main

.section .text.tinygo_scanCurrentStack
.global tinygo_scanCurrentStack
tinygo_scanCurrentStack:
    // TODO: save callee saved registers on the stack
    j tinygo_scanstack
