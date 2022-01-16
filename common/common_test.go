package common

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
	line, err = buf.ReadLine()
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
		line, err = buf.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			tt.AssertNoError(err)
		}
	}
}
