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

.extern _xt_level1_int_handler
.extern _xt_unhandled_exception
.extern _frxt_int_depth
.extern _xt_user_exc

/* Provide storage for interrupt counters */
.section .bss._frxt_int_depth, "aw", @nobits
.align 4
.global _frxt_int_depth
_frxt_int_depth:
    .space 4

.section .bss.VectorEntryCount, "aw", @nobits
.align 4
.global VectorEntryCount
VectorEntryCount:
    .space 4

.section .bss.IsrCountBeforeGo, "aw", @nobits
.align 4
.global IsrCountBeforeGo
IsrCountBeforeGo:
    .space 4

.section .bss.IsrCountAfterGo, "aw", @nobits
.align 4
.global IsrCountAfterGo
IsrCountAfterGo:
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
.end        literal_prefix

/* User exception dispatcher: check if Level-1 interrupt or exception */
.section .text._xt_user_exc, "ax", @progbits
.align 4
.type _xt_user_exc,@function
_xt_user_exc:
    /* DEBUG: Increment VectorEntryCount to track vector entry */
    wsr     a2, EXCSAVE_1                   /* save a2 temporarily */
    movi    a0, VectorEntryCount
    l32i    a2, a0, 0
    addi    a2, a2, 1
    s32i    a2, a0, 0
    rsr     a2, EXCSAVE_1                   /* restore a2 */
    rsr     a0, EXCSAVE_1                   /* restore a0 (was saved by vector) */
    
    /* Check if this is a Level-1 interrupt (EXCCAUSE == 4) */
    wsr     a0, EXCSAVE_1                   /* save a0 again for exccause read */
    rsr     a0, EXCCAUSE                    /* read EXCCAUSE */
    bnei    a0, 4, _xt_unhandled_exception  /* if not level1 interrupt, handle as exception */
    /* Fall through to _xt_lowint1 for interrupts */

/* Low-level level-1 interrupt entry: create stack frame and dispatch */
.section .text._xt_lowint1, "ax", @progbits
.align 4
.global _xt_lowint1
_xt_lowint1:
    /* Save minimal context on stack (following ESP-IDF model) */
    mov     a0, sp                          /* a0 = current sp */
    addi    sp, sp, -XT_STK_FRMSZ           /* allocate stack frame (112 bytes) */
    s32i    a0, sp, XT_STK_A1               /* save pre-interrupt SP */
    
    rsr     a0, PS                          /* save PS */
    s32i    a0, sp, XT_STK_PS
    
    rsr     a0, EPC_1                       /* save interruptee's PC from EPC_1 */
    s32i    a0, sp, XT_STK_PC
    
    rsr     a0, EXCSAVE_1                   /* restore original a0 from EXCSAVE_1 */
    s32i    a0, sp, XT_STK_A0               /* save original a0 */
    
    movi    a0, _xt_user_exit               /* save exit point */
    s32i    a0, sp, XT_STK_EXIT
    
    /* Now jump to the main interrupt handler */
    call0   _xt_level1_int_handler_entry
    /* never returns here */

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

/*
 * Exception and Interrupt Vectors (Level 2-7, Double, Kernel, NMI)
 * Based on ESP-IDF esp-idf/components/xtensa/xtensa_vectors.S
 */

/* DoubleException - nested exception (VECBASE+0x020) */
    .begin      literal_prefix .DoubleExceptionVector
    .section    .DoubleExceptionVector.text, "ax"
    .align      4
_DoubleExceptionVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* KernelException - privileged instruction in user mode (VECBASE+0x040) */
    .begin      literal_prefix .KernelExceptionVector
    .section    .KernelExceptionVector.text, "ax"
    .align      4
_KernelExceptionVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* NMI - Non-Maskable Interrupt (VECBASE+0x060) */
    .begin      literal_prefix .NMIExceptionVector
    .section    .NMIExceptionVector.text, "ax"
    .align      4
_NMIExceptionVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* Level 2 Interrupt - medium priority (VECBASE+0x080) */
    .begin      literal_prefix .Level2InterruptVector
    .section    .Level2InterruptVector.text, "ax"
    .align      4
_Level2InterruptVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* Level 3 Interrupt - high priority (VECBASE+0x0A0) */
    .begin      literal_prefix .Level3InterruptVector
    .section    .Level3InterruptVector.text, "ax"
    .align      4
_Level3InterruptVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* Level 4 Interrupt (VECBASE+0x0C0) */
    .begin      literal_prefix .Level4InterruptVector
    .section    .Level4InterruptVector.text, "ax"
    .align      4
_Level4InterruptVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* Level 5 Interrupt (VECBASE+0x0E0) */
    .begin      literal_prefix .Level5InterruptVector
    .section    .Level5InterruptVector.text, "ax"
    .align      4
_Level5InterruptVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* Level 6 Interrupt (VECBASE+0x100) */
    .begin      literal_prefix .Level6InterruptVector
    .section    .Level6InterruptVector.text, "ax"
    .align      4
_Level6InterruptVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix

/* Level 7 Interrupt - highest priority, NMI-like (VECBASE+0x120) */
    .begin      literal_prefix .Level7InterruptVector
    .section    .Level7InterruptVector.text, "ax"
    .align      4
_Level7InterruptVector:
    call0   _xt_unhandled_exception
    .end        literal_prefix


