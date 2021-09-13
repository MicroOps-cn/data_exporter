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
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"testing"
)

var testConfig = []byte(`
collects:
- name: "test-http"
  labels: { }
  data_format: "json"
  metric_name_prefix: ""
  datasource:
    - type: "http"
      url: "https://localhost/examples/my_data.json"
      labels:
        position: "server"
    - type: "file"
      metric_name_prefix: ""
      url: "./examples/my_data.json"
  datapoints:
    - name: "Point1"
      labels: {}
      metric_name:
        regex: ".*"
        replace: "-$1-"
      match:
        datapoint: ""
        metric_name: ""
        labels: ""
        timestamp: ""
        value: ""
`)

func TestConfig(t *testing.T) {
	var conf = Config{}
	if err := yaml.Unmarshal(testConfig, &conf); err != nil {
		t.Error(err)
	}
	assert.Contains(t, conf.Collects[0].Datapoint[0].MetricName.Replace, "-$1-")
	assert.Contains(t, conf.Collects[0].Datasource[0].Url, "https://localhost/examples/my_data.json")
}
