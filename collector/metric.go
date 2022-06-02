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
	"fmt"
	"github.com/MicroOps-cn/data_exporter/pkg/values"
	"github.com/MicroOps-cn/data_exporter/pkg/wrapper"
	"github.com/beevik/etree"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MetricType string

const (
	Gauge     MetricType = "gauge"
	Counter   MetricType = "counter"
	Histogram MetricType = "histogram"
)

func (d MetricType) ToLower() MetricType {
	return MetricType(strings.ToLower(string(d)))
}

type MetricMatch struct {
	Datapoint        string            `yaml:"datapoint"`
	Labels           map[string]string `yaml:"labels"`
	datapointRegexp  *regexp.Regexp
	labelsRegexp     map[string]*regexp.Regexp
	datapointXmlPath *etree.Path
	labelsTmpl       map[string]*Template
}

func (mc *MetricConfig) BuildRegexp(pointPrefix string) (err error) {
	if mc.Match.datapointRegexp, err = regexCompile(mc.Match.Datapoint, false, pointPrefix+".Datapoint"); err != nil {
		return err
	}
	mc.Match.labelsRegexp = make(map[string]*regexp.Regexp)
	for i2, label := range mc.Match.Labels {
		if mc.Match.labelsRegexp[i2], err = regexCompile(label, true, fmt.Sprintf(pointPrefix+".Labels[%d]", i2)); err != nil {
			return err
		}
	}
	return nil
}

func (mc *MetricConfig) BuildTemplate(pointPrefix string) (err error) {
	if mc.Match.datapointXmlPath, err = xmlPathCompile(mc.Match.Datapoint, false, pointPrefix+".Datapoint"); err != nil {
		return err
	}
	mc.Match.labelsTmpl = make(map[string]*Template)
	for i2, labelMatch := range mc.Match.Labels {
		mc.Match.labelsTmpl[i2], err = NewTemplate(fmt.Sprintf("%s[%s]", mc.Name, i2), labelMatch)
		if err != nil {
			return err
		}
	}
	return nil
}

type MetricConfig struct {
	Name           string         `yaml:"name"`
	RelabelConfigs RelabelConfigs `yaml:"relabel_configs"`
	Match          MetricMatch    `yaml:"match"`
	MetricType     MetricType     `yaml:"metric_type"`
	logger         log.Logger
}

func (mc *MetricConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain MetricConfig
	if err := value.Decode((*plain)(mc)); err != nil {
		return err
	}
	if mc.MetricType == "" {
		mc.MetricType = Gauge
	}
	return nil
}

func (mc *MetricConfig) GetMetricByRegex(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
	var err error
	var dds [][][]byte
	var names []string
	if mc.Match.datapointRegexp != nil {
		names = mc.Match.datapointRegexp.SubexpNames()
		dds = mc.Match.datapointRegexp.FindAllSubmatch(data, -1)
		level.Debug(logger).Log("msg", "regexp match - datapoint", "data", string(wrapper.Limit[byte](data, 100, []byte("...")...)), "exp", mc.Match.datapointRegexp, "result", len(dds))
	} else {
		dds = [][][]byte{{data}}
	}
	for _, dd := range dds {
		var (
			m = MetricGenerator{
				logger:     logger,
				MetricType: mc.MetricType,
				Name:       mc.Name,
				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
			}
		)
		if len(names) > 1 {
			for idx, name := range names[1:] {
				m.Labels.Append(name, string(dd[idx+1]))
			}
		}
		for name, labelRegexp := range mc.Match.labelsRegexp {
			dp := string(dd[0])
			val := labelRegexp.FindStringSubmatch(dp)
			level.Debug(logger).Log("msg", "regexp match - label", "data", string(wrapper.Limit[byte]([]byte(dp), 100, []byte("...")...)), "exp", labelRegexp, "result", fmt.Sprint(val), "label", name)
			if len(val) > 0 {
				labelNames := labelRegexp.SubexpNames()
				if len(labelNames) > 1 {
					for idx, labelName := range labelNames[1:] {
						if labelName == name {
							m.Labels.Append(name, val[idx+1])
							break
						}
					}
				} else {
					m.Labels.Append(name, val[0])
				}
			}
		}
		level.Debug(logger).Log("msg", "relabel process - before", "labels", m.Labels)
		if m.Labels, err = rcs.Process(m.Labels); err != nil {
			level.Error(logger).Log("msg", "failed to relabel", "err", err)
			continue
		}

		if !m.Labels.Has(LabelMetricName) {
			metricName := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(mc.Name))
			if model.IsValidMetricName(model.LabelValue(metricName)) {
				b := NewBuilder(m.Labels)
				b.Set(LabelMetricName, metricName)
				m.Labels.Append(LabelMetricName, metricName)
				if m.Labels.Get("name") == mc.Name {
					b.Del("name")
				}
				m.Labels = b.Labels()
			}
		}
		level.Debug(logger).Log("msg", "relabel process - after", "labels", m.Labels)
		metrics <- m
	}
}
func (mc *MetricConfig) GetMetricByJson(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
	jn := gjson.ParseBytes(data)
	if len(mc.Match.Datapoint) != 0 {
		jn = jn.Get(mc.Match.Datapoint)
		level.Debug(logger).Log("msg", "json match - datapoint", "data", string(wrapper.Limit[byte](data, 100, []byte("...")...)), "exp", mc.Match.Datapoint, "result", jn.Raw)
	}
	if len(jn.Raw) == 0 {
		return
	}
	var jns []gjson.Result
	if jn.IsArray() {
		jns = jn.Array()
	} else {
		jns = []gjson.Result{jn}
	}
	var err error
	for _, j := range jns {
		var (
			m = MetricGenerator{
				logger:     logger,
				MetricType: mc.MetricType,
				Name:       mc.Name,
				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
			}
		)
		for name, valMatch := range mc.Match.Labels {
			val := j.Get(valMatch).String()
			level.Debug(logger).Log("msg", "json match - label", "data", string(wrapper.Limit[byte]([]byte(j.Raw), 100, []byte("...")...)), "exp", valMatch, "result", val, "label", name)
			if len(val) > 0 {
				m.Labels.Append(name, val)
			}
		}
		level.Debug(logger).Log("msg", "relabel process - before", "labels", m.Labels)

		if m.Labels, err = rcs.Process(m.Labels); err != nil {
			level.Error(logger).Log("msg", "failed to relabel", "err", err)
			continue
		}
		if !m.Labels.Has(LabelMetricName) {
			metricName := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(mc.Name))
			if model.IsValidMetricName(model.LabelValue(metricName)) {
				b := NewBuilder(m.Labels)
				b.Set(LabelMetricName, metricName)
				m.Labels.Append(LabelMetricName, metricName)
				if m.Labels.Get("name") == mc.Name {
					b.Del("name")
				}
				m.Labels = b.Labels()
			}
		}
		level.Debug(logger).Log("msg", "relabel process - after", "labels", m.Labels)
		metrics <- m
	}
}
func (mc *MetricConfig) GetMetricByXml(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
		level.Error(logger).Log("msg", "failed to parse xml data.", "err", err)
		return
	}
	var elems []*etree.Element
	if mc.Match.datapointXmlPath != nil {
		elems = doc.FindElementsPath(*mc.Match.datapointXmlPath)
		level.Debug(logger).Log("msg", "xml match - datapoint", "data", string(wrapper.Limit[byte](data, 100, []byte("...")...)), "exp", mc.Match.Datapoint, "result", len(elems))
	} else {
		elems = []*etree.Element{&doc.Element}
	}
	var err error
	for _, elem := range elems {
		var (
			m = MetricGenerator{
				logger:     logger,
				MetricType: mc.MetricType,
				Name:       mc.Name,
				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
			}
		)
		for name, labelMatch := range mc.Match.labelsTmpl {
			val, err := labelMatch.Execute(elem)
			if err != nil {
				collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
				level.Error(logger).Log("msg", "failed to parse xml data: failed to execute template.", "err", err)
				continue
			}
			level.Debug(logger).Log("msg", "xml match - label", "data",
				fmt.Sprintf("<%s>%s</%s>", elem.Tag, string(wrapper.Limit[byte]([]byte(strings.TrimSpace(elem.Text())), 100, []byte("...")...)), elem.Tag),
				"exp", mc.Match.Labels[name], "result", val, "label", name)
			if len(val) > 0 {
				m.Labels.Append(name, string(val))
			}
		}

		level.Debug(logger).Log("msg", "relabel process - before", "labels", m.Labels)
		if m.Labels, err = rcs.Process(m.Labels); err != nil {
			level.Error(logger).Log("msg", "failed to relabel", "err", err)
			continue
		}
		if !m.Labels.Has(LabelMetricName) {
			metricName := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(mc.Name))
			if model.IsValidMetricName(model.LabelValue(metricName)) {
				b := NewBuilder(m.Labels)
				b.Set(LabelMetricName, metricName)
				m.Labels.Append(LabelMetricName, metricName)
				if m.Labels.Get("name") == mc.Name {
					b.Del("name")
				}
				m.Labels = b.Labels()
			}
		}
		level.Debug(logger).Log("msg", "relabel process - after", "labels", m.Labels)
		metrics <- m
	}
}

func (mc *MetricConfig) GetMetricByYaml(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
	if jsonData, err := yamlToJson(data, nil); err != nil {
		collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
		level.Error(logger).Log("msg", "failed to parse yaml data.", "err", err, "yaml", data)
	} else {
		level.Debug(logger).Log("msg", "YAML转JSON", "json", string(jsonData), "yaml", data)
		mc.GetMetricByJson(logger, jsonData, rcs, metrics)
	}
}

func (mc *MetricConfig) SetLogger(logger log.Logger) {
	mc.logger = log.With(logger, "metric", mc.Name)
}

type MetricConfigs []*MetricConfig

type MetricGenerator struct {
	MetricType MetricType
	Labels     Labels
	Datasource *Datasource
	Datapoint  *MetricConfig
	logger     log.Logger
	Name       string
}

var ErrValueIsNull = fmt.Errorf("metric value is null")

func (m *MetricGenerator) getMetricName() string {
	return strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(m.Labels.Get(LabelMetricName)))
}
func (m *MetricGenerator) getValue() (float64, error) {
	val := strings.TrimSpace(m.Labels.Get(LabelMetricValue))
	if val == "" {
		return 1, ErrValueIsNull
	} else if val == "true" {
		return 1.0, nil
	} else if val == "false" {
		return 0.0, nil
	}
	return strconv.ParseFloat(val, 64)
}

func (m *MetricGenerator) getOpts() (prometheus.Opts, error) {
	opts := prometheus.Opts{}

	opts.Name = m.getMetricName()
	if opts.Name == "" {
		return opts, fmt.Errorf(`"%s" is not a valid metric name`, opts.Name)
	}
	opts.Namespace = m.Labels.Get(LabelMetricNamespace)
	opts.Subsystem = m.Labels.Get(LabelMetricSubsystem)
	opts.Help = m.Labels.Get(LabelMetricHelp)
	return opts, nil
}

func (m *MetricGenerator) getMetric() (prometheus.Metric, error) {
	opts := prometheus.Opts{}

	opts.Name = m.getMetricName()
	if opts.Name == "" {
		return nil, fmt.Errorf(`"%s" is not a valid metric name`, opts.Name)
	}
	opts.Namespace = m.Labels.Get(LabelMetricNamespace)
	opts.Subsystem = m.Labels.Get(LabelMetricSubsystem)
	opts.Help = m.Labels.Get(LabelMetricHelp)
	t := m.getTime()
	labels := m.Labels.WithoutEmpty().WithoutLabels()
	switch m.MetricType.ToLower() {
	case Gauge:
		metric := prometheus.NewGaugeVec(prometheus.GaugeOpts(opts), labels.Keys()).With(labels.Map())
		if value, err := m.getValue(); err != nil && err != ErrValueIsNull {
			return nil, err
		} else {
			metric.Set(value)
		}
		if !t.IsZero() {
			return prometheus.NewMetricWithTimestamp(t, metric), nil
		}
		return metric, nil
	case Counter:
		metric := prometheus.NewCounterVec(prometheus.CounterOpts(opts), labels.Keys()).With(labels.Map())
		if value, err := m.getValue(); err != nil && err != ErrValueIsNull {
			return nil, err
		} else {
			metric.Add(value)
		}
		if !t.IsZero() {
			return prometheus.NewMetricWithTimestamp(t, metric), nil
		}
		return metric, nil

	case Histogram:
		buckets, err := values.NewValues(m.Labels.Get(LabelMetricBuckets), "").Float64s()
		if err != nil {
			return nil, fmt.Errorf("bucket format error: %s", err)
		} else if len(buckets) == 0 {
			return nil, fmt.Errorf("bucket length == 0")
		}
		histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:   opts.Namespace,
			Subsystem:   opts.Subsystem,
			Name:        opts.Name,
			Help:        opts.Help,
			ConstLabels: opts.ConstLabels,
			Buckets:     append(buckets, math.Inf(+1)),
		}, labels.Keys())
		if value, err := m.getValue(); err != nil {
			return nil, err
		} else {
			histogram.With(labels.Map()).Observe(value)
		}
		metric, err := histogram.MetricVec.GetMetricWith(labels.Map())
		if err != nil {
			return nil, fmt.Errorf("failed to get metric from histogram: %s", m.MetricType)
		}

		if !t.IsZero() {
			return prometheus.NewMetricWithTimestamp(t, metric), nil
		}
		return metric, nil
	default:
		return nil, fmt.Errorf("unknown metric type: %s", m.MetricType)
	}
}

func (m *MetricGenerator) getTime() time.Time {
	timeStr := m.Labels.Get(LabelMetricTime)
	timeFormat := m.Labels.Get(LabelMetricTimeFormat)
	if len(timeStr) != 0 {
		if ts, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
			if ts >= 1e15 {
				return time.Unix(ts/1e6, (ts%1e6)*1e3)
			} else if ts >= 1e12 {
				return time.Unix(ts/1e3, (ts%1e3)*1e6)
			} else {
				return time.Unix(ts, 0)
			}
		}
		if len(timeFormat) == 0 {
			timeFormat = time.RFC3339Nano
		}

		t, err := time.Parse(timeFormat, timeStr)
		if err != nil {
			return time.Time{}
		} else {
			return t
		}
	}
	return time.Time{}
}

type MetricGroup struct {
	metrics map[string]prometheus.Collector
	mux     sync.Mutex
}

func (mg *MetricGroup) handle(mgr MetricGenerator) error {
	opts, err := mgr.getOpts()
	if err != nil {
		return err
	}
	fqName := prometheus.BuildFQName(opts.Namespace, opts.Subsystem, opts.Name)
	labelKeys := mgr.Labels.WithoutEmpty().WithoutLabels().Keys()
	sort.Strings(labelKeys)
	metricHash := string(mgr.MetricType.ToLower()) + "\x00" + fqName + "\x00" + strings.Join(labelKeys, "\x00")

	labels := mgr.Labels.WithoutEmpty().WithoutLabels().Map()
	promMetric, ok := mg.metrics[metricHash]
	if !ok || promMetric == nil {
		switch mgr.MetricType.ToLower() {
		case Gauge:
			promMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts(opts), labelKeys)
			mg.createMetric(metricHash, promMetric)
		case Counter:
			promMetric = prometheus.NewCounterVec(prometheus.CounterOpts(opts), labelKeys)
			mg.createMetric(metricHash, promMetric)
		case Histogram:
			buckets, err := values.NewValues(mgr.Labels.Get(LabelMetricBuckets), "").Float64s()
			if err != nil {
				return fmt.Errorf("bucket format error: %s", err)
			} else if len(buckets) == 0 {
				return fmt.Errorf("bucket length == 0")
			}
			promMetric := prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace:   opts.Namespace,
				Subsystem:   opts.Subsystem,
				Name:        opts.Name,
				Help:        opts.Help,
				ConstLabels: opts.ConstLabels,
				Buckets:     append(buckets, math.Inf(+1)),
			}, labelKeys)
			mg.createMetric(metricHash, promMetric)
		default:
			return fmt.Errorf("unknown metric type: %s", mgr.MetricType)
		}
	}
	switch mgr.MetricType.ToLower() {
	case Gauge:
		if counterVec, ok := promMetric.(*prometheus.GaugeVec); ok {
			if value, err := mgr.getValue(); err != nil && err != ErrValueIsNull {
				return err
			} else {
				counterVec.With(labels).Set(value)
			}
		}
	case Counter:
		if counterVec, ok := promMetric.(*prometheus.CounterVec); ok {
			if value, err := mgr.getValue(); err != nil && err != ErrValueIsNull {
				return err
			} else {
				counterVec.With(labels).Add(value)
			}
		}
	case Histogram:
		if histogramVec, ok := promMetric.(*prometheus.HistogramVec); ok {
			if value, err := mgr.getValue(); err != nil {
				return err
			} else {
				histogramVec.With(labels).Observe(value)
			}
		}
	default:
		return fmt.Errorf("unknown metric type: %s", mgr.MetricType)
	}
	return nil
}

func (mg *MetricGroup) createMetric(metricHash string, collector prometheus.Collector) {
	mg.mux.Lock()
	defer mg.mux.Unlock()
	mg.metrics[metricHash] = collector
}

func (mg *MetricGroup) Collect(metrics chan<- prometheus.Metric) {
	mg.mux.Lock()
	defer mg.mux.Unlock()
	for _, metric := range mg.metrics {
		metric.Collect(metrics)
	}
}
