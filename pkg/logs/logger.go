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

package logs

import (
	"fmt"
	"github.com/MicroOps-cn/data_exporter/pkg/term"
	"io"
	"strings"
)

type debugLogger struct {
	w io.Writer
}

var debugBlackName = []string{"collect", "metric", "datasource", "msg", "oldLabels", "relabelConfigs", "labels", "data", "exp", "result", "resultCount", "label", "err"}

func (d debugLogger) Log(keyvals ...interface{}) error {
	var name, val interface{}
	var kvMap = map[interface{}]interface{}{}
	for i := 0; i < len(keyvals); i += 2 {
		name = keyvals[i]
		val = ""
		if len(keyvals) > i {
			val = keyvals[i+1]
		}
		kvMap[name] = val
	}
	if title, ok := kvMap["title"].(string); ok && len(title) > 0 {
		d.w.Write([]byte(term.TitleStr(title, '-')))
	}
	for _, name = range debugBlackName {
		if val, ok := kvMap[name]; ok && val != nil {
			switch v := val.(type) {
			case []byte:
				d.w.Write([]byte(fmt.Sprintf("%-20s%s\n", fmt.Sprintf("[%s]", name), strings.ReplaceAll(fmt.Sprintf("%s", string(v)), "\n", " "))))
			default:
				d.w.Write([]byte(fmt.Sprintf("%-20s%s\n", fmt.Sprintf("[%s]", name), strings.ReplaceAll(fmt.Sprintf("%v", val), "\n", " "))))
			}
		}
	}
	if title, ok := kvMap["title"].(string); ok && len(title) > 0 {
		d.w.Write([]byte(term.TitleStr("", '-')))
	}
	return nil
}

func newDebugLogger(w io.Writer) *debugLogger {
	return &debugLogger{w}
}
