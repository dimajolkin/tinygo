/* Linker script for the ESP32-C3 (application entry via external bootloader)
 *
 * This is a copy of esp32c3.ld but with a different entry point:
 *   ENTRY(call_start_cpu0_app)
 * so that an external bootloader (ESP-IDF) can set up flash/MMU and then
 * jump directly into the TinyGo app without reconfiguring MMU again.
 */

MEMORY
{
    /* Note: DRAM and IRAM below are actually in the same 384K address space. */
    DRAM (rw) : ORIGIN = 0x3FC80000, LENGTH = 384K /* Internal SRAM 1 (data bus) */
    IRAM (x)  : ORIGIN = 0x40380000, LENGTH = 384K /* Internal SRAM 1 (instruction bus) */

    /* Note: DROM and IROM below are actually in the same 8M address space. */
    DROM (r)  : ORIGIN = 0x3C000000, LENGTH = 8M /* Data bus (read-only) */
    IROM (rx) : ORIGIN = 0x42000000, LENGTH = 8M /* Instruction bus */
}

/* The entry point. For app-with-bootloader we enter at call_start_cpu0_app,
 * which just jumps to _start without touching Cache/MMU again.
 */
ENTRY(call_start_cpu0_app)

SECTIONS
{
    /* Dummy section to make sure the .rodata section starts exactly behind the
     * image header.
     */
    .rodata_dummy (NOLOAD): ALIGN(4)
    {
        . += 0x18; /* image header at start of flash: esp_image_header_t */
        . += 0x8;  /* DROM segment header (8 bytes) */
    } > DROM

    /* Constant global variables, stored in DROM.
     */
    .rodata : ALIGN(4)
    {
        *(.rodata*)
        . = ALIGN (4);
    } >DROM

    /* Put the stack at the bottom of DRAM, so that the application will
     * crash on stack overflow instead of silently corrupting memory.
     * See: http://blog.japaric.io/stack-overflow-protection/
     * TODO: this might not actually work because memory protection hasn't been set up.
     */
    .stack (NOLOAD) :
    {
        . = ALIGN(16);
        . += _stack_size;
        _stack_top = .;
    } >DRAM

    /* Global variables that are mutable and zero-initialized.
     * These must be zeroed at startup (unlike data, which is loaded by the
     * bootloader).
     */
    .bss (NOLOAD) : ALIGN(4)
    {
        . = ALIGN (4);
        _sbss = ABSOLUTE(.);
        *(.sbss)
        *(.bss .bss.*)
        . = ALIGN (4);
        _ebss = ABSOLUTE(.);
    } >DRAM

    /* Mutable global variables. This data (in the DRAM segment) is initialized
     * by the ROM bootloader.
     */
    .data : ALIGN(4)
    {
        . = ALIGN (4);
        _sdata = ABSOLUTE(.);
        *(.sdata)
        *(.data .data.*)
        *(.dram*)
        . = ALIGN (4);
        _edata = ABSOLUTE(.);
    } >DRAM

    /* Dummy section to make sure the .init section (in the IRAM segment) is just
     * behind the DRAM segment. For IRAM and DRAM, we luckily don't have to
     * worry about 64kB pages or image headers as they're loaded in RAM by the
     * bootloader (not mapped from flash).
     */
    .iram_dummy (NOLOAD): ALIGN(4)
    {
        . += SIZEOF(.stack);
        . += SIZEOF(.bss);
        . += SIZEOF(.data);
    } > IRAM

    /* IRAM segment. This contains some functions that always need to be loaded
     * in IRAM, and contains initialization code.
     * The initialization code is later reclaimed for the heap, so no RAM is
     * wasted.
     */
    .iram : ALIGN(4)
    {
        *(.iram*)
        *(.wifislprxiram*)
        *(.wifiextrairam*)
        *(.wifi0iram*)
        *(.wifislpiram*)
        *(.wifirxiram*)
        __init_start = .;
        *(.init)
        __init_end = .;
    } >IRAM

    /* Dummy section to put the IROM segment exactly behind the IRAM segment.
     * This has to follow the app image format exactly.
     */
    .text_dummy (NOLOAD): ALIGN(4)
    {
        /* Note: DRAM and DROM are not always present so the header should only
         * be inserted if it actually exists.
         */
        . += 0x18;                                                 /* esp_image_header_t */
        . += SIZEOF(.rodata) + ((SIZEOF(.rodata) != 0) ? 0x8 : 0); /* DROM segment (optional) */
        . += SIZEOF(.data)   + ((SIZEOF(.data)   != 0) ? 0x8 : 0); /* DRAM segment (optional) */
        . += SIZEOF(.iram)   + 0x8;                                /* IRAM segment */
        . += 0x8;                                                  /* IROM segment header */
    } > IROM

    /* IROM segment. This contains all the actual code and is placed right after
     * the DROM segment.
     */
    .text : ALIGN(4)
    {
        . = ALIGN (256);
        *(.text.exception_vectors)
        . = ALIGN (4);
        *(.text .text.*)
    } >IROM

    /DISCARD/ :
    {
        *(.eh_frame)       /* causes 'no memory region specified' error in lld */
    }

    /* Check that the boot ROM stack (for the APP CPU) does not overlap with the
     * data that is loaded by the boot ROM. This is unlikely to happen in
     * practice. 
     * The magic value comes from here:
     * https://github.com/espressif/esp-idf/blob/61299f879e/components/bootloader/subproject/main/ld/esp32c3/bootloader.ld#L191
     */
    ASSERT((_edata + SIZEOF(.iram)) < 0x3FCDE710, "the .iram section overlaps with the stack used by the boot ROM, possibly causing corruption at startup")
}

/* For the garbage collector.
 * Note that _heap_start starts after _edata + most of the IRAM section.
 * It starts just before the initialisation code, which isn't necessary anymore
 * after startup and can thus be overwritten by the heap.
 */
_globals_start = _sbss;
_globals_end = _edata;
_heap_start = _edata + SIZEOF(.iram) - (__init_end - __init_start);
_heap_end = ORIGIN(DRAM) + LENGTH(DRAM);

_stack_size = 8K;

/* ROM function interfaces from ESP-IDF linker scripts. */
INCLUDE targets/res/esp32c3/Id/esp32c3.rom.libgcc.ld
INCLUDE targets/res/esp32c3/Id/esp32c3.rom.newlib.ld
INCLUDE targets/res/esp32c3/Id/esp32c3.rom.newlib-time.ld
INCLUDE targets/res/esp32c3/Id/esp32c3.rom.newlib-nano.ld
INCLUDE targets/res/esp32c3/Id/esp32c3.rom.ld
INCLUDE targets/res/esp32c3/Id/esp32c3.rom.version.ld
INCLUDE targets/res/esp32c3/Id/esp32c3.rom.api.ld
