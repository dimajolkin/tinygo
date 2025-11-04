//go:build tinygo && esp32s3

/*
 * VECBASE manipulation functions for ESP32-S3
 * These MUST be in pure assembly to avoid TinyGo inline ASM issues
 * IMPORTANT: Use Call0 ABI (no entry/retw, just call0/ret)
 */

.extern _vector_base

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

    // Load _vector_base address (hardcoded since TinyGo linker cannot resolve symbols in literal pools)
    // MUST match linker script: _vector_base = 0x40374000
    movi    a4, 0x4037                  // Upper 16 bits
    slli    a4, a4, 16                  // Shift left 16 bits
    movi    a5, 0x4000                  // Lower 16 bits
    or      a4, a4, a5                  // Combine: a4 = 0x40374000

    // Save current PS register
    rsr.ps  a3

    // Disable all interrupts: set INTLEVEL=15, preserve other PS bits
    movi    a2, ~0x0F                   // mask to clear INTLEVEL[3:0]
    and     a2, a3, a2
    movi    a5, 15
    or      a2, a2, a5
    wsr.ps  a2
    rsync

    // If VECBASE already equals desired value, skip write
    rsr.vecbase a2                      // a2 = current VECBASE
    beq     a2, a4, 1f                  // if equal, skip update

    // Update VECBASE to the new table base
    wsr.vecbase a4
    isync                               // synchronize instruction stream after VECBASE change

1:
    // Restore original PS
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