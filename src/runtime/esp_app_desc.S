//go:build esp32s3

// ESP app descriptor in assembly for ESP32-S3
// This creates the required esp_app_desc structure for ESP-IDF bootloader compatibility

.section .rodata,"a",@progbits
.global esp_app_desc
.type esp_app_desc, @object
.align 4
esp_app_desc:
    // magic (ESP_APP_DESC_MAGIC_WORD)
    .word 0xABCD5432
    
    // secure_version
    .word 0
    
    // reserv1[2]
    .word 0, 0
    
    // version[32] - "TinyGo v0.34"
    .ascii "TinyGo v0.34\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0"
    
    // project_name[32] - "TinyGo App"
    .ascii "TinyGo App\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0"
    
    // time[16] - "00:00:00"
    .ascii "00:00:00\0\0\0\0\0\0\0\0"
    
    // date[16] - "Jan  1 2024"
    .ascii "Jan  1 2024\0\0\0\0\0"
    
    // idf_ver[32] - "TinyGo Compat"
    .ascii "TinyGo Compat\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0"
    
    // app_elf_sha256[32] - all zeros
    .space 32, 0
    
    // reserv2[20] - all zeros
    .space 80, 0
    
    // min_efuse_blk_rev_full - set to 0 for maximum compatibility
    .word 0
    
    // max_efuse_blk_rev_full - set to 9999 for maximum compatibility  
    .word 9999

.size esp_app_desc, . - esp_app_desc
