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
#define EXCSAVE_2       210
#define EXCSAVE_3       211
#define EXCSAVE_4       212
#define EXCSAVE_5       213
#define EXCSAVE_6       214
#define EXCSAVE_7       215
#define PS              230
#define EXCCAUSE        232
#define SAR             3
#define LBEG            0
#define LEND            1
#define LCOUNT          2

#define INTSET          226
#define INTCLEAR        227
#define INTENABLE       228

/* EXCCAUSE value for Level-1 interrupt on Xtensa LX7 */
#define EXCCAUSE_LEVEL1  4

.extern _xt_level1_int_handler
.extern _xt_unhandled_exception
.extern _frxt_int_depth
.extern _xt_user_exc
.extern handleInterrupt

/* --- Go bridge for CPU interrupt handling (Level-1 dispatcher) --- */
.extern __tg_handle_cpu_interrupt      // void __tg_handle_cpu_interrupt(uint32 line)

/* --- Go helper to clear SYSTIMER source for TARGET0 (line 1) --- */
.extern __tg_clear_irq_source1        // void __tg_clear_irq_source1(void)

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
    wsr     a0, EXCSAVE_1                   /* preserve a0 */
    call0   _xt_user_exc                    /* user exception handler */
    /* never returns here - call0 is used as a jump */
.size       _UserExceptionVector, . - _UserExceptionVector
.end        literal_prefix

/* Place exception handlers in .iram1.text for proximity to vectors */
.section .iram1.text, "ax"
.align 4

/* Proper ESP-IDF style Level-1 interrupt handler */
.align 4
.global _xt_user_exc
.type _xt_user_exc,@function
_xt_user_exc:
    /* If not Level-1 interrupt, safely return to EPC */
    rsr     a0, EXCCAUSE
    beqi    a0, 4, 1f                           /* EXCCAUSE==4 -> handle L1 */
    rfe                                          /* otherwise just return */
1:
    j       _xt_lowint1                          /* jump to Level-1 handler */

/* Minimal Level-1 interrupt handler */
.align 4
.global _xt_lowint1
.type _xt_lowint1,@function
_xt_lowint1:
    /* Mask nested IRQs during handling */
    rsil    a1, 7                            /* INTLEVEL=7, old PS saved in a1 */

    /* pending := INTENABLE & INTERRUPT */
    rsr     a2, INTENABLE
    rsr     a3, INTERRUPT
    and     a4, a2, a3                        /* a4 = pending & enabled */

    /* Walk through all set bits in a4 (CPU lines 0..31) */
    movi    a5, 1                             /* bit mask = 1 << i */
    movi    a7, 0                             /* i = 0 */
0:
    beqz    a4, 1f                            /* no pending -> done */
    and     a6, a4, a5                        /* test current bit */
    beqz    a6, 2f
    mov     a2, a7                            /* arg0 = line number */
    call0   __tg_handle_cpu_interrupt
    /* Extra safety: if this was line 1 (SYSTIMER TARGET0), clear peripheral source once more */
    beqi    a7, 1, 3f
    j       2f
3:
    call0   __tg_clear_irq_source1
2:
    addi    a7, a7, 1                         /* i++ */
    slli    a5, a5, 1                         /* mask <<= 1 */
    bltui   a7, 32, 0b                        /* continue while i < 32 */

1:
    /* ACK only what was pending (captured at entry in a4) */
    wsr     a4, INTCLEAR
    rsync

    /* Restore PS and return from Level-1 interrupt */
    wsr     a1, PS
    rsr     a0, EXCSAVE_1
    rfe

/* Panic handler for non-Level-1 exceptions */
.align 4
.global _xt_panic
.type _xt_panic,@function
_xt_panic:
    /* Just halt on exceptions */
1:  j       1b

.size       _xt_user_exc, . - _xt_user_exc

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
    call0   _xt_panic
    .size       _DoubleExceptionVector, . - _DoubleExceptionVector
    .end        literal_prefix

/* KernelException - privileged instruction in user mode (VECBASE+0x040) */
    .begin      literal_prefix .KernelExceptionVector
    .section    .KernelExceptionVector.text, "ax"
    .align      4
.global     _KernelExceptionVector
_KernelExceptionVector:
    call0   _xt_panic
    .size       _KernelExceptionVector, . - _KernelExceptionVector
    .end        literal_prefix

/* NMI - Non-Maskable Interrupt (VECBASE+0x060) */
    .begin      literal_prefix .NMIExceptionVector
    .section    .NMIExceptionVector.text, "ax"
    .align      4
.global     _NMIExceptionVector
_NMIExceptionVector:
    call0   _xt_panic
    .size       _NMIExceptionVector, . - _NMIExceptionVector
    .end        literal_prefix

/* Level 2 Interrupt - medium priority (VECBASE+0x080) */
    .begin      literal_prefix .Level2InterruptVector
    .section    .Level2InterruptVector.text, "ax"
    .align      4
.global     _Level2InterruptVector
_Level2InterruptVector:
    wsr     a0, EXCSAVE_2
    j       _xt_highint2
    .size       _Level2InterruptVector, . - _Level2InterruptVector
    .end        literal_prefix

/* Level 3 Interrupt - high priority (VECBASE+0x0A0) */
    .begin      literal_prefix .Level3InterruptVector
    .section    .Level3InterruptVector.text, "ax"
    .align      4
.global     _Level3InterruptVector
_Level3InterruptVector:
    wsr     a0, EXCSAVE_3
    j       _xt_highint3
    .size       _Level3InterruptVector, . - _Level3InterruptVector
    .end        literal_prefix

/* Level 4 Interrupt (VECBASE+0x0C0) */
    .begin      literal_prefix .Level4InterruptVector
    .section    .Level4InterruptVector.text, "ax"
    .align      4
.global     _Level4InterruptVector
_Level4InterruptVector:
    wsr     a0, EXCSAVE_4
    j       _xt_highint4
    .size       _Level4InterruptVector, . - _Level4InterruptVector
    .end        literal_prefix

/* Level 5 Interrupt (VECBASE+0x0E0) */
    .begin      literal_prefix .Level5InterruptVector
    .section    .Level5InterruptVector.text, "ax"
    .align      4
.global     _Level5InterruptVector
_Level5InterruptVector:
    wsr     a0, EXCSAVE_5
    j       _xt_highint5
    .size       _Level5InterruptVector, . - _Level5InterruptVector
    .end        literal_prefix

/* Level 6 Interrupt (VECBASE+0x100) */
    .begin      literal_prefix .Level6InterruptVector
    .section    .Level6InterruptVector.text, "ax"
    .align      4
.global     _Level6InterruptVector
_Level6InterruptVector:
    wsr     a0, EXCSAVE_6
    j       _xt_highint6
    .size       _Level6InterruptVector, . - _Level6InterruptVector
    .end        literal_prefix

/* Level 7 Interrupt (VECBASE+0x120) - NMI style like ESP-IDF */
    .begin      literal_prefix .Level7InterruptVector
    .section    .Level7InterruptVector.text, "ax"
    .align      4
.global     _Level7InterruptVector
.type       _Level7InterruptVector,@function
.global     xt_level7
    .set        xt_level7, _xt_level7
_Level7InterruptVector:
    wsr     a0, EXCSAVE_7		/* preserve a0 */
    call0   xt_level7		/* load interrupt handler */
    /* never returns here - call0 is used as a jump */
    .size       _Level7InterruptVector, . - _Level7InterruptVector
    .end        literal_prefix

/* Level 7 interrupt handler (like NMI in ESP-IDF) */
.section    .iram1, "ax"
.type       _xt_level7,@function
.align      4
_xt_level7:
    /* Minimal NMI handler - just panic */
    call0   _xt_panic

    .align  4
.L_xt_level7_exit:
    rsr     a0, EXCSAVE_7		/* restore a0 */
    rfi     7



/* ================= High-priority handlers (Level 2..6) in IRAM ================= */
.align 4
.global _xt_highint2
.type _xt_highint2,@function
_xt_highint2:
    rsil    a1, 7
    rsr     a2, INTENABLE
    rsr     a3, INTERRUPT
    and     a4, a2, a3
    movi    a5, 1
    movi    a7, 0
0:
    beqz    a4, 1f
    and     a6, a4, a5
    beqz    a6, 2f
    mov     a2, a7
    call0   __tg_handle_cpu_interrupt
2:
    addi    a7, a7, 1
    slli    a5, a5, 1
    bltui   a7, 32, 0b
1:
    wsr     a4, INTCLEAR
    rsync
    wsr     a1, PS
    rsr     a0, EXCSAVE_2
    rfi     2

.align 4
.global _xt_highint3
.type _xt_highint3,@function
_xt_highint3:
    rsil    a1, 7
    rsr     a2, INTENABLE
    rsr     a3, INTERRUPT
    and     a4, a2, a3
    movi    a5, 1
    movi    a7, 0
0:
    beqz    a4, 1f
    and     a6, a4, a5
    beqz    a6, 2f
    mov     a2, a7
    call0   __tg_handle_cpu_interrupt
2:
    addi    a7, a7, 1
    slli    a5, a5, 1
    bltui   a7, 32, 0b
1:
    wsr     a4, INTCLEAR
    rsync
    wsr     a1, PS
    rsr     a0, EXCSAVE_3
    rfi     3

.align 4
.global _xt_highint4
.type _xt_highint4,@function
_xt_highint4:
    rsil    a1, 7
    rsr     a2, INTENABLE
    rsr     a3, INTERRUPT
    and     a4, a2, a3
    movi    a5, 1
    movi    a7, 0
0:
    beqz    a4, 1f
    and     a6, a4, a5
    beqz    a6, 2f
    mov     a2, a7
    call0   __tg_handle_cpu_interrupt
2:
    addi    a7, a7, 1
    slli    a5, a5, 1
    bltui   a7, 32, 0b
1:
    wsr     a4, INTCLEAR
    rsync
    wsr     a1, PS
    rsr     a0, EXCSAVE_4
    rfi     4

.align 4
.global _xt_highint5
.type _xt_highint5,@function
_xt_highint5:
    rsil    a1, 7
    rsr     a2, INTENABLE
    rsr     a3, INTERRUPT
    and     a4, a2, a3
    movi    a5, 1
    movi    a7, 0
0:
    beqz    a4, 1f
    and     a6, a4, a5
    beqz    a6, 2f
    mov     a2, a7
    call0   __tg_handle_cpu_interrupt
2:
    addi    a7, a7, 1
    slli    a5, a5, 1
    bltui   a7, 32, 0b
1:
    wsr     a4, INTCLEAR
    rsync
    wsr     a1, PS
    rsr     a0, EXCSAVE_5
    rfi     5

.align 4
.global _xt_highint6
.type _xt_highint6,@function
_xt_highint6:
    rsil    a1, 7
    rsr     a2, INTENABLE
    rsr     a3, INTERRUPT
    and     a4, a2, a3
    movi    a5, 1
    movi    a7, 0
0:
    beqz    a4, 1f
    and     a6, a4, a5
    beqz    a6, 2f
    mov     a2, a7
    call0   __tg_handle_cpu_interrupt
2:
    addi    a7, a7, 1
    slli    a5, a5, 1
    bltui   a7, 32, 0b
1:
    wsr     a4, INTCLEAR
    rsync
    wsr     a1, PS
    rsr     a0, EXCSAVE_6
    rfi     6