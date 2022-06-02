//go:build darwin || linux

package term

import (
	"syscall"
	"unsafe"
)

func terminalWidth() (int, error) {
	type window struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}
	w := new(window)
	tio := syscall.TIOCGWINSZ
	res, _, err := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(tio),
		uintptr(unsafe.Pointer(w)),
	)
	if int(res) == -1 {
		return 0, err
	}
	return int(w.Col), nil
}
