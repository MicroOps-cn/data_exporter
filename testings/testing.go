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

package testings

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

type T struct {
	*testing.T
}

func NewTesting(t *testing.T) *T {
	return &T{T: t}
}
func (t *T) AssertNoError(args ...interface{}) {
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			assert.NoError(t, err)
			os.Exit(1)
		}
	}
}

func (t *T) LogAndAssertNoError(args ...interface{}) {
	t.Log(args...)
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			assert.NoError(t, err)
			os.Exit(1)
		}
	}
}

func (t *T) AssertEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	if !assert.Equal(t.T, expected, actual, msgAndArgs) {
		os.Exit(1)
	}
}
func (t *T) AssertNotEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	if !assert.NotEqual(t.T, expected, actual, msgAndArgs) {
		os.Exit(1)
	}
}
