//go:build scheduler.tasks && esp32s3

package task

//go:extern port_xSchedulerRunning
var port_xSchedulerRunning uint32

//go:extern port_interruptNesting
var port_interruptNesting uint32

//go:extern port_switch_flag
var port_switch_flag uint32

//go:extern _xt_tick_divisor
var _xt_tick_divisor uint32

//go:extern port_yield_from_isr
func port_yield_from_isr()
