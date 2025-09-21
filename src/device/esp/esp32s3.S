
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

    // Jump to the runtime start function written in Go.
    call4 main

.section .text.tinygo_scanCurrentStack
.global tinygo_scanCurrentStack
tinygo_scanCurrentStack:
    // TODO: save callee saved registers on the stack
    j tinygo_scanstack

// ============================================================================
// GPIO INTERRUPT HANDLER - ROM HOOK APPROACH
// ============================================================================

.section .text.gpio_interrupt_handler
.global gpio_interrupt_handler
.align 4
gpio_interrupt_handler:
    // Простой обработчик GPIO прерывания
    // Включает GPIO4 как индикатор срабатывания
    
    entry   a1, 32                  // Стандартный entry для Xtensa
    
    // Зажечь GPIO4 как индикатор
    // GPIO_OUT_W1TS_REG = 0x60004008
    movi    a2, 0x60004008          // GPIO_OUT_W1TS_REG адрес
    movi    a3, (1 << 4)            // GPIO4 bit
    s32i    a3, a2, 0               // Включить GPIO4
    
    // Очистить GPIO interrupt status
    // GPIO_STATUS_W1TC_REG = 0x60004014  
    movi    a2, 0x60004014          // GPIO_STATUS_W1TC_REG адрес
    movi    a3, (1 << 0)            // GPIO0 bit (предполагаем GPIO0 источник)
    s32i    a3, a2, 0               // Очистить статус
    
    retw                            // Возврат из прерывания

// ============================================================================
// ROM FUNCTION CALL WRAPPER - ПРАВИЛЬНЫЙ XTENSA CALLING CONVENTION
// ============================================================================

.section .text.call_rom_function
.global call_rom_function
.align 4
call_rom_function:
    // Вызов ROM функции с правильным Xtensa ABI
    // Входные параметры: a2=addr, a3=arg1, a4=arg2, a5=arg3, a6=arg4
    // Результат возвращается в a2
    
    entry   a1, 32                  // Стандартный entry
    
    // Сохраняем адрес функции
    mov     a8, a2                  // a8 = ROM function address
    
    // Переставляем аргументы в правильные регистры для ROM ABI
    mov     a2, a3                  // a2 = arg1 (первый аргумент ROM функции)
    mov     a3, a4                  // a3 = arg2
    mov     a4, a5                  // a4 = arg3  
    mov     a5, a6                  // a5 = arg4
    
    // Вызываем ROM функцию
    callx0  a8                      // Непрямой вызов через регистр
    
    // Результат уже в a2 - просто возвращаем
    retw
