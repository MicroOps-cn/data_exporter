package collector

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type MetricType string

const (
	Gauge   MetricType = "gauge"
	Counter MetricType = "counter"
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

func (mc *MetricConfig) GetMetricByRegex(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- Metric) {
	var dds [][]byte
	if mc.Match.datapointRegexp != nil {
		dds = mc.Match.datapointRegexp.FindAll(data, -1)
		level.Debug(logger).Log("msg", "regexp match - datapoint", "data", string(data), "exp", mc.Match.datapointRegexp, "result", len(dds))
	} else {
		dds = [][]byte{data}
	}
	for _, dd := range dds {
		var (
			m = Metric{
				MetricType: mc.MetricType,
				Name:       mc.Name,
				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
			}
		)
		for name, labelRegexp := range mc.Match.labelsRegexp {
			val := labelRegexp.Find(dd)
			level.Debug(logger).Log("msg", "regexp match - label", "data", string(dd), "exp", labelRegexp, "result", string(val), "label", name)
			if len(val) > 0 {
				m.Labels.Append(name, string(val))
			}
		}
		level.Debug(logger).Log("msg", "relabel process - before", "labels", m.Labels)
		m.Labels = rcs.Process(m.Labels)
		level.Debug(logger).Log("msg", "relabel process - after", "labels", m.Labels)
		metrics <- m
	}
}
func (mc *MetricConfig) GetMetricByJson(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- Metric) {
	jn := gjson.ParseBytes(data)
	if len(mc.Match.Datapoint) != 0 {
		jn = jn.Get(mc.Match.Datapoint)
		level.Debug(logger).Log("msg", "json match - datapoint", "data", string(data), "exp", mc.Match.Datapoint, "result", jn.Raw)
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

	for _, j := range jns {
		var (
			m = Metric{
				MetricType: mc.MetricType,
				Name:       mc.Name,
				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
			}
		)
		for name, valMatch := range mc.Match.Labels {
			val := j.Get(valMatch).String()
			level.Debug(logger).Log("msg", "json match - label", "data", j.Raw, "exp", valMatch, "result", val, "label", name)
			if len(val) > 0 {
				m.Labels.Append(name, val)
			}
		}
		level.Debug(logger).Log("msg", "relabel process - before", "labels", m.Labels)
		m.Labels = rcs.Process(m.Labels)
		level.Debug(logger).Log("msg", "relabel process - after", "labels", m.Labels)
		metrics <- m
	}
}
func (mc *MetricConfig) GetMetricByXml(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- Metric) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		collectErrorCount.WithLabelValues("metric").Inc()
		level.Error(logger).Log("msg", "failed to parse xml data.", "err", err)
		return
	}
	var elems []*etree.Element
	if mc.Match.datapointXmlPath != nil {
		elems = doc.FindElementsPath(*mc.Match.datapointXmlPath)
		level.Debug(logger).Log("msg", "xml match - datapoint", "data", string(data), "exp", mc.Match.Datapoint, "result", len(elems))
	} else {
		elems = []*etree.Element{&doc.Element}
	}
	for _, elem := range elems {
		var (
			m = Metric{
				MetricType: mc.MetricType,
				Name:       mc.Name,
				Labels:     Labels{Label{Name: "name", Value: mc.Name}},
			}
		)
		for name, labelMatch := range mc.Match.labelsTmpl {
			val, err := labelMatch.Execute(elem)
			if err != nil {
				collectErrorCount.WithLabelValues("metric").Inc()
				level.Error(logger).Log("msg", "failed to parse xml data: failed to execute template.", "err", err)
				continue
			}
			level.Debug(logger).Log("msg", "xml match - label", "data",
				fmt.Sprintf("<%s>%s</%s>", elem.Tag, strings.TrimSpace(elem.Text()), elem.Tag),
				"exp", mc.Match.Labels[name], "result", val, "label", name)
			if len(val) > 0 {
				m.Labels.Append(name, string(val))
			}
		}

		level.Debug(logger).Log("msg", "relabel process - before", "labels", m.Labels)
		m.Labels = rcs.Process(m.Labels)
		level.Debug(logger).Log("msg", "relabel process - after", "labels", m.Labels)
		metrics <- m
	}
}

func (mc *MetricConfig) GetMetricByYaml(logger log.Logger, data []byte, rcs RelabelConfigs, metrics chan<- Metric) {
	if jsonData, err := yamlToJson(data, nil); err != nil {
		collectErrorCount.WithLabelValues("metric").Inc()
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

type Metric struct {
	MetricType MetricType
	Labels     Labels
	Datasource *Datasource
	Datapoint  *MetricConfig
	logger     log.Logger
	Name       string
}

func (m *Metric) getMetricName() string {
	return strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(m.Labels.Get(LabelMetricName)))
}
func (m *Metric) getValue() (float64, error) {
	val := strings.TrimSpace(m.Labels.Get(LabelMetricValue))
	if val == "" {
		return 0, fmt.Errorf("metric value is null")
	}
	return strconv.ParseFloat(val, 64)
}
func (m *Metric) getMetric() (prometheus.Metric, error) {
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
		if value, err := m.getValue(); err != nil {
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
		if value, err := m.getValue(); err != nil {
			return nil, err
		} else {
			metric.Add(value)
		}
		if !t.IsZero() {
			return prometheus.NewMetricWithTimestamp(t, metric), nil
		}
		return metric, nil
	default:
		return nil, fmt.Errorf("unknown metric type: %s", m.MetricType)
	}
}

func (m *Metric) getTime() time.Time {
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
