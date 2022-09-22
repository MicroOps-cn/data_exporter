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

package transport

import (
	"bytes"
	"fmt"
	"github.com/MicroOps-cn/data_exporter/collector"
	"github.com/MicroOps-cn/data_exporter/config"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var yamlConfigContent = `
collects:
- name: "test-http"
  relabel_configs: []
  data_format: "json"
  datasource:
    - type: "file"
      url: "examples/my_data.json"
  metrics:
    - name: "Point1"
      metric_type: "counter"
      relabel_configs:
        - source_labels: [__name__]
          target_label: name
          regex: "([^.]+)\\.metrics\\..+"
          replacement: "$1"
          action: replace
        - source_labels: [__name__]
          target_label: __name__
          regex: "[^.]+\\.metrics\\.(.+)"
          replacement: "server_$1"
          action: replace
        - source_labels: [__value__]
          target_label: __value__
          action: templexec
          template: "{{ .|parseInt 0 64 }}"
      match:
        datapoint: "data|@expand|@expand|@to_entries:__name__:__value__"
- name: "weather"
  relabel_configs:
    - target_label: __namespace__
      replacement: "weather"
      action: replace
    - target_label: __subsystem__
      replacement: "temperature"
      action: replace
    - target_label: zone
      replacement: "china"
      action: replace
  data_format: "xml"
  datasource:
    - type: "http"
      url: ""
  metrics:
    - name: "weather - hour"
      match:
        datapoint: "//china[@dn='hour']/weather/city"
        labels:
          __value__: "{{ .Text }}"
          name: '{{ (.SelectAttr "quName").Value }}'
          __name__: "hour"
    - name: "weather - day"
      match:
        datapoint: "//china[@dn='day']/weather/city"
        labels:
          __value__: "{{ .Text }}"
          name: '{{ (.SelectAttr "quName").Value }}'
          __name__: "day"
    - name: "weather - week"
      match:
        datapoint: "//china[@dn='week']/city/weather"
        labels:
          __value__: "{{ .Text }}"
          name: '{{ ((.FindElement "../").SelectAttr "quName").Value }}'
          __name__: "week"
`

func init() {
	_ = os.Chdir("..")
	defaultTimeout, err := time.ParseDuration("30s")
	if err != nil {
		panic(err)
	}
	collector.DatasourceDefaultTimeout = defaultTimeout
}

var streamTestConfig = `
collects:
- name: "test-tcp-json"
  relabel_configs: []
  data_format: "json"
  datasource: 
  - type: tcp
    read_mode: stream
    config: {}
  metrics:
    - name: "Point1"
      metric_type: "gauge"
      relabel_configs:
        - source_labels: [__name__]
          target_label: name
          regex: "([^.]+)\\.metrics\\..+"
          replacement: "$1"
          action: replace
        - source_labels: [__name__]
          target_label: __name__
          regex: "[^.]+\\.metrics\\.(.+)"
          replacement: "server_$1"
          action: replace
        - source_labels: [__value__]
          target_label: __value__
          action: templexec
          template: "{{ . }}"
      match:
        datapoint: "data|@expand|@expand|@to_entries:__name__:__value__"
`

func TestStreamCollect(t *testing.T) {
	sc := config.NewSafeConfig()
	rand.Seed(time.Now().UTC().UnixNano())
	logger := log.NewLogfmtLogger(os.Stdout)

	addr := fmt.Sprintf("127.0.1.1:%d", rand.Intn(50000)+15530)
	t.Logf("start %s listen serv: %s", collector.Tcp.ToLowerString(), addr)
	listen, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	go func() {
		for {
			conn, _ := listen.Accept()
			if conn != nil {
				t.Logf("accept connect from clinet: %s", conn.RemoteAddr())
				go func(c net.Conn) {
					var i = 0
					for {
						i++
						var data = `{"data":{"server1":{"metrics":{"Memory":"%d","CPU":"10"}},"server2":{"metrics":{"Memory":"21293728632","CPU":"%d"}}},"code":0}`
						unix := time.Now().Unix()
						_, werr := c.Write([]byte(fmt.Sprintf(data+"\n", unix, i)))
						require.NoError(t, werr)
						time.Sleep(time.Second)
					}
				}(conn)
			}
		}
	}()
	defer func() {
		time.Sleep(time.Second)
		t.Logf("stop %s listen serv: %s", collector.Tcp.ToLowerString(), addr)
		listen.Close()
	}()

	reader := bytes.NewReader([]byte(streamTestConfig))
	var c = config.NewConfig()
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	if err = decoder.Decode(c); err != nil {
		require.NoError(t, err)
	}
	ds := c.Collects[0].Datasource[0]
	ds.Name = fmt.Sprintf("Test %s Datasource", collector.Tcp.ToLowerString())
	ds.Url = addr
	sc.SetConfig(c)
	err = c.Init(logger)
	require.NoError(t, err)
	time.Sleep(time.Second)

	for i := 0; i < 10; i++ {
		req, err := http.NewRequest("GET", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		server, err := NewHttpServer(logger, sc)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Log("get metric")
			server.collectMetricsByName(logger, "test-tcp-json", w, r)
		})
		handler.ServeHTTP(rr, req)
		require.Equal(t, rr.Code, 200)
		body := rr.Body.String()
		require.Contains(t, body, `server_cpu{name="server1"}`)
		exp := regexp.MustCompile(`server_memory\{name="server1"\} (\S+)`)
		match := exp.FindStringSubmatch(body)
		require.Equal(t, len(match), 2)
		val, err := strconv.ParseFloat(match[1], 64)
		require.NoError(t, err)
		delta := time.Now().Unix() - int64(val)
		require.Equal(t, delta >= 0 && delta <= 2, true)
		time.Sleep(time.Second - time.Millisecond*400)
	}
}

func TestCollectMetrics(t *testing.T) {
	sc := config.NewSafeConfig()
	logger := log.NewLogfmtLogger(os.Stdout)
	reader := bytes.NewReader([]byte(yamlConfigContent))
	require.NoError(t, sc.ReloadConfigFromReader(io.NopCloser(reader), logger))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("examples/weather.xml")
		require.NoError(t, err)
		defer f.Close()
		_, err = io.Copy(w, f)
		require.NoError(t, err)
	}))
	time.Sleep(time.Second)
	defer ts.Close()
	sc.C.Collects[1].Datasource[0].Url = ts.URL
	sc.C.Collects[0].Datasource[0].Url = "examples/my_data.json"

	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewHttpServer(logger, sc)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.collectMetrics(logger, w, r)
	})
	handler.ServeHTTP(rr, req)
	require.Equal(t, rr.Code, 200)
	body := rr.Body.String()
	require.Contains(t, body, `weather_temperature_week{name="黑龙江",zone="china"} 18`)
	require.Contains(t, body, `weather_temperature_hour{name="吉林",zone="china"} 16`)
	require.Contains(t, body, `server_memory{name="server1"} 6.8719476736e+10`)
}

func TestCollectMetricsByName(t *testing.T) {
	sc := config.NewSafeConfig()
	logger := log.NewLogfmtLogger(os.Stdout)
	reader := bytes.NewReader([]byte(yamlConfigContent))
	require.NoError(t, sc.ReloadConfigFromReader(io.NopCloser(reader), logger))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("examples/weather.xml")
		require.NoError(t, err)
		defer f.Close()
		_, err = io.Copy(w, f)
		require.NoError(t, err)
	}))
	time.Sleep(time.Second)
	defer ts.Close()
	sc.C.Collects[1].Datasource[0].Url = ts.URL
	sc.C.Collects[0].Datasource[0].Url = "examples/my_data.json"

	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewHttpServer(logger, sc)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.collectMetricsByName(logger, "test-http", w, r)
	})
	handler.ServeHTTP(rr, req)
	require.Equal(t, rr.Code, 200)
	body := rr.Body.String()
	require.NotContains(t, body, `weather_temperature_week{name="黑龙江",zone="china"} 18`)
	require.NotContains(t, body, `weather_temperature_hour{name="吉林",zone="china"} 16`)
	require.Contains(t, body, `server_memory{name="server1"} 6.8719476736e+10`)
}

func TestHTTPRoute(t *testing.T) {
	sc := config.NewSafeConfig()
	logger := log.NewLogfmtLogger(os.Stdout)
	reader := bytes.NewReader([]byte(yamlConfigContent))
	require.NoError(t, sc.ReloadConfigFromReader(io.NopCloser(reader), logger))
	type args struct {
		targetUrl string

		enablePprof bool
		pprofUrl    string
		routePrefix string
		externalURL string
		enableUi    bool
		uiPrefix    string
	}
	type want struct {
		newHttpServerError bool
		code               int
		contentContain     []string
		notContentContain  []string
	}
	tests := []struct {
		name string
		args args
		want want
	}{{
		name: "Test pprof root path - ok",
		args: args{targetUrl: "/-/pprof/", pprofUrl: "/-/pprof/", enablePprof: true},
		want: want{contentContain: []string{"Types of profiles available"}, code: 200},
	}, {
		name: "Test pprof cmdline path - ok",
		args: args{targetUrl: "/-/pprof/cmdline", pprofUrl: "/-/pprof/", enablePprof: true},
		want: want{contentContain: []string{strings.Join(os.Args, "\x00")}, code: 200},
	}, {
		name: "Test pprof goroutine path - ok",
		args: args{targetUrl: "/-/pprof/goroutine?debug=1", pprofUrl: "/-/pprof/", enablePprof: true},
		want: want{contentContain: []string{"goroutine profile: total"}, code: 200},
	}, {
		name: "Test pprof root path over Specify pprofUrl - ok",
		args: args{targetUrl: "/sokpda/", pprofUrl: "/sokpda", enablePprof: true},
		want: want{contentContain: []string{"Types of profiles available"}, code: 200},
	}, {
		name: "Test pprof cmdline path over Specify pprofUrl - ok",
		args: args{targetUrl: "/sokpda/cmdline", pprofUrl: "/sokpda", enablePprof: true},
		want: want{contentContain: []string{strings.Join(os.Args, "\x00")}, code: 200},
	}, {
		name: "Test pprof goroutine path over Specify pprofUrl - ok",
		args: args{targetUrl: "/sokpda/goroutine?debug=1", pprofUrl: "/sokpda/", enablePprof: true},
		want: want{contentContain: []string{"goroutine profile: total"}, code: 200},
	}, {
		name: "Test pprof root path over Specify routePrefix - ok",
		args: args{targetUrl: "/aaa/sokpda/", routePrefix: "/aaa/", pprofUrl: "/sokpda", enablePprof: true},
		want: want{contentContain: []string{"Types of profiles available"}, code: 200},
	}, {
		name: "Test pprof cmdline path over Specify routePrefix - ok",
		args: args{targetUrl: "/aaa/sokpda/cmdline", routePrefix: "/aaa/", pprofUrl: "/sokpda", enablePprof: true},
		want: want{contentContain: []string{strings.Join(os.Args, "\x00")}, code: 200},
	}, {
		name: "Test pprof goroutine path over Specify routePrefix - ok",
		args: args{targetUrl: "/aaa/sokpda/goroutine?debug=1", routePrefix: "/aaa/", pprofUrl: "/sokpda/", enablePprof: true},
		want: want{contentContain: []string{"goroutine profile: total"}, code: 200},
	}, {
		name: "Test health path  - ok",
		args: args{targetUrl: "/-/healthy", routePrefix: "/", enablePprof: true},
		want: want{contentContain: []string{"Healthy"}, code: 200},
	}, {
		name: "Test health path over Specify routePrefix - ok",
		args: args{targetUrl: "/aaa/-/healthy", routePrefix: "/aaa/", enablePprof: true},
		want: want{contentContain: []string{"Healthy"}, code: 200},
	}, {
		name: "Test metrics path  - ok",
		args: args{targetUrl: "/metrics", routePrefix: "/", enablePprof: true},
		want: want{contentContain: []string{`server_cpu{name="server1"} 16`}, code: 200},
	}, {
		name: "Test health path over Specify routePrefix - ok",
		args: args{targetUrl: "/aaa/metrics", routePrefix: "/aaa/", enablePprof: true},
		want: want{contentContain: []string{`server_cpu{name="server1"} 16`}, code: 200},
	}, {
		name: "Test specified metrics path  - ok",
		args: args{targetUrl: "/test-http/metrics", routePrefix: "/", enablePprof: true},
		want: want{contentContain: []string{`server_cpu{name="server1"} 16`}, code: 200},
	}, {
		name: "Test health path over Specify routePrefix - ok",
		args: args{targetUrl: "/aaa/test-http/metrics", routePrefix: "/aaa/", enablePprof: true},
		want: want{contentContain: []string{`server_cpu{name="server1"} 16`}, code: 200},
	}, {
		name: "Test health path over Specify routePrefix - ok",
		args: args{targetUrl: "/aaa/weather/metrics", routePrefix: "/aaa/", enablePprof: true},
		want: want{notContentContain: []string{`server_cpu{name="server1"} 16`}, code: 200},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*enablePprof = tt.args.enablePprof
			*pprofUrl = tt.args.pprofUrl
			*routePrefix = tt.args.routePrefix
			*externalURL = tt.args.externalURL
			*enableUi = tt.args.enableUi
			*uiPrefix = tt.args.uiPrefix
			server, err := NewHttpServer(logger, sc)
			if tt.want.newHttpServerError {
				require.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}
			req := httptest.NewRequest("GET", "http://127.0.0.1"+tt.args.targetUrl, nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				server.ServeHTTP(w, r)
			})
			handler.ServeHTTP(rr, req)
			require.Equal(t, rr.Code, tt.want.code)
			if len(tt.want.contentContain) > 0 || len(tt.want.notContentContain) > 0 {
				body := rr.Body.String()
				for _, s := range tt.want.contentContain {
					require.Contains(t, body, s)
				}
				for _, s := range tt.want.notContentContain {
					require.NotContains(t, body, s)
				}
			}
		})
	}
}
