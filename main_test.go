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

package main

import (
	"bytes"
	"fmt"
	"github.com/MicroOps-cn/data_exporter/collector"
	"github.com/MicroOps-cn/data_exporter/testings"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
        datapoint: "data|@expand|@expand|@to_entries:name:value"
        labels:
          __value__: "value"
          __name__: "name"
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
	defaultTimeout, err := time.ParseDuration("30s")
	if err != nil {
		panic(err)
	}
	collector.DefaultTimeout = &defaultTimeout
}
func TestCollectMetrics(t *testing.T) {
	tt := testings.NewTesting(t)
	logger := log.NewLogfmtLogger(os.Stdout)
	reader := bytes.NewReader([]byte(yamlConfigContent))
	tt.AssertNoError(sc.ReloadConfigFromReader(reader, logger))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("examples/weather.xml")
		tt.AssertNoError(err)
		defer f.Close()
		tt.AssertNoError(io.Copy(w, f))
	}))
	time.Sleep(time.Second)
	defer ts.Close()
	sc.C.Collects[1].Datasource[0].Url = ts.URL
	sc.C.Collects[0].Datasource[0].Url = "examples/my_data.json"

	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectMetrics(logger, w, r)
	})
	handler.ServeHTTP(rr, req)
	tt.AssertEqual(rr.Code, 200)
	body := rr.Body.String()
	assert.Contains(t, body, `weather_temperature_week{name="黑龙江",zone="china"} 18`)
	assert.Contains(t, body, `weather_temperature_hour{name="吉林",zone="china"} 16`)
	assert.Contains(t, body, `server_memory{name="server1"} 6.8719476736e+10`)
}

func TestCollectMetricsByName(t *testing.T) {
	tt := testings.NewTesting(t)
	logger := log.NewLogfmtLogger(os.Stdout)
	reader := bytes.NewReader([]byte(yamlConfigContent))
	tt.AssertNoError(sc.ReloadConfigFromReader(reader, logger))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("examples/weather.xml")
		tt.AssertNoError(err)
		defer f.Close()
		tt.AssertNoError(io.Copy(w, f))
	}))
	time.Sleep(time.Second)
	defer ts.Close()
	sc.C.Collects[1].Datasource[0].Url = ts.URL
	sc.C.Collects[0].Datasource[0].Url = "examples/my_data.json"

	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectMetricsByName(logger, "test-http", w, r)
	})
	handler.ServeHTTP(rr, req)
	tt.AssertEqual(rr.Code, 200)
	body := rr.Body.String()
	fmt.Println(body)
	assert.NotContains(t, body, `weather_temperature_week{name="黑龙江",zone="china"} 18`)
	assert.NotContains(t, body, `weather_temperature_hour{name="吉林",zone="china"} 16`)
	assert.Contains(t, body, `server_memory{name="server1"} 6.8719476736e+10`)
}
