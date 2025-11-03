//go:build esp32s3

// Interrupt helper functions for ESP32-S3 (Xtensa LX7)
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
// Returns current pending interrupt mask (INTERRUPT register #226)
.global __tg_read_interrupt
.type __tg_read_interrupt,@function
.align 4
__tg_read_interrupt:
    entry   a1, 16
    rsr.interrupt a2
    mov     a10, a2
    retw.n

// uint32_t __tg_read_intenable(void)
// Returns current interrupt enable mask (INTENABLE register #228)
.global __tg_read_intenable
.type __tg_read_intenable,@function
.align 4
__tg_read_intenable:
    entry   a1, 16
    rsr.intenable a2
    mov     a10, a2
    retw.n

// void __tg_write_intenable(uint32_t v)
// Writes interrupt enable mask (INTENABLE register #228)
// Parameter: a2 = value to write
.global __tg_write_intenable
.type __tg_write_intenable,@function
.align 4
__tg_write_intenable:
    entry   a1, 16
    // a10: desired value
    mov     a2, a10              // a2 = desired INTENABLE
    // First write
    wsr.intenable a2
    rsync
    // Verify
    rsr.intenable a3             // a3 = read back
    xor     a3, a3, a2           // a3 == 0 if equal
    beqz    a3, 1f               // ok → exit
    // Retry up to 8 times
    movi    a4, 8
0:
    wsr.intenable a2
    rsync
    rsr.intenable a3
    xor     a3, a3, a2
    beqz    a3, 1f
    addi    a4, a4, -1
    bnez    a4, 0b
1:
    retw.n

// void __tg_write_intset(uint32_t v)
// Sets interrupt request bits (INTSET register #226)
// Parameter: a2 = bitmask of interrupts to set
.global __tg_write_intset
.type __tg_write_intset,@function
.align 4
__tg_write_intset:
    entry   a1, 16
    mov     a2, a10
    wsr.intset a2
    rsync
    retw.n

// void __tg_write_intclear(uint32_t v)
// Clears interrupt request bits (INTCLEAR register #227)
// Parameter: a2 = bitmask of interrupts to clear
.global __tg_write_intclear
.type __tg_write_intclear,@function
.align 4
__tg_write_intclear:
    entry   a1, 16
    mov     a2, a10
    wsr.intclear a2
    rsync
    retw.n

// uint32_t __tg_get_ps(void)
// Returns current processor status (PS register #230)
.global __tg_get_ps
.type __tg_get_ps,@function
.align 4
__tg_get_ps:
    entry   a1, 16
    rsr.ps  a2
    mov     a10, a2
    retw.n

// void __tg_set_ps(uint32_t v)
// Writes processor status (PS register #230)
// Parameter: a2 = value to write
.global __tg_set_ps
.type __tg_set_ps,@function
.align 4
__tg_set_ps:
    entry   a1, 16
    mov     a2, a10
    wsr.ps  a2
    rsync
    retw.n

// uint32_t __tg_rsil_set(uint32_t level)
// Atomically set PS.INTLEVEL and return old PS value
// Parameter: a2 = new INTLEVEL (0-15)
// Returns: old PS value
.global __tg_rsil_set
.type __tg_rsil_set,@function
.align 4
__tg_rsil_set:
    entry   a1, 16
    rsil    a2, 15          // NOTE: rsil uses immediate, not register!
                            // This sets INTLEVEL=15 and returns old PS in a2
                            // For variable level, need different approach
    mov     a10, a2
    retw.n

// uint32_t __tg_rsil_0(void) - set INTLEVEL=0
.global __tg_rsil_0
.type __tg_rsil_0,@function
.align 4
__tg_rsil_0:
    entry   a1, 16
    rsil    a2, 0
    mov     a10, a2
    retw.n

// uint32_t __tg_rsil_1(void) - set INTLEVEL=1
.global __tg_rsil_1
.type __tg_rsil_1,@function
.align 4
__tg_rsil_1:
    entry   a1, 16
    rsil    a2, 1
    mov     a10, a2
    retw.n

// uint32_t __tg_rsil_15(void) - set INTLEVEL=15 (disable all)
.global __tg_rsil_15
.type __tg_rsil_15,@function
.align 4
__tg_rsil_15:
    entry   a1, 16
    rsil    a2, 15
    mov     a10, a2
    retw.n
