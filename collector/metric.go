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
	"bytes"
	"encoding/json"
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

const (
	LabelMetricName                 = "__name__"
	LabelMetricNamespace            = "__namespace__"
	LabelMetricSubsystem            = "__subsystem__"
	LabelMetricHelp                 = "__help__"
	LabelMetricTime                 = "__time__"
	LabelMetricTimeFormat           = "__time_format__"
	LabelMetricValue                = "__value__"
	LabelMetricBuckets              = "__buckets__"
	LabelMetricValues               = "__values__"
	LabelMetricValuesSeparator      = "__values_separator__"
	LabelMetricValuesIndex          = "__values_index__"
	LabelMetricValuesIndexSeparator = "__values_index_separator__"
	LabelMetricValuesIndexLabelName = "__values_index_label_name__"
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

type Datapoint map[string]string

func (d *Datapoint) UnmarshalJSON(raw []byte) error {
	if bytes.HasPrefix(raw, []byte{'"'}) {
		var tmp string
		if err := json.Unmarshal(raw, &tmp); err != nil {
			return err
		}
		raw = []byte(tmp)
	}
	return json.Unmarshal(raw, (*map[string]string)(d))
}

type MetricConfig struct {
	Name           string         `yaml:"name,omitempty"`
	RelabelConfigs RelabelConfigs `yaml:"relabel_configs,omitempty" json:"relabel_configs"`
	Match          MetricMatch    `yaml:"match"`
	MetricType     MetricType     `yaml:"metric_type" json:"metric_type"`
	logger         log.Logger
}

func (mc *MetricConfig) UnmarshalJSON(raw []byte) error {
	type plain MetricConfig
	if err := json.Unmarshal(raw, (*plain)(mc)); err != nil {
		return err
	}
	if mc.MetricType == "" {
		mc.MetricType = Gauge
	}
	return nil
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

func (mc *MetricConfig) Relabels(logger log.Logger, rcs RelabelConfigs, lvs Labels) (newLvs Labels, err error) {
	defer func() {
		if err == nil {
			level.Debug(logger).Log("title", "Relabel Process", "labels", newLvs, "oldLabels", lvs, "relabelConfigs", rcs)
		}
	}()
	if newLvs, err = rcs.Process(lvs); err != nil {
		level.Error(logger).Log("msg", "failed to relabel", "err", err, "labels", lvs, "relabelConfigs", rcs)
		return nil, err
	}
	if !newLvs.Has(LabelMetricName) {
		metricName := strings.ToLower(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(mc.Name))
		if model.IsValidMetricName(model.LabelValue(metricName)) {
			b := NewBuilder(newLvs)
			b.Set(LabelMetricName, metricName)
			newLvs.Append(LabelMetricName, metricName)
			if newLvs.Get("name") == mc.Name {
				b.Del("name")
			}
			newLvs = b.Labels()
		}
	}
	return newLvs, nil
}

func (mc *MetricConfig) GetDatapointsByRegex(logger log.Logger, data []byte) []Datapoint {
	var dps []Datapoint
	var dds [][][]byte
	var names []string
	if mc.Match.datapointRegexp != nil {
		names = mc.Match.datapointRegexp.SubexpNames()
		dds = mc.Match.datapointRegexp.FindAllSubmatch(data, -1)
		level.Debug(logger).Log("title", "Datapoint Match by Regex", "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", mc.Match.datapointRegexp, "resultCount", len(dds))
	} else {
		dds = [][][]byte{{data}}
	}
	if len(names) == 0 {
		if len(dds) > 0 && len(dds[0]) > 0 {
			dps = append(dps, Datapoint{"__line__": string(dds[0][0])})
		}
	} else {
		for _, dd := range dds {
			if len(dd) > 0 {
				var dp = map[string]string{}
				dp["__line__"] = string(dd[0])
				for idx, name := range names[1:] {
					dp[name] = string(dd[idx+1])
				}
				dps = append(dps, dp)
			}
		}
	}
	for i, dp := range dps {
		for name, labelRegexp := range mc.Match.labelsRegexp {
			val := labelRegexp.FindStringSubmatch(dp["__line__"])
			level.Debug(logger).Log("title", "Label Match by Regex", "data", string(wrapper.Limit[byte]([]byte(dp["__line__"]), 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", labelRegexp, "result", fmt.Sprint(val), "label", name)
			if len(val) > 0 {
				labelNames := labelRegexp.SubexpNames()
				if len(labelNames) > 1 {
					for idx, labelName := range labelNames[1:] {
						if labelName == name {
							dps[i][name] = val[idx+1]
							break
						}
					}
				} else {
					dps[i][name] = val[0]
				}
			}
		}
	}
	return dps
}

//func (mc *MetricConfig) GetMetricByRegex(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
//	var err error
//	var dds [][][]byte
//	var names []string
//	if mc.Match.datapointRegexp != nil {
//		names = mc.Match.datapointRegexp.SubexpNames()
//		dds = mc.Match.datapointRegexp.FindAllSubmatch(data, -1)
//		level.Debug(logger).Log("title", "Datapoint Match by Regex", "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", mc.Match.datapointRegexp, "resultCount", len(dds))
//	} else {
//		dds = [][][]byte{{data}}
//	}
//	for _, dd := range dds {
//		var (
//			m = MetricGenerator{
//				logger:     logger,
//				MetricType: mc.MetricType,
//				Name:       mc.Name,
//				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
//			}
//		)
//		if len(names) > 1 {
//			for idx, name := range names[1:] {
//				m.Labels.Append(name, string(dd[idx+1]))
//			}
//		}
//		for name, labelRegexp := range mc.Match.labelsRegexp {
//			dp := string(dd[0])
//			val := labelRegexp.FindStringSubmatch(dp)
//			level.Debug(logger).Log("title", "Label Match by Regex", "data", string(wrapper.Limit[byte]([]byte(dp), 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", labelRegexp, "result", fmt.Sprint(val), "label", name)
//			if len(val) > 0 {
//				labelNames := labelRegexp.SubexpNames()
//				if len(labelNames) > 1 {
//					for idx, labelName := range labelNames[1:] {
//						if labelName == name {
//							m.Labels.Append(name, val[idx+1])
//							break
//						}
//					}
//				} else {
//					m.Labels.Append(name, val[0])
//				}
//			}
//		}
//		m.Labels, err = mc.Relabels(logger, rcs, m.Labels)
//		if err != nil {
//			continue
//		}
//		metrics <- m
//	}
//}

func (mc *MetricConfig) GetDatapointsByJson(logger log.Logger, data []byte) []Datapoint {
	jn := gjson.ParseBytes(data)
	if len(mc.Match.Datapoint) != 0 {
		jn = jn.Get(mc.Match.Datapoint)
		level.Debug(logger).Log("title", "Datapoint Match By Gjson", "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", mc.Match.Datapoint, "result", jn.Raw)
	}
	if len(jn.Raw) == 0 {
		return nil
	}
	var jns []gjson.Result
	if jn.IsArray() {
		jns = jn.Array()
	} else {
		jns = []gjson.Result{jn}
	}
	var results []Datapoint
	for _, j := range jns {
		var result = Datapoint{"__line__": j.String()}
		j.ForEach(func(key, value gjson.Result) bool {
			result[key.String()] = value.String()
			return true
		})
		for name, valMatch := range mc.Match.Labels {
			val := j.Get(valMatch).String()
			level.Debug(logger).Log("title", "Label Match by Gjson", "data", string(wrapper.Limit[byte]([]byte(j.Raw), 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", valMatch, "result", val, "label", name)
			if len(val) > 0 {
				result[name] = val
			}
		}
		results = append(results, result)
	}
	return results
}

//func (mc *MetricConfig) GetMetricByJson(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
//	jn := gjson.ParseBytes(data)
//	if len(mc.Match.Datapoint) != 0 {
//		jn = jn.Get(mc.Match.Datapoint)
//		level.Debug(logger).Log("title", "Datapoint Match By Gjson", "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", mc.Match.Datapoint, "result", jn.Raw)
//	}
//	if len(jn.Raw) == 0 {
//		return
//	}
//	var jns []gjson.Result
//	if jn.IsArray() {
//		jns = jn.Array()
//	} else {
//		jns = []gjson.Result{jn}
//	}
//	var err error
//	for _, j := range jns {
//		var (
//			m = MetricGenerator{
//				logger:     logger,
//				MetricType: mc.MetricType,
//				Name:       mc.Name,
//				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
//			}
//		)
//		for name, valMatch := range mc.Match.Labels {
//			val := j.Get(valMatch).String()
//			level.Debug(logger).Log("title", "Label Match by Gjson", "data", string(wrapper.Limit[byte]([]byte(j.Raw), 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", valMatch, "result", val, "label", name)
//			if len(val) > 0 {
//				m.Labels.Append(name, val)
//			}
//		}
//
//		m.Labels, err = mc.Relabels(logger, rcs, m.Labels)
//		if err != nil {
//			continue
//		}
//		metrics <- m
//	}
//}

func (mc *MetricConfig) GetDatapointsByXml(logger log.Logger, data []byte) []Datapoint {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
		level.Error(logger).Log("msg", "failed to parse xml data.", "err", err, "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)))
		return nil
	}
	var elems []*etree.Element
	if mc.Match.datapointXmlPath != nil {
		elems = doc.FindElementsPath(*mc.Match.datapointXmlPath)
		level.Debug(logger).Log("title", "Datapoint Match by XML(etree.FindElementsPath)", "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", mc.Match.Datapoint, "resultCount", len(elems))
	} else {
		elems = []*etree.Element{&doc.Element}
	}
	var results []Datapoint
	for _, elem := range elems {
		var result = Datapoint{"__line__": elem.Text()}
		vdoc := etree.NewDocument()
		vdoc.AddChild(elem.Copy())
		elemRaw, err := vdoc.WriteToString()
		if err != nil {
			level.Error(logger).Log("msg", "failed to parse xml data: failed to write to string.", "err", err)
		}

		for name, labelMatch := range mc.Match.labelsTmpl {
			val, err := labelMatch.Execute(elem)
			if err != nil {
				collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
				level.Error(logger).Log("msg", "failed to parse xml data: failed to execute template.", "err", err)
				continue
			}
			level.Debug(logger).Log("title", "Label Match by XML(etree.FindElementsPath)", "data",
				string(wrapper.Limit[byte]([]byte(strings.TrimSpace(elemRaw)), 256, wrapper.PosCenter, []byte(" ... ")...)),
				"exp", mc.Match.Labels[name], "result", val, "label", name)

			if len(val) > 0 {
				result[name] = string(val)
			}
		}
		results = append(results, result)
	}
	return results
}

//func (mc *MetricConfig) GetMetricByXml(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
//	doc := etree.NewDocument()
//	if err := doc.ReadFromBytes(data); err != nil {
//		collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
//		level.Error(logger).Log("msg", "failed to parse xml data.", "err", err, "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)))
//		return
//	}
//	var elems []*etree.Element
//	if mc.Match.datapointXmlPath != nil {
//		elems = doc.FindElementsPath(*mc.Match.datapointXmlPath)
//		level.Debug(logger).Log("title", "Datapoint Match by XML(etree.FindElementsPath)", "data", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)), "exp", mc.Match.Datapoint, "resultCount", len(elems))
//	} else {
//		elems = []*etree.Element{&doc.Element}
//	}
//
//	for _, elem := range elems {
//		var (
//			m = MetricGenerator{
//				logger:     logger,
//				MetricType: mc.MetricType,
//				Name:       mc.Name,
//				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
//			}
//		)
//
//		vdoc := etree.NewDocument()
//		vdoc.AddChild(elem.Copy())
//		elemRaw, err := vdoc.WriteToString()
//		if err != nil {
//			level.Error(logger).Log("msg", "failed to parse xml data: failed to write to string.", "err", err)
//		}
//
//		for name, labelMatch := range mc.Match.labelsTmpl {
//			val, err := labelMatch.Execute(elem)
//			if err != nil {
//				collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
//				level.Error(logger).Log("msg", "failed to parse xml data: failed to execute template.", "err", err)
//				continue
//			}
//			level.Debug(logger).Log("title", "Label Match by XML(etree.FindElementsPath)", "data",
//				string(wrapper.Limit[byte]([]byte(strings.TrimSpace(elemRaw)), 256, wrapper.PosCenter, []byte(" ... ")...)),
//				"exp", mc.Match.Labels[name], "result", val, "label", name)
//
//			if len(val) > 0 {
//				m.Labels.Append(name, string(val))
//			}
//		}
//		m.Labels, err = mc.Relabels(logger, rcs, m.Labels)
//		if err != nil {
//			continue
//		}
//		metrics <- m
//	}
//}

func (mc *MetricConfig) GetDatapointsByYaml(logger log.Logger, data []byte) []Datapoint {
	if jsonData, err := yamlToJson(data, nil); err != nil {
		collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
		level.Error(logger).Log("msg", "failed to parse yaml data.", "err", err, "yaml", data)
	} else {
		level.Debug(logger).Log("title", "YAML to JSON", "json", string(wrapper.Limit[byte](jsonData, 256, wrapper.PosCenter, []byte(" ... ")...)), "yaml", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)))
		return mc.GetDatapointsByJson(logger, jsonData)
	}
	return nil
}

//func (mc *MetricConfig) GetMetricByYaml(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- MetricGenerator) {
//	if jsonData, err := yamlToJson(data, nil); err != nil {
//		collectErrorCount.WithLabelValues("metric", mc.Name).Inc()
//		level.Error(logger).Log("msg", "failed to parse yaml data.", "err", err, "yaml", data)
//	} else {
//		level.Debug(logger).Log("title", "YAML to JSON", "json", string(wrapper.Limit[byte](jsonData, 256, wrapper.PosCenter, []byte(" ... ")...)), "yaml", string(wrapper.Limit[byte](data, 256, wrapper.PosCenter, []byte(" ... ")...)))
//		mc.GetMetricByJson(logger, jsonData, rcs, metrics)
//	}
//}

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

func NewMetricGenerator(logger log.Logger, name string, metricType MetricType) *MetricGenerator {
	return &MetricGenerator{
		logger:     logger,
		MetricType: metricType,
		Name:       name,
		Labels:     Labels{{Name: "name", Value: name}},
	}
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

func (m *MetricGenerator) getValues() (vals []float64, index []string, errs []error) {
	separator := m.Labels.Get(LabelMetricValuesSeparator)
	if separator == "" {
		separator = " "
	}
	indexSeparator := m.Labels.Get(LabelMetricValuesIndexSeparator)
	if indexSeparator == "" {
		indexSeparator = separator
	}
	rawVals := strings.Split(strings.TrimSpace(m.Labels.Get(LabelMetricValues)), separator)
	rawIndexs := strings.Split(strings.TrimSpace(m.Labels.Get(LabelMetricValuesIndex)), indexSeparator)
	if len(rawIndexs) != len(rawVals) {
		return nil, nil, []error{fmt.Errorf(`values length not equal to index length`)}
	}
	errs = make([]error, len(rawVals))
	index = make([]string, len(rawVals))
	vals = make([]float64, len(rawVals))
	for i, rawVal := range rawVals {
		index[i] = rawIndexs[i]
		if rawVal == "" || rawVal == "true" {
			vals[i] = 1
		} else if rawVal == "false" {
			vals[i] = 0
		} else {
			vals[i], errs[i] = strconv.ParseFloat(rawVal, 64)
		}
	}
	return
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

func (m *MetricGenerator) GetMetrics() ([]prometheus.Metric, []error) {
	opts, err := m.getOpts()
	if err != nil {
		return nil, []error{err}
	}
	t := m.getTime()
	labels := m.Labels.WithoutEmpty().WithoutLabels()
	vals, index, errs := m.getValues()
	if len(vals) == 0 && len(errs) == 1 {
		return nil, errs
	} else if len(vals) != len(index) || len(vals) != len(errs) {
		return nil, []error{fmt.Errorf(`unknown error: len(vals)=%d, len(index)=%d, len(errs)=%d`, len(vals), len(index), len(errs))}
	}
	indexLabelName := m.Labels.Get(LabelMetricValuesIndexLabelName)
	if indexLabelName == "" {
		indexLabelName = "index"
	}
	var metrics []prometheus.Metric
	for i, value := range vals {
		if errs[i] != nil {
			continue
		}
		lvs := labels.Copy()
		lvs.Append(indexLabelName, index[i])
		if metric, err := m.getMetricFromLvs(opts, lvs, t, value); err != nil {
			errs[i] = err
		} else {
			metrics = append(metrics, metric)
		}
	}
	return metrics, errs
}

func (m *MetricGenerator) getMetricFromLvs(opts prometheus.Opts, lvs Labels, t time.Time, value float64) (prometheus.Metric, error) {
	switch m.MetricType.ToLower() {
	case Gauge:
		metric := prometheus.NewGaugeVec(prometheus.GaugeOpts(opts), lvs.Keys()).With(lvs.Map())
		metric.Set(value)
		if !t.IsZero() {
			return prometheus.NewMetricWithTimestamp(t, metric), nil
		}
		return metric, nil
	case Counter:
		metric := prometheus.NewCounterVec(prometheus.CounterOpts(opts), lvs.Keys()).With(lvs.Map())
		metric.Add(value)
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
		}, lvs.Keys())
		histogram.With(lvs.Map()).Observe(value)
		metric, err := histogram.MetricVec.GetMetricWith(lvs.Map())
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

func (m *MetricGenerator) getMetric() (prometheus.Metric, error) {
	opts, err := m.getOpts()
	if err != nil {
		return nil, err
	}
	t := m.getTime()
	labels := m.Labels.WithoutEmpty().WithoutLabels()
	value, err := m.getValue()
	if err != nil && err != ErrValueIsNull {
		return nil, err
	}
	return m.getMetricFromLvs(opts, labels, t, value)
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
