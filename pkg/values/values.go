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

package values

import (
	"bytes"
	"strconv"
	"strings"
	"time"
)

type Values struct {
	val    string
	dftVal string
}

func NewValues(val, dftVal string) *Values {
	return &Values{val: val, dftVal: dftVal}
}
func (v *Values) Set(val string) *Values {
	v.val = val
	return v
}
func (v *Values) Default(dftVal string) *Values {
	v.dftVal = dftVal
	return v
}

func (v Values) String() string {
	if len(v.val) > 0 {
		return v.val
	}
	return v.dftVal
}

func (v Values) Split(seps ...byte) []Values {
	val := v.Bytes()
	if len(val) == 0 {
		return nil
	}
	if bytes.HasPrefix(val, []byte{'['}) && bytes.HasSuffix(val, []byte{']'}) {
		val = bytes.TrimSpace(bytes.TrimSuffix(bytes.TrimPrefix(val, []byte{'['}), []byte{']'}))
	}
	if len(val) == 0 {
		return nil
	}
	if len(seps) == 0 {
		seps = []byte{','}
	}
	var vals []Values
loop:
	for pos, i := 0, 0; i < len(val); i++ {
		for _, sep := range seps {
			if sep == val[i] {
				vals = append(vals, Values{val: string(val[pos:i])})
				pos = i + 1
				continue loop
			}
		}
		if i == len(val)-1 {
			vals = append(vals, Values{val: string(val[pos:])})
		}
	}
	return vals
}
func (v Values) Int() (int, error) {
	return strconv.Atoi(strings.TrimSpace(v.String()))
}
func (v Values) Int64() (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(v.String()), 10, 64)
}
func (v Values) Float32() (float32, error) {
	float, err := strconv.ParseFloat(strings.TrimSpace(v.String()), 64)
	return float32(float), err
}
func (v Values) Float64() (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(v.String()), 64)
}

func (v Values) Float64s(seps ...byte) (vals []float64, err error) {
	for _, s := range v.Split(seps...) {
		if f, err := s.Float64(); err != nil {
			return nil, err
		} else {
			vals = append(vals, f)
		}
	}
	return vals, nil
}

func (v Values) Bool() (bool, error) {
	return strconv.ParseBool(strings.TrimSpace(v.String()))
}

func (v Values) Duration() (time.Duration, error) {
	s := strings.TrimSpace(v.String())
	if len(s) > 0 && s[0] == '-' {
		duration, err := time.ParseDuration(s[1:])
		return -duration, err
	}
	return time.ParseDuration(s)
}

func (v Values) Durations(seps ...byte) (vals []time.Duration, err error) {
	for _, s := range v.Split(seps...) {
		if f, err := s.Duration(); err != nil {
			return nil, err
		} else {
			vals = append(vals, f)
		}
	}
	return vals, nil
}

func (v Values) Time(layout string) (time.Time, error) {
	return time.Parse(layout, v.String())
}

func (v Values) Bytes() []byte {
	return []byte(v.String())
}
