//go:build windows

package banner

import "syscall"

func init() {
	// Switch the Windows console to UTF-8 so the block-art banner renders correctly.
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("SetConsoleOutputCP")
	_, _, _ = proc.Call(65001) // CP_UTF8
}
