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
	t.Log(args)
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
