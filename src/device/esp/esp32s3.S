
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
     * OPTION 1 (CURRENT): Use ROM_Boot_Cache_Init() - complete init
     * OPTION 2 (MANUAL): Use individual functions for more control
     * 
     * ROM function symbols defined in targets/esp32s3.ld from ESP-IDF.
     * These functions are in ROM chip memory and always available.
     ******************************************************************/
    
    // OPTION 1: Full cache initialization using ROM bootloader function
    // void ROM_Boot_Cache_Init(void)
    // This does: invalidate + enable + MMU init + owner init
    movi a0, ROM_Boot_Cache_Init  // Symbol from linker script
    callx0 a0
    
    /* OPTION 2: Manual initialization (commented out, but can be used)
    // 1. Invalidate ICache
    movi a0, Cache_Invalidate_ICache_All
    callx0 a0
    
    // 2. Invalidate DCache
    movi a0, Cache_Invalidate_DCache_All
    callx0 a0
    
    // 3. Initialize MMU (identity mapping for Flash access)
    movi a0, Cache_MMU_Init
    callx0 a0
    
    // 4. Enable ICache with autoload=0
    movi a2, 0                    // autoload = 0 (disabled)
    movi a0, Cache_Enable_ICache
    callx0 a0
    
    // 5. Enable DCache with autoload=0
    movi a2, 0                    // autoload = 0 (disabled)
    movi a0, Cache_Enable_DCache
    callx0 a0
    */
    
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
