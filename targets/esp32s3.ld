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

/* Define _vector_base BEFORE the section so TinyGo can see it */
_vector_base = 0x40374000;

SECTIONS
{
    /* Vectors: VMA in IRAM, LMA in Flash (copied by call_start_cpu0)
     * 
     * TinyGo is self-booting - we initialize cache and copy IRAM segments ourselves.
     * VMA = Virtual address (0x40374000 IRAM) for call0 relocations
     * LMA = Load address (Flash DROM) where esptool places it
     */
    .vectors _vector_base : ALIGN(0x400)
    {
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
        _vectors_end = .;
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
        _text_start = .;
        
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
        _sdata = .;
        *(.data)
        *(.data.*)
        _edata = .;
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
        _sbss = .;
        *(.bss)
        *(.bss.*)
        . = ALIGN (4);
        _ebss = .;
    } >DRAM
    
    /* CRITICAL: Verify .bss section is within valid DRAM range
     * ESP32-S3 DRAM: 0x3FC88000 - 0x3FCF0000 (416 KB)
     * This prevents clearbss() from writing to invalid memory
     */
    ASSERT(_sbss >= 0x3FC88000, "ERROR: .bss starts before DRAM region!")
    ASSERT(_ebss <= 0x3FCF0000, "ERROR: .bss ends after DRAM region!")
    ASSERT(_sbss < _ebss, "ERROR: .bss section has invalid size!")
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

/* From ESP-IDF:
 * components/esp_rom/esp32s3/ld/esp32s3.rom.ld
 * ALL Cache management functions for ESP32-S3 (84 functions total)
 * These functions are in ROM chip memory and always available.
 * Total size in ROM: ~4-6 KB, cost in firmware: 0 bytes.
 */

/* Information functions */
Cache_Get_ICache_Line_Size = 0x400015fc;
Cache_Get_DCache_Line_Size = 0x40001608;
Cache_Get_Mode = 0x40001614;
Cache_Get_IROM_MMU_End = 0x4000195c;
Cache_Get_DROM_MMU_End = 0x40001968;
Cache_Get_Virtual_Addr = 0x400019ec;
Cache_Get_Memory_BaseAddr = 0x400019f8;
Cache_Get_Memory_Addr = 0x40001a04;
Cache_Get_Memory_value = 0x40001a10;

/* Mode configuration */
Cache_Set_ICache_Mode = 0x40001620;
Cache_Set_DCache_Mode = 0x4000162c;
Cache_Set_Default_Mode = 0x40001650;
Cache_Enable_Defalut_ICache_Mode = 0x4000165c;

/* Cache initialization - ⭐ USED IN TINYGO */
ROM_Boot_Cache_Init = 0x40001668;
Cache_Invalidate_ICache_All = 0x400016d4;  /* ⭐ USED */
Cache_Invalidate_DCache_All = 0x400016e0;  /* ⭐ USED */
Cache_Enable_ICache = 0x40001878;          /* ⭐ USED */
Cache_Enable_DCache = 0x40001890;          /* ⭐ USED */
Cache_Disable_ICache = 0x4000186c;
Cache_Disable_DCache = 0x40001884;

/* Invalidation, clean, write-back */
Cache_Invalidate_ICache_Items = 0x40001674;
Cache_Invalidate_DCache_Items = 0x40001680;
Cache_Invalidate_Addr = 0x400016b0;
Cache_Clean_Items = 0x4000168c;
Cache_Clean_Addr = 0x400016bc;
Cache_Clean_All = 0x400016ec;
Cache_WriteBack_Items = 0x40001698;
rom_Cache_WriteBack_Addr = 0x400016c8;
Cache_WriteBack_All = 0x400016f8;
Cache_Op_Addr = 0x400016a4;

/* Bus control */
Cache_Address_Through_ICache = 0x40001638;
Cache_Address_Through_DCache = 0x40001644;
Cache_Mask_All = 0x40001704;
Cache_UnMask_Dram0 = 0x40001710;

/* Autoload */
Cache_Config_ICache_Autoload = 0x40001794;
Cache_Config_DCache_Autoload = 0x400017c4;
Cache_Config_ICache_Region_Autoload = 0x400017a0;
Cache_Config_DCache_Region_Autoload = 0x400017d0;
Cache_Enable_ICache_Autoload = 0x400017ac;
Cache_Disable_ICache_Autoload = 0x400017b8;
Cache_Enable_DCache_Autoload = 0x400017dc;
Cache_Disable_DCache_Autoload = 0x400017e8;
Cache_Suspend_ICache_Autoload = 0x4000171c;
Cache_Resume_ICache_Autoload = 0x40001728;
Cache_Suspend_DCache_Autoload = 0x40001734;
Cache_Resume_DCache_Autoload = 0x40001740;

/* Preload */
Cache_Start_ICache_Preload = 0x4000174c;
Cache_ICache_Preload_Done = 0x40001758;
Cache_End_ICache_Preload = 0x40001764;
Cache_Start_DCache_Preload = 0x40001770;
Cache_DCache_Preload_Done = 0x4000177c;
Cache_End_DCache_Preload = 0x40001788;

/* Lock/PreLock */
Cache_Enable_ICache_PreLock = 0x400017f4;
Cache_Disable_ICache_PreLock = 0x40001800;
Cache_Lock_ICache_Items = 0x4000180c;
Cache_Unlock_ICache_Items = 0x40001818;
Cache_Enable_DCache_PreLock = 0x40001824;
Cache_Disable_DCache_PreLock = 0x40001830;
Cache_Lock_DCache_Items = 0x4000183c;
Cache_Unlock_DCache_Items = 0x40001848;
Cache_Lock_Addr = 0x40001854;
Cache_Unlock_Addr = 0x40001860;

/* Suspend/Resume */
rom_Cache_Suspend_ICache = 0x4000189c;
Cache_Resume_ICache = 0x400018a8;
rom_Cache_Suspend_DCache = 0x400018b4;
Cache_Resume_DCache = 0x400018c0;

/* Freeze */
rom_Cache_Freeze_ICache_Enable = 0x400018e4;
Cache_Freeze_ICache_Disable = 0x400018f0;
rom_Cache_Freeze_DCache_Enable = 0x400018fc;
Cache_Freeze_DCache_Disable = 0x40001908;

/* MMU functions - ⭐ POTENTIALLY USEFUL */
Cache_Set_IDROM_MMU_Size = 0x40001914;
Cache_Set_IDROM_MMU_Info = 0x40001950;
Cache_Owner_Init = 0x40001974;
Cache_MMU_Init = 0x40001998;  /* ⭐ Initialize MMU */
Cache_Ibus_MMU_Set = 0x400019a4;
Cache_Dbus_MMU_Set = 0x400019b0;

/* Memory occupation */
Cache_Occupy_ICache_MEMORY = 0x40001980;
Cache_Occupy_DCache_MEMORY = 0x4000198c;
Cache_Occupy_Items = 0x400018cc;
Cache_Occupy_Addr = 0x400018d8;

/* Auxiliary */
rom_Cache_Count_Flash_Pages = 0x400019bc;
Cache_Flash_To_SPIRAM_Copy = 0x400019c8;
Cache_Travel_Tag_Memory = 0x400019d4;
Cache_Travel_Tag_Memory2 = 0x400019e0;
