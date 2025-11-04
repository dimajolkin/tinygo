//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Xtensa Context Save/Restore for ESP32-S3 (Call0 ABI)
 * Based on ESP-IDF: components/xtensa/xtensa_context.S
 * 
 * Этот файл содержит низкоуровневые функции для сохранения и восстановления
 * контекста процессора при обработке прерываний.
 * 
 * Используется Call0 ABI (не Windowed):
 * - Сохраняется только 16 регистров (A0-A15)
 * - Контекст = XT_STK_FRMSZ байт (144 байта для ESP32-S3)
 * - Быстрое переключение контекста
 */

/* Special Register Numbers (from ESP-IDF xtensa/specreg.h) */
#define SREG_SAR        3
#define SREG_EXCCAUSE   232
#define SREG_EXCVADDR   238
#define SREG_LBEG       0
#define SREG_LEND       1
#define SREG_LCOUNT     2
#define SREG_PS         230
#define SREG_EPC1       177

/*
 * _xt_context_save - Сохранить контекст текущего исполнения на стек
 * Based on ESP-IDF: components/xtensa/xtensa_context.S
 * 
 * ВАЖНО: Вызывающий код должен УЖЕ выделить XT_STK_FRMSZ байт на стеке!
 * ВАЖНО: a12, a13 должны быть УЖЕ сохранены вызывающим кодом!
 * 
 * Входные параметры:
 *   a1 (sp) = указатель на уже выделенный stack frame
 *   
 * Действия:
 *   1. Сохранить оставшиеся регистры A2-A11, A14-A15 на стек
 *   2. Сохранить регистры управления (SAR, LBEG/LEND/LCOUNT)
 *   
 * Выходные параметры:
 *   Все регистры сохранены в [sp]
 *   
 * Использует: a2-a3 (рабочие регистры)
 * Сохраняет: a0 (return address сохраняется через стек)
 */
.section .text._xt_context_save,"ax",@progbits
.align 4
.global _xt_context_save
.type _xt_context_save,@function

_xt_context_save:
    /* NOTE: a12, a13 already saved by caller (_frxt_int_enter) */
    
    /* Save a2-a11 */
    s32i    a2, sp, XT_STK_A2
    s32i    a3, sp, XT_STK_A3
    s32i    a4, sp, XT_STK_A4
    s32i    a5, sp, XT_STK_A5
    s32i    a6, sp, XT_STK_A6
    s32i    a7, sp, XT_STK_A7
    s32i    a8, sp, XT_STK_A8
    s32i    a9, sp, XT_STK_A9
    s32i    a10, sp, XT_STK_A10
    s32i    a11, sp, XT_STK_A11

    /* Save a14-a15 (a12-a13 already saved by caller) */
    s32i    a14, sp, XT_STK_A14
    s32i    a15, sp, XT_STK_A15

    /* Save special registers using numbered format (not dot notation) */
    
    /* SAR (Shift Amount Register) */
    rsr     a2, SREG_SAR
    s32i    a2, sp, XT_STK_SAR

    /* Loop registers (if present) */
    #if XCHAL_HAVE_LOOPS
    rsr     a2, SREG_LBEG
    s32i    a2, sp, XT_STK_LBEG
    rsr     a2, SREG_LEND
    s32i    a2, sp, XT_STK_LEND
    rsr     a2, SREG_LCOUNT
    s32i    a2, sp, XT_STK_LCOUNT
    #endif

    /* NOTE: EXCCAUSE and EXCVADDR already saved by _xt_lowint1 */
    /* NOTE: PS and PC already saved by _xt_lowint1 */
    /* NOTE: a0 and a1 already saved by _xt_lowint1 */

    ret

/*
 * _xt_context_restore - Восстановить контекст из стека
 * Based on ESP-IDF: components/xtensa/xtensa_context.S
 *
 * ВАЖНО: Предполагается, что sp уже указывает на сохранённый фрейм!
 * Вызывающий код должен установить sp перед вызовом.
 *
 * Входные параметры:
 *   sp = указатель на сохраненный контекст (stack frame)
 *
 * Действия:
 *   1. Восстановить специальные регистры (SAR, LBEG/LEND/LCOUNT)
 *   2. Восстановить регистры общего назначения a2-a15
 *
 * Выходные параметры:
 *   Все регистры восстановлены из [sp]
 *   sp НЕ изменяется (вызывающий код должен восстановить sp после)
 *
 * Использует: a2-a3 (рабочие регистры при восстановлении)
 */
.section .text._xt_context_restore,"ax",@progbits
.align 4
.global _xt_context_restore
.type _xt_context_restore,@function

_xt_context_restore:
    /* Restore special registers using numbered format (not dot notation) */
    
    /* SAR (Shift Amount Register) */
    l32i    a2, sp, XT_STK_SAR
    wsr     a2, SREG_SAR

    /* Loop registers (if present) */
    #if XCHAL_HAVE_LOOPS
    l32i    a2, sp, XT_STK_LBEG
    wsr     a2, SREG_LBEG
    l32i    a2, sp, XT_STK_LEND
    wsr     a2, SREG_LEND
    l32i    a2, sp, XT_STK_LCOUNT
    wsr     a2, SREG_LCOUNT
    #endif

    /* Sync after special register writes */
    rsync

    /* Restore general registers a2-a15 */
    /* NOTE: a0, a1 will be restored by caller (_xt_user_exit) */
    
    l32i    a2, sp, XT_STK_A2
    l32i    a3, sp, XT_STK_A3
    l32i    a4, sp, XT_STK_A4
    l32i    a5, sp, XT_STK_A5
    l32i    a6, sp, XT_STK_A6
    l32i    a7, sp, XT_STK_A7
    l32i    a8, sp, XT_STK_A8
    l32i    a9, sp, XT_STK_A9
    l32i    a10, sp, XT_STK_A10
    l32i    a11, sp, XT_STK_A11
    l32i    a12, sp, XT_STK_A12
    l32i    a13, sp, XT_STK_A13
    l32i    a14, sp, XT_STK_A14
    l32i    a15, sp, XT_STK_A15

    ret

/*
 * _xt_context_restore_and_return - Вспомогательная функция
 * Используется в качестве адреса выхода из обработчика
 */
.section .text._xt_context_restore_and_return,"ax",@progbits
.align 4
.global _xt_context_restore_and_return
.type _xt_context_restore_and_return,@function

_xt_context_restore_and_return:
    // Эта функция вызывается при завершении прерывания
    // Для переключения контекста используется:
    // 1. _xt_context_save() сохраняет текущий контекст
    // 2. Загружается новый контекст
    // 3. _xt_context_restore() восстанавливает регистры
    // 4. ret инструкция возвращает управление
    ret

/*
 * Helper function: no longer needed with ESP-IDF style exit
 * Kept for compatibility if needed in future
 */

/*
 * _xt_check_context_alignment - Проверка выравнивания контекста
 * 
 * Входные параметры:
 *   a2 = указатель на контекст
 *   
 * Выходные параметры:
 *   a2 = 0 если выровнено, 1 если ошибка
 *   
 * Проверяет, что контекст выровнен на 16 байт
 */
.section .text._xt_check_context_alignment,"ax",@progbits
.align 4
.global _xt_check_context_alignment
.type _xt_check_context_alignment,@function

_xt_check_context_alignment:
    // Проверить выравнивание на 16 байт
    // Маска для проверки нижних 4 бит (16 = 2^4)
    movi    a3, 0xF
    and     a2, a2, a3
    
    // Если a2 == 0, то выровнено
    movi    a3, 0
    beq     a2, a3, .alignment_ok
    
    // Ошибка выравнивания
    movi    a2, 1
    ret
    
.alignment_ok:
    movi    a2, 0
    ret

/*
 * Макрос для отладочного вывода состояния контекста
 * (используется только при отладке)
 */
.macro DUMP_CONTEXT_REGS
    // Это место для вспомогательного кода отладки
    // В production версии может быть пусто
.endm
