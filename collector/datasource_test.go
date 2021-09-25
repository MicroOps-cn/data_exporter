package collector

import (
	"context"
	"gitee.com/paycooplus/data_exporter/testings"
	"io"
	"testing"
)

func TestReadLineClose(t *testing.T) {
	tt := testings.NewTesting(t)
	var maxContentLength int64 = 110
	ds := Datasource{ReadMode: StreamLine, Url: "../examples/my_data.txt", MaxContentLength: maxContentLength, Type: File}
	stream, err := ds.GetLineStream(context.TODO())
	tt.AssertNoError(err)
	defer stream.Close()
	var line []byte
	var byteCount int
	for {
		line, err = stream.ReadLine()
		if err != nil {
			if err != io.EOF {
				t.Errorf("文件读取异常: %s", err)
			}
			break
		}
		tt.NotContains(string(line), "\n")
		byteCount += len(line) + 1
	}
	if int64(byteCount-1) != maxContentLength && int64(byteCount) != maxContentLength {
		tt.AssertEqual(int64(byteCount-1), maxContentLength)
	}
}

func TestReadAll(t *testing.T) {
	tt := testings.NewTesting(t)
	var maxContentLength int64 = 110
	ds := Datasource{ReadMode: StreamLine, Url: "../examples/my_data.txt", MaxContentLength: maxContentLength, Type: File}
	all, err := ds.ReadAll(context.TODO())
	tt.AssertNoError(err)
	if int64(len(all)-1) != maxContentLength && int64(len(all)) != maxContentLength {
		tt.AssertEqual(int64(len(all)-1), maxContentLength)
	}
}
