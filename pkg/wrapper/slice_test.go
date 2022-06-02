package wrapper

import (
	"reflect"
	"testing"
)

func TestLimit(t *testing.T) {
	type args struct {
		s            []byte
		limit        int
		manySuffix   []byte
		hidePosition int
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
		want: []byte("hello  ..."),
	}, {
		name: "Test OK - 3",
		args: args{s: []byte("helloworld"), limit: 10},
		want: []byte("helloworld"),
	}, {
		name: "Test OK - 4",
		args: args{
			s:            []byte(`level=info ts=2022-06-02T01:06:10.469Z caller=tls_config.go:191 msg="TLS is disabled." http2=false`),
			limit:        20,
			manySuffix:   []byte(" ... "),
			hidePosition: PosCenter,
		},
		want: []byte("level=i ... p2=false"),
	}, {
		name: "Test OK - 4",
		args: args{
			s:            []byte(`level=info ts=2022-06-02T01:06:10.469Z caller=tls_config.go:191 msg="TLS is disabled." http2=false`),
			limit:        20,
			manySuffix:   []byte(" ... "),
			hidePosition: PosLeft,
		},
		want: []byte(` ... d." http2=false`),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var oldData []byte
			for _, b := range tt.args.s {
				oldData = append(oldData, b)
			}
			got := Limit(tt.args.s, tt.args.limit, tt.args.hidePosition, tt.args.manySuffix...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Limit() = %v, want %v", string(got), string(tt.want))
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("old data = %v, want %v", string(oldData), string(tt.args.s))
			}
		})
	}
}
