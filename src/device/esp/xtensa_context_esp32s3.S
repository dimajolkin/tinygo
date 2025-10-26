//go:build tinygo && esp32s3

#include "xtensa_esp32s3_macros.inc"

/*
 * Xtensa Context Save/Restore for ESP32-S3 (Call0 ABI)
 * 
 * Этот файл содержит низкоуровневые функции для сохранения и восстановления
 * контекста процессора при обработке прерываний.
 * 
 * Используется Call0 ABI (не Windowed):
 * - Сохраняется только 16 регистров (A0-A15)
 * - Контекст = 112 байт (не 256+ как в Windowed)
 * - Быстрое переключение контекста
 */

/*
 * _xt_context_save - Сохранить контекст текущего исполнения на стек
 * 
 * Входные параметры:
 *   a1 (sp) = текущий указатель стека
 *   
 * Действия:
 *   1. Сохранить все регистры A0-A15 на стек
 *   2. Сохранить регистры управления
 *   3. Выровнять стек на 16 байт
 *   
 * Выходные параметры:
 *   a1 (sp) = новый указатель стека (на 112 байт ниже)
 *   [sp] = сохраненный контекст
 *   
 * Использует: a2-a7 (рабочие регистры)
 */
.section .text._xt_context_save,"ax",@progbits
.align 4
.global _xt_context_save
.type _xt_context_save,@function

_xt_context_save:
    // Вычесть из стека 112 байт (размер контекста)
    addi    sp, sp, -XT_STK_FRMSZ           // sp -= 112
    
    // ===== Сохранить регистры общего назначения =====
    // Сохранить A0 на смещение XT_STK_A0 (12 байт)
    s32i    a0, sp, XT_STK_A0
    s32i    a1, sp, XT_STK_A1               // Исходный SP (до вычитания)
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
    s32i    a12, sp, XT_STK_A12
    s32i    a13, sp, XT_STK_A13
    s32i    a14, sp, XT_STK_A14
    s32i    a15, sp, XT_STK_A15
    
    // ===== Сохранить регистры управления =====
    
    // SAR (Shift Amount Register)
    rsr.sar a2
    s32i    a2, sp, XT_STK_SAR
    
    // EXCCAUSE (Exception Cause)
    rsr.exccause a2
    s32i    a2, sp, XT_STK_EXCCAUSE
    
    // EXCVADDR (Exception Virtual Address)
    rsr.excvaddr a2
    s32i    a2, sp, XT_STK_EXCVADDR
    
    // LBEG, LEND, LCOUNT (Loop registers)
    rsr.lbeg a2
    s32i    a2, sp, XT_STK_LBEG
    
    rsr.lend a2
    s32i    a2, sp, XT_STK_LEND
    
    rsr.lcount a2
    s32i    a2, sp, XT_STK_LCOUNT
    
    // ===== Сохранить Program Status и PC =====
    // PS уже был сохранен в ESP-IDF обработчике, но сохраним для консистентности
    rsr.ps a2
    s32i    a2, sp, XT_STK_PS
    
    // PC сохраняется из a0 (return address)
    // Это адрес инструкции, которая должна выполниться при возврате
    l32i    a2, sp, XT_STK_A0               // Загрузить A0 (return address)
    s32i    a2, sp, XT_STK_PC
    
    // Добавить адрес выхода (используется в некоторых обработчиках)
    movi    a2, _xt_context_restore_and_return
    s32i    a2, sp, XT_STK_EXIT
    
    // Возвращаемое значение = указатель на сохраненный контекст
    // a2 (возвращаемое значение) = sp (новый стек указатель)
    mov     a2, sp
    ret

/*
 * _xt_context_restore - Восстановить контекст из стека
 * 
 * Входные параметры:
 *   a2 = указатель на сохраненный контекст (на стеке)
 *   
 * Действия:
 *   1. Восстановить все регистры из стека
 *   2. Восстановить регистры управления
 *   3. Увеличить стек на 112 байт
 *   4. Перейти на восстановленный адрес (PC)
 *   
 * Регистр a1 (sp) восстанавливается из контекста
 * 
 * Использует: a3-a7 (рабочие регистры при восстановлении)
 */
.section .text._xt_context_restore,"ax",@progbits
.align 4
.global _xt_context_restore
.type _xt_context_restore,@function

_xt_context_restore:
    // a2 = указатель на контекст
    // Используем a2 как базовый указатель для загрузки
    
    // ===== Восстановить регистры управления =====
    
    // SAR (Shift Amount Register)
    l32i    a3, a2, XT_STK_SAR
    wsr.sar a3
    
    // EXCCAUSE (Exception Cause)
    l32i    a3, a2, XT_STK_EXCCAUSE
    wsr.exccause a3
    
    // EXCVADDR (Exception Virtual Address)
    l32i    a3, a2, XT_STK_EXCVADDR
    wsr.excvaddr a3
    
    // LBEG, LEND, LCOUNT (Loop registers)
    l32i    a3, a2, XT_STK_LBEG
    wsr.lbeg a3
    
    l32i    a3, a2, XT_STK_LEND
    wsr.lend a3
    
    l32i    a3, a2, XT_STK_LCOUNT
    wsr.lcount a3
    
    // Program Status (PS)
    l32i    a3, a2, XT_STK_PS
    wsr.ps  a3
    
    // Синхронизация после записи в регистры управления
    rsync
    
    // ===== Восстановить регистры общего назначения =====
    // A0 восстанавливаем в последний момент (нужен для возврата)
    
    // Восстановить A1-A15 (SP восстанавливаем в конце)
    l32i    a1, a2, XT_STK_A1
    l32i    a3, a2, XT_STK_A3
    l32i    a4, a2, XT_STK_A4
    l32i    a5, a2, XT_STK_A5
    l32i    a6, a2, XT_STK_A6
    l32i    a7, a2, XT_STK_A7
    l32i    a8, a2, XT_STK_A8
    l32i    a9, a2, XT_STK_A9
    l32i    a10, a2, XT_STK_A10
    l32i    a11, a2, XT_STK_A11
    l32i    a12, a2, XT_STK_A12
    l32i    a13, a2, XT_STK_A13
    l32i    a14, a2, XT_STK_A14
    l32i    a15, a2, XT_STK_A15
    
    // Восстановить PC в A0 и возвращаемое значение в A2
    l32i    a0, a2, XT_STK_PC
    l32i    a2, a2, XT_STK_A2
    
    // Возврат из обработчика прерывания
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
 * _xt_context_restore_rfe - Восстановить контекст и вернуться через RFE
 *
 * Входные параметры:
 *   a2 = указатель на сохраненный контекст (на стеке)
 *
 * Действия:
 *   1. Восстановить ключевые регистры и SP
 *   2. Восстановить регистры управления (PS, SAR, L*)
 *   3. Выполнить RFE (возврат по EPC1/PS)
 */
.section .text._xt_context_restore_rfe,"ax",@progbits
.align 4
.global _xt_context_restore_rfe
.type _xt_context_restore_rfe,@function

_xt_context_restore_rfe:
    // Восстановить SP из сохраненного контекста
    l32i    a1, a2, XT_STK_A1

    // Восстановить регистры управления (порядок как в restore)
    l32i    a3, a2, XT_STK_SAR
    wsr.sar a3

    l32i    a3, a2, XT_STK_EXCCAUSE
    wsr.exccause a3

    l32i    a3, a2, XT_STK_EXCVADDR
    wsr.excvaddr a3

    l32i    a3, a2, XT_STK_LBEG
    wsr.lbeg a3

    l32i    a3, a2, XT_STK_LEND
    wsr.lend a3

    l32i    a3, a2, XT_STK_LCOUNT
    wsr.lcount a3

    // Восстановить PS (INTLEVEL и др.)
    l32i    a3, a2, XT_STK_PS
    wsr.ps  a3
    rsync

    // Восстановить часть общих регистров (минимально необходимое)
    l32i    a3, a2, XT_STK_A3
    l32i    a4, a2, XT_STK_A4
    l32i    a5, a2, XT_STK_A5
    l32i    a6, a2, XT_STK_A6
    l32i    a7, a2, XT_STK_A7
    l32i    a8, a2, XT_STK_A8
    l32i    a9, a2, XT_STK_A9
    l32i    a10, a2, XT_STK_A10
    l32i    a11, a2, XT_STK_A11
    l32i    a12, a2, XT_STK_A12
    l32i    a13, a2, XT_STK_A13
    l32i    a14, a2, XT_STK_A14
    l32i    a15, a2, XT_STK_A15

    // Возврат из исключения
    rfe
    nop
    nop

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
