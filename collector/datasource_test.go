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

package collector

import (
	"context"
	"github.com/MicroOps-cn/data_exporter/testings"
	"io"
	"testing"
)

func TestReadLineClose(t *testing.T) {
	tt := testings.NewTesting(t)
	var maxContentLength int64 = 110
	ds := Datasource{ReadMode: StreamLine, Url: "../examples/my_data.txt", MaxContentLength: &maxContentLength, Type: File}
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
	ds := Datasource{ReadMode: Line, Url: "../examples/my_data.txt", MaxContentLength: &maxContentLength, Type: File}
	all, err := ds.ReadAll(context.TODO())
	tt.AssertNoError(err)
	if int64(len(all)-1) != maxContentLength && int64(len(all)) != maxContentLength {
		tt.AssertEqual(int64(len(all)-1), maxContentLength)
	}
}
