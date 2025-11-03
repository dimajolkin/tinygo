//go:build tinygo && esp32s3

/* Helper functions for interrupt testing and control on ESP32-S3 */

.section .text

/* main.triggerSoftwareInterrupt(mask uint32) */
.global main.triggerSoftwareInterrupt
.type main.triggerSoftwareInterrupt,@function
.align 4
main.triggerSoftwareInterrupt:
    /* a2 = interrupt mask (1 << intNum) */
    wsr.intset a2
    rsync
    ret

/* main.getINTERRUPT() uint32 */
.global main.getINTERRUPT  
.type main.getINTERRUPT,@function
.align 4
main.getINTERRUPT:
    rsr a2, interrupt
    ret

/* main.getINTENABLE() uint32 */
.global main.getINTENABLE
.type main.getINTENABLE,@function
.align 4
main.getINTENABLE:
    rsr a2, intenable
    ret

/* main.setINTENABLE(val uint32) */
.global main.setINTENABLE
.type main.setINTENABLE,@function
.align 4
main.setINTENABLE:
    /* a2 = new INTENABLE value */
    wsr a2, intenable
    rsync
    ret

/* main.getPS() uint32 */
.global main.getPS
.type main.getPS,@function
.align 4
main.getPS:
    rsr a2, ps
    ret

/* main.clearInterrupt(mask uint32) */
.global main.clearInterrupt
.type main.clearInterrupt,@function
.align 4
main.clearInterrupt:
    /* a2 = interrupt mask */
    wsr.intclear a2
    rsync
    ret

/* main.getVECBASE() uint32 */
.global main.getVECBASE
.type main.getVECBASE,@function
.align 4
main.getVECBASE:
    rsr a2, vecbase
    ret

