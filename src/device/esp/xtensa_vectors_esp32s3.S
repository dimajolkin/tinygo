//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Xtensa Vector Table for ESP32-S3
 * Sections are kept by linker at fixed IRAM offsets and VECBASE is set to _vector_base.
 * We route Level-1 user exceptions to low-level interrupt handler which calls Go ISR.
 *
 * ============================================================================
 * ВАЖНЫЕ ЗАМЕЧАНИЯ ПО РАБОТЕ С LLVM/TinyGo:
 * ============================================================================
 *
 * 1. ПРОБЛЕМА: `.begin/.end literal_prefix` + линкер ld.lld
 *    ---------------------------------------------------------
 *    При попытке использовать отдельные секции вида:
 *        .section .UserExceptionVector.literal
 *        .section .UserExceptionVector.text
 *    
 *    Возникала ошибка линкера:
 *        ld.lld: error: relocation R_XTENSA_SLOT0_OP out of range: 287
 *        references section '.UserExceptionVector.literal'
 *        improper alignment: 0x11F is not aligned to 4 bytes
 *    
 *    ПРИЧИНА:
 *    - Literal pool размещался в конце секции .vectors (после всех векторов)
 *    - Инструкция `l32r` в векторе не могла достать literal (288 байт)
 *    - Literal pool не был выровнен правильно
 *
 * 2. РЕШЕНИЕ: Использовать `.begin/.end literal_prefix` (ESP-IDF стиль)
 *    ---------------------------------------------------------
 *    Директивы `.begin literal_prefix` / `.end literal_prefix` заставляют
 *    ассемблер создать literal pool непосредственно ПОСЛЕ кода вектора,
 *    но ДО следующего вектора (в пределах 32 байт на вектор).
 *    
 *    Пример (работает):
 *        .begin literal_prefix .UserExceptionVector
 *        .section .UserExceptionVector.text, "ax"
 *        _UserExceptionVector:
 *            wsr a0, EXCSAVE_1
 *            call0 _xt_user_exc    # literal для call0 будет рядом
 *        .end literal_prefix
 *    
 *    Линкер-скрипт (esp32s3.ld) собирает literal pools через:
 *        . = ALIGN(4);
 *        *(.*Vector.literal)
 *
 * 3. СИНТАКСИС ИНСТРУКЦИЙ: wsr/rsr БЕЗ точки
 *    ---------------------------------------------------------
 *    TinyGo/LLVM требует:
 *        ✓ ПРАВИЛЬНО: wsr a0, EXCSAVE_1  (номер через #define)
 *        ✗ НЕВЕРНО:   wsr.excsave1 a0    (dot notation не работает корректно)
 *    
 *    Все специальные регистры определены через #define:
 *        #define EXCSAVE_1  209
 *        #define PS         230
 *        #define EPC_1      177
 *        и т.д.
 *
 * 4. ВЫРАВНИВАНИЕ В ЛИНКЕР-СКРИПТЕ
 *    ---------------------------------------------------------
 *    КРИТИЧНО: перед *(.*Vector.literal) добавить:
 *        . = ALIGN(4);
 *    
 *    Это гарантирует 4-byte выравнивание literal pool'а для l32r.
 *
 * 5. ТЕСТИРОВАНИЕ
 *    ---------------------------------------------------------
 *    Для проверки работы векторной таблицы:
 *    - Переменная VectorEntryCount инкрементируется при входе в _xt_user_exc
 *    - Можно вызвать программное прерывание: wsr.intset (1 << 23)
 *    - Если счётчик растёт - векторы работают корректно
 *
 * ЗАКЛЮЧЕНИЕ:
 * -----------
 * Данная реализация полностью повторяет ESP-IDF паттерн для Xtensa векторов.
 * Использование `.begin/.end literal_prefix` + правильное выравнивание в линкере
 * решает проблему с размещением literal pool'ов.
 *
 * Дата: 2024-10-27
 * ============================================================================
 */

/* Special Register definitions (from ESP-IDF xtensa/specreg.h) */
#define EPC_1           177
#define EXCSAVE_1       209
#define PS              230
#define EXCCAUSE        232
#define SAR             3
#define LBEG            0
#define LEND            1
#define LCOUNT          2

/* EXCCAUSE value for Level-1 interrupt on Xtensa LX7 */
#define EXCCAUSE_LEVEL1  4

.extern _xt_level1_int_handler
.extern _xt_unhandled_exception
.extern _frxt_int_depth
.extern _xt_user_exc
.extern handleInterrupt

/* Provide storage for interrupt counters */
.section .bss._frxt_int_depth, "aw", @nobits
.align 4
.global _frxt_int_depth
_frxt_int_depth:
    .space 4

/* Provide a simple ISR call counter storage (optional) */
.section .bss._isr_call_count, "aw", @nobits
.align 4
.global _isr_call_count
_isr_call_count:
    .space 4

/* Debug counters for exception path tracking */
.section .bss._user_exc_called, "aw", @nobits
.align 4
.global _user_exc_called
_user_exc_called:
    .space 4

.section .bss._to_unhandled_exc_called, "aw", @nobits
.align 4
.global _to_unhandled_exc_called
_to_unhandled_exc_called:
    .space 4


/*
 * User Exception Vector (Level-1) - handles all Level-1 interrupts and exceptions
 * Based on ESP-IDF: esp-idf/components/xtensa/xtensa_vectors.S
 */

.begin      literal_prefix .UserExceptionVector
.section    .UserExceptionVector.text, "ax"
.global     _UserExceptionVector
.type       _UserExceptionVector,@function
.align      4
_UserExceptionVector:
    wsr     a0, EXCSAVE_1       /* preserve a0 in EXCSAVE1 */
    call0   _xt_user_exc        /* jump to user exception handler */
.size       _UserExceptionVector, . - _UserExceptionVector
.end        literal_prefix

/* Place exception handlers in .iram1.text for proximity to vectors */
.section .iram1.text, "ax"

/* User exception dispatcher: check if Level-1 interrupt or exception */
.align 4
.type _xt_user_exc,@function
_xt_user_exc:
    /* DEBUG: Toggle GPIO6 to signal we entered _xt_user_exc */
    .literal_position
    l32r    a2, .Lgpio_enable_reg
    l32i    a3, a2, 0
    movi    a4, (1 << 6)
    or      a3, a3, a4
    s32i    a3, a2, 0                       /* Enable GPIO6 output */
    
    l32r    a2, .Lgpio_set_reg
    s32i    a4, a2, 0                       /* Set GPIO6 high */
    
    /* DEBUG: Mark that we got to _xt_user_exc (use counter) */
    l32r    a2, .Luser_exc_addr
    l32i    a3, a2, 0
    addi    a3, a3, 1
    s32i    a3, a2, 0
    
    /* Check if this is a Level-1 interrupt (EXCCAUSE == 4) */
    rsr     a0, EXCSAVE_1                   /* restore a0 saved by vector entry */
    rsr     a3, EXCCAUSE                    /* read EXCCAUSE into a3 */
    bnei    a3, EXCCAUSE_LEVEL1, _xt_to_unhandled_exc     /* not Level-1 interrupt? go to exception handler */
    j       _xt_lowint1                     /* EXPLICIT jump to _xt_lowint1! */

/* Trampoline for unhandled exception (must be close for bnei) */
.align 4
_xt_to_unhandled_exc:
    /* DEBUG: Toggle GPIO4 to signal we entered _xt_to_unhandled_exc */
    .literal_position
    l32r    a2, .Lgpio_enable_reg
    l32i    a3, a2, 0
    movi    a4, (1 << 4)
    or      a3, a3, a4
    s32i    a3, a2, 0                       /* Enable GPIO4 output */
    
    l32r    a2, .Lgpio_set_reg
    s32i    a4, a2, 0                       /* Set GPIO4 high */
    
    /* DEBUG: Mark that we got to _xt_to_unhandled_exc (use counter) */
    l32r    a2, .Lto_unhandled_addr
    l32i    a3, a2, 0
    addi    a3, a3, 1
    s32i    a3, a2, 0
    
    /* CRITICAL: Use 'j' (jump) NOT 'call0' to avoid overwriting a0!
     * a0 contains the original register value that we need to preserve.
     * call0 would overwrite a0 with return address, losing the original value.
     */
    j       _xt_unhandled_exception         /* jump to real handler (preserves a0) */

.size       _xt_user_exc, . - _xt_user_exc

/* Literal pool for _xt_user_exc debug counters and GPIO */
.align 4
.Lgpio_enable_reg:
    .word 0x60004024                    /* GPIO_ENABLE_REG */
.Lgpio_set_reg:
    .word 0x60004008                    /* GPIO_OUT_W1TS_REG */
.Luser_exc_addr:
    .word _user_exc_called
.Lto_unhandled_addr:
    .word _to_unhandled_exc_called

/* Low-level level-1 interrupt entry: ESP-IDF compatible version */
.align 4
.global _xt_lowint1
_xt_lowint1:
    /* At entry: a0 saved in EXCSAVE_1 by _UserExceptionVector */

    /* Allocate full interrupt stack frame (XT_STK_FRMSZ = 144 bytes) */
    mov     a0, sp                          /* save original SP */
    addi    sp, sp, -XT_STK_FRMSZ           /* allocate interrupt stack frame */
    s32i    a0, sp, XT_STK_A1               /* save pre-interrupt SP */

    /* Save minimal context (PS, PC, A0) */
    rsr     a0, PS                          /* save interruptee's PS */
    s32i    a0, sp, XT_STK_PS
    rsr     a0, EPC_1                       /* save interruptee's PC */
    s32i    a0, sp, XT_STK_PC
    rsr     a0, EXCSAVE_1                   /* save interruptee's a0 */
    s32i    a0, sp, XT_STK_A0

    /* Save exit point for dispatch (CRITICAL: required for proper return!) */
    .literal_position
    l32r    a0, .Lxt_user_exit_addr         /* load address of _xt_user_exit */
    s32i    a0, sp, XT_STK_EXIT

    /* EXCSAVE_1 is now free to use for backtrace */
    #ifdef XT_DEBUG_BACKTRACE
    #ifndef __XTENSA_CALL0_ABI__
    mov     a0, sp
    wsr     a0, EXCSAVE_1                   /* save current SP for backtrace */
    #endif
    #endif

    /* Save rest of interrupt context and enter RTOS */
    call0   _frxt_int_enter                 /* common RTOS interrupt entry */

    /* !! We are now on the interrupt context with full frame saved !! */

    /* Set up PS for C, enable interrupts above this level and clear EXCM */
    /* This allows nested interrupts of higher priority */
    /* PS = INTLEVEL(1) | PS_UM for Call0 ABI */
    /* INTLEVEL(1) = 0x00000001, PS_UM = 0x00000020 */
    #ifdef __XTENSA_CALL0_ABI__
    movi    a0, 0x00000021                  /* PS_INTLEVEL(1) | PS_UM */
    #else
    movi    a0, 0x00040021                  /* PS_INTLEVEL(1) | PS_UM | PS_WOE */
    #endif
    wsr     a0, PS
    rsync

    /* OK to call C code at this point, dispatch to Go ISR */
    /* Increment debug counter before calling Go */
    .literal_position
    l32r    a2, .Lisr_count_addr
    l32i    a3, a2, 0
    addi    a3, a3, 1
    s32i    a3, a2, 0

    /* Call Go interrupt dispatcher */
    .literal_position
    l32r    a0, .LhandleInterrupt_addr
    callx0  a0                              /* call handleInterrupt() */

    /* Done handling interrupts, transfer control to RTOS exit */
    call0   _frxt_int_exit                  /* does not return directly here */

    /* Exit through saved XT_STK_EXIT (points to _xt_user_exit) */
    /* Note: _frxt_int_exit may switch tasks, so we return via saved exit point */
    l32i    a0, sp, XT_STK_EXIT
    jx      a0                              /* jump to _xt_user_exit */

.size       _xt_lowint1, . - _xt_lowint1

/* Literal pool for _xt_lowint1 */
.align 4
.Lisr_count_addr:
    .word _isr_call_count
.LhandleInterrupt_addr:
    .word handleInterrupt
.Lxt_user_exit_addr:
    .word _xt_user_exit

.global _xt_user_exit
_xt_user_exit:
    /* This is the return point after interrupt handling */
    l32i    a0, sp, XT_STK_PS               /* restore PS */
    wsr     a0, PS                          /* restore PS */
    l32i    a0, sp, XT_STK_PC               /* restore PC */
    wsr     a0, EPC_1                       /* restore to EPC_1 */
    l32i    a0, sp, XT_STK_A0               /* restore a0 */
    l32i    sp, sp, XT_STK_A1               /* restore SP */
    rsync
    rfe                                      /* return from exception */

.size       _xt_user_exit, . - _xt_user_exit

/*
 * Exception and Interrupt Vectors (Level 2-7, Double, Kernel, NMI)
 * Based on ESP-IDF esp-idf/components/xtensa/xtensa_vectors.S
 */

/* DoubleException - nested exception (VECBASE+0x020) */
    .begin      literal_prefix .DoubleExceptionVector
    .section    .DoubleExceptionVector.text, "ax"
    .align      4
.global     _DoubleExceptionVector
_DoubleExceptionVector:
    call0   _xt_unhandled_exception
    .size       _DoubleExceptionVector, . - _DoubleExceptionVector
    .end        literal_prefix

/* KernelException - privileged instruction in user mode (VECBASE+0x040) */
    .begin      literal_prefix .KernelExceptionVector
    .section    .KernelExceptionVector.text, "ax"
    .align      4
.global     _KernelExceptionVector
_KernelExceptionVector:
    call0   _xt_unhandled_exception
    .size       _KernelExceptionVector, . - _KernelExceptionVector
    .end        literal_prefix

/* NMI - Non-Maskable Interrupt (VECBASE+0x060) */
    .begin      literal_prefix .NMIExceptionVector
    .section    .NMIExceptionVector.text, "ax"
    .align      4
.global     _NMIExceptionVector
_NMIExceptionVector:
    call0   _xt_unhandled_exception
    .size       _NMIExceptionVector, . - _NMIExceptionVector
    .end        literal_prefix

/* Level 2 Interrupt - medium priority (VECBASE+0x080) */
    .begin      literal_prefix .Level2InterruptVector
    .section    .Level2InterruptVector.text, "ax"
    .align      4
.global     _Level2InterruptVector
_Level2InterruptVector:
    call0   _xt_unhandled_exception
    .size       _Level2InterruptVector, . - _Level2InterruptVector
    .end        literal_prefix

/* Level 3 Interrupt - high priority (VECBASE+0x0A0) */
    .begin      literal_prefix .Level3InterruptVector
    .section    .Level3InterruptVector.text, "ax"
    .align      4
.global     _Level3InterruptVector
_Level3InterruptVector:
    call0   _xt_unhandled_exception
    .size       _Level3InterruptVector, . - _Level3InterruptVector
    .end        literal_prefix

/* Level 4 Interrupt (VECBASE+0x0C0) */
    .begin      literal_prefix .Level4InterruptVector
    .section    .Level4InterruptVector.text, "ax"
    .align      4
.global     _Level4InterruptVector
_Level4InterruptVector:
    call0   _xt_unhandled_exception
    .size       _Level4InterruptVector, . - _Level4InterruptVector
    .end        literal_prefix

/* Level 5 Interrupt (VECBASE+0x0E0) */
    .begin      literal_prefix .Level5InterruptVector
    .section    .Level5InterruptVector.text, "ax"
    .align      4
.global     _Level5InterruptVector
_Level5InterruptVector:
    call0   _xt_unhandled_exception
    .size       _Level5InterruptVector, . - _Level5InterruptVector
    .end        literal_prefix

/* Level 6 Interrupt (VECBASE+0x100) */
    .begin      literal_prefix .Level6InterruptVector
    .section    .Level6InterruptVector.text, "ax"
    .align      4
.global     _Level6InterruptVector
_Level6InterruptVector:
    call0   _xt_unhandled_exception
    .size       _Level6InterruptVector, . - _Level6InterruptVector
    .end        literal_prefix

/* Level 7 Interrupt - highest priority, NMI-like (VECBASE+0x120) */
    .begin      literal_prefix .Level7InterruptVector
    .section    .Level7InterruptVector.text, "ax"
    .align      4
.global     _Level7InterruptVector
_Level7InterruptVector:
    call0   _xt_unhandled_exception
    .size       _Level7InterruptVector, . - _Level7InterruptVector
    .end        literal_prefix


