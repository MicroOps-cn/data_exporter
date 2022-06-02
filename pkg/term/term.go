package term

import (
	"bytes"
	"fmt"
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
func TitleStr(name string, sep rune) string {
	var width int
	var err error

	if width, err = terminalWidth(); err != nil || width <= 0 {
		width = 50
	} else if len(name) > 0 && len(name)+4 >= width {
		width = len(name) + 4
	}
	buf := bytes.NewBuffer(nil)
	if len(name) > 0 {
		for i := 0; i < (width-(len(name)+6))/2; i++ {
			buf.WriteRune(sep)
		}
		buf.WriteString(" [" + name + "] ")
		for i := 0; i < (width-(len(name)+6))/2; i++ {
			buf.WriteRune(sep)
		}
	} else {
		for i := 0; i < width; i++ {
			buf.WriteRune(sep)
		}
	}

	buf.WriteString("\n")
	return buf.String()
}
func Title(name string, sep rune) {
	fmt.Print(TitleStr(name, sep))
}
