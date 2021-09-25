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
	"context"
	"fmt"
	"github.com/beevik/etree"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
	"io"
	"regexp"
	"strings"
	"sync"
)

type DataFormat string

func (d DataFormat) ToLower() DataFormat {
	return DataFormat(strings.ToLower(string(d)))
}

var (
	collectErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "data_collect_error_count",
		Help: "datasource or metric collect error count",
	}, []string{"type", "name"})
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

type CollectConfig struct {
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

func (c *CollectConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain CollectConfig
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

const LoggerContextName = "_logger_"

func (c *CollectConfig) GetMetricByDs(ctx context.Context, logger log.Logger, ds *Datasource, metrics chan<- Metric) {
	defer func() {
		if r := recover(); r != nil {
			collectErrorCount.WithLabelValues("datasource", ds.Name).Inc()
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, ds.Timeout)
	defer cancel()
	logger = log.With(c.logger, "datasource", ds.Name)
	ctx = context.WithValue(ctx, LoggerContextName, logger)
	rcs := append(c.RelabelConfigs, ds.RelabelConfigs...)
	if ds.ReadMode == StreamLine {
		err := func() error {
			stream, err := ds.GetLineStream(ctx)
			if err != nil {
				return err
			}
			defer stream.Close()
			var line []byte
			for {
				line, err = stream.ReadLine()
				if err != nil {
					if io.EOF != err {
						return err
					}
					return nil
				}
				c.GetMetric(logger, line, rcs, metrics)
			}
		}()
		if err != nil {
			collectErrorCount.WithLabelValues("datasource", ds.Name).Inc()
			level.Error(logger).Log("msg", "Failed to read data.", "err", err)
		}
	} else {
		data, err := ds.ReadAll(ctx)
		if err != nil {
			collectErrorCount.WithLabelValues("datasource", ds.Name).Inc()
			level.Error(c.logger).Log("msg", "Failed to get datasource.", "err", err)
			return
		}
		c.GetMetric(logger, data, rcs, metrics)
	}
}
func (c *CollectConfig) GetMetric(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- Metric) {
	for _, mc := range c.Metrics {
		rcs = append(rcs, mc.RelabelConfigs...)
		logger = log.With(logger, "metric", mc.Name)
		level.Debug(logger).Log("msg", "get metric", "data_format", c.DataFormat, "data", data)
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

func (c *CollectConfig) SetLogger(logger log.Logger) {
	c.logger = log.With(logger, "collect", c.Name)
	for i := range c.Metrics {
		c.Metrics[i].SetLogger(c.logger)
	}
}

type CollectContext struct {
	*CollectConfig
	context.Context
}

func (c *CollectContext) Describe(_ chan<- *prometheus.Desc) {
}

func (c *CollectContext) Collect(proMetrics chan<- prometheus.Metric) {
	metrics := make(chan Metric, 10)
	go func() {
		wg := sync.WaitGroup{}
		wg.Add(len(c.Datasource))
		for i := range c.Datasource {
			go func(idx int) {
				defer wg.Done()
				c.GetMetricByDs(c.Context, c.logger, c.Datasource[idx], metrics)
			}(i)
		}
		wg.Wait()
		close(metrics)
	}()
	for metric := range metrics {
		m, err := metric.getMetric()
		if err != nil {
			collectErrorCount.WithLabelValues("metric", metric.Name).Inc()
			level.Error(log.With(c.logger, "metric", metric.Name)).Log("log", "failed to get prometheus metric", "err", err)
		} else {
			proMetrics <- m
		}
	}
}

type Collects []CollectConfig
