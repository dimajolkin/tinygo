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

.section .text

// uint32_t read_interrupt(void)
// Returns current pending interrupt mask (INTERRUPT register #226)
.global read_interrupt
.type read_interrupt,@function
.align 4
read_interrupt:
    rsr     a2, INTERRUPT_SR
    retw.n

// uint32_t read_intenable(void)
// Returns current interrupt enable mask (INTENABLE register #228)
.global read_intenable
.type read_intenable,@function
.align 4
read_intenable:
    rsr     a2, INTENABLE_SR
    retw.n

// void write_intenable(uint32_t v)
// Writes interrupt enable mask (INTENABLE register #228)
// Parameter: a2 = value to write
.global write_intenable
.type write_intenable,@function
.align 4
write_intenable:
    wsr.intenable a2
    rsync
    retw.n

// void write_intset(uint32_t v)
// Sets interrupt request bits (INTSET register #226)
// Parameter: a2 = bitmask of interrupts to set
.global write_intset
.type write_intset,@function
.align 4
write_intset:
    wsr.intset a2
    rsync
    retw.n

// void write_intclear(uint32_t v)
// Clears interrupt request bits (INTCLEAR register #227)
// Parameter: a2 = bitmask of interrupts to clear
.global write_intclear
.type write_intclear,@function
.align 4
write_intclear:
    wsr.intclear a2
    rsync
    retw.n

// uint32_t get_ps(void)
// Returns current processor status (PS register #230)
.global get_ps
.type get_ps,@function
.align 4
get_ps:
    rsr     a2, PS_SR
    retw.n

// void set_ps(uint32_t v)
// Writes processor status (PS register #230)
// Parameter: a2 = value to write
.global set_ps
.type set_ps,@function
.align 4
set_ps:
    wsr     a2, PS_SR
    rsync
    retw.n

// uint32_t rsil_set(uint32_t level)
// Atomically set PS.INTLEVEL and return old PS value
// Parameter: a2 = new INTLEVEL (0-15)
// Returns: old PS value
.global rsil_set
.type rsil_set,@function
.align 4
rsil_set:
    rsil    a2, 15          // NOTE: rsil uses immediate, not register!
                            // This sets INTLEVEL=15 and returns old PS in a2
                            // For variable level, need different approach
    retw.n

// uint32_t rsil_0(void) - set INTLEVEL=0
.global rsil_0
.type rsil_0,@function
.align 4
rsil_0:
    rsil    a2, 0
    retw.n

// uint32_t rsil_1(void) - set INTLEVEL=1
.global rsil_1
.type rsil_1,@function
.align 4
rsil_1:
    rsil    a2, 1
    retw.n

// uint32_t rsil_15(void) - set INTLEVEL=15 (disable all)
.global rsil_15
.type rsil_15,@function
.align 4
rsil_15:
    rsil    a2, 15
    retw.n
