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
	"github.com/MicroOps-cn/data_exporter/pkg/buffer"
	"github.com/MicroOps-cn/data_exporter/pkg/wrapper"
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

const ExporterName string = "data_exporter"

type DataFormat string

func (d DataFormat) ToLower() DataFormat {
	return DataFormat(strings.ToLower(string(d)))
}

var (
	collectErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: ExporterName,
		Name:      "collect_error_count",
		Help:      "datasource or metric collect error count",
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
	Name           string         `yaml:"name,omitempty"`
	RelabelConfigs RelabelConfigs `yaml:"relabel_configs,omitempty"`
	DataFormat     DataFormat     `yaml:"data_format"`
	Datasource     []*Datasource  `yaml:"datasource"`
	Metrics        MetricConfigs  `yaml:"metrics"`
	logger         log.Logger
	metrics        MetricGroup
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
					fmt.Println(err)
					return err
				}
			} else if c.DataFormat == Xml {
				if err = c.Metrics[i].BuildTemplate(pointPrefix); err != nil {
					return err
				}
			}
		}
		c.metrics.metrics = make(map[string]prometheus.Collector)
	}
	return nil
}

type ContextKey string

var LoggerContextName ContextKey = "_logger_"

func (c *CollectConfig) GetMetricByDs(ctx context.Context, logger log.Logger, ds *Datasource, metrics chan<- MetricGenerator) {
	defer func() {
		if r := recover(); r != nil {
			collectErrorCount.WithLabelValues("datasource", ds.Name).Inc()
			level.Error(logger).Log("msg", "Failed to get metrics from datasource.", "err", r)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, ds.Timeout)
	defer cancel()
	logger = log.With(c.logger, "datasource", ds.Name)
	ctx = context.WithValue(ctx, LoggerContextName, logger)
	rcs := append(c.RelabelConfigs, ds.RelabelConfigs...)
	if ds.ReadMode == Line {
		err := func() error {
			stream, err := ds.GetLineStream(ctx, logger)
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
	} else if ds.ReadMode == Stream {
	} else if ds.ReadMode == Full {
		data, err := ds.ReadAll(ctx)
		if err != nil {
			collectErrorCount.WithLabelValues("datasource", ds.Name).Inc()
			level.Error(c.logger).Log("msg", "Failed to get datasource.", "err", err)
			return
		}
		c.GetMetric(logger, data, rcs, metrics)
	}
}
func (c *CollectConfig) GetMetric(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
	var err error
	for _, mc := range c.Metrics {
		var dps []Datapoint
		rcs = append(rcs, mc.RelabelConfigs...)
		metricLogger := log.With(logger, "metric", mc.Name)
		level.Debug(metricLogger).Log("title", "Raw Data", "data_format", c.DataFormat, "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)))
		switch c.DataFormat.ToLower() {
		case Regex:
			dps = mc.GetDatapointsByRegex(metricLogger, data)
		case Json:
			dps = mc.GetDatapointsByJson(metricLogger, data)
		case Xml:
			dps = mc.GetDatapointsByXml(metricLogger, data)
		case Yaml:
			dps = mc.GetDatapointsByYaml(metricLogger, data)
		}

		for _, dp := range dps {
			m := MetricGenerator{
				logger:     logger,
				MetricType: mc.MetricType,
				Name:       mc.Name,
				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
			}
			for name, val := range dp {
				m.Labels.Append(name, val)
			}
			m.Labels, err = mc.Relabels(logger, rcs, m.Labels)
			if err != nil {
				continue
			}
			metrics <- m
		}
	}
}

func (c *CollectConfig) SetLogger(logger log.Logger) {
	c.logger = log.With(logger, "collect", c.Name)
	for i := range c.Metrics {
		c.Metrics[i].SetLogger(c.logger)
	}
}

func (c *CollectConfig) StopStreamCollect() {
	for i := range c.Datasource {
		c.Datasource[i].Close()
	}
}

func (c *CollectConfig) tailDsStream(ctx context.Context, ds *Datasource, stream buffer.ReadLineCloser, metrics chan<- MetricGenerator) {
	defer stream.Close()
	var line []byte
	var err error
	logger := log.With(c.logger, "datasource", ds.Name)
	rcs := append(c.RelabelConfigs, ds.RelabelConfigs...)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err = stream.ReadLine()
			if err != nil {
				level.Warn(c.logger).Log("log", "failed to read line", "err", err)
				return
			}
			c.GetMetric(logger, line, rcs, metrics)
		}
	}
}

func (c *CollectConfig) StartStreamCollect(ctx context.Context) error {
	metrics := make(chan MetricGenerator, 10)
	for i := range c.Datasource {
		if c.Datasource[i].ReadMode == Stream {
			stream, err := c.Datasource[i].GetLineStream(ctx, log.With(c.logger, "datasource", c.Datasource[i].Name))
			if err != nil {
				level.Error(c.logger).Log("log", "failed to start stream collect", "err", err, "datasource", c.Datasource[i].Name)
				return err
			}
			go func(ds *Datasource, buf buffer.ReadLineCloser) {
				var e error
				for {
					select {
					case <-ctx.Done():
						return
					default:
						c.tailDsStream(ctx, ds, buf, metrics)
					}

					buf, e = ds.GetLineStream(ctx, log.With(c.logger, "datasource", ds.Name))
					if e != nil {
						level.Error(c.logger).Log("log", "failed to start stream collect, retry...", "err", err, "datasource", ds.Name)
					}
				}
			}(c.Datasource[i], stream)
		}
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(metrics)
				return
			case metric := <-metrics:
				err := c.metrics.handle(metric)
				if err != nil {
					level.Info(metric.logger).Log("msg", "failed to parse metric", "err", err)
				}
			}
		}
	}()
	return nil
}

type CollectContext struct {
	*CollectConfig
	cancelFunc context.CancelFunc
	context.Context
	DatasourceName string
	DatasourceUrl  string
}

func (c *CollectContext) Describe(_ chan<- *prometheus.Desc) {
}

func (c *CollectContext) Collect(proMetrics chan<- prometheus.Metric) {
	mgs := NewMetricGenerators(10, c.logger)
	go func() {
		wg := sync.WaitGroup{}
		for i := range c.Datasource {
			ds := *c.Datasource[i]
			if len(c.DatasourceName) != 0 {
				if c.DatasourceName != c.Datasource[i].Name {
					continue
				} else if ds.AllowReplace {
					if len(c.DatasourceUrl) != 0 {
						ds.Url = c.DatasourceUrl
					}
				}
			}
			if ds.ReadMode != Stream {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					c.GetMetricByDs(c.Context, c.logger, &ds, mgs.metrics)
				}(i)
			}
		}
		wg.Wait()
		close(mgs.metrics)
	}()
	mgs.Collect(proMetrics)
	c.metrics.Collect(proMetrics)
}

type MetricGenerators struct {
	metrics chan MetricGenerator
	logger  log.Logger
}

func NewMetricGenerators(size int, logger log.Logger) *MetricGenerators {
	return &MetricGenerators{
		metrics: make(chan MetricGenerator, 10),
		logger:  logger,
	}
}

func (*MetricGenerators) Describe(_ chan<- *prometheus.Desc) {
}

func (mgs *MetricGenerators) Ch() chan MetricGenerator {
	return mgs.metrics
}
func (mgs *MetricGenerators) Collect(proMetrics chan<- prometheus.Metric) {
	for metric := range mgs.metrics {
		if metric.Labels.Has(LabelMetricValues) {
			ms, errs := metric.GetMetrics()
			for _, err := range errs {
				if err != nil {
					collectErrorCount.WithLabelValues("metric", metric.Name).Inc()
					level.Error(log.With(mgs.logger, "metric", metric.Name)).Log("log", "failed to get prometheus metric", "err", err)
				}
			}
			for _, m := range ms {
				if m != nil {
					proMetrics <- m
				}
			}
		} else {
			m, err := metric.getMetric()
			if err != nil {
				collectErrorCount.WithLabelValues("metric", metric.Name).Inc()
				level.Error(log.With(mgs.logger, "metric", metric.Name)).Log("log", "failed to get prometheus metric", "err", err)
			} else {
				proMetrics <- m
			}
		}
	}
}

type Collects []CollectConfig

func (c Collects) Get(name string) *CollectConfig {
	for idx := range c {
		if c[idx].Name == name {
			return &c[idx]
		}
	}
	return nil
}

func (c *Collects) SetLogger(logger log.Logger) {
	for idx := range *c {
		(*c)[idx].SetLogger(logger)
	}
}
func (c *Collects) StartStreamCollect(ctx context.Context) error {
	for idx := range *c {
		if err := (*c)[idx].StartStreamCollect(ctx); err != nil {
			return err
		}
	}
	return nil
}
func (c *Collects) StopStreamCollect() {
	for idx := range *c {
		(*c)[idx].StopStreamCollect()
	}
}
