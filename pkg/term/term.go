package term

import (
	"bytes"
	"fmt"
)

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
