//go:build scheduler.tasks && esp32s3

package task

import "unsafe"

//go:extern tinygo_startTask
var tinygo_startTask [0]byte

const (
	call4WindowBits = uintptr(1 << 30)
	psUserMask      = 0x00000020 // PS_UM
	psExcmMask      = 0x00000010 // PS_EXCM
	psWoeMask       = 0x00040000 // PS_WOE
	psCallInc1      = 0x00010000 // PS_CALLINC(1)
)

// Layout must match Xtensa interrupt frame expected by tinygo_context_restore.
type calleeSavedRegs struct {
	exit   uint32
	pc     uint32
	ps     uint32
	a0     uint32
	a1     uint32
	a2     uint32
	a3     uint32
	a4     uint32
	a5     uint32
	a6     uint32
	a7     uint32
	a8     uint32
	a9     uint32
	a10    uint32
	a11    uint32
	a12    uint32
	a13    uint32
	a14    uint32
	a15    uint32
	sar    uint32
	exc    uint32
	vaddr  uint32
	lbeg   uint32
	lend   uint32
	lcount uint32
	tmp0   uint32
	tmp1   uint32
	tmp2   uint32
}

var systemStack uintptr

func (s *state) archInit(r *calleeSavedRegs, fn uintptr, args unsafe.Pointer) {
	entry := uintptr(unsafe.Pointer(&tinygo_startTask))
	frame := uintptr(unsafe.Pointer(r))

	r.exit = 0
	r.pc = uint32(entry)
	r.ps = psUserMask | psExcmMask | psWoeMask | psCallInc1
	r.a0 = uint32((entry &^ (3 << 30)) | call4WindowBits)
	r.a1 = uint32(frame + unsafe.Sizeof(*r))
	r.a2 = uint32(uintptr(args))
	r.a3 = uint32(fn)
	r.a4 = 0
	r.a5 = 0
	r.a6 = 0
	r.a7 = 0
	r.a8 = 0
	r.a9 = 0
	r.a10 = 0
	r.a11 = 0
	r.a12 = 0
	r.a13 = 0
	r.a14 = 0
	r.a15 = 0
	r.sar = 0
	r.exc = 0
	r.vaddr = 0
	r.lbeg = 0
	r.lend = 0
	r.lcount = 0
	r.tmp0 = 0
	r.tmp1 = 0
	r.tmp2 = 0

	s.sp = frame
}

func (s *state) resume() {
	swapTask(s.sp, &systemStack)
}

func (s *state) pause() {
	saved := systemStack
	systemStack = 0
	swapTask(saved, &s.sp)
}

func SystemStack() uintptr {
	return systemStack
}
