//go:build esp32s3

// Interrupt helper functions for ESP32-S3 (Xtensa LX7)
//
// CRITICAL: These functions use Call0 ABI (ret.n), NOT Windowed ABI (retw.n)!
// TinyGo always uses Call0 ABI for Xtensa targets.
//
// Call0 ABI conventions:
//   - Parameters: a2-a7 (a2=first param, a3=second, etc)
//   - Return value: a2
//   - Preserved: a12-a15 (callee-saved)
//   - Scratch: a2-a11 (caller-saved)
//   - Return: ret.n (NOT retw.n!)
//
// These functions provide reliable access to special registers using explicit
// SR numbers instead of dot notation (wsr.intset), which may not work correctly
// with LLVM inline assembly.
//
// Special Register Numbers (from Xtensa ISA):
//   INTSET    = 226 (write-only) - set interrupt request bit
//   INTCLEAR  = 227 (write-only) - clear interrupt request bit
//   INTENABLE = 228 (read/write) - interrupt enable mask
//   INTERRUPT = 226 (read-only)  - interrupt status (same address as INTSET for read)
//   PS        = 230 (read/write) - processor status
//
// Reference: .cursor/llvm_xtensa_vectors_issue.md lines 149-152

// Define SR numbers using #define (like in xtensa_vectors_esp32s3.S)
#define INTERRUPT_SR   226
#define INTSET_SR      226
#define INTCLEAR_SR    227
#define INTENABLE_SR   228
#define PS_SR          230


.section .iram1.text, "ax"

// uint32_t __tg_read_interrupt(void)
// Returns current pending interrupt mask (INTERRUPT register)
// Call0 ABI: result in a2
.global __tg_read_interrupt
.type __tg_read_interrupt,@function
.align 4
__tg_read_interrupt:
    rsr.interrupt a2
    ret.n
.size __tg_read_interrupt, . - __tg_read_interrupt

// uint32_t __tg_read_intenable(void)
// Call0 ABI: result in a2
.global __tg_read_intenable
.type __tg_read_intenable,@function
.align 4
__tg_read_intenable:
    rsr.intenable a2
    ret.n
.size __tg_read_intenable, . - __tg_read_intenable

// void __tg_write_intenable(uint32_t v)
// Call0 ABI: a2 = desired INTENABLE value
.global __tg_write_intenable
.type __tg_write_intenable,@function
.align 4
__tg_write_intenable:
    // a2: desired INTENABLE value
    wsr.intenable a2
    rsync
    // verify
    rsr.intenable a3
    xor a3, a3, a2
    beqz a3, 1f
    // retry up to 8 times
    movi a4, 8
0:
    wsr.intenable a2
    rsync
    rsr.intenable a3
    xor a3, a3, a2
    beqz a3, 1f
    addi a4, a4, -1
    bnez a4, 0b
1:
    ret.n
.size __tg_write_intenable, . - __tg_write_intenable

// void __tg_write_intset(uint32_t v)
// Call0 ABI: a2 = bitmask
.global __tg_write_intset
.type __tg_write_intset,@function
.align 4
__tg_write_intset:
    // a2: bitmask
    wsr.intset a2
    rsync
    ret.n
.size __tg_write_intset, . - __tg_write_intset

// void __tg_write_intclear(uint32_t v)
// Call0 ABI: a2 = bitmask
.global __tg_write_intclear
.type __tg_write_intclear,@function
.align 4
__tg_write_intclear:
    // a2: bitmask
    wsr.intclear a2
    rsync
    ret.n
.size __tg_write_intclear, . - __tg_write_intclear

// uint32_t __tg_get_ps(void)
// Call0 ABI: result in a2
.global __tg_get_ps
.type __tg_get_ps,@function
.align 4
__tg_get_ps:
    rsr.ps a2
    ret.n
.size __tg_get_ps, . - __tg_get_ps

// void __tg_set_ps(uint32_t v)
// Call0 ABI: a2 = new PS value
.global __tg_set_ps
.type __tg_set_ps,@function
.align 4
__tg_set_ps:
    // a2: new PS value
    wsr.ps a2
    rsync
    ret.n
.size __tg_set_ps, . - __tg_set_ps

// uint32_t __tg_rsil_set(uint32_t level)
// Call0 ABI: a2 = level (0-15), returns old PS in a2
.global __tg_rsil_set
.type __tg_rsil_set,@function
.align 4
__tg_rsil_set:
    rsr.ps  a3            // a3 = old PS
    movi    a4, ~0xF
    and     a5, a3, a4    // clear INTLEVEL bits
    movi    a4, 0xF       // mask for INTLEVEL bits
    and     a2, a2, a4    // clamp level [0..15]
    or      a5, a5, a2    // new PS with level
    wsr.ps  a5
    rsync
    mov     a2, a3        // return old PS in a2
    ret.n
.size __tg_rsil_set, . - __tg_rsil_set

// uint32_t __tg_rsil_0(void)
// Call0 ABI: returns old PS in a2
// RSIL is a real Xtensa ISA instruction (atomic), works in both Call0 and Windowed ABI
.global __tg_rsil_0
.type __tg_rsil_0,@function
.align 4
__tg_rsil_0:
    rsil a2, 0            // atomically set INTLEVEL=0, return old PS in a2
    ret.n                 // Call0 return
.size __tg_rsil_0, . - __tg_rsil_0

// uint32_t __tg_rsil_1(void)
// Call0 ABI: returns old PS in a2
.global __tg_rsil_1
.type __tg_rsil_1,@function
.align 4
__tg_rsil_1:
    rsil a2, 1            // atomically set INTLEVEL=1, return old PS in a2
    ret.n                 // Call0 return
.size __tg_rsil_1, . - __tg_rsil_1

// uint32_t __tg_rsil_15(void)
// Call0 ABI: returns old PS in a2
.global __tg_rsil_15
.type __tg_rsil_15,@function
.align 4
__tg_rsil_15:
    rsil a2, 15           // atomically set INTLEVEL=15, return old PS in a2
    ret.n                 // Call0 return
.size __tg_rsil_15, . - __tg_rsil_15
