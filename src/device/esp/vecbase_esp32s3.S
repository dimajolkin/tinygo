//go:build tinygo && esp32s3

/*
 * VECBASE manipulation functions for ESP32-S3
 * These MUST be in pure assembly to avoid TinyGo inline ASM issues
 * IMPORTANT: Use Call0 ABI (no entry/retw, just call0/ret)
 */

.section .text.runtime.setVecbaseToVectorBase
.global runtime.setVecbaseToVectorBase
.type runtime.setVecbaseToVectorBase,@function
.align 4
.literal_position                       // Place literals here

// void runtime.setVecbaseToVectorBase()
// Sets VECBASE to _vector_base address from linker script
// IMPORTANT: Disables interrupts during VECBASE change to avoid race conditions
// 
// Uses hardcoded address 0x40374000 since TinyGo linker cannot properly
// resolve //go:extern symbols in assembly literal pools.
// This address MUST match the linker script: .vectors 0x40374000
runtime.setVecbaseToVectorBase:
    .align 4
    
    // Load _vector_base address (0x40374000 from linker script)
    movi    a4, 0x4037                  // Upper 16 bits
    slli    a4, a4, 16                  // Shift left 16 bits
    movi    a5, 0x4000                  // Lower 16 bits
    or      a4, a4, a5                  // Combine: a4 = 0x40374000
    
    // Save current PS register
    rsr.ps  a3

    // Disable all interrupts (set INTLEVEL=15, keep other bits)
    movi    a2, ~0x0F                   // Mask to clear INTLEVEL bits
    and     a2, a3, a2                  // Clear INTLEVEL
    movi    a5, 15                      // INTLEVEL = 15 (mask all)
    or      a2, a2, a5
    wsr.ps  a2
    rsync

    // Now it's safe to change VECBASE
    mov     a2, a4                      // a2 = 0x40374000
    wsr.vecbase a2                      // VECBASE = a2
    isync                               // synchronize instruction stream after VECBASE change

    // Restore original PS (re-enable interrupts to previous level)
    wsr.ps  a3
    rsync
    
    ret                                 // return (Call0 ABI)

.size runtime.setVecbaseToVectorBase, .-runtime.setVecbaseToVectorBase


.section .text.runtime.getVecbase
.global runtime.getVecbase
.type runtime.getVecbase,@function
.align 4

// uint32_t runtime.getVecbase()
// Returns current VECBASE value (in a2 for Call0 ABI)
runtime.getVecbase:
    rsr.vecbase a2                      // a2 = VECBASE (return value), SR 97
    ret                                 // return (Call0 ABI)

.size runtime.getVecbase, .-runtime.getVecbase


// ============================================================================
// void copyVectorsToIRAM()
// Copies interrupt vectors from Flash (LMA) to IRAM (VMA) 
// MUST be called BEFORE setting VECBASE!
//
// Linker provides:
//   _vector_base        = 0x40374000 (VMA in IRAM)
//   _vectors_flash_start = LOADADDR(.vectors) (LMA in Flash)
//   _vectors_end         = end of vectors in IRAM
// ============================================================================
.section .text.copyVectorsToIRAM
.global copyVectorsToIRAM
.type copyVectorsToIRAM,@function
.align 4

.extern _vector_base
.extern _vectors_end
.extern _vectors_flash_start

copyVectorsToIRAM:
    // Load addresses from linker symbols
    movi    a2, 0x40374000              // a2 = _vector_base (IRAM destination)
    
    // Load _vectors_flash_start (Flash source)
    .literal_position
    l32r    a3, .Lvectors_flash_addr
    l32i    a3, a3, 0                   // a3 = _vectors_flash_start (Flash source)
    
    // Load _vectors_end (end of IRAM region)
    l32r    a4, .Lvectors_end_addr
    l32i    a4, a4, 0                   // a4 = _vectors_end
    
    // Calculate size in bytes: size = _vectors_end - _vector_base
    sub     a5, a4, a2                  // a5 = size in bytes
    
    // Convert to word count (divide by 4)
    srli    a5, a5, 2                   // a5 = size / 4 (word count)
    
    // Copy loop: copy word by word from Flash to IRAM
    beqz    a5, .Lcopy_done             // if size == 0, skip
.Lcopy_loop:
    l32i    a6, a3, 0                   // Load word from Flash
    s32i    a6, a2, 0                   // Store word to IRAM
    addi    a3, a3, 4                   // Advance Flash pointer
    addi    a2, a2, 4                   // Advance IRAM pointer
    addi    a5, a5, -1                  // Decrement counter
    bnez    a5, .Lcopy_loop             // Loop if counter != 0
    
.Lcopy_done:
    // Memory barrier to ensure all writes complete
    memw
    
    ret                                 // return (Call0 ABI)

// Literal pool
.align 4
.Lvectors_flash_addr:
    .word _vectors_flash_start
.Lvectors_end_addr:
    .word _vectors_end

.size copyVectorsToIRAM, .-copyVectorsToIRAM

