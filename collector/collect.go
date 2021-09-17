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

package collector

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/beevik/etree"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
	"io"
	"regexp"
	"strings"
)

type DataFormat string

func (d DataFormat) ToLower() DataFormat {
	return DataFormat(strings.ToLower(string(d)))
}

var (
	collectErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "data_collect_error_count",
		Help: "Blackbox exporter config loaded successfully.",
	}, []string{"type"})
)

func RegisterCollector(reg prometheus.Registerer) {
	reg.MustRegister(collectErrorCount)
}

const (
	Regex DataFormat = "regex"
	Json  DataFormat = "json"
	Xml   DataFormat = "xml"
	Yaml  DataFormat = "yaml"
)

type Collect struct {
	Name           string
	RelabelConfigs RelabelConfigs `yaml:"relabel_configs"`
	DataFormat     DataFormat     `yaml:"data_format"`
	Datasource     []*Datasource  `yaml:"datasource"`
	Metrics        MetricConfigs  `yaml:"metrics"`
	logger         log.Logger
}

func regexCompile(regexStr string, require bool, point string) (*regexp.Regexp, error) {
	if len(regexStr) == 0 {
		if require {
			return nil, fmt.Errorf("%s value cannot be empty", point)
		}
		return nil, nil
	} else if compile, err := regexp.Compile(regexStr); err != nil {
		return nil, fmt.Errorf("%s syntax error: %s", point, err)
	} else {
		return compile, err
	}
}
func xmlPathCompile(pathStr string, require bool, point string) (*etree.Path, error) {
	if len(pathStr) == 0 {
		if require {
			return nil, fmt.Errorf("%s value cannot be empty", point)
		}
		return nil, nil
	} else if compile, err := etree.CompilePath(pathStr); err != nil {
		return nil, fmt.Errorf("%s syntax error: %s", point, err)
	} else {
		return &compile, err
	}
}

func (c *Collect) UnmarshalYAML(value *yaml.Node) error {
	type plain Collect
	if err := value.Decode((*plain)(c)); err != nil {
		return err
	} else {
		c.DataFormat = c.DataFormat.ToLower()
		for i := range c.Metrics {
			pointPrefix := fmt.Sprintf("Collect.Metrics[%d].Match", i)
			if c.DataFormat == Regex {
				if err = c.Metrics[i].BuildRegexp(pointPrefix); err != nil {
					return err
				}
			} else if c.DataFormat == Xml {
				if err = c.Metrics[i].BuildTemplate(pointPrefix); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Collect) Describe(_ chan<- *prometheus.Desc) {
}

func (c *Collect) Collect(proMetrics chan<- prometheus.Metric) {
	//fmt.Println("Collect")
	metrics := make(chan Metric, 10)
	go func() {
		for _, ds := range c.Datasource {
			var f = func(data []byte) {
				for _, mc := range c.Metrics {
					logger := log.With(mc.logger, "datasource", ds.Name)
					level.Debug(logger).Log("msg", "get metric", "data_format", c.DataFormat, "data", data)
					rcs := append(append(c.RelabelConfigs, ds.RelabelConfigs...), mc.RelabelConfigs...)
					switch c.DataFormat.ToLower() {
					case Regex:
						mc.GetMetricByRegex(logger, data, rcs, metrics)
					case Json:
						mc.GetMetricByJson(logger, data, rcs, metrics)
					case Xml:
						mc.GetMetricByXml(logger, data, rcs, metrics)
					case Yaml:
						mc.GetMetricByYaml(logger, data, rcs, metrics)
					}
				}
			}
			if ds.ReadMode == StreamLine {
				err := func() error {
					stream, err := ds.getStream()
					if err != nil {
						return err
					}
					var readCount int64
					defer stream.Close()
					buf := bufio.NewReader(stream)
					var lineBuf, line []byte
					var isPrefix bool
					for {
						line, isPrefix, err = buf.ReadLine()
						if err != nil {
							if io.EOF != err {
								return err
							}
							return nil
						}
						lineBuf = append(lineBuf, line...)
						if !isPrefix && len(lineBuf) > 0 {
							f(lineBuf)
							lineBuf = nil
						}
						readCount += int64(len(line))
						if readCount > ds.MaxContentLength {
							if len(lineBuf) > 0 {
								f(lineBuf)
							}
							return nil
						}
					}
				}()
				if err != nil {
					collectErrorCount.WithLabelValues("datasource").Inc()
					level.Error(c.logger).Log("msg", "Failed to read data.", "err", err)
				}
			} else {
				data, err := ds.getData()
				if err != nil {
					collectErrorCount.WithLabelValues("datasource").Inc()
					level.Error(c.logger).Log("msg", "Failed to get datasource.", "err", err)
					continue
				}
				if ds.ReadMode == Line {
					buf := bufio.NewReader(bytes.NewBuffer(data))
					var lineBuf, line []byte
					var isPrefix bool
					for {
						line, isPrefix, err = buf.ReadLine()
						if err != nil {
							if io.EOF != err {
								collectErrorCount.WithLabelValues("datasource").Inc()
								level.Error(c.logger).Log("msg", "Failed to read data.", "err", err)
							}
							break
						}
						lineBuf = append(lineBuf, line...)
						if !isPrefix && len(lineBuf) > 0 {
							f(append(lineBuf, line...))
							lineBuf = nil
						}
					}
				} else {
					f(data)
				}
			}
		}
		close(metrics)
	}()
	for metric := range metrics {
		m, err := metric.getMetric()
		if err != nil {
			collectErrorCount.WithLabelValues("datasource").Inc()
			level.Error(log.With(c.logger, "metric", metric.Name)).Log("log", "failed to get prometheus metric", "err", err)
		} else {
			proMetrics <- m
		}
	}
}

func (c *Collect) SetLogger(logger log.Logger) {
	c.logger = log.With(logger, "collect", c.Name)
	for i := range c.Metrics {
		c.Metrics[i].SetLogger(logger)
	}
}

type Collects []Collect
