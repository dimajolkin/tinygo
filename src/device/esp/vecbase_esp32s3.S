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
runtime.setVecbaseToVectorBase:
    .align 4
    
    // Save current PS register
    rsr     a3, PS
    
    // Disable all interrupts (set INTLEVEL=15, keep other bits)
    movi    a4, ~0x0F                   // Mask to clear INTLEVEL bits
    and     a4, a3, a4                  // Clear INTLEVEL
    movi    a5, 15                      // INTLEVEL = 15 (mask all)
    or      a4, a4, a5
    wsr     a4, PS
    rsync
    
    // Now it's safe to change VECBASE
    l32r    a2, .Lvecbase_addr          // Load _vector_base address from literal pool
    wsr     a2, VECBASE                 // VECBASE = a2
    rsync                               // synchronize
    
    // Restore original PS (re-enable interrupts to previous level)
    wsr     a3, PS
    rsync
    
    ret                                 // return (Call0 ABI)

.align 4
.Lvecbase_addr:
    .word   _vector_base                // Literal: address of _vector_base

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

