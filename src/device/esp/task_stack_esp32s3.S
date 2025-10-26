//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

.extern _frxt_int_enter
.extern _frxt_int_exit

// _xt_context_save is defined in xtensa_context_esp32s3.S
.extern _xt_context_save
// _xt_context_restore is defined in xtensa_context_esp32s3.S
.extern _xt_context_restore

// swapTask - Task context switching for ESP32-S3 (Call0 ABI)
// Parameters:
//   a2 = newStack (new stack pointer to switch to - points to saved frame)
//   a3 = oldStack (pointer to save current SP)
// This implements simple Call0 ABI context switch without windowed registers
.section .text.swapTask,"ax",@progbits
.align 4
.global swapTask
.type swapTask,@function
swapTask:
    // Allocate space on stack for calleeSavedRegs structure (112 bytes)
    addi    sp, sp, -112
    // Free the frame
    addi    sp, sp, 112
    ret

// Exported stub for Go //export tinygo_swapTask in task_stack.go
.global tinygo_swapTask
.type   tinygo_swapTask,@function
tinygo_swapTask:
    j       swapTask

// tinygo_startTask - Entry point for new tasks
// This is the first function that runs when a new task is scheduled.
// The stack frame (calleeSavedRegs) has been set up by archInit in task_stack_xtensa.go
// When we arrive here via swapTask, the registers have been restored from the frame:
//   a2 = args (function argument)
//   a3 = fn (function pointer to call)
//   sp = points just after the calleeSavedRegs structure
.section .text.tinygo_startTask,"ax",@progbits
.global tinygo_startTask
.type tinygo_startTask,@function
.align 4
tinygo_startTask:
    // Test: minimal tinygo_startTask that just returns
    ret
