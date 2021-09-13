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
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
	"os"
	"sync"
)

type DataFormat string

const (
	Text DataFormat = "Text"
	Json DataFormat = "json"
	Xml  DataFormat = "xml"
	Yaml DataFormat = "yaml"
)

type DatasourceType string

const (
	Http DatasourceType = "http"
	File DatasourceType = "file"
)

type Datasource struct {
	Name             string         `yaml:"name"`
	MetricNamePrefix string         `yaml:"metric_name_prefix"`
	Labels           Labels         `yaml:"labels"`
	Url              string         `yaml:"url"`
	Type             DatasourceType `yaml:"type"`
}

type Label struct {
	Name              string `yaml:"name"`
	Value             string `yaml:"value"`
	ValueRegexReplace string `yaml:"value_regex_replace"`
	Weight            uint8  `yaml:"weight"` // 0~255，默认权重100
}

type Labels []Label

func (l *Labels) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var sliceLabels []Label
	if err := unmarshal(&sliceLabels); err != nil {
		var mapLabels = map[string]string{}
		if err := unmarshal(&mapLabels); err != nil {
			return err
		}
		for key, value := range mapLabels {
			*l = append(*l, Label{Name: key, Value: value, Weight: 100})
		}
	}

	for _, label := range *l {
		if len(label.Value) == 0 || len(label.Name) == 0 {
			return fmt.Errorf("label format error: name and value cannot be empty")
		}
	}
	return nil
}

type DatapointMatch struct {
	Datapoint  string `yaml:"datapoint"`
	MetricName string `yaml:"metric_name"`
	Labels     string `yaml:"labels"`
	Timestamp  string `yaml:"timestamp"`
	Value      string `yaml:"value"`
}

type MetricName struct {
	Replace string `yaml:"replace"`
	Regex   string `yaml:"regex"`
}

func (m *MetricName) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain MetricName
	m.Regex = "(.*)"
	m.Replace = "$1"
	if err := unmarshal((*plain)(m)); err != nil {
		return err
	}
	return nil
}

type Datapoint struct {
	Name       string         `yaml:"name"`
	MetricName MetricName     `yaml:"metric_name"`
	Labels     Labels         `yaml:"labels"`
	Match      DatapointMatch `yaml:"match"`
}

type Datapoints []Datapoint

type Collect struct {
	Name       string
	Labels     Labels       `yaml:"labels"`
	DataFormat DataFormat   `yaml:"data_format"`
	Datasource []Datasource `yaml:"datasource"`
	Datapoint  Datapoints   `yaml:"datapoints"`
}

type Collects []Collect

type Config struct {
	Collects Collects `yaml:"collects"`
}

var (
	configReloadSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "blackbox_exporter",
		Name:      "config_last_reload_successful",
		Help:      "Blackbox exporter config loaded successfully.",
	})

	configReloadSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "blackbox_exporter",
		Name:      "config_last_reload_success_timestamp_seconds",
		Help:      "Timestamp of the last successful configuration reload.",
	})
)

func init() {
	prometheus.MustRegister(configReloadSuccess)
	prometheus.MustRegister(configReloadSeconds)
}

type SafeConfig struct {
	sync.RWMutex
	C *Config
}

func (sc *SafeConfig) ReloadConfig(confFile string, logger interface{}) (err error) {
	var c = &Config{}
	defer func() {
		if err != nil {
			configReloadSuccess.Set(0)
		} else {
			configReloadSuccess.Set(1)
			configReloadSeconds.SetToCurrentTime()
		}
	}()

	yamlReader, err := os.Open(confFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %s", err)
	}
	defer yamlReader.Close()
	decoder := yaml.NewDecoder(yamlReader)
	decoder.KnownFields(true)

	if err = decoder.Decode(c); err != nil {
		return fmt.Errorf("error parsing config file: %s", err)
	}

	sc.Lock()
	sc.C = c
	sc.Unlock()
	return nil
}
