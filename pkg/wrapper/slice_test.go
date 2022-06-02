package wrapper

import (
	"reflect"
	"testing"
)

func TestLimit(t *testing.T) {
	type args struct {
		s          []byte
		limit      int
		manySuffix []byte
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{{
		name: "Test OK - 1",
		args: args{s: []byte("hello"), limit: 10},
		want: []byte("hello"),
	}, {
		name: "Test OK - 2",
		args: args{s: []byte("hello world!!!!!!!!!!!!!"), limit: 10},
		want: []byte("hello worl"),
	}, {
		name: "Test OK - 2",
		args: args{s: []byte("hello world!!!!!!!!!!!!!"), limit: 10, manySuffix: []byte(" ...")},
		want: []byte("hello worl ..."),
	}, {
		name: "Test OK - 3",
		args: args{s: []byte("helloworld"), limit: 10},
		want: []byte("helloworld"),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Limit(tt.args.s, tt.args.limit, tt.args.manySuffix...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Limit() = %v, want %v", got, tt.want)
			}
		})
	}
}
