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

package config

import (
	"bytes"
	"gitee.com/paycooplus/data_exporter/collector"
	testings "gitee.com/paycooplus/data_exporter/testings"
	"github.com/go-kit/log"
	"os"
	"strings"
	"testing"
)

func TestLoadConfigFromFile(t *testing.T) {
	tt := testings.NewTesting(t)
	wd, err := os.Getwd()
	tt.AssertNoError(err)
	tt.Logf("当前路径: %s", wd)
	sc := NewConfig()
	tt.AssertNoError(sc.ReloadConfig("../examples/data_exporter.yaml", log.NewLogfmtLogger(os.Stdout)))
	tt.AssertEqual(len(sc.C.Collects), 2)
	for _, collect := range sc.C.Collects {
		for _, ds := range collect.Datasource {
			tt.AssertNotEqual(len(ds.Type), 0)
			tt.AssertNotEqual(len(ds.Url), 0)
		}
		for _, metric := range collect.Metrics {
			tt.AssertNotEqual(len(metric.MetricType), 0)
		}
	}
}

var yamlConfigContent = `
collects:
- name: "test-http"
  relabel_configs: 
    - source_labels: [__name__,"name"]
      target_label: name
      regex: "([^.]+)\\.metrics\\..+"
      replacement: "$1"
      separator: "."			
      action: drop
  data_format: "json"
  datasource:
    - type: "file"
      url: "../examples/my_data.json"
      relabel_configs: 
        - target_label: __namespace__
          replacement: "server"
  metrics:
    - name: "Point1"
      metric_type: "counter"
      relabel_configs:
        - source_labels: [__name__]
          target_label: __name__
          regex: "[^.]+\\.metrics\\.(.+)"
          replacement: "$1"
          action: replace
      match:
        datapoint: "data|@expand|@expand|@to_entries:name:value"
        labels:
          __value__: "value"
          __name__: "name"
    - name: "Point2"
`

func TestReloadConfig(t *testing.T) {
	tt := testings.NewTesting(t)
	logger := log.NewLogfmtLogger(os.Stdout)
	sc := NewConfig()
	reader := bytes.NewReader([]byte(yamlConfigContent))
	tt.AssertNoError(sc.ReloadConfigFromReader(reader, logger))
	tt.AssertEqual(sc.C.Collects[0].DataFormat, collector.Json)
	reader = bytes.NewReader([]byte(strings.ReplaceAll(yamlConfigContent, `data_format: "json"`, `data_format: "yaml"`)))
	tt.AssertNoError(sc.ReloadConfigFromReader(reader, logger))
	tt.AssertEqual(sc.C.Collects[0].DataFormat, collector.Yaml)
}

func TestLoadConfig(t *testing.T) {
	tt := testings.NewTesting(t)
	logger := log.NewLogfmtLogger(os.Stdout)
	sc := NewConfig()
	reader := bytes.NewReader([]byte(yamlConfigContent))
	tt.AssertNoError(sc.ReloadConfigFromReader(reader, logger))
	tt.AssertEqual(len(sc.C.Collects), 1)
	collect := sc.C.Collects[0]
	tt.AssertEqual(collect.DataFormat, collector.Json)
	tt.AssertEqual(collect.Name, "test-http")

	tt.AssertEqual(len(collect.RelabelConfigs), 1)
	relabelConfig := collect.RelabelConfigs[0]
	tt.AssertEqual(relabelConfig.Regex.Regexp.String(), "^(?:"+`([^.]+)\.metrics\..+`+")$")
	tt.AssertEqual(relabelConfig.Action, collector.Drop)
	tt.AssertEqual(relabelConfig.Replacement, "$1")
	tt.AssertEqual(relabelConfig.TargetLabel, "name")
	tt.AssertEqual(relabelConfig.SourceLabels.String(), strings.Join([]string{"__name__", "name"}, ", "))
	tt.AssertEqual(relabelConfig.Separator, ".")

	tt.AssertEqual(len(collect.Datasource), 1)
	ds := collect.Datasource[0]
	tt.AssertEqual(ds.Url, "../examples/my_data.json")
	tt.AssertEqual(ds.Type, collector.File)
	tt.AssertEqual(len(ds.RelabelConfigs), 1)
	dsRelabelConfig := ds.RelabelConfigs[0]
	tt.AssertEqual(dsRelabelConfig.Regex.Regexp.String(), "^(?:"+`(.*)`+")$")
	tt.AssertEqual(dsRelabelConfig.Action, collector.Replace)
	tt.AssertEqual(dsRelabelConfig.Replacement, "server")
	tt.AssertEqual(dsRelabelConfig.TargetLabel, "__namespace__")
	tt.AssertEqual(dsRelabelConfig.SourceLabels.String(), strings.Join([]string{}, ", "))
	tt.AssertEqual(dsRelabelConfig.Separator, ";")

	tt.AssertEqual(len(collect.Metrics), 2)
	metric := collect.Metrics[0]
	tt.AssertEqual(metric.Name, "Point1")
	tt.AssertEqual(len(metric.RelabelConfigs), 1)
	tt.AssertEqual(metric.MetricType, collector.Counter)
	tt.AssertEqual(collect.Metrics[1].MetricType, collector.Gauge)
	tt.AssertEqual(metric.Match.Datapoint, "data|@expand|@expand|@to_entries:name:value")
	tt.AssertEqual(metric.Match.Labels["__value__"], "value")
	tt.AssertEqual(metric.Match.Labels["__name__"], "name")

}
