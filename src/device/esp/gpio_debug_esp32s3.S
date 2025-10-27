//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Lightweight GPIO debug helpers for ISR usage (IRAM, no panics/allocs).
 * - _gpio_debug_set6 / _gpio_debug_clear6: specialized for GPIO6
 * - _gpio_debug_set / _gpio_debug_clear: a2 = bit number (0..31)
 *
 * Registers:
 *   GPIO_ENABLE_W1TS_REG @ 0x60004024
 *   GPIO_OUT_W1TS_REG    @ 0x60004008
 *   GPIO_OUT_W1TC_REG    @ 0x6000400C
 */

.section .text._gpio_debug_set6,"ax",@progbits
.align 4
.global _gpio_debug_set6
.type _gpio_debug_set6,@function
_gpio_debug_set6:
    // enable output bit
    movi    a3, 0x60004024
    movi    a4, (1 << 6)
    s32i    a4, a3, 0
    // set output bit
    movi    a3, 0x60004008
    s32i    a4, a3, 0
    ret

.section .text._gpio_debug_clear6,"ax",@progbits
.align 4
.global _gpio_debug_clear6
.type _gpio_debug_clear6,@function
_gpio_debug_clear6:
    // clear output bit
    movi    a3, 0x6000400C
    movi    a4, (1 << 6)
    s32i    a4, a3, 0
    ret

.section .text._gpio_debug_set,"ax",@progbits
.align 4
.global _gpio_debug_set
.type _gpio_debug_set,@function
// a2 = bit number (0..31)
_gpio_debug_set:
    movi    a4, 1
    sll     a4, a2             // a4 = 1 << bit
    // enable output
    movi    a3, 0x60004024
    s32i    a4, a3, 0
    // set output
    movi    a3, 0x60004008
    s32i    a4, a3, 0
    ret

.section .text._gpio_debug_clear,"ax",@progbits
.align 4
.global _gpio_debug_clear
.type _gpio_debug_clear,@function
// a2 = bit number (0..31)
_gpio_debug_clear:
    movi    a4, 1
    sll     a4, a2             // a4 = 1 << bit
    // clear output
    movi    a3, 0x6000400C
    s32i    a4, a3, 0
    ret


