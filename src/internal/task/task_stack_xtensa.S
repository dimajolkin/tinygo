//go:build tinygo && esp32s3

// Объявляем внешние функции
.extern debugMark
.extern debugStoreValues

.section .text.tinygo_startTask,"ax",@progbits
.global  tinygo_startTask
.type    tinygo_startTask,@function

// tinygo_startTask is the entry point for new goroutines.
// a2 contains the argument pointer, a3 the function pointer.
tinygo_startTask:
    mov.n  a6, a2          // move argument into position
    movi    a2, 0xDEADBEEF
    call0   debugMark
    callx4 a3              // call goroutine function
    call4  tinygo_task_exit // should not return


.section .text.tinygo_swapTask,"ax",@progbits
.global  tinygo_swapTask
.type    tinygo_swapTask,@function

// tinygo_swapTask(oldStack uintptr, newStack *uintptr)
// According to task_stack.go: func swapTask(oldStack uintptr, newStack *uintptr)
// a2 = oldStack (stack to switch to)
// a3 = newStack (pointer where to save current stack)
tinygo_swapTask:
    // Add some nops to test
    nop
    nop
    nop
    // Just return without doing anything - test basic function call
    ret
    
    // For now, just do a simple stub - save current SP and return
    l32i    a3, sp, 4       // restore oldStack ptr
    s32i    sp, a3, 0       // save current SP to *oldStack
    
    // Debug: mark completion
    movi    a2, 0x5678      // Completion marker
    call0   debugMark
    
    // Restore and return
    l32i    a0, sp, 12      // restore return address
    addi    sp, sp, 16      // deallocate stack
    ret                     // return
    
    // Now try entry
    entry   sp, 16
    
    // Save parameters after entry
    mov     a6, a2          // save newStack (context to switch to)
    mov     a7, a3          // save oldStack pointer (where to store current context)
    
    // Call context functions with correct parameters
    mov     a2, a7          // a2 = oldStack pointer for context_save
    call0   tinygo_context_save
    mov     a2, a6          // a2 = newStack for context_restore
    call0   tinygo_context_restore
    
    retw.n                  // should not be reached



