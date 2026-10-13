package diag

import (
	"os"
	"syscall"
	"unsafe"
)

// enableVT turns on ANSI escape processing in the Windows console (on by
// default in Windows Terminal, off in older conhost windows).
func enableVT() {
	const enableVirtualTerminalProcessing = 0x0004
	k32 := syscall.NewLazyDLL("kernel32.dll")
	get, set := k32.NewProc("GetConsoleMode"), k32.NewProc("SetConsoleMode")
	h := os.Stdout.Fd()
	var mode uint32
	if r, _, _ := get.Call(h, uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}
	set.Call(h, uintptr(mode|enableVirtualTerminalProcessing))
}
