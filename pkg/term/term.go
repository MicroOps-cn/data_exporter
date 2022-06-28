// Copyright 2021 MicroOps
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
