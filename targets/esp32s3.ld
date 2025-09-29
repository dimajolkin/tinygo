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
    /* Place Xtensa vector table at the very start of IRAM and reserve 0x400 bytes. */
    .vectors : ALIGN(0x400)
    {
        . = ORIGIN(IRAM);
        _vector_table = ABSOLUTE(.);
        /* Window vectors (fixed layout within first 0x180 bytes) */
        KEEP(*(.WindowVectors.text))

        /* Interrupt/exception vectors at specified offsets from VECBASE */
        . = _vector_table + 0x180; KEEP(*(.Level2InterruptVector.text))
        . = _vector_table + 0x1C0; KEEP(*(.Level3InterruptVector.text))
        . = _vector_table + 0x200; KEEP(*(.Level4InterruptVector.text))
        . = _vector_table + 0x240; KEEP(*(.Level5InterruptVector.text))
        . = _vector_table + 0x280; KEEP(*(.Level6InterruptVector.text))
        . = _vector_table + 0x2C0; KEEP(*(.NMIExceptionVector.text))
        . = _vector_table + 0x300; KEEP(*(.KernelExceptionVector.text))
        . = _vector_table + 0x340; KEEP(*(.UserExceptionVector.text))
        . = _vector_table + 0x3C0; KEEP(*(.DoubleExceptionVector.text))

        /* Reserve full 0x400 bytes for vector area */
        . = _vector_table + 0x400;
    } >IRAM
    /* Constant literals and code. Loaded into IRAM for now. Eventually, most
     * code should be executed directly from flash.
     * Note that literals must be before code for the l32r instruction to work.
     */
    .text : ALIGN(4)
    {
        *(.literal.call_start_cpu0)
        *(.text.call_start_cpu0)
        *(.literal .text)
        *(.literal.* .text.*)
    } >IRAM

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

_stack_size = 8K;

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
