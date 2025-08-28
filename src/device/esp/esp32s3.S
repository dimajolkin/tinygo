// ESP App Descriptor required by ESP32-S3 bootloader
.section .rodata_desc, "a", %progbits
.global esp_app_desc
esp_app_desc:
    .long 0xABCD5432            // magic word
    .long 0                     // secure_version
    .long 0, 0                  // reserv1[2]
    .ascii "1.0.0"              // version[32]
    .space 32-5                 // pad to 32 bytes
    .ascii "TinyGo-ESP32S3"     // project_name[32]
    .space 32-14                // pad to 32 bytes  
    .ascii "00:00:00"           // time[16]
    .space 16-8                 // pad to 16 bytes
    .ascii "Jan  1 2024"        // date[16]
    .space 16-11                // pad to 16 bytes
    .ascii "TinyGo-v1"          // idf_ver[32]
    .space 32-9                 // pad to 32 bytes
    .space 32                   // app_elf_sha256[32] - zeros
    .short 0x0000               // min_efuse_blk_rev_full - compatible with all chips (v0.0)
    .short 0x0000               // max_efuse_blk_rev_full - compatible with all chips (unlimited)
    .byte 0x00                  // mmu_page_size - default
    .space 3                    // reserv3[3] - zeros
    .space 72                   // reserv2[18] - zeros (18 * 4 = 72 bytes)
// External symbols
.extern main
.extern esp_app_desc

// The following definitions were copied from:
// esp-idf/components/xtensa/include/xtensa/corebits.h
#define PS_WOE_MASK                0x00040000
#define PS_OWB_MASK                0x00000F01
#define PS_CALLINC_MASK            0x00030000
#define PS_WOE                     PS_WOE_MASK

// Only calling it call_start_cpu0 for consistency with ESP-IDF.
.section .text.call_start_cpu0
1:
    .long _stack_top
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
