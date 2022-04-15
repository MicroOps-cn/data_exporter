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
	"reflect"
	"testing"
)

func TestValues_Float64s(t *testing.T) {
	type fields struct {
		val    string
		dftVal string
	}
	type args struct {
		seps []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []float64
		wantErr bool
	}{{
		name: "Test Floats(Include square brackets)",
		fields: struct {
			val    string
			dftVal string
		}{val: "[1, 2 ,3, 49999.11]", dftVal: ""},
		want:    []float64{1, 2, 3, 49999.11},
		wantErr: false,
	}, {
		name: "Test Floats(Include square brackets)",
		fields: struct {
			val    string
			dftVal string
		}{val: "[1, 2 ,3, 49999.11]", dftVal: ""},
		want:    []float64{1, 2, 3, 49999.11},
		wantErr: false,
	}, {
		name: "Test Floats(not include square brackets)",
		fields: struct {
			val    string
			dftVal string
		}{val: "1, 2 ,3, 49999.11", dftVal: ""},
		want:    []float64{1, 2, 3, 49999.11},
		wantErr: false,
	}, {
		name: "Test Error Floats",
		fields: struct {
			val    string
			dftVal string
		}{val: "1, 2 ,3, 49999.11]", dftVal: ""},
		want:    nil,
		wantErr: true,
	}, {
		name: "Test Floats(has null)",
		fields: struct {
			val    string
			dftVal string
		}{val: "1, 2 ,3,, 49999.11", dftVal: ""},
		want:    nil,
		wantErr: true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Values{
				val:    tt.fields.val,
				dftVal: tt.fields.dftVal,
			}
			got, err := v.Float64s(tt.args.seps...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Float64s() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Float64s() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValues(t *testing.T) {
	vals := NewValues("1,2,3,4", "")
	vals.Split(',')

}
