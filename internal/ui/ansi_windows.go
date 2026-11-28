package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableANSI turns on escape-sequence processing in the Windows console (off by default in conhost).
func enableANSI(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
