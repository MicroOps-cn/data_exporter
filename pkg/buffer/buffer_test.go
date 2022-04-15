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

package buffer

import (
	"github.com/MicroOps-cn/data_exporter/testings"
	"io"
	"os"
	"testing"
)

func TestNewLineBuffer(t *testing.T) {
	tt := testings.NewTesting(t)
	f, err := os.Open("../examples/my_data.txt")
	tt.AssertNoError(err)
	buf := NewLineBuffer(f, 64, 0, []string{"emory=", "\n"}, []byte("server3"))
	line, err := buf.ReadLine()
	tt.AssertNoError(err)
	// Test lineSep LF(\n)
	tt.AssertEqual("[server4]", string(line))
	_, err = buf.ReadLine()
	tt.AssertNoError(err)
	line, err = buf.ReadLine()
	tt.AssertNoError(err)
	tt.AssertEqual("m", string(line))
	line, err = buf.ReadLine()
	tt.AssertNoError(err)
	tt.AssertEqual("24359738368", string(line))
	line, err = buf.ReadLine()
	tt.AssertNoError(err)
	tt.AssertEqual("hostname=database1", string(line))
	line, err = buf.ReadLine()
	tt.AssertNoError(err)
	tt.AssertEqual("ip=1.1.1.", string(line))
	f.Close()
	f, err = os.Open("../examples/my_data.txt")
	tt.AssertNoError(err)
	buf = NewLineBuffer(f, 0, 5, []string{"]"}, []byte(""))
	line, err = buf.ReadLine()
	tt.AssertNoError(err)
	tt.AssertEqual("[serv", string(line))
	for {
		_, err = buf.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			tt.AssertNoError(err)
		}
	}
}
