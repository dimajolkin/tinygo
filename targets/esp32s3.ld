/* Linker script for the ESP32-S3 */


MEMORY
{
    /* Note: DRAM and IRAM below are actually in the same 416K address space. */
    DRAM (rw) : ORIGIN = 0x3FC88000, LENGTH = 416K /* Internal SRAM 1 (data bus) */
    IRAM (x)  : ORIGIN = 0x40374000, LENGTH = 400K /* Internal SRAM 1 (correct for ESP32-S3) */

    /* Note: DROM and IROM below are actually in the same 32M address space. */
    DROM (r)  : ORIGIN = 0x3C000000, LENGTH = 32M /* Data bus (read-only) */
    IROM (rx) : ORIGIN = 0x42000000, LENGTH = 32M /* Instruction bus */
}

/* The entry point. It is set in the image flashed to the chip, so must be
 * defined.
 */
ENTRY(call_start_cpu0)

SECTIONS
{
    /* Vectors: VMA in IRAM, LMA in Flash (copied by call_start_cpu0)
     * 
     * TinyGo is self-booting - we initialize cache and copy IRAM segments ourselves.
     * VMA = Virtual address (0x40374000 IRAM) for call0 relocations
     * LMA = Load address (Flash DROM) where esptool places it
     */
    .vectors 0x40374000 : ALIGN(0x400)
    {
        _vector_base = ABSOLUTE(.);
        _vectors_flash_start = LOADADDR(.vectors);

        /* ESP32-S3 Call0 ABI Vector Table Layout (from Xtensa ISA)
         * Each vector at fixed 32-byte (0x20) offset from VECBASE
         * Based on ESP-IDF esp_system/ld/esp32s3/sections.ld.in
         */
        
        /* UserException at +0x00 */
        KEEP(*(.UserExceptionVector.text));
        . = _vector_base + 0x20;
        
        /* DoubleException at +0x20 */
        KEEP(*(.DoubleExceptionVector.text));
        . = _vector_base + 0x40;
        
        /* KernelException at +0x40 */
        KEEP(*(.KernelExceptionVector.text));
        . = _vector_base + 0x60;
        
        /* NMIException at +0x60 */
        KEEP(*(.NMIExceptionVector.text));
        . = _vector_base + 0x80;
        
        /* Level2Interrupt at +0x80 */
        KEEP(*(.Level2InterruptVector.text));
        . = _vector_base + 0xA0;
        
        /* Level3Interrupt at +0xA0 */
        KEEP(*(.Level3InterruptVector.text));
        . = _vector_base + 0xC0;
        
        /* Level4Interrupt at +0xC0 */
        KEEP(*(.Level4InterruptVector.text));
        . = _vector_base + 0xE0;
        
        /* Level5Interrupt at +0xE0 */
        KEEP(*(.Level5InterruptVector.text));
        . = _vector_base + 0x100;
        
        /* Level6Interrupt at +0x100 */
        KEEP(*(.Level6InterruptVector.text));
        . = _vector_base + 0x120;
        
        /* Level7Interrupt at +0x120 */
        KEEP(*(.Level7InterruptVector.text));
        . = _vector_base + 0x140;
        
        /* Set explicit boundary for vectors (ESP-IDF pattern)
         * Reserve 384 bytes for vectors:
         * - 10 vectors × 32 bytes = 320 bytes
         * - 64 bytes for alignment and safety margin
         */
        . = _vector_base + 0x180;
        
        /* Collect ALL vector literals AFTER the vector code (ESP-IDF style) */
        . = ALIGN(4);
        *(.*Vector.literal)
        
        /* Mark end of vectors section */
        . = ALIGN(16);
        _vectors_end = ABSOLUTE(.);
        _vectors_flash_end = _vectors_flash_start + SIZEOF(.vectors);
    } >IRAM AT>DROM

    /* Verify vectors section size (must be <= 1KB like ESP-IDF) */
    ASSERT((SIZEOF(.vectors) <= 0x400), "ERROR: .vectors section exceeds 1KB!")
    ASSERT((_vectors_end <= (_vector_base + 0x400)), "ERROR: .vectors end address exceeds VECBASE+1KB!")

    /* Constant literals and code. Loaded into IRAM for now. Eventually, most
     * code should be executed directly from flash.
     * Note that literals must be before code for the l32r instruction to work.
     */
    .text : ALIGN(4)
    {
        /* Mark start of .text section for diagnostics */
        _text_start = ABSOLUTE(.);
        
        /* Dispatcher and low-level ISR code (moved from .vectors, ESP-IDF pattern)
         * These must be in IRAM for interrupt handling but are not part of vector table */
        *(.literal._xt_user_exc)
        *(.text._xt_user_exc)
        *(.literal._xt_lowint1)
        *(.text._xt_lowint1)
        *(.literal._xt_user_exit)
        *(.text._xt_user_exit)
        *(.literal._xt_unhandled_exception)
        *(.text._xt_unhandled_exception)
        
        /* Regular code */
        *(.literal.call_start_cpu0)
        *(.text.call_start_cpu0)
        *(.literal .text)
        *(.literal.* .text.*)
    } >IRAM
    
    /* Verify no overlap between .vectors and .text */
    ASSERT((_text_start >= _vectors_end), "ERROR: .text section overlaps with .vectors!")

    /* Put the stack at the bottom of DRAM, so that the application will
     * crash on stack overflow instead of silently corrupting memory.
     * See: http://blog.japaric.io/stack-overflow-protection/ */
    .stack (NOLOAD) :
    {
        . = ALIGN(16);
        . += _stack_size;
        _stack_top = .;
    } >DRAM

    /* Constant global variables.
     * They are loaded in DRAM for ease of use. Eventually they should be stored
     * in flash and loaded directly from there but they're kept in RAM to make
     * sure they can always be accessed (even in interrupts).
     */
    .rodata : ALIGN(4)
    {
        *(.rodata)
        *(.rodata.*)
    } >DRAM

    /* Mutable global variables.
     */
    .data : ALIGN(4)
    {
        _sdata = ABSOLUTE(.);
        *(.data)
        *(.data.*)
        _edata = ABSOLUTE(.);
    } >DRAM

    /* Check that the boot ROM stack does not overlap with the
     * data that is loaded by the boot ROM. 
     * ESP32-S3 has different memory layout than ESP32.
     */
    ASSERT(_edata < 0x3fce0000, "the .data section overlaps with the stack used by the boot ROM, possibly causing corruption at startup")

    /* Global variables that are mutable and zero-initialized.
     * These must be zeroed at startup (unlike data, which is loaded by the
     * bootloader).
     */
    .bss (NOLOAD) : ALIGN(4)
    {
        . = ALIGN (4);
        _sbss = ABSOLUTE(.);
        *(.bss)
        *(.bss.*)
        . = ALIGN (4);
        _ebss = ABSOLUTE(.);
    } >DRAM
}

/* For the garbage collector.
 * Limit heap size to avoid excessive metadata overhead.
 * Reserve 64KB for heap - this should be enough for most applications
 * while keeping metadata size reasonable.
 */
_globals_start = _sdata;
_globals_end = _ebss;
_heap_start = _ebss;
_heap_end = _ebss + 64K;

_stack_size = 16K;  /* Increased for interrupt handling */

/* From ESP-IDF:
 * components/esp_rom/esp32s3/ld/esp32s3.rom.newlib-funcs.ld
 * Basic memory functions for ESP32-S3
 * Temporarily disable ROM functions due to memset hanging issue
 */
/* memset = 0x40000354; */
/* memcpy = 0x40000358; */
/* memmove = 0x4000035c; */

/* From ESP-IDF:
 * components/esp_rom/esp32s3/ld/esp32s3.rom.libgcc.ld
 * Essential libgcc functions for ESP32-S3
 */
__divsi3 = 0x400007c0;
__udivsi3 = 0x400008b4;
__modsi3 = 0x40000840;
__umodsi3 = 0x400008c0;
__ashldi3 = 0x4000077c;
__ashrdi3 = 0x40000780;
__lshrdi3 = 0x40000830;
